# Package Execution as Jobs design

This document specifies the migration of package-stage execution from operator-managed raw Pods to `batch/v1` Jobs — one Job per (skyhook, package, version, stage, node). It is the design companion to [issue #223](https://github.com/NVIDIA/nodewright/issues/223); implementation is tracked by sub-issues #299–#305, each delivered as its own PR to the `feature/package-as-jobs` branch.

CRD/schema changes, cordon/drain ownership, DeploymentPolicy semantics, and multi-node Job parallelism are **out of scope**. The ImagePullBackOff surfacing gap is explicitly **not** solved by this migration (see [Rejected: `podFailurePolicy`](#rejected-podfailurepolicy-for-imagepullbackoff)) and is tracked separately in issue #306.

## Problems

### Debugging requires racing the operator

Completion of a stage is detected by the operator reading init-container statuses, after which the operator **deletes the pod** ([`PodReconcile`](../../operator/internal/controller/pod_controller.go)). Logs for a completed stage are gone the moment the operator processes it; debugging requires catching the pod before deletion or knowing its generated name in advance.

### Hand-rolled lifecycle

Cleanup, completion tracking, "does this stage already run" checks, and stale-spec invalidation are all custom code across [`skyhook_controller.go`](../../operator/internal/controller/skyhook_controller.go) (`PodExists`, `ValidateRunningPackages`, `InvalidPackage`) and [`pod_controller.go`](../../operator/internal/controller/pod_controller.go). Kubernetes has a first-class run-to-completion primitive that carries most of this for free, plus ecosystem benefits (`kubectl logs job/<name>`, `kubectl describe job`, k9s/ArgoCD/dashboard treatment of Jobs as completable work).

## Goals

1. **Stable, queryable execution records**: one Job per (skyhook, package, version, stage, node); `kubectl logs job/<name>` works during and after execution.
2. **Logs outlive completion**: finished work is retained for a configurable window; the logs that matter most for humans (failing steps) are retained the longest.
3. **No behavior change to the lifecycle state machine**: Stage progression, Status/State derivation, interrupt/cordon/drain sequencing, DeploymentPolicy interactions, and agent-visible env are unchanged. In particular, a pod lost to disruption (eviction, node churn) is silently re-executed, exactly as today — it does not surface as `erroring` and does not count against DeploymentPolicy batch thresholds.
4. **Safe upgrade**: an operator upgrade with in-flight raw pods completes those stages correctly, without double-executing them.

## Current behavior (baseline)

Facts about today's implementation that shape the design:

- **Package pods never terminate.** All work runs sequentially in `initContainers` (init-copy → `<stage>` → `<stage>-check`); the main container is a `pause` image that runs forever ([`createPodFromPackage`](../../operator/internal/controller/skyhook_controller.go)). The lingering Running pod doubles as the "stage in flight, don't create another" marker for [`PodExists`](../../operator/internal/controller/skyhook_controller.go).
- **The operator is the completion detector.** [`containerExitedSuccessfully`](../../operator/internal/controller/pod_controller.go) inspects `InitContainerStatuses`; on success the operator updates node state and then deletes the pod. Deletion is also the **processed-once marker**: the `pod.DeletionTimestamp == nil` gate prevents double-processing of success events. (Note the write pair is not atomic today either: node Patch, then pod Delete — a crash between them can re-process. The Jobs design keeps the same window, not a larger one; see the marker section.)
- **Retries are kubelet in-place retries.** `restartPolicy: OnFailure` means a failing init container crash-loops forever; the operator only reports `erroring` with restart counts — it never recreates a failing pod. A pod that *disappears* (evicted, deleted) produces **no state write at all**: node state stays `in_progress` and a later reconcile silently recreates the pod via the `PodExists` miss; `ProcessInterrupt` even carries an explicit recreate comment for exactly this race.
- **Pods are owned by the Skyhook CR** (`SetControllerReference` in [`ApplyPackage`](../../operator/internal/controller/skyhook_controller.go)), carry labels `skyhook.nvidia.com/name` and `…/package` (interrupt pods additionally `…/interrupt: "True"`), and a JSON package annotation ([`annotations.go`](../../operator/internal/controller/annotations.go)) that round-trips (skyhook, package, version, stage, image).
- **Interrupt pods have their own name formula**: `generateSafeName(63, skyhook, stage, interruptType, node)` — no package/version — which is what dedupes the merged interrupt (one per node/stage/type, independent of which package's interrupt won `fudgeInterruptWithPriority`).
- **[`HandleConfigUpdates`](../../operator/internal/controller/skyhook_controller.go) deletes erroring package pods directly** so they are recreated with the updated configmap.
- **[`cluster_state_v2.go`](../../operator/internal/controller/cluster_state_v2.go) does not read pods.** Node annotations are the state store; pods are ephemeral executors. Stage metrics (`skyhook_package_stage_count`) are derived from node state in `setAllMetrics` — the stage-counting map inside `ValidateRunningPackages` is dead code (populated, never consumed) and is deleted, not ported.

## Design

### Job shape

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: <generateSafeName(63, skyhook, package, version, stage, node)>   # same input as today's package pod names
  # interrupt Jobs keep today's interrupt formula: generateSafeName(63, skyhook, stage, interruptType, node)
  namespace: <operator namespace>
  labels:
    skyhook.nvidia.com/name: <skyhook>            # existing
    skyhook.nvidia.com/package: <name>-<version>  # existing
    skyhook.nvidia.com/interrupt: "True"          # existing (interrupt Jobs only)
    skyhook.nvidia.com/stage: <stage>             # new
    skyhook.nvidia.com/node: <node>               # new; see Labels for >63-char node names
    skyhook.nvidia.com/generation: "<n>"          # new; the SCR generation that produced this Job
  annotations:
    skyhook.nvidia.com/resource-id: <name>-<uid>-<generation>   # full provenance; too long for a label
  ownerReferences: [<Skyhook CR>]                  # cascade delete; pods are owned by the Job
spec:
  parallelism: 1
  completions: 1
  backoffLimit: 0                 # see Retry semantics
  # With backoffLimit: 0 the Job controller never creates a replacement pod, so old/new
  # overlap on the shared hostPath mounts can't happen regardless. podReplacementPolicy:
  # Failed still matters for failure *timing*: a terminating-but-not-terminal pod is not
  # counted as failed yet, so the Job doesn't flip Failed mid-eviction.
  podReplacementPolicy: Failed
  # ttlSecondsAfterFinished: unset at creation — set by the operator at completion (see TTL)
  template:
    metadata:
      labels: <same label set>    # child pods inherit labels → existing CLI label queries keep working
      annotations:
        skyhook.nvidia.com/package: <same JSON package annotation as today's pods>
        # required on the CHILD POD: the in-flight erroring path (Pod watch → UpdateNodeState)
        # reads the package annotation from the pod, exactly as it does today
    spec:
      # identical to today's pod spec (initContainers, hostPID/hostNetwork, resources,
      # gracefulShutdown, imagePullSecrets), with two changes:
      tolerations:
      # existing: unschedulable (Exists), runtime-required, spec.additionalTolerations, plus NEW:
      - key: node.kubernetes.io/not-ready
        operator: Exists
        effect: NoExecute
      - key: node.kubernetes.io/unreachable
        operator: Exists
        effect: NoExecute
        # Without these, DefaultTolerationSeconds admission adds 300s variants and the taint
        # manager evicts the pod when a reboot-class interrupt keeps the node NotReady >5min —
        # failing the Job (backoffLimit: 0) for a routine slow reboot. These pods are
        # node-bound host-maintenance agents; evicting them is never useful.
      containers:
      - name: done                # replaces the forever-running `pause` container
        image: <agent image>      # already pulled for the init containers
        command: ["/bin/sh", "-c", "exit 0"]
```

**Why the main container changes:** a Job only records completion when its pod reaches `Succeeded`, and a pod only succeeds when all containers terminate. The init-container chain stays (regular containers run concurrently — `initContainers` are the only native run-to-completion sequencing primitive, and per-step container names are load-bearing for state reporting). The `pause` container's other job — keeping the pod object alive as the "in flight" marker — is taken over by the Job object itself, which outlives its pods.

Interrupt-over-reboot: for reboots that keep the node NotReady under ~5 minutes the pod object survives, kubelet re-runs the init containers, the agent skips the already-done interrupt via the `SKYHOOK_RESOURCE_ID` flag file and exits 0, and the Job completes. For slower reboots the new NoExecute tolerations (above) keep the pod bound instead of letting the taint manager evict it. If the pod is lost anyway (e.g. node object deleted and re-created), the Failed-Job path below re-executes the stage — the agent's flag files make re-execution idempotent.

### Completion flow and the processed-once marker

The pseudo-controller pattern is kept (issue #223 option A): a Jobs watch maps events into the single global reconcile queue as `job---<name>` requests, alongside the existing `pod---<name>` routing. No second reconciler writes SCR/node state, so the existing serialization guarantee holds. The Jobs informer is scoped to the operator namespace (`cache.Options.ByObject`) — the operator has no business caching every CronJob-spawned Job in the cluster.

Because completed pods now linger, pod-deletion can no longer be the processed-once marker. `JobReconcile` replaces it with a persisted annotation, `skyhook.nvidia.com/state-recorded: "true"`:

- **Job `Complete` and not yet marked** → read the child pod (via the `batch.kubernetes.io/job-name` label) for container name + restarts — falling back to the Job's own `stage`/`node`/`interrupt` labels and restarts `0` if the pod has been GC'd — → existing `UpdateNodeState(..., StateComplete, ...)` path (upgrade/uninstall/interrupt special cases in `HandleCompletePod` unchanged, including the uninstall → `uninstall-interrupt` transition) → **one Job Update** setting the `state-recorded` annotation and the success TTL. Already-marked Jobs are skipped, making duplicate events harmless.
- **Job `Failed`** (disruption only — see Retry semantics) → **no state write**. Node state stays `in_progress`, matching today's vanished-pod behavior; the failed Job is deleted by the rerun predicate below and the stage is silently re-executed. This deliberately does *not* surface as `erroring`, so DeploymentPolicy batch-failure counting is unaffected by transient disruptions (Goal 3). Genuine step failures still surface as `erroring` through the Pod watch (crash-looping pods), exactly as today.
- **Active Jobs** → no-op; the Pod watch keeps reporting in-flight crash-loop `erroring` and restart counts. This path requires the package annotation on the *child pod* (hence the pod-template annotation in the Job shape).

**Crash-window honesty:** node-state Patch and Job Update are writes to two objects and cannot be atomic. The order is state-first, marker-second — inverting it could permanently lose a completion (marked Job, state still `in_progress`). A crash between the two writes re-serves the completion event; since `UpdateNodeState`/`HandleCompletePod` are not idempotent, the re-processing path is guarded: the completion write is skipped (and the Job just marked) when the package's node-state entry already records progress at or beyond the Job's stage. Today's code has the same two-write window (node Patch, then pod Delete) with no guard; this is strictly an improvement.

### Retry semantics

Unchanged by design, and worth stating precisely because the Job knobs are easy to misread:

- The pod template keeps `restartPolicy: OnFailure` → a failing step crash-loops **in place** under kubelet backoff, forever, with the Job staying Active. This is today's behavior for genuine failures, and it is the path that produces `erroring` (via the Pod watch). The Job controller's `backoffLimit` does not see in-place restarts.
- `backoffLimit: 0` therefore only fires when the pod as a whole is lost (eviction, node churn, manual delete). Today that case is silently recreated via the `PodExists` miss; with Jobs it is the `Failed` → delete → recreate flow above. Same convergence, now with an observable Job event trail.

### TTL: outcome-based, set at completion

`ttlSecondsAfterFinished` applies to both `Complete` and `Failed` Jobs and is **mutable**. Jobs are created with the field unset; `JobReconcile` sets it in the same Update that writes `state-recorded`:

| Outcome | Operator env | Chart value (`controllerManager.manager.env.*`) | Default |
| --- | --- | --- | --- |
| Succeeded | `JOB_TTL_SUCCEEDED` | `jobTtlSucceeded` | `1h` |
| Failed | `JOB_TTL_FAILED` | `jobTtlFailed` | `24h` |

Both are wired through the chart's `controllerManager.manager.env` block exactly like `copyDirRoot`/`agentLogRoot` (see [`chart/values.yaml`](../../chart/values.yaml)).

Where failure logs actually live, per failure class: **crash-looping steps** (the common case) keep the Job Active indefinitely with a live, log-bearing pod — never TTL'd, matching today. **Disruption-failed** Jobs usually have no child pod left to read (that's what failed them) and are deleted promptly by the rerun predicate; `JOB_TTL_FAILED` is the backstop for Failed Jobs that nothing re-schedules (e.g. the node-state entry was removed while the Job was failing). If the operator is down when a Job finishes, the TTL is simply set when it resyncs — level-triggered, no timer lost.

Scale note: succeeded pods are retained for the success-TTL window — roughly `nodes × packages × 3 stages` extra terminated pods in etcd and the informer cache at peak. Two bounds apply: keep `JOB_TTL_SUCCEEDED` short (default 1h), and note that kube-controller-manager's terminated-pod GC (`--terminated-pod-gc-threshold`, default 12500) may delete retained *pods* (not Jobs) earlier on large clusters — retention is "until TTL or terminated-pod GC pressure". [`operator_resources_at_scale.md`](../operator_resources_at_scale.md) carries the sizing guidance.

### Naming, reruns, and the lifecycle of a finished Job

Job names are deterministic (same `generateSafeName` inputs as today's pods), which makes creation idempotent — but a finished Job retains its name until TTL, so every "run this stage again" path must first clear the name. Three precise rules replace today's delete-on-success:

**1. `ValidateRunningPackages` scoping.** Its existing checks — spec-mismatch (`jobMatchesPackage`), package-not-in-node-state, stage-mismatch-vs-node-state — apply to **unfinished Jobs only**. Applying them to finished Jobs would delete every retained Job the moment the node progresses to the next stage (the retained `apply` Job "stage-mismatches" as soon as `config` starts), silently gutting Goal 2. During the upgrade window the same checks also keep running against legacy raw pods.

**2. The rerun predicate (finished Jobs' only cleanup besides TTL).** A finished Job with stage `S` for package `P` is deleted (foreground) when it is **processed** — `Failed`, or `Complete` with `state-recorded` — and the node state no longer records `S` as done: `P`'s entry is absent (node reset, package removed), or its recorded progress is at or before `S` without `S` being complete (CLI `reset`/`package rerun`, `update-state`, `REAPPLY_ON_REBOOT` state reset, config-update reruns, disruption-failed Jobs). `Complete` Jobs that are *not* yet `state-recorded` are never touched by the predicate — their completion hasn't been recorded and deleting them would discard it; `JobReconcile` owns them. Progress comparison uses the state machine's existing stage ordering (`NextStage`/`IsPackageComplete`), not string equality.

**3. `AlreadyExists` on create is not blindly benign.** `ApplyPackage`/`Interrupt` handle it by GETting the Job: if it is unfinished and `jobMatchesPackage`, another pass won the race — return without touching node state. If it is finished or mismatched, delete it (foreground) and requeue; do **not** `Upsert` `in_progress` on this path, or the state machine records a run that isn't happening.

`JobExists` (successor of `PodExists`) counts only **unfinished** Jobs as "in flight" — a finished Job never blocks progression; rule 2 clears names ahead of recreation, and rule 3 is the belt-and-suspenders for same-pass races (the invalidation → deletion flow is asynchronous relative to the global pass).

`HandleConfigUpdates`, which today deletes erroring package *pods* so they restart with new config, deletes the **Job** instead (deleting just the child pod would fail the parent Job) and uses `JobExists` for its existence check.

### Labels

`ResourceID()` is `<name>-<uid>-<generation>` — routinely over the 63-character label-value limit, so issue #223's `resource-id` label is replaced by: full resource-id as an **annotation**, plus a numeric `skyhook.nvidia.com/generation` **label** carrying the SCR generation that produced the Job (note: config-update reruns and reboot re-applies reuse a generation; the annotation is the exact provenance). Node names are legal up to 253 characters, so the `…/node` label value is the node name when it fits and `generateSafeName(63, node)` otherwise; the operator computes the same transform on lookup, and the label-based queries below degrade only for such names. The issue's query goals are preserved with valid label values:

```bash
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init                         # all Jobs in one SCR
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init,skyhook.nvidia.com/package=pkg-1.0,skyhook.nvidia.com/node=worker-7,skyhook.nvidia.com/generation=4
                                                                             # one rollout of one package on one node
kubectl get jobs -l skyhook.nvidia.com/node=worker-7                         # everything touching a node
```

### RBAC

The operator ClusterRole gains `batch/jobs` `get;list;watch;create;update;patch;delete` (kubebuilder marker + `make manifests`, hand-mirrored into `chart/`). The role stays cluster-scoped for consistency with the existing single-ClusterRole layout, but the Jobs *informer* is namespace-scoped as noted above, and all Job writes target the operator namespace. Pod verbs stay: the operator still reads child pods for restarts/container names, reads workload pods for drain, and drives legacy pods during the upgrade window.

## Upgrade and compatibility

No persisted schema changes → **no `zz.migration` shim**. In-flight raw pods from the previous operator version are distinguishable by the absence of the `batch.kubernetes.io/job-name` label. For **one minor release** the operator runs legacy-aware, then all three legacy accommodations are removed together (release-notes entry on removal):

- **Completion:** the Pod watch keeps the old full path (update node state, delete pod) for legacy pods. Job-owned pods only report in-flight erroring/restarts; completion is owned by the Job path.
- **Existence gating:** `JobExists` and `HasRunningPackages` OR-in legacy pods (name label present, `job-name` label absent). Without this, the first reconcile after upgrade would create a duplicate Job executor for a stage already running as a legacy pod — two privileged chroot processes against the same host paths — and `Interrupt` could cordon/drain under a running legacy stage.
- **Validation:** `ValidateRunningPackages` keeps its legacy-pod sweep alongside the Job checks.

New stages created after the upgrade are Jobs immediately. Nothing needs to be proactively deleted (issue #223's migration option 1). Legacy pod names equal new Job names byte-for-byte, but they are different kinds and Job child pods get a `-<suffix>`, so there are no API name conflicts.

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

### Rejected: recording `erroring` on disruption-failed Jobs

An earlier draft had Job `Failed` → `StateErroring`. That would flip node/Skyhook Status and count the node against DeploymentPolicy `failureThreshold`/batch success for what today is an invisible, self-healing event (a vanished pod). Disruption failures are therefore silent re-executions (Goal 3); `erroring` remains reserved for genuinely failing steps, reported by the Pod watch from crash-loop/exit-code evidence — the same signal source DeploymentPolicy consumes today.

### Rejected: random Job name suffixes instead of deterministic names

Suffixes would sidestep the finished-Job name collision but break creation idempotency (a reconcile crash between Create and the node-state write could leave two Jobs racing the same stage) and make "does this stage already run" a list-and-filter instead of a name hit. Deterministic names + the rerun predicate keep the level-triggered property: the name *is* the claim.
