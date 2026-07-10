# Package Execution as Jobs design

This document specifies the migration of package-stage execution from operator-managed raw Pods to `batch/v1` Jobs — one Job per (skyhook, package, stage, node). It is the design companion to [issue #223](https://github.com/NVIDIA/nodewright/issues/223); implementation is tracked by sub-issues #299–#305, each delivered as its own PR to the `feature/package-as-jobs` branch.

CRD/schema changes, cordon/drain ownership, DeploymentPolicy semantics, and multi-node Job parallelism are **out of scope**. The ImagePullBackOff surfacing gap is explicitly **not** solved by this migration (see [Rejected: `podFailurePolicy`](#rejected-podfailurepolicy-for-imagepullbackoff)) and is tracked separately in issue #306.

## Problems

### Debugging requires racing the operator

Completion of a stage is detected by the operator reading init-container statuses, after which the operator **deletes the pod** ([`PodReconcile`](../../operator/internal/controller/pod_controller.go)). Logs for a completed stage are gone the moment the operator processes it; debugging requires catching the pod before deletion or knowing its generated name in advance.

### Hand-rolled lifecycle

Cleanup, completion tracking, "does this stage already run" checks, and stale-spec invalidation are all custom code across [`skyhook_controller.go`](../../operator/internal/controller/skyhook_controller.go) (`PodExists`, `ValidateRunningPackages`, `InvalidPackage`) and [`pod_controller.go`](../../operator/internal/controller/pod_controller.go). Kubernetes has a first-class run-to-completion primitive that carries most of this for free, plus ecosystem benefits (`kubectl logs job/<name>`, `kubectl describe job`, k9s/ArgoCD/dashboard treatment of Jobs as completable work).

## Goals

1. **Stable, queryable execution records**: one Job per (skyhook, package, stage, node); `kubectl logs job/<name>` works during and after execution.
2. **Logs outlive completion**: finished work is retained for a configurable window, longer on failure than on success.
3. **No behavior change to the lifecycle state machine**: Stage progression, Status/State derivation, interrupt/cordon/drain sequencing, DeploymentPolicy interactions, and agent-visible env are unchanged.
4. **Safe upgrade**: an operator upgrade with in-flight raw pods completes those stages correctly.

## Current behavior (baseline)

Facts about today's implementation that shape the design:

- **Package pods never terminate.** All work runs sequentially in `initContainers` (init-copy → `<stage>` → `<stage>-check`); the main container is a `pause` image that runs forever ([`createPodFromPackage`](../../operator/internal/controller/skyhook_controller.go)). The lingering Running pod doubles as the "stage in flight, don't create another" marker for [`PodExists`](../../operator/internal/controller/skyhook_controller.go).
- **The operator is the completion detector.** [`containerExitedSuccessfully`](../../operator/internal/controller/pod_controller.go) inspects `InitContainerStatuses`; on success the operator updates node state and then deletes the pod. Deletion is also the **processed-once marker**: the `pod.DeletionTimestamp == nil` gate prevents double-processing of success events.
- **Retries are kubelet in-place retries.** `restartPolicy: OnFailure` means a failing init container crash-loops forever; the operator only reports `erroring` with restart counts — it never recreates a failing pod. A pod that *disappears* (evicted, deleted) is silently recreated on a later reconcile via the `PodExists` check.
- **Pods are owned by the Skyhook CR** (`SetControllerReference` in [`ApplyPackage`](../../operator/internal/controller/skyhook_controller.go)), carry labels `skyhook.nvidia.com/name` and `…/package`, and a JSON package annotation ([`annotations.go`](../../operator/internal/controller/annotations.go)) that round-trips (skyhook, package, version, stage, image).
- **[`cluster_state_v2.go`](../../operator/internal/controller/cluster_state_v2.go) does not read pods.** Node annotations are the state store; pods are ephemeral executors. This keeps the migration's blast radius to creation sites, the pod pseudo-controller, and the exists/validate checks.

## Design

### Job shape

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: <generateSafeName(63, skyhook, package, version, stage, node)>   # same input as today's pod names
  namespace: <operator namespace>
  labels:
    skyhook.nvidia.com/name: <skyhook>          # existing
    skyhook.nvidia.com/package: <name>-<version> # existing
    skyhook.nvidia.com/stage: <stage>            # new
    skyhook.nvidia.com/node: <node>              # new
    skyhook.nvidia.com/generation: "<n>"         # new; distinguishes rollouts (see Labels below)
  annotations:
    skyhook.nvidia.com/resource-id: <name>-<uid>-<generation>   # full provenance; too long for a label
  ownerReferences: [<Skyhook CR>]                # cascade delete; pods are owned by the Job
spec:
  parallelism: 1
  completions: 1
  backoffLimit: 0                 # see Retry semantics
  podReplacementPolicy: Failed    # no old+new pod overlap on the shared hostPath mounts
  # ttlSecondsAfterFinished: unset at creation — set by the operator at completion (see TTL)
  template:
    metadata:
      labels: <same label set>    # child pods inherit labels → existing CLI label queries keep working
    spec:
      # identical to today's pod spec (initContainers, hostPID/hostNetwork, tolerations,
      # resources, gracefulShutdown, imagePullSecrets), EXCEPT the main container:
      containers:
      - name: done                # replaces the forever-running `pause` container
        image: <agent image>      # already pulled for the init containers
        command: ["/bin/sh", "-c", "exit 0"]
```

**Why the main container changes:** a Job only records completion when its pod reaches `Succeeded`, and a pod only succeeds when all containers terminate. The init-container chain stays (regular containers run concurrently — `initContainers` are the only native run-to-completion sequencing primitive, and per-step container names are load-bearing for state reporting). The `pause` container's other job — keeping the pod object alive as the "in flight" marker — is taken over by the Job object itself, which outlives its pods.

Interrupt pods get the same treatment ([`createInterruptPodForPackage`](../../operator/internal/controller/skyhook_controller.go) → Job). Interrupt-over-reboot still works: the pod is bound to the node and survives reboot; kubelet re-runs the init containers; the agent skips the already-done interrupt via the `SKYHOOK_RESOURCE_ID` flag file and exits 0; the pod then succeeds and the Job completes.

### Completion flow and the processed-once marker

The pseudo-controller pattern is kept (issue #223 option A): a Jobs watch maps events into the single global reconcile queue as `job---<name>` requests, alongside the existing `pod---<name>` routing. No second reconciler writes SCR/node state, so the existing serialization guarantee holds.

Because completed pods now linger, pod-deletion can no longer be the processed-once marker. `JobReconcile` replaces it with a persisted annotation:

- Job `Complete` and not yet marked → read the child pod (via the `batch.kubernetes.io/job-name` label) for container name + restarts → existing `UpdateNodeState(..., StateComplete, ...)` path (upgrade/uninstall/interrupt special cases in `HandleCompletePod` unchanged) → **one Job Update** setting annotation `skyhook.nvidia.com/state-recorded: "true"` and the outcome TTL. Already-marked Jobs are skipped, making duplicate events harmless. `UpdateNodeState`/`HandleCompletePod` are *not* idempotent (re-processing an uninstall completion after state removal would resurrect the state via `Upsert`), which is why the marker must be written before any event can be re-served.
- Job `Failed` (see below — disruption only) → `UpdateNodeState(..., StateErroring, ...)` → same marker + failure TTL. Recreation is decided by the main reconcile pass, not the Job handler.
- Active Jobs → no-op; the Pod watch keeps reporting in-flight crash-loop `erroring` and restart counts exactly as today.

### Retry semantics

Unchanged by design, and worth stating precisely because the Job knobs are easy to misread:

- The pod template keeps `restartPolicy: OnFailure` → a failing step crash-loops **in place** under kubelet backoff, forever, with the Job staying Active. This is today's behavior. The Job controller's `backoffLimit` does not see in-place restarts.
- `backoffLimit: 0` therefore only fires when the pod as a whole is lost (eviction, node scale-down, manual delete). Today that case is silently recreated via `PodExists`; with Jobs it becomes explicit — the Job goes `Failed`, the operator records `erroring`, and the main reconcile deletes the failed Job and recreates it if node state says the stage should still run.

### TTL: outcome-based, set at completion

`ttlSecondsAfterFinished` applies to both `Complete` and `Failed` Jobs and is **mutable**. Jobs are created with the field unset; `JobReconcile` sets it in the same Update that writes `state-recorded`:

| Outcome | Operator option | Default |
| --- | --- | --- |
| Succeeded | `JOB_TTL_SUCCEEDED` | `1h` |
| Failed | `JOB_TTL_FAILED` | `24h` |

Rationale: the retention requirement is asymmetric — failure logs are for humans arriving late; success logs are a convenience. Setting TTL at completion (rather than statically at creation) is what makes the asymmetry possible. If the operator is down when a Job finishes, the TTL is simply set when it resyncs — level-triggered, no timer lost. The most common failure mode (crash-looping step) keeps the Job Active indefinitely and is never TTL'd, matching today's lingering-pod behavior.

Scale note: succeeded pods are retained for the success-TTL window — roughly `nodes × packages × 3 stages` extra terminated pods in etcd and the informer cache at peak. This is why the success default is short and configurable; [`operator_resources_at_scale.md`](../operator_resources_at_scale.md) carries the sizing guidance.

### Naming, idempotency, and reruns

Job names are deterministic (same `generateSafeName` inputs as today's pods), so create-after-crash collapses into a benign `AlreadyExists`. Two rules make deterministic names coexist with retained finished Jobs:

- `JobExists` (successor of `PodExists`) counts only **unfinished** Jobs as "in flight". A finished Job for the same package does not block the next stage (which has a different name anyway) — and a finished Job for the *same* stage means "already done", which is new, free idempotency.
- `ValidateRunningPackages` (now iterating Jobs, via the new `node` label instead of a field index) keeps its spec-mismatch and node-state cross-checks, and gains one rule: a **finished** Job whose (package, stage) the node state expects to run again — `kubectl skyhook reset` / `package rerun` — is deleted so the next pass can recreate the name.

### Labels

`ResourceID()` is `<name>-<uid>-<generation>` — routinely over the 63-character label-value limit, so issue #223's `resource-id` label is replaced by: full resource-id as an **annotation**, plus a numeric `skyhook.nvidia.com/generation` **label**. The issue's query goals are preserved with valid label values:

```bash
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init                         # all Jobs in one SCR
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init,skyhook.nvidia.com/package=pkg-1.0,skyhook.nvidia.com/node=worker-7,skyhook.nvidia.com/generation=4
                                                                             # one rollout of one package on one node
kubectl get jobs -l skyhook.nvidia.com/node=worker-7                         # everything touching a node
```

### RBAC

The operator ClusterRole gains `batch/jobs` `get;list;watch;create;update;patch;delete` (kubebuilder marker + `make manifests`, hand-mirrored into `chart/`). Pod verbs stay: the operator still reads child pods for restarts/container names, reads workload pods for drain, and drives legacy pods during the upgrade window.

## Upgrade and compatibility

No persisted schema changes → **no `zz.migration` shim**. In-flight raw pods from the previous operator version are distinguishable by the absence of the `batch.kubernetes.io/job-name` label:

- Legacy pods (no label): the old completion path — update node state, delete pod — is kept for **one minor release**, then removed (release-notes entry on removal).
- Job-owned pods (label present): the Pod watch only reports in-flight erroring/restarts; completion is owned by the Job path.

New stages created after the upgrade are Jobs immediately. Nothing needs to be proactively deleted (issue #223's migration option 1).

CLI: child pods inherit the full label set via the pod template, so `kubectl skyhook package logs/status/rerun` label queries work unchanged against both operator generations; retained pods make post-completion `package logs` *better*. No CLI code change is expected; the verification is part of #305.

## Rejected alternatives

### Rejected: `podFailurePolicy` for ImagePullBackOff

`podFailurePolicy` evaluates `OnExitCodes` (requires a terminated container) and `OnPodConditions` (e.g. `DisruptionTarget`). An unpullable image never starts a container: the container sits in `Waiting` (`ErrImagePull`/`ImagePullBackOff`), the pod stays `Pending`, the Job stays Active. No declarative Job policy fires. The gap — packages with unpullable images report `in_progress` forever instead of `erroring` — predates this migration and survives it; the operator-side fix (extend the Waiting-reason handling in the Pod watch) is issue #306. `podFailurePolicy` is not set at all.

### Rejected: Job `spec.suspend` as a pause primitive

Suspending a Job with a running pod SIGTERMs the pod and recreates it from scratch on resume — no checkpointing. The `skyhook.nvidia.com/pause` annotation (block new stage scheduling, let in-flight work finish) remains the pause mechanism.

### Rejected: a separate JobReconciler with its own writes

A second reconciler doing read-modify-write against node annotations/SCR status would race the global "grab the world" pass; per-controller `MaxConcurrentReconciles: 1` does not serialize *across* controllers. Splitting the controller is a separate design if ever wanted; this migration keeps the single-queue pseudo-pattern.

### Rejected: one long-lived Job (or JobSet) per rollout

`JobSet` targets coordinated parallel workloads (ML training), not a sequential per-stage lifecycle; a Job per stage keeps the state machine's granularity identical to today. Cross-stage grouping is served by labels.

### Rejected: static TTL at creation

A single `ttlSecondsAfterFinished` value at creation can't distinguish success from failure, forcing a choice between losing failure logs early or retaining success pods for a long window at scale. Mutability of the field makes outcome-based TTL strictly better at the cost of one field in an Update the operator already makes.
