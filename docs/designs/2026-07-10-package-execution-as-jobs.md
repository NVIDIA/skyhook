# Package Execution as Jobs design

This document specifies the migration of package-stage execution from operator-managed raw Pods to `batch/v1` Jobs — one Job per (skyhook, package, version, stage, node). It is the design companion to [issue #223](https://github.com/NVIDIA/nodewright/issues/223); implementation is tracked by sub-issues #299–#305, each delivered as its own PR to the `feature/package-as-jobs` branch.

Cordon/drain ownership, DeploymentPolicy semantics, and multi-node Job parallelism are **out of scope**. CRD/schema changes are out of scope with **one deliberate additive exception**: a new per-package `stageTimeout` field (see [Stage deadline](#stage-deadline-bounded-runtime-parked-failures)). The ImagePullBackOff surfacing gap keeps its precise fix in issue #306 (see [Rejected: `podFailurePolicy`](#rejected-podfailurepolicy-for-imagepullbackoff)), though the stage deadline now bounds how long *any* stuck stage — unpullable images included — can sit unreported.

## Problems

### Debugging requires racing the operator

Completion of a stage is detected by the operator reading init-container statuses, after which the operator **deletes the pod** ([`PodReconcile`](../../operator/internal/controller/pod_controller.go)). Logs for a completed stage are gone the moment the operator processes it; debugging requires catching the pod before deletion or knowing its generated name in advance.

### Hand-rolled lifecycle

Cleanup, completion tracking, "does this stage already run" checks, and stale-spec invalidation are all custom code across [`skyhook_controller.go`](../../operator/internal/controller/skyhook_controller.go) (`PodExists`, `ValidateRunningPackages`, `InvalidPackage`) and [`pod_controller.go`](../../operator/internal/controller/pod_controller.go). Kubernetes has a first-class run-to-completion primitive that carries most of this for free, plus ecosystem benefits (`kubectl logs job/<name>`, `kubectl describe job`, k9s/ArgoCD/dashboard treatment of Jobs as completable work).

## Goals

1. **Stable, queryable execution records**: one Job per (skyhook, package, version, stage, node); `kubectl logs job/<name>` works during and after execution.
2. **Logs outlive completion**: finished work is retained for a configurable window; the logs that matter most for humans (failing steps) are retained the longest.
3. **No behavior change to the lifecycle state machine, with deliberate enhancements**: Stage progression, Status/State derivation, interrupt/cordon/drain sequencing, DeploymentPolicy interactions, and agent-visible env are unchanged. A pod lost to *disruption* (eviction, preemption, taint manager) is silently re-executed with no `erroring` and no DeploymentPolicy impact, exactly as today. The enhancements: a stage that runs past its **deadline** (hung, crash-looping, or unpullable) is failed, surfaced as `erroring`, and **parked**; **pause becomes a true stop** (cascades to Job suspension); and each genuinely failing stage keeps a **full-log archive pod** of its most recent failed attempt.
4. **Failure evidence outlives failure**: the most recent failed attempt of a stage stays `kubectl logs`-queryable while the stage retries and after it parks.
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
  # Effectively unlimited, deliberately: the Job controller must never give up before the
  # operator does. Under restartPolicy: Never, backoffLimit counts pods in phase Failed —
  # and with the podFailurePolicy rule below, only *genuine* step failures count, not
  # disruptions. The retry loop is: attempt fails → Failed pod archived (see the
  # failed-attempt archive) → Job controller creates the next attempt under its
  # per-Job exponential backoff (10s doubling, capped at 6 min).
  backoffLimit: 2147483647
  # Disruptions (eviction API, preemption, taint manager, PodGC) set the DisruptionTarget
  # pod condition; Ignore means they neither count toward backoffLimit nor as a failed
  # attempt — the replacement is created and nothing is recorded, preserving today's
  # "disruption is silent" behavior. Requires restartPolicy: Never (API rule); GA at our
  # 1.31 floor. Genuine failures (nonzero exit, OOMKilled) match no rule and count normally.
  podFailurePolicy:
    rules:
    - action: Ignore
      onPodConditions:
      - type: DisruptionTarget
  # Wall-clock bound on the whole stage, all attempts included. From the package's
  # stageTimeout field (default: operator JOB_STAGE_TIMEOUT, 1h; "0" means the field is
  # OMITTED — a literal 0 would insta-fail every Job). On expiry
  # the Job controller deletes the active pod and fails the Job (DeadlineExceeded) — see
  # "Stage deadline" for the erroring + parked semantics that follow.
  activeDeadlineSeconds: <stageTimeout>

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
      # gracefulShutdown, imagePullSecrets), with three changes:
      # (1) restartPolicy: Never — each attempt is a fresh pod, so the previous failure
      #     survives as a full-log archive pod instead of being destroyed by an in-place
      #     restart. INTERRUPT JOBS ARE THE EXCEPTION: they keep restartPolicy: OnFailure
      #     (and no podFailurePolicy — the API forbids combining them), because a reboot
      #     interrupt kills its own pod BY DESIGN; under Never every successful reboot
      #     would mint a spurious failed attempt, while in-place restart after the node
      #     returns is the proven recovery shape (agent skips via the resource-id flag).
      restartPolicy: Never
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

- **Job `Complete` and not yet marked** → read the **`Succeeded`** child pod for the container name (a Complete Job may still own the archived `Failed` pod of an earlier attempt), selected by `batch.kubernetes.io/controller-uid == job.UID` (never by name-label alone: deterministic names mean a pod from a previous same-named Job instance can still be terminating); the attempts figure recorded as "restarts" comes from the Job's `status.failed` (interrupt Jobs: the succeeded pod's `restartCount`, as today). This is a deliberate, user-visible improvement to the `nodeState_*` annotation and CLI output — today's recorded number is the final container's `restartCount`, usually `0` even after a crashy apply. If no child pod survives (terminated-pod GC), fall back to the Job's own labels: `stage`, `node`, and `interrupt` — container name is only ever *compared* against `InterruptContainerName` in `HandleCompletePod`, so the interrupt label fully determines it. Then the existing `UpdateNodeState(..., StateComplete, ...)` path runs (upgrade/uninstall/interrupt special cases in `HandleCompletePod` unchanged, including the uninstall → `uninstall-interrupt` transition), followed by **one Job Update** setting the `state-recorded` annotation and the success TTL. Already-marked Jobs are skipped, making duplicate events harmless.
- **Job with `FailureTarget` (deadline tripped, pods still terminating)** — not terminal yet: take the best-effort **log-tail snapshot** into the `skyhook.nvidia.com/last-logs` annotation while the dying pod's logs are still readable (see Stage deadline). Terminal handling waits for `Failed`.
- **Job `Failed` with reason `DeadlineExceeded`** — the stage ran past its deadline (see [Stage deadline](#stage-deadline-bounded-runtime-parked-failures)): a genuine failure. Retry the log-tail snapshot if it's still missing and a child pod survives, write `UpdateNodeState(..., StateErroring, ...)` (postcondition-guarded like the Complete path; crash-looping stages are usually already `erroring` from the Pod watch, so this mostly matters for *hung* stages), then one Job Update setting `state-recorded` plus `JOB_TTL_FAILED`. The Failed Job is then deliberately **left in place as the park marker** — see rule 2.
- **Job `Failed`, any other reason** — a backstop branch: with effectively-unlimited backoff and the deadline handled above, the Job controller should never fail a Job on its own. If it happens anyway (operator bug, manual meddling, future policy), the handling is: **no node-state write** (matching today's disruption behavior — no `erroring`, no DeploymentPolicy impact), and one Job Update setting `state-recorded` plus `JOB_TTL_FAILED`. A `Failed` Job already counts as "processed" for the rerun predicate with or without the marker; the Update exists to carry the TTL. Whether the predicate deletes the Job before or after the TTL lands is immaterial — both converge.
- **Node gone:** if the completion path's node `Get` returns NotFound (node scaled down right after finishing, or while the operator was down), there is no state to record — mark the Job `state-recorded` with the outcome TTL and stop, rather than error-looping on a node that will never come back. Rule 4 (below) cleans the Job up.
- **Active Jobs** → no terminal handling; the Pod watch keeps reporting in-flight `erroring` from failed-attempt evidence (subject to the guards in Retry semantics), with the attempt count taken from the Job's `status.failed`, and `JobReconcile` runs the failed-attempt pruner (keep newest `Failed` pod, delete older). The evidence path requires the package annotation on the *child pod* (hence the pod-template annotation in the Job shape).

**Crash-window honesty:** node-state Patch and Job Update are writes to two objects and cannot be atomic. The order is state-first, marker-second — inverting it could permanently lose a completion (marked Job, state still `in_progress`). A crash between the two writes re-serves the completion event; since `UpdateNodeState`/`HandleCompletePod` are not idempotent, the re-processing path is guarded by a **postcondition check**: before re-applying, `JobReconcile` verifies whether the transition's outcome already holds — normal stage → entry records the stage complete; upgrade → stale-version entries gone and entry complete; uninstall without interrupt → entry absent; uninstall with interrupt → entry at `uninstall-interrupt` or beyond, or absent; interrupt (the `InterruptContainerName` path, which also runs `ProgressSkipped`) → the winning package's entry at `interrupt` complete or beyond **and** no spec package still at (`interrupt`, `skipped`). If the postcondition holds, the Job is only marked, not re-processed. (Keying dedup by Job UID inside the node-state entry was considered and rejected: the `nodeState_*` annotation's `map[string]PackageStatus` shape has been stable since v0.7.5 and older CLIs' `update-state` rewrites the whole value, silently dropping fields they don't know.) Today's code has the same two-write window (node Patch, then pod Delete) with no guard at all; this is strictly an improvement.

### Retry semantics and the failed-attempt archive

Retries stay unlimited-until-the-deadline, but the substrate changes from kubelet in-place restarts to Job-controller attempts, in exchange for durable failure evidence:

- **Each attempt is a fresh pod** (`restartPolicy: Never` on package/stage Jobs). A genuinely failing step fails its pod; the Job controller creates the next attempt under its per-Job exponential backoff (10s doubling, capped at 6 min — comparable to kubelet crash-loop backoff's 5-min cap), pinned to the same node by the template's `nodeName` and gated by `podReplacementPolicy: Failed` so attempts never overlap on the shared hostPath mounts. The agent's host-side flag files carry across attempts: completed steps are skipped, so each retry re-runs only the init-copy and the failing step. Image-pull cost is unchanged — `imagePullPolicy: Always` re-queries the registry on every container start in both models.
- **The failed-attempt archive (keep one).** Terminal-phase pods are not touched by the Job controller, so failed attempts would accumulate; instead, the operator prunes them: select the Job's children by `batch.kubernetes.io/controller-uid`, order by `creationTimestamp`, and keep as the archive the newest `Failed` pod **without a `DisruptionTarget` condition** (an Ignored disruption casualty carries no failure verdict and must not shadow a real one, nor suppress the deadline snapshot); delete the rest by normal deletion — never force-delete, since the job-tracking finalizer is what guarantees a failure is counted and policy-classified before removal. Steady state is one running attempt plus one full-log archive of the most recent genuine failure — `kubectl logs` on the archive works while the stage retries, after it parks on deadline, and through a pause.
- **Disruptions stay silent.** The `podFailurePolicy` `Ignore`-on-`DisruptionTarget` rule (Job shape above) makes evictions, preemptions, taint-manager kills, and PodGC deletions count neither toward `backoffLimit` nor as failed attempts — the replacement appears and nothing is recorded, matching today's invisible `PodExists`-miss recreate. Two honest deltas around **hard node crashes / unexpected reboots** mid-stage (no `DisruptionTarget`): the crash lands one archived, counted attempt, and whether it can *briefly* flap node state depends on what the kubelet recorded — `ContainerStatusUnknown` is guarded, but a plain `terminated{137, Error}` shape passes the guards and shows `erroring` until the replacement attempt completes (self-healing via the agent's flag files; `REAPPLY_ON_REBOOT` deployments reset the node anyway). Separately, a rebooting node can transiently reject the nodeName-pinned replacement at kubelet admission (`OutOfPods` etc.) — those `Failed` pods have no container statuses (never `erroring`) but do count attempts and churn the archive slot; this replaces a *worse* latent behavior today, where an admission-rejected raw pod satisfies `PodExists` forever and wedges the stage.
- **Attempt accounting**: the "restarts" figure recorded in node state becomes the parent Job's `status.failed` (genuine attempts only, thanks to the `Ignore` rule). Interrupt Jobs — which keep `restartPolicy: OnFailure` (Job shape note) — retain today's container `restartCount` semantics.
- **Erroring evidence** still comes from pod observation, with three guards — a `Failed` package pod marks the node `erroring` only if: (a) it carries no `DisruptionTarget` condition; (b) its step container terminated with a real verdict (nonzero exit incl. `OOMKilled`; `ContainerStatusUnknown` — the kubelet-couldn't-tell artifact of node crashes — is skipped, as are kubelet admission rejections, which have `status.reason` set and no container statuses at all); and (c) **its `DeletionTimestamp` is unset** — a pod killed *by deletion* is never a step verdict. Guard (c) is load-bearing: pause suspension, rule 2/3 Job deletions, `HandleConfigUpdates`, the orphan sweep, and a human's `kubectl delete pod` all kill pods without `DisruptionTarget`, and every one of them must stay silent (deadline `erroring` comes from the Job's `DeadlineExceeded` branch, not from pod evidence, so nothing is lost). Today's `CrashLoopBackOff` waiting-state detection now applies **only to interrupt Jobs** (a `Never` pod fails once and goes terminal — it never enters `CrashLoopBackOff`); package-Job erroring comes solely from `Failed`-pod evidence under these guards.
- A **`Failed` Job means deadline or backstop**: `DeadlineExceeded` is the one first-class Job failure (next section); any other reason is the backstop branch of the completion flow. Individual *pod* failures never fail the Job (effectively-unlimited `backoffLimit`).

### Stage deadline: bounded runtime, parked failures

Today a repeatedly-failing stage churns forever and a *hung* stage (stuck script, unpullable image, dead registry) sits `in_progress` invisibly forever. Each stage Job now carries `activeDeadlineSeconds` — a wall-clock bound on the whole stage, all attempts and (for interrupt Jobs) reboot time included.

**Configuration:** a new **additive** per-package SCR field, following the existing `gracefulShutdown` `metav1.Duration` pattern:

```yaml
spec:
  packages:
    mypackage:
      version: 1.0.0
      image: ghcr.io/example/pkg
      stageTimeout: 2h        # optional; bounds each stage Job for this package
```

Unset falls back to the operator default `JOB_STAGE_TIMEOUT` (chart value `controllerManager.manager.env.jobStageTimeout`), default `1h`; an explicit `"0"` disables the deadline for that package — meaning the builders **omit** `activeDeadlineSeconds` entirely (a literal `0` in the field would insta-fail every Job). Webhook validation: non-negative duration. This is the only CRD change in the migration; it is additive and needs no `zz.migration` shim (absent field = operator default).

**Semantics on expiry:** the Job controller deletes the active pod and marks the Job `Failed`/`DeadlineExceeded` — note the terminal `Failed` condition only lands once the pods have *fully terminated*, so on an **unreachable node** (kubelet gone, unbounded tolerations holding the pod) the Job can sit at `FailureTarget` without going terminal indefinitely; `JobReconcile` therefore treats a `FailureTarget` older than a grace window as erroring evidence too (state write only — the marker/TTL/park still wait for `Failed`), so a reboot interrupt that bricks its node still surfaces. On the normal path `JobReconcile` records `erroring` (for repeatedly-failing stages the Pod watch usually already has; for hung stages this is the *first* signal anyone gets) and marks the Job processed with `JOB_TTL_FAILED`. The Failed Job then **parks**: rule 2 deliberately does not delete it while the package's entry sits at (same stage, `erroring`), so — unlike every other failure path — nothing recreates the stage. The churn stops. The park clears on any explicit signal: CLI `package rerun`/`reset`/`update-state` (entry changes → predicate fires), a config update (`HandleConfigUpdates` deletes erroring Jobs), a spec change, or `JOB_TTL_FAILED` expiry — after which the still-`erroring` state causes one fresh attempt, i.e. a deliberate slow retry cadence (~`stageTimeout` on, `JOB_TTL_FAILED` off) until a human intervenes.

**Log-visibility caveat, and the log-tail snapshot:** deadline expiry *deletes* the running pod — pod deletion is the only way Kubernetes can stop a hung container — and the kubelet garbage-collects the container log files with it, so `kubectl logs job/<name>` has nothing to read *after* the deadline. To keep the last evidence API-queryable, the operator exploits the termination window: since 1.31 the Job controller adds the **`FailureTarget`** condition when the deadline trips, *before* the pods finish terminating, and the pod then lives on through its grace period (extended by the package's `gracefulShutdown`). `JobReconcile` reacts to `FailureTarget` with a **best-effort log-tail snapshot**:

- if a **failed-attempt archive pod** exists (Retry semantics), the snapshot is unnecessary — the archive already carries the full logs of the last real failure and survives the deadline;
- otherwise identify the stuck container on the terminating child pod (first init container not terminated with exit 0) and fetch its log tail via the pod-logs API with a hard byte cap (constant, ~16KiB — annotations share a ~256KiB-per-object metadata budget), sanitized to valid UTF-8 (a byte-capped read can split a rune);
- if that container **never started** (`Waiting` the whole time — unpullable image, missing configmap), there are no logs: record the waiting `reason` + `message` instead (e.g. `ImagePullBackOff: Back-off pulling image "…"`), so the parked tombstone names the actual problem;
- write the result to the Job annotation **`skyhook.nvidia.com/last-logs`** (value prefixed with the container name), in a plain Update.

If the pod is already gone or the logs API errors, the snapshot is skipped silently — it must never delay or fail the erroring/park path. A second attempt happens when `Failed` lands if the annotation is still absent and a child pod still exists. The parked tombstone then carries the tail for the whole `JOB_TTL_FAILED` window: `kubectl get job <name> -o yaml` (or `kubectl describe`) shows the dying container's last output next to the `DeadlineExceeded` condition. Mechanics: pod logs are not readable through the controller-runtime client, so this adds a small client-go clientset built once from the manager's `rest.Config`, exposed through `dal` (`GetPodLogTail`) for mockability, plus a `pods/log` `get` RBAC rule (mirrored into the chart).

What survives the deadline, all told: the **failed-attempt archive pod** with full `kubectl logs` for genuinely-failing stages (terminal-phase pods are not reaped by deadline expiry), the `last-logs` annotation covering the cases with no archive (hangs, never-started containers), everything observable during the deadline window, the `DeadlineExceeded` condition and attempt history in node state, and the agent's full host-side logs under `SKYHOOK_LOG_DIR` (`/var/log/skyhook/<skyhook>` by default). Teams wanting complete multi-attempt history should still ship logs (cluster aggregation), same as today.

**DeploymentPolicy interaction:** deadline-`erroring` counts toward compartment failure thresholds like any `erroring`. For crash loops that's identical to today (the Pod watch already reports them); for hung stages it's new signal — a stuck node now *correctly* slows or halts a rollout instead of silently absorbing a batch slot forever.

### Pause cascades to Job suspension

The second deliberate enhancement (Goal 3). Today the `skyhook.nvidia.com/pause` annotation only blocks *new* stage scheduling — in-flight stage pods run to completion, so "pause" (documented in the CLI as the **Emergency Stop**) cannot actually stop a running stage. With Jobs, pause gets teeth: when the operator observes the pause annotation (the existing `UpdatePauseStatus` path, which runs before the pause short-circuit), it sets `spec.suspend: true` on all of the Skyhook's **unfinished** Jobs; on resume it clears it. The annotation remains the user-facing primitive; suspension is the enforcement mechanism.

Semantics, precisely:

- **Suspension SIGTERMs the running pod** (honoring the package's `gracefulShutdown` grace period) and the Job controller creates no pods until resume. Pods deleted by suspension count as neither succeeded nor failed, and the failed-attempt archive pod — being terminal — survives the pause untouched. On resume, the fresh pod re-runs the stage; the agent's flag files skip already-completed steps and re-run the interrupted one — the identical recovery shape as a reboot or eviction mid-stage, which the step-idempotency contract already requires packages to tolerate. This is the trade-off the earlier draft rejected `suspend` over ("no checkpointing"); it is now accepted *deliberately*, because a pause that can't stop anything isn't an emergency stop.
- **The stage deadline pauses with the Job.** Suspension clears the Job's `startTime` and resume resets it, so `activeDeadlineSeconds` stops ticking while paused and a resumed stage gets a fresh, full `stageTimeout`. Without the cascade, pause and the deadline would interact badly: an in-flight stage could hit its deadline and park as `erroring` while the user believed the Skyhook was frozen.
- **Resume has an explicit owner and ordering.** `UpdatePauseStatus` only runs for paused Skyhooks, and paused Skyhooks skip the validation pass entirely — so un-suspension is a separate, level-triggered step for *non-paused* Skyhooks, ordered **after** `ValidateRunningPackages` in the same pass: first invalidate suspended Jobs whose spec changed while paused (a suspended Job is unfinished, so the ordinary unfinished-Job checks apply the moment the Skyhook unpauses), then clear `suspend` on the survivors. Ordering matters — clearing `suspend` first would let the Job controller launch one stale-spec attempt before validation catches it.
- **The rest of the machinery is indifferent**: `JobExists` counts the suspended Job (no duplicate creation) and the completion flow ignores it (`Suspended` is not a terminal condition). Node state stays `in_progress` — guard (c) of the erroring evidence (Retry semantics) is what makes that true, since suspension kills the active pod by deletion; the Skyhook-level `paused` Status already reports the situation.
- **Interrupt Jobs**: if the interrupt already fired the reboot, suspension can't un-ring that bell — the node reboots, and on resume the replacement pod skips the interrupt via the `SKYHOOK_RESOURCE_ID` flag file and completes. Cordon ownership is untouched (a paused Skyhook keeps its cordon annotation, as today).
- **Upgrade window**: legacy raw pods have no suspend — for them pause keeps today's let-finish semantics until the legacy window closes. This makes pause's stop-the-world strength operator-version-dependent, which the CLI docs must state (`docs/cli.md` pause/Emergency Stop sections; `docs/ordering_of_skyhooks.md` flow-control annotation definitions).

`disable` is unchanged: it skips a Skyhook from processing but has never claimed to halt in-flight work, and extending the cascade there is a separate decision if ever wanted.

### TTL: outcome-based, set at completion

`ttlSecondsAfterFinished` applies to both `Complete` and `Failed` Jobs and is **mutable**. Jobs are created with the field unset; `JobReconcile` sets it in the same Update that writes `state-recorded` — for `Complete` and for backstop-`Failed` Jobs alike:

| Outcome | Operator env | Chart value (`controllerManager.manager.env.*`) | Default |
| --- | --- | --- | --- |
| Succeeded | `JOB_TTL_SUCCEEDED` | `jobTtlSucceeded` | `1h` |
| Failed | `JOB_TTL_FAILED` | `jobTtlFailed` | `24h` |

Both are wired through the chart's `controllerManager.manager.env` block exactly like `copyDirRoot`/`agentLogRoot` (see [`chart/values.yaml`](../../chart/values.yaml)).

Where failure logs actually live: for a **genuinely failing step**, the failed-attempt archive pod carries the full logs of the most recent failure — during retries, past the deadline, and through the `JOB_TTL_FAILED` window (it is only removed with its parent Job). For stages with no failed attempt to archive — **hangs** and **never-started containers** — the `last-logs` annotation carries the tail or the waiting reason (see Stage deadline). Host-side agent logs back both. If the operator is down when a Job finishes, the TTL is simply set when it resyncs — level-triggered, no timer lost.

Scale note: succeeded pods are retained for the success-TTL window — roughly `nodes × packages × 3 stages` extra terminated pods in etcd and the informer cache at peak — plus at most **one** archived failed pod per actively-failing stage (the pruner's bound). Two bounds apply: keep `JOB_TTL_SUCCEEDED` short (default 1h), and note that kube-controller-manager's terminated-pod GC (`--terminated-pod-gc-threshold`, default 12500) may delete retained *pods* (not Jobs) earlier on large clusters — retention is "until TTL or terminated-pod GC pressure" (the completion path's no-child-pod fallback exists for exactly this). [`operator_resources_at_scale.md`](../operator_resources_at_scale.md) carries the sizing guidance.

### Naming, reruns, and the lifecycle of a finished Job

Job names are deterministic (same `generateSafeName` inputs as today's pods), which makes creation idempotent — but a finished Job retains its name until TTL, so every "run this stage again" path must first clear the name. Five precise rules replace today's delete-on-success; together with TTL they are a finished Job's complete lifecycle. Throughout, "processed" means `Failed`, or `Complete` with `state-recorded`; deletions of Jobs are always **foreground**, which matters twice over: the deterministic name only frees after the children are fully gone, so a recreate can never overlap a terminating predecessor's pods on the hostPath mounts.

**1. `ValidateRunningPackages` scoping.** Its existing checks — spec-mismatch (`jobMatchesPackage`), package-not-in-node-state, stage-mismatch-vs-node-state — apply to **unfinished Jobs only**. Applying them to finished Jobs would delete every retained Job the moment the node progresses to the next stage (the retained `apply` Job "stage-mismatches" as soon as `config` starts), silently gutting Goal 2. During the upgrade window the same checks also keep running against legacy raw pods.

**2. The rerun predicate.** A **processed** finished Job with stage `S` for package `P` is deleted when the node state no longer records `S` as done: `P`'s entry is absent (node reset via CLI, package removed), or its recorded progress is at or before `S` without `S` being complete (`package rerun`, `update-state`, config-update reruns). **Park exception:** a `DeadlineExceeded`-failed Job is *not* deleted while `P`'s entry sits at (`S`, `erroring`) — that pair is the parked state that stops deadline churn (see Stage deadline); every other entry value (absent, rerun, progressed) unparks it via the normal predicate. **Unprocessed `Complete` Jobs are never deleted by any rule except the two sweeps (rules 4 and 5)** — their completion hasn't been recorded yet and deleting them would ordinarily discard it; `JobReconcile` owns them. Progress comparison uses the state machine's existing stage ordering (`NextStage`/`IsPackageComplete`), not string equality. Two retention carve-outs follow from the predicate and are accepted: a successful no-interrupt **uninstall** Job's recorded completion *is* entry removal, so it satisfies the absent-entry branch immediately and gets no retention window; and because `uninstall` precedes `apply` in the stage ordering, starting an uninstall cycle also clears the package's retained `apply`/`config` Jobs. Uninstall is the "remove all trace" operation — prompt cleanup is coherent with its semantics, and crash-loop *failures* of uninstall still retain logs (Active Job).

**3. `AlreadyExists` on create is not blindly benign.** `ApplyPackage`/`Interrupt` handle it by GETting the Job: **unfinished and `jobMatchesPackage`** → another pass won the race; return without touching node state. **Parked** (`DeadlineExceeded` + entry at (`S`, `erroring`) — the same pair rule 2 protects) → return without creating; the park is doing its job. This is the branch that actually stops the churn: level-triggered `ApplyPackage` keeps trying to recreate erroring stages, and the parked Job is what absorbs those attempts. **Otherwise processed, or unfinished-but-mismatched** → foreground-delete and requeue; do **not** `Upsert` `in_progress` on this path, or the state machine records a run that isn't happening. **`Complete` but not yet `state-recorded`** → do not delete (that would discard an unrecorded completion); return and let `JobReconcile` process it — the requeued pass then finds the state advanced.

**4. Orphaned-node sweep.** Jobs — **regardless of status** — whose `node` no longer exists in the cluster are deleted. Today this case needs nothing: PodGC deletes raw pods bound to deleted nodes and the state died with the node. With Jobs, two failure shapes appear: the Job controller would replace each PodGC'd pod of an *unfinished* Job with another one pinned to the missing node, churning forever; and a *finished* Job for a deleted node has no node state left to compare against, so no other rule ever claims it. The operator deletes both.

**5. Reboot-reset sweep.** When `REAPPLY_ON_REBOOT` resets a node's state (`TrackReboots` on boot-ID change), the node's Jobs for that Skyhook are deleted **regardless of status — including unprocessed `Complete` ones**. A completion from a previous boot must not land after the reset: without this sweep, a Complete Job whose recording was still queued (or that finished while the operator was down across the reboot) would pass the postcondition check against the freshly-reset node state and write a pre-reboot "complete" into a node that is supposed to fully re-apply. Rule 2's protection of unprocessed completions explicitly yields to this sweep.

`HasRunningPackages` — the "wait until nothing is running" gate `Interrupt` uses before firing an interrupt — is reimplemented over Jobs with the same rule as `JobExists`: **unfinished Jobs only** (plus legacy pods during the upgrade window). It must not be left pod-based: it has no phase filter today (any pod with the name label counts), so retained Succeeded child pods would otherwise hold every interrupt hostage — node cordoned and drained — until the success TTL expired.

`JobExists` (successor of `PodExists`) counts only **unfinished** Jobs as "in flight" — a finished Job never blocks progression; rule 2 clears names ahead of recreation, and rule 3 is the belt-and-suspenders for same-pass races (the invalidation → deletion flow is asynchronous relative to the global pass).

`HandleConfigUpdates`, which today deletes erroring package *pods* so they restart with new config, deletes the package's **unfinished or parked Job(s)** on the node instead (deleting just the child pod would merely trigger a same-config replacement pod). It must not sweep the package's *retained finished* Jobs — a literal port of today's delete-all-matching-pods loop would gut retention on every config update — and it uses `JobExists` for its existence check.

### Labels

`ResourceID()` is `<name>-<uid>-<generation>` — routinely over the 63-character label-value limit, so issue #223's `resource-id` label is replaced by: full resource-id as an **annotation**, plus a numeric `skyhook.nvidia.com/generation` **label** carrying the SCR generation that produced the Job (note: same-generation reruns exist — `REAPPLY_ON_REBOOT` resets and erroring retries don't bump the generation, though config updates do; the annotation is the exact provenance). Node names are legal up to 253 characters, so the `…/node` label value is the node name when it fits and `generateSafeName(63, node)` otherwise; the operator computes the same transform on lookup, `spec.template.spec.nodeName` always carries the full authoritative node name, and the label-based queries below degrade only for such names. The issue's query goals are preserved with valid label values:

```bash
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init                         # all Jobs in one SCR
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init,skyhook.nvidia.com/package=pkg-1.0,skyhook.nvidia.com/node=worker-7,skyhook.nvidia.com/generation=4
                                                                             # one rollout of one package on one node
kubectl get jobs -l skyhook.nvidia.com/node=worker-7                         # everything touching a node
```

### RBAC

The operator ClusterRole gains `batch/jobs` `get;list;watch;create;update;patch;delete` and `core/pods/log` `get` (for the deadline log-tail snapshot) — kubebuilder markers + `make manifests`, hand-mirrored into `chart/`. The role stays cluster-scoped for consistency with the existing single-ClusterRole layout, but the Jobs *informer* is namespace-scoped as noted above, and all Job writes target the operator namespace. Pod verbs stay: the operator still reads child pods for restarts/container names, reads workload pods for drain, and drives legacy pods during the upgrade window.

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

`podFailurePolicy` evaluates `OnExitCodes` (requires a terminated container) and `OnPodConditions` (e.g. `DisruptionTarget`). An unpullable image never starts a container: the container sits in `Waiting` (`ErrImagePull`/`ImagePullBackOff`), the pod stays `Pending`, the Job stays Active — no declarative Job policy fires, ever, for this failure mode. (The design *does* set `podFailurePolicy`, but only the `Ignore`-on-`DisruptionTarget` rule, which is unrelated.) The right signal is on the live pod the operator already watches — `state.waiting.reason == ImagePullBackOff` plus the registry error in `.message`, a reason that only appears after repeated pull failures and is therefore naturally debounced. Within this design the exposure is bounded twice: the stage deadline fails the Job after `stageTimeout`, and the `last-logs` snapshot records the waiting reason/message on the parked tombstone. The *fast* surfacing (marking `erroring` within seconds of `ImagePullBackOff` via the Pod watch) remains issue #306.

### Rejected: a separate JobReconciler with its own writes

A second reconciler doing read-modify-write against node annotations/SCR status would race the global "grab the world" pass; per-controller `MaxConcurrentReconciles: 1` does not serialize *across* controllers. Splitting the controller is a separate design if ever wanted; this migration keeps the single-queue pseudo-pattern.

### Rejected: one long-lived Job (or JobSet) per rollout

`JobSet` targets coordinated parallel workloads (ML training), not a sequential per-stage lifecycle; a Job per stage keeps the state machine's granularity identical to today. Cross-stage grouping is served by labels.

### Rejected: static TTL at creation

A single `ttlSecondsAfterFinished` value at creation can't distinguish success from failure, forcing a choice between losing failure logs early or retaining success pods for a long window at scale. Mutability of the field makes outcome-based TTL strictly better at the cost of one field in an Update the operator already makes.

### Rejected: recording `erroring` on disruption-failed Jobs

An earlier draft had Job `Failed` → `StateErroring`. That would flip node/Skyhook Status and count the node against DeploymentPolicy `failureThreshold`/batch success for what today is an invisible, self-healing event (a vanished pod). Disruption recovery is therefore silent (the Job controller's replacement pod); `erroring` remains reserved for genuinely failing steps, reported by the Pod watch from crash-loop/exit-code evidence — the same signal source DeploymentPolicy consumes today.

### Rejected: `backoffLimit: 0` with operator-driven recreation

Two earlier drafts tried small `backoffLimit` values. `backoffLimit: 0` with `restartPolicy: OnFailure` is broken outright — the Job controller counts in-place container restarts toward the limit and terminates the pod, so the first step retry would kill the package. Pairing `backoffLimit: 0` with `restartPolicy: Never` would make every step retry an operator-driven Job delete/recreate — new Job object per attempt, deterministic-name collisions with the retention rules, and the retry loop moving into code this migration is trying to delete. Effectively-unlimited backoff keeps retries inside the Job controller, where its attempt pacing and `status.failed` accounting do the work.

### Rejected: `restartPolicy: OnFailure` for package Jobs (the earlier draft)

Two earlier revisions kept `OnFailure` everywhere for maximal parity with today's kubelet in-place crash loops. Superseded by `Never` + the failed-attempt archive once the stage deadline existed, because the deadline had already removed `OnFailure`'s decisive advantage (parity of "retry forever in place") while its costs remained: an in-place restart destroys the previous attempt's container (only the immediately-previous logs are reachable, and everything dies with the pod at the deadline); `backoffLimit` accounting under `OnFailure` counts restart *sums* — a subtle trap; `podFailurePolicy` is forbidden, so disruptions can't be declaratively ignored; and no archive is possible. The remaining real costs of `Never` were measured and accepted: per-attempt init-copy re-runs (completed steps skip via host flag files), Job-controller pacing (6-min cap) instead of kubelet backoff (5-min cap), attempts counted from `status.failed` instead of `restartCount`, and one honest edge — hard node crashes mint an archived attempt (guarded from flapping node state). Interrupt Jobs keep `OnFailure` where in-place restart is load-bearing for reboot survival.

### Rejected: random Job name suffixes instead of deterministic names

Suffixes would sidestep the finished-Job name collision but break creation idempotency (a reconcile crash between Create and the node-state write could leave two Jobs racing the same stage) and make "does this stage already run" a list-and-filter instead of a name hit. Deterministic names + the rerun predicate keep the level-triggered property: the name *is* the claim.
