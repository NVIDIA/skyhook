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
- **Retries are kubelet in-place retries.** `restartPolicy: OnFailure` means a failing init container crash-loops forever; the operator only reports `erroring` with restart counts — it never recreates a failing pod. A pod that *disappears* (evicted, deleted) produces **no state write at all**: node state stays `in_progress` and a later reconcile silently recreates the pod via the `PodExists` miss; `ProcessInterrupt` even carries an explicit recreate comment for exactly this race. Pods bound to a **deleted node** are cleaned up by kube-controller-manager's PodGC, not by the operator.
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
  # Effectively unlimited, deliberately. With restartPolicy: OnFailure the Job controller
  # counts the SUM OF IN-PLACE CONTAINER RESTARTS toward backoffLimit and terminates the
  # pod when the limit is reached — a small value (or 0) would hand retry ownership to the
  # Job controller and kill a crash-looping step after its first restarts, breaking today's
  # "kubelet retries in place forever, operator only reports" model. Retry policy stays
  # with the kubelet and the operator; the Job controller must never give up first.
  backoffLimit: 2147483647
  # A lost pod (evicted, PodGC'd) is replaced by the Job controller — the template pins
  # nodeName, so the replacement lands on the same node. Failed means: only create the
  # replacement once the old pod is FULLY terminated, so two executors never overlap on
  # the shared hostPath mounts. Beta + on-by-default since 1.29 (GA 1.34). If a cluster
  # disables the gate the field is dropped and replacement reverts to TerminatingOrFailed:
  # operator-initiated recreates stay safe (foreground deletion frees the name only after
  # children are gone) but Job-controller replacements can briefly overlap a terminating
  # predecessor — accepted for that non-default configuration; the agent's flag files
  # keep re-execution idempotent.
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
        # manager evicts the pod when a reboot-class interrupt keeps the node NotReady >5min,
        # forcing a pointless replacement + hostPath re-copy for a node that is coming back.
        # Deliberately unbounded (no tolerationSeconds): these pods are node-bound host-
        # maintenance agents — running them anywhere else is meaningless, so eviction is
        # never useful. A node that stays NotReady forever holds the Job Active exactly as
        # today's raw pod would sit unreachable; a node that is REMOVED is handled by the
        # orphaned-node sweep (below), which today's PodGC-based cleanup does not need but
        # Jobs do (the Job controller would otherwise recreate pods for the missing node).
      containers:
      - name: done                # replaces the forever-running `pause` container
        image: <agent image>      # already pulled for the init containers
        command: ["/bin/sh", "-c", "exit 0"]
```

**Why the main container changes:** a Job only records completion when its pod reaches `Succeeded`, and a pod only succeeds when all containers terminate. The init-container chain stays (regular containers run concurrently — `initContainers` are the only native run-to-completion sequencing primitive, and per-step container names are load-bearing for state reporting). The `pause` container's other job — keeping the pod object alive as the "in-flight" marker — is taken over by the Job object itself, which outlives its pods.

Interrupt-over-reboot: for reboots that keep the node NotReady under ~5 minutes the pod object survives, kubelet re-runs the init containers, the agent skips the already-done interrupt via the `SKYHOOK_RESOURCE_ID` flag file and exits 0, and the Job completes. For slower reboots the NoExecute tolerations (above) keep the pod bound instead of letting the taint manager evict it. If the pod is lost anyway (force-deleted, PodGC), the Job controller creates a replacement on the same node and the agent's flag files make re-execution idempotent — the same silent self-healing today's `PodExists`-miss recreate provides.

### Completion flow and the processed-once marker

The pseudo-controller pattern is kept (issue #223 option A): a Jobs watch maps events into the single global reconcile queue as `job---<name>` requests, alongside the existing `pod---<name>` routing. No second reconciler writes SCR/node state, so the existing serialization guarantee holds. The Jobs informer is scoped to the operator namespace (`cache.Options.ByObject`) — the operator has no business caching every CronJob-spawned Job in the cluster.

Because completed pods now linger, pod-deletion can no longer be the processed-once marker. `JobReconcile` replaces it with a persisted annotation, `skyhook.nvidia.com/state-recorded: "true"`:

- **Job `Complete` and not yet marked** → read the terminal child pod for container name + restarts, selected by `batch.kubernetes.io/controller-uid == job.UID` (never by name-label alone: deterministic names mean a pod from a previous same-named Job instance can still be terminating). If no child pod survives (terminated-pod GC), fall back to the Job's own labels: `stage`, `node`, and `interrupt` — container name is only ever *compared* against `InterruptContainerName` in `HandleCompletePod`, so the interrupt label fully determines it; restarts default to `0`. Then the existing `UpdateNodeState(..., StateComplete, ...)` path runs (upgrade/uninstall/interrupt special cases in `HandleCompletePod` unchanged, including the uninstall → `uninstall-interrupt` transition), followed by **one Job Update** setting the `state-recorded` annotation and the success TTL. Already-marked Jobs are skipped, making duplicate events harmless.
- **Job `Failed`** — a backstop branch: with effectively-unlimited backoff the Job controller should never fail a Job on its own (see Retry semantics). If a Job is `Failed` anyway (operator bug, manual meddling, future policy), the handling is: **no node-state write** (matching today's disruption behavior — no `erroring`, no DeploymentPolicy impact), and one Job Update setting `state-recorded` plus `JOB_TTL_FAILED`. A `Failed` Job already counts as "processed" for the rerun predicate with or without the marker; the Update exists to carry the TTL, the backstop for Failed Jobs nothing reschedules. Whether the predicate deletes the Job before or after the TTL lands is immaterial — both converge.
- **Node gone:** if the completion path's node `Get` returns NotFound (node scaled down right after finishing, or while the operator was down), there is no state to record — mark the Job `state-recorded` with the outcome TTL and stop, rather than error-looping on a node that will never come back. Rule 4 (below) cleans the Job up.
- **Active Jobs** → no-op; the Pod watch keeps reporting in-flight crash-loop `erroring` and restart counts. This path requires the package annotation on the *child pod* (hence the pod-template annotation in the Job shape).

**Crash-window honesty:** node-state Patch and Job Update are writes to two objects and cannot be atomic. The order is state-first, marker-second — inverting it could permanently lose a completion (marked Job, state still `in_progress`). A crash between the two writes re-serves the completion event; since `UpdateNodeState`/`HandleCompletePod` are not idempotent, the re-processing path is guarded by a **postcondition check**: before re-applying, `JobReconcile` verifies whether the transition's outcome already holds — normal stage → entry records the stage complete; upgrade → stale-version entries gone and entry complete; uninstall without interrupt → entry absent; uninstall with interrupt → entry at `uninstall-interrupt` or beyond, or absent; interrupt (the `InterruptContainerName` path, which also runs `ProgressSkipped`) → the winning package's entry at `interrupt` complete or beyond **and** no spec package still at (`interrupt`, `skipped`). If the postcondition holds, the Job is only marked, not re-processed. (Keying dedup by Job UID inside the node-state entry was considered and rejected: the `nodeState_*` annotation's `map[string]PackageStatus` shape has been stable since v0.7.5 and older CLIs' `update-state` rewrites the whole value, silently dropping fields they don't know.) Today's code has the same two-write window (node Patch, then pod Delete) with no guard at all; this is strictly an improvement.

### Retry semantics

The retry model is unchanged from today — but expressing that with Jobs requires care, because the Job controller's failure accounting is easy to misread:

- The pod template keeps `restartPolicy: OnFailure` → a failing step crash-loops **in place** under kubelet backoff, with the Job staying Active and a live, log-bearing pod. This is today's behavior for genuine failures, and it is the path that produces `erroring` (via the Pod watch).
- **`backoffLimit` counts in-place restarts under `OnFailure`.** The Job controller's retry accounting is *pods with phase `Failed`* **plus, for `restartPolicy: OnFailure`, the sum of container restart counts across the Job's pods** — and when the limit is reached it terminates the pod and fails the Job. So `backoffLimit: 0` would kill every package on its first step retry. The design therefore sets `backoffLimit` to `math.MaxInt32`: the Job controller never gives up before the operator does, which is exactly today's contract.
- **Pod loss is self-healing inside the Job.** A pod that fully terminates or disappears (eviction, force delete, PodGC after node deletion-and-recreation) counts one failure against the effectively-unlimited budget, and the Job controller creates a replacement pod — pinned to the same node by the template's `nodeName`, gated by `podReplacementPolicy: Failed` so it never overlaps a terminating predecessor on the shared hostPath mounts. No operator state write, no `erroring`, no DeploymentPolicy impact: the same silent recreate today's `PodExists` miss provides, minus the hand-rolled code.
- A **`Failed` Job is a backstop state**, not part of the normal model (see the completion-flow bullet above).

### TTL: outcome-based, set at completion

`ttlSecondsAfterFinished` applies to both `Complete` and `Failed` Jobs and is **mutable**. Jobs are created with the field unset; `JobReconcile` sets it in the same Update that writes `state-recorded` — for `Complete` and for backstop-`Failed` Jobs alike:

| Outcome | Operator env | Chart value (`controllerManager.manager.env.*`) | Default |
| --- | --- | --- | --- |
| Succeeded | `JOB_TTL_SUCCEEDED` | `jobTtlSucceeded` | `1h` |
| Failed | `JOB_TTL_FAILED` | `jobTtlFailed` | `24h` |

Both are wired through the chart's `controllerManager.manager.env` block exactly like `copyDirRoot`/`agentLogRoot` (see [`chart/values.yaml`](../../chart/values.yaml)).

Where failure logs actually live: **crash-looping steps** (the real failure mode) keep the Job Active indefinitely with a live, log-bearing pod — never TTL'd, matching today. Backstop-`Failed` Jobs get `JOB_TTL_FAILED` as insurance against accumulation. If the operator is down when a Job finishes, the TTL is simply set when it resyncs — level-triggered, no timer lost.

Scale note: succeeded pods are retained for the success-TTL window — roughly `nodes × packages × 3 stages` extra terminated pods in etcd and the informer cache at peak. Two bounds apply: keep `JOB_TTL_SUCCEEDED` short (default 1h), and note that kube-controller-manager's terminated-pod GC (`--terminated-pod-gc-threshold`, default 12500) may delete retained *pods* (not Jobs) earlier on large clusters — retention is "until TTL or terminated-pod GC pressure" (the completion path's no-child-pod fallback exists for exactly this). [`operator_resources_at_scale.md`](../operator_resources_at_scale.md) carries the sizing guidance.

### Naming, reruns, and the lifecycle of a finished Job

Job names are deterministic (same `generateSafeName` inputs as today's pods), which makes creation idempotent — but a finished Job retains its name until TTL, so every "run this stage again" path must first clear the name. Four precise rules replace today's delete-on-success; together with TTL they are a finished Job's complete lifecycle. Throughout, "processed" means `Failed`, or `Complete` with `state-recorded`; deletions of Jobs are always **foreground**, which matters twice over: the deterministic name only frees after the children are fully gone, so a recreate can never overlap a terminating predecessor's pods on the hostPath mounts.

**1. `ValidateRunningPackages` scoping.** Its existing checks — spec-mismatch (`jobMatchesPackage`), package-not-in-node-state, stage-mismatch-vs-node-state — apply to **unfinished Jobs only**. Applying them to finished Jobs would delete every retained Job the moment the node progresses to the next stage (the retained `apply` Job "stage-mismatches" as soon as `config` starts), silently gutting Goal 2. During the upgrade window the same checks also keep running against legacy raw pods.

**2. The rerun predicate.** A **processed** finished Job with stage `S` for package `P` is deleted when the node state no longer records `S` as done: `P`'s entry is absent (node reset via CLI or `REAPPLY_ON_REBOOT`, package removed), or its recorded progress is at or before `S` without `S` being complete (`package rerun`, `update-state`, config-update reruns). **Unprocessed `Complete` Jobs are never deleted by anyone** — their completion hasn't been recorded yet and deleting them would discard it; `JobReconcile` owns them. Progress comparison uses the state machine's existing stage ordering (`NextStage`/`IsPackageComplete`), not string equality. Two retention carve-outs follow from the predicate and are accepted: a successful no-interrupt **uninstall** Job's recorded completion *is* entry removal, so it satisfies the absent-entry branch immediately and gets no retention window; and because `uninstall` precedes `apply` in the stage ordering, starting an uninstall cycle also clears the package's retained `apply`/`config` Jobs. Uninstall is the "remove all trace" operation — prompt cleanup is coherent with its semantics, and crash-loop *failures* of uninstall still retain logs (Active Job).

**3. `AlreadyExists` on create is not blindly benign.** `ApplyPackage`/`Interrupt` handle it by GETting the Job: **unfinished and `jobMatchesPackage`** → another pass won the race; return without touching node state. **Processed, or unfinished-but-mismatched** → foreground-delete and requeue; do **not** `Upsert` `in_progress` on this path, or the state machine records a run that isn't happening. **`Complete` but not yet `state-recorded`** → do not delete (that would discard an unrecorded completion); return and let `JobReconcile` process it — the requeued pass then finds the state advanced.

**4. Orphaned-node sweep.** Jobs — **regardless of status** — whose `node` no longer exists in the cluster are deleted. Today this case needs nothing: PodGC deletes raw pods bound to deleted nodes and the state died with the node. With Jobs, two failure shapes appear: the Job controller would replace each PodGC'd pod of an *unfinished* Job with another one pinned to the missing node, churning forever; and a *finished* Job for a deleted node has no node state left to compare against, so no other rule ever claims it. The operator deletes both.

`HasRunningPackages` — the "wait until nothing is running" gate `Interrupt` uses before firing an interrupt — is reimplemented over Jobs with the same rule as `JobExists`: **unfinished Jobs only** (plus legacy pods during the upgrade window). It must not be left pod-based: it has no phase filter today (any pod with the name label counts), so retained Succeeded child pods would otherwise hold every interrupt hostage — node cordoned and drained — until the success TTL expired.

`JobExists` (successor of `PodExists`) counts only **unfinished** Jobs as "in flight" — a finished Job never blocks progression; rule 2 clears names ahead of recreation, and rule 3 is the belt-and-suspenders for same-pass races (the invalidation → deletion flow is asynchronous relative to the global pass).

`HandleConfigUpdates`, which today deletes erroring package *pods* so they restart with new config, deletes the **Job** instead (deleting just the child pod would merely trigger a same-config replacement pod) and uses `JobExists` for its existence check.

### Labels

`ResourceID()` is `<name>-<uid>-<generation>` — routinely over the 63-character label-value limit, so issue #223's `resource-id` label is replaced by: full resource-id as an **annotation**, plus a numeric `skyhook.nvidia.com/generation` **label** carrying the SCR generation that produced the Job (note: same-generation reruns exist — `REAPPLY_ON_REBOOT` resets and erroring retries don't bump the generation, though config updates do; the annotation is the exact provenance). Node names are legal up to 253 characters, so the `…/node` label value is the node name when it fits and `generateSafeName(63, node)` otherwise; the operator computes the same transform on lookup, `spec.template.spec.nodeName` always carries the full authoritative node name, and the label-based queries below degrade only for such names. The issue's query goals are preserved with valid label values:

```bash
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init                         # all Jobs in one SCR
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init,skyhook.nvidia.com/package=pkg-1.0,skyhook.nvidia.com/node=worker-7,skyhook.nvidia.com/generation=4
                                                                             # one rollout of one package on one node
kubectl get jobs -l skyhook.nvidia.com/node=worker-7                         # everything touching a node
```

### RBAC

The operator ClusterRole gains `batch/jobs` `get;list;watch;create;update;patch;delete` (kubebuilder marker + `make manifests`, hand-mirrored into `chart/`). The role stays cluster-scoped for consistency with the existing single-ClusterRole layout, but the Jobs *informer* is namespace-scoped as noted above, and all Job writes target the operator namespace. Pod verbs stay: the operator still reads child pods for restarts/container names, reads workload pods for drain, and drives legacy pods during the upgrade window.

## Upgrade and compatibility

No persisted schema changes → **no `zz.migration` shim**. In-flight raw pods from the previous operator version are distinguishable by the absence of the `batch.kubernetes.io/job-name` label. For **one minor release** the operator runs legacy-aware, then all four legacy accommodations are removed together (release-notes entry on removal):

- **Completion:** the Pod watch keeps the old full path (update node state, delete pod) for legacy pods. Job-owned pods only report in-flight erroring/restarts; completion is owned by the Job path.
- **Existence gating:** `JobExists` and `HasRunningPackages` OR-in legacy pods (name label present, `job-name` label absent). Without this, the first reconcile after upgrade would create a duplicate Job executor for a stage already running as a legacy pod — two privileged chroot processes against the same host paths — and `Interrupt` could cordon/drain under a running legacy stage.
- **Validation:** `ValidateRunningPackages` keeps its legacy-pod sweep alongside the Job checks.
- **Config updates:** `HandleConfigUpdates` also keeps its direct pod deletion for legacy erroring pods. Its Job-shaped rewrite alone would deadlock a config update issued during the window: a legacy pod crash-looping on stale config has no Job to delete, nothing else removes it (its stage matches node state, and `jobMatchesPackage` can't see configMap *content*), and the existence gating above blocks a replacement from being created.

New stages created after the upgrade are Jobs immediately. Nothing needs to be proactively deleted (issue #223's migration option 1). Legacy pod names equal new Job names byte-for-byte, but they are different kinds and Job child pods get a `-<suffix>`, so there are no API name conflicts.

CLI: child pods inherit the full label set via the pod template, so `kubectl skyhook package logs/status/rerun` label queries work unchanged against both operator generations; retained pods make post-completion `package logs` *better*. No CLI code change is expected; the verification is part of #305.

## Rejected alternatives

### Rejected: `podFailurePolicy` for ImagePullBackOff

`podFailurePolicy` evaluates `OnExitCodes` (requires a terminated container) and `OnPodConditions` (e.g. `DisruptionTarget`). An unpullable image never starts a container: the container sits in `Waiting` (`ErrImagePull`/`ImagePullBackOff`), the pod stays `Pending`, the Job stays Active. No declarative Job policy fires. The gap — packages with unpullable images report `in_progress` forever instead of `erroring` — predates this migration and survives it; the operator-side fix (extend the Waiting-reason handling in the Pod watch) is issue #306. `podFailurePolicy` is not set at all (it also cannot be combined with `restartPolicy: OnFailure`, which the retry model requires).

### Rejected: Job `spec.suspend` as a pause primitive

Suspending a Job with a running pod SIGTERMs the pod and recreates it from scratch on resume — no checkpointing. The `skyhook.nvidia.com/pause` annotation (block new stage scheduling, let in-flight work finish) remains the pause mechanism.

### Rejected: a separate JobReconciler with its own writes

A second reconciler doing read-modify-write against node annotations/SCR status would race the global "grab the world" pass; per-controller `MaxConcurrentReconciles: 1` does not serialize *across* controllers. Splitting the controller is a separate design if ever wanted; this migration keeps the single-queue pseudo-pattern.

### Rejected: one long-lived Job (or JobSet) per rollout

`JobSet` targets coordinated parallel workloads (ML training), not a sequential per-stage lifecycle; a Job per stage keeps the state machine's granularity identical to today. Cross-stage grouping is served by labels.

### Rejected: static TTL at creation

A single `ttlSecondsAfterFinished` value at creation can't distinguish success from failure, forcing a choice between losing failure logs early or retaining success pods for a long window at scale. Mutability of the field makes outcome-based TTL strictly better at the cost of one field in an Update the operator already makes.

### Rejected: recording `erroring` on disruption-failed Jobs

An earlier draft had Job `Failed` → `StateErroring`. That would flip node/Skyhook Status and count the node against DeploymentPolicy `failureThreshold`/batch success for what today is an invisible, self-healing event (a vanished pod). Disruption recovery is therefore silent (the Job controller's replacement pod); `erroring` remains reserved for genuinely failing steps, reported by the Pod watch from crash-loop/exit-code evidence — the same signal source DeploymentPolicy consumes today.

### Rejected: `backoffLimit: 0` with operator-driven recreation

Two earlier drafts tried small `backoffLimit` values. `backoffLimit: 0` with `restartPolicy: OnFailure` is broken outright — the Job controller counts in-place container restarts toward the limit and terminates the pod, so the first step retry would kill the package. Pairing `backoffLimit: 0` with `restartPolicy: Never` would make every step retry an operator-driven Job delete/recreate (new pod, image re-pull, restart accounting lost) and hand the crash-loop lifecycle to code this migration is trying to delete. Effectively-unlimited backoff keeps both retry classes where they live today: in-place restarts with the kubelet, pod replacement with the Job controller.

### Rejected: random Job name suffixes instead of deterministic names

Suffixes would sidestep the finished-Job name collision but break creation idempotency (a reconcile crash between Create and the node-state write could leave two Jobs racing the same stage) and make "does this stage already run" a list-and-filter instead of a name hit. Deterministic names + the rerun predicate keep the level-triggered property: the name *is* the claim.
