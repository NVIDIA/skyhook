# Package Execution as Jobs design

This document specifies migrating package-stage execution from operator-managed raw Pods to `batch/v1` Jobs — one Job per (skyhook, package, version, stage, node). It is the design companion to [issue #223](https://github.com/NVIDIA/nodewright/issues/223); implementation is tracked by sub-issues #299–#305, each a separate PR to the `feature/package-as-jobs` branch.

Cordon/drain ownership, DeploymentPolicy semantics, and multi-node Job parallelism are **out of scope**. The migration makes **one deliberate, additive CRD change**: a per-package `stageTimeout` field. Fast ImagePullBackOff surfacing keeps its own fix in issue #306; the stage deadline here bounds that hang as a side effect but does not replace the precise fix.

## TL;DR

One `batch/v1` Job runs each package stage on each node. The Job controller owns the pod lifecycle we hand-roll today, and `kubectl logs job/<name>` works during and after a run. The lifecycle state machine — stages, Status/State derivation, interrupt/cordon/drain, DeploymentPolicy — is unchanged; three deliberate enhancements ride along (stage deadline, pause-as-stop, failure-log archive).

| Decision | Choice | Why |
| --- | --- | --- |
| Execution unit | one Job per (skyhook, package, version, stage, node) | Job controller owns pod lifecycle; stable `kubectl logs job/<name>` |
| Main container | exit-0 container replaces the forever-`pause` | a pod must reach `Succeeded` for its Job to complete |
| Retry substrate | `restartPolicy: Never` + effectively-unlimited backoff (interrupt Jobs keep `OnFailure`) | each failed attempt survives as a full-log archive pod |
| Disruptions | `podFailurePolicy` ignores `DisruptionTarget` | eviction/preemption stay silent and self-heal, exactly as today |
| Completion marker | persisted `state-recorded` Job annotation | pods now linger, so delete-on-success can't be the marker |
| Stage bound | `activeDeadlineSeconds` + park (new `stageTimeout` field) | hung/unpullable stages surface as `erroring` instead of hanging forever |
| Pause | cascades to `spec.suspend` on unfinished Jobs | the Emergency Stop can finally stop a *running* stage |
| TTL | outcome-based, set at completion | failure logs outlive success logs |
| Finished-Job cleanup | deterministic names + a rerun predicate | the name is the claim; reruns and resets clear it |
| Upgrade | runtime dual-path, no migration shim | in-flight legacy pods finish under the old path for one minor |

Everything below is optional depth. The load-bearing invariants: node state is the source of truth (Jobs are executors, not state), completion is recorded exactly once via the `state-recorded` marker, and `erroring` is reserved for genuine step failures — never for disruptions.

## Problems

### Debugging requires racing the operator

The operator detects stage completion by reading init-container statuses, then **deletes the pod**. Logs for a completed stage are gone the moment the operator processes it; debugging requires catching the pod before deletion or knowing its generated name in advance.

### Hand-rolled lifecycle

Cleanup, completion tracking, "does this stage already run" checks, and stale-spec invalidation are all custom code spread across the controller and pod-controller. Kubernetes has a first-class run-to-completion primitive that carries most of this for free, plus ecosystem benefits (`kubectl logs`/`describe job`, k9s/ArgoCD/dashboard treatment of Jobs as completable work).

## Goals

1. **Stable, queryable execution records**: one Job per (skyhook, package, version, stage, node); `kubectl logs job/<name>` works during and after execution.
2. **Logs outlive completion**: finished work is retained for a configurable window; the logs that matter most for humans (failing steps) are retained the longest.
3. **No behavior change to the lifecycle state machine, with deliberate enhancements**: stage progression, Status/State derivation, interrupt/cordon/drain sequencing, DeploymentPolicy interactions, and agent-visible env are unchanged. A pod lost to *disruption* is silently re-executed with no `erroring` and no DeploymentPolicy impact, exactly as today. The enhancements: a stage that runs past its **deadline** is failed, surfaced as `erroring`, and **parked**; **pause becomes a true stop**; and each genuinely failing stage keeps a **full-log archive** of its most recent failed attempt.
4. **Failure evidence outlives failure**: the most recent failed attempt stays `kubectl logs`-queryable while the stage retries and after it parks.
5. **Safe upgrade**: an operator upgrade with in-flight raw pods completes those stages correctly, without double-executing them.

## Current behavior (baseline)

Facts about today's implementation that shape the design; symbols are named here so the design sections below can stay at altitude.

- **Package pods never terminate.** All work runs sequentially in `initContainers` (init-copy → `<stage>` → `<stage>-check`); the main container is a `pause` image that runs forever ([`createPodFromPackage`](../../operator/internal/controller/skyhook_controller.go)). The lingering Running pod doubles as the "stage in-flight, don't create another" marker ([`PodExists`](../../operator/internal/controller/skyhook_controller.go)).
- **The operator is the completion detector.** [`containerExitedSuccessfully`](../../operator/internal/controller/pod_controller.go) inspects init-container statuses; on success the operator updates node state and then **deletes the pod** — deletion is also the processed-once marker (the `DeletionTimestamp == nil` gate prevents double-processing). The write pair is not atomic today either (node Patch, then pod Delete); the Jobs design keeps the same crash window, not a larger one.
- **Retries are kubelet in-place restarts.** `restartPolicy: OnFailure` crash-loops a failing init container forever; the operator only reports `erroring` with restart counts — it never recreates a failing pod. A pod that *disappears* (evicted, deleted) produces no state write: node state stays `in_progress` and a later reconcile silently recreates it via the `PodExists` miss. Pods on a **deleted node** are cleaned up by kube-controller-manager's PodGC, not by the operator.
- **Pods are owned by the Skyhook CR**, carry `skyhook.nvidia.com/name` + `…/package` labels (interrupt pods add `…/interrupt: "True"`), and a JSON package annotation that round-trips (skyhook, package, version, stage, image).
- **Interrupt pods have their own name formula** (skyhook, stage, interrupt-type, node — no package/version), which dedupes the merged interrupt: one per node/stage/type, independent of which package's interrupt won.
- **Config updates delete erroring package pods directly** ([`HandleConfigUpdates`](../../operator/internal/controller/skyhook_controller.go)) so they are recreated with the updated configmap.
- **[`cluster_state_v2.go`](../../operator/internal/controller/cluster_state_v2.go) does not read pods.** Node annotations are the state store; pods are ephemeral executors.

## Design

### Job shape

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: <generateSafeName(63, skyhook, package, version, stage, node)>   # interrupt Jobs: skyhook, stage, interruptType, node
  namespace: <operator namespace>
  labels:
    skyhook.nvidia.com/name: <skyhook>            # existing
    skyhook.nvidia.com/package: <name>-<version>  # existing
    skyhook.nvidia.com/interrupt: "True"          # existing (interrupt Jobs only)
    skyhook.nvidia.com/stage: <stage>             # new
    skyhook.nvidia.com/node: <node>               # new (hashed for >63-char node names — see Labels)
    skyhook.nvidia.com/generation: "<n>"          # new: the SCR generation that produced this Job
  annotations:
    skyhook.nvidia.com/resource-id: <full resource id>   # provenance; too long for a label
  ownerReferences: [<Skyhook CR>]                 # cascade delete; pods are owned by the Job
spec:
  parallelism: 1
  completions: 1
  backoffLimit: <effectively unlimited>           # the Job controller must never give up before the operator does
  podFailurePolicy:                               # package Jobs only (the API forbids it with OnFailure)
    rules:
    - action: Ignore
      onPodConditions:
      - type: DisruptionTarget
  activeDeadlineSeconds: <stageTimeout>           # omitted when the deadline is disabled
  podReplacementPolicy: Failed                    # replace only after full termination — no hostPath overlap
  # ttlSecondsAfterFinished: unset at creation — set by the operator at completion (see TTL)
  template:
    metadata:
      labels: <same label set>                    # child pods inherit labels → existing CLI label queries keep working
      annotations:
        skyhook.nvidia.com/package: <same JSON package annotation>   # the in-flight erroring path reads it off the child pod
    spec:
      # identical to today's pod spec, with the three changes described below
      restartPolicy: Never                        # interrupt Jobs: OnFailure
      tolerations:
      # existing tolerations, plus unbounded NoExecute for not-ready / unreachable
      - { key: node.kubernetes.io/not-ready,   operator: Exists, effect: NoExecute }
      - { key: node.kubernetes.io/unreachable, operator: Exists, effect: NoExecute }
      containers:
      - name: done                                # replaces the forever-running `pause` container
        image: <agent image>
        command: ["/bin/sh", "-c", "exit 0"]
```

Three things about this shape are non-obvious:

- **The main container changes** because a Job records completion only when its pod reaches `Succeeded`, which needs every container to terminate. The init-container chain stays — it is the only native run-to-completion sequencing primitive, and per-step container names are load-bearing for state reporting. The `pause` container's *other* job, being the in-flight marker, is taken over by the Job object, which outlives its pods.
- **`restartPolicy: Never` on package Jobs** makes each attempt a fresh pod, so a failure survives as a full-log archive pod instead of being destroyed by an in-place restart. Interrupt Jobs are the exception — they keep `OnFailure`, because a reboot interrupt kills its own pod by design; under `Never` every successful reboot would mint a spurious failed attempt, whereas in-place restart after the node returns is the proven recovery shape (the agent skips the already-done interrupt via its resource-id flag file).
- **The unbounded not-ready/unreachable tolerations** stop the taint manager from evicting a node-pinned pod when a reboot-class interrupt keeps the node NotReady past the default eviction timeout. These pods are node-bound host agents — running them anywhere else is meaningless, so eviction is never useful. A node that stays NotReady forever holds the Job Active exactly as today's raw pod would; a *removed* node is handled by the orphaned-node sweep.

`backoffLimit` is effectively unlimited on purpose: under `restartPolicy: Never` it counts Failed pods, and the `Ignore`-on-`DisruptionTarget` rule means only *genuine* step failures count, not disruptions. The retry loop lives inside the Job controller — attempt fails → the operator archives the Failed pod → the Job controller paces the next attempt under its own backoff — so the operator never drives recreation. `podReplacementPolicy: Failed` guarantees the replacement lands only after the previous pod has fully terminated, so two executors never overlap on the shared hostPath mounts.

### Completion and the processed-once marker

The pseudo-controller pattern is kept (issue #223 option A): a Jobs watch maps events into the single global reconcile queue as `job---<name>` requests, alongside the existing `pod---<name>` routing, so no second reconciler writes state and the serialization guarantee holds. The Jobs informer is namespace-scoped.

Because completed pods now linger, pod-deletion can no longer be the processed-once marker. A persisted Job annotation, `skyhook.nvidia.com/state-recorded: "true"`, replaces it. On each Job event:

- **`Complete`, not yet marked** → read the `Succeeded` child pod for the container name (selected by the Job's controller UID, not by name — a same-named prior Job's pod can still be terminating), record completion through the existing node-state path (upgrade/uninstall/interrupt special cases unchanged), then one Job Update sets `state-recorded` and the success TTL. If the child pod was already GC'd, fall back to the Job's own stage/node/interrupt labels. Already-marked Jobs are skipped, so duplicate events are harmless. The recorded attempt count now comes from the Job's failed-pod count — a user-visible improvement over today's usually-zero restart count.
- **`Failed` with reason `DeadlineExceeded`** → the stage ran past its deadline (Stage deadline, below): a genuine failure. Record `erroring`, mark processed with the failure TTL, and **leave the Job in place as the park marker**.
- **`Failed`, any other reason** → a backstop that unlimited backoff plus the deadline should make unreachable. No node-state write (matching today's silent disruption behavior); one Update carries the marker and failure TTL.
- **Node gone** → if the node `Get` returns NotFound, there is no state to record: mark the Job processed and stop, rather than error-looping on a node that will never return.
- **Active** → no terminal handling; the Pod watch keeps reporting in-flight `erroring` from failed-attempt evidence, and the operator prunes failed attempts to one archive.

Recording a completion is two writes to two objects (node state, then the Job marker) and cannot be atomic; a crash between them re-serves the event. The re-processing path is guarded by per-transition postcondition checks so a re-served completion is only marked, not re-applied — detail in [Edge cases](#edge-cases-and-correctness-arguments). This is strictly better than today, which has the same two-write window with no guard.

### Retry and the failed-attempt archive

Retries stay unlimited-until-the-deadline, but the substrate moves from kubelet in-place restarts to Job-controller attempts, buying durable failure evidence:

- **Each attempt is a fresh pod.** A failing step fails its pod; the Job controller paces the next attempt (comparable to kubelet crash-loop backoff), pinned to the same node and gated so attempts never overlap on the hostPath mounts. The agent's host-side flag files carry across attempts, so each retry re-runs only init-copy and the failing step.
- **The archive (keep two).** Terminal-phase pods are not reaped by the Job controller, so failed attempts would accumulate; the operator prunes them, keeping the **first** genuine failure (most likely the root cause, before cascading errors obscure it) and the **most recent** — both carrying no `DisruptionTarget` condition, since a disruption casualty has no failure verdict and must not shadow a real one. Steady state is one running attempt plus up to two full-log archives — the first and latest real failures — queryable while the stage retries, after it parks, and through a pause.
- **Disruptions stay silent.** The `Ignore`-on-`DisruptionTarget` rule makes evictions, preemptions, taint-manager kills, and PodGC deletions count neither toward backoff nor as failed attempts — the replacement appears and nothing is recorded, matching today's invisible recreate. (Two honest deltas around hard node crashes are in [Edge cases](#edge-cases-and-correctness-arguments).)
- **Erroring evidence** still comes from pod observation, now triple-guarded: a Failed package pod marks the node `erroring` only if it carries no `DisruptionTarget` condition, its step container terminated with a real verdict, and its `DeletionTimestamp` is unset. The last guard is load-bearing — pause suspension, Job deletions, config updates, the sweeps, and a human's `kubectl delete pod` all kill pods without `DisruptionTarget`, and every one of them must stay silent. Full guard rules are in [Edge cases](#edge-cases-and-correctness-arguments).

A Failed *Job* therefore means only deadline (next section) or the backstop; individual *pod* failures never fail the Job.

### Stage deadline: bounded runtime, parked failures

Today a repeatedly-failing stage churns forever and a *hung* stage (stuck script, unpullable image, dead registry) sits `in_progress` invisibly forever. Each stage Job now carries `activeDeadlineSeconds` — a wall-clock bound on the whole stage, all attempts (and, for interrupt Jobs, reboot time) included.

**Configuration.** A new additive per-package field, following the existing `gracefulShutdown` duration pattern:

```yaml
spec:
  packages:
    mypackage:
      version: 1.0.0
      image: ghcr.io/example/pkg
      stageTimeout: 2h        # optional; bounds each stage Job for this package
```

Unset falls back to an operator default (`JOB_STAGE_TIMEOUT`, chart value `controllerManager.manager.env.jobStageTimeout`, default 1h). An explicit `0` disables the deadline — the builders omit `activeDeadlineSeconds` entirely, since a literal zero would insta-fail every Job. Webhook validation: non-negative. This is the only CRD change in the migration; it is additive and needs no migration shim (absent field = operator default).

**On expiry** the Job controller deletes the active pod and fails the Job (`DeadlineExceeded`). The operator records `erroring` — for crash loops the Pod watch usually already has; for *hung* stages this is the first signal anyone gets — and the Failed Job **parks**: the finished-Job rules deliberately do not delete it while the package's entry sits at (same stage, `erroring`), so nothing recreates the stage and the churn stops. The park clears on any explicit signal — `package rerun`/`reset`, a config update, a spec change, or failure-TTL expiry — after which the still-`erroring` state drives one fresh attempt: a deliberate slow-retry cadence until a human intervenes.

**Log visibility.** Deadline expiry deletes the running pod — the only way Kubernetes can stop a hung container — and the kubelet garbage-collects its logs with it. To keep the last evidence queryable, the operator reacts to the pre-terminal `FailureTarget` condition with a **best-effort log-tail snapshot** into the `skyhook.nvidia.com/last-logs` Job annotation:

- if a failed-attempt archive pod already exists, the snapshot is unnecessary — the archive carries full logs and survives the deadline;
- otherwise it tails the stuck container's logs (a small byte cap, because annotations share a per-object metadata budget), sanitized to valid UTF-8;
- if the container never started (unpullable image, missing configmap), there are no logs — it records the waiting reason and message instead, so the parked tombstone names the actual problem.

The snapshot is skipped silently on any error and never delays the erroring/park path; it retries once when the Job goes terminal. Pod logs are not readable through the controller-runtime client, so this adds a small client-go clientset and a `pods/log` `get` RBAC rule. On an **unreachable node** the Job can sit at `FailureTarget` without ever going terminal, so a stale `FailureTarget` is treated as erroring evidence too (state write only; the marker/TTL/park still wait for `Failed`), and a reboot interrupt that bricks its node still surfaces.

Deadline-`erroring` counts toward DeploymentPolicy failure thresholds like any `erroring` — new signal for hung stages, identical to today for crash loops.

### Pause cascades to Job suspension

Today the `skyhook.nvidia.com/pause` annotation (the CLI's **Emergency Stop**) only blocks *new* stage scheduling — an in-flight stage runs to completion, so pause cannot actually stop it. With Jobs, pause gets teeth: when the operator observes the annotation it sets `spec.suspend: true` on all of the Skyhook's unfinished Jobs, and clears it on resume. The annotation stays the user-facing primitive; suspension is the enforcement.

- **Suspension SIGTERMs the running pod** (honoring `gracefulShutdown`) and no pods start until resume. On resume the fresh pod re-runs the stage; the agent's flag files skip completed steps and re-run the interrupted one — the same recovery shape as an eviction or reboot mid-stage, which packages must already tolerate. This is the "no checkpointing" cost the earlier draft rejected `suspend` over, now accepted deliberately, because a pause that can't stop anything isn't an emergency stop.
- **The deadline pauses with the Job.** Suspension clears the Job's start time, so `activeDeadlineSeconds` stops ticking while paused and a resumed stage gets a fresh, full timeout — otherwise a "paused" stage could quietly hit its deadline and park while the user believed everything was frozen.
- **Resume has an explicit owner and ordering.** Because the pause path only runs for paused Skyhooks and paused Skyhooks skip validation, un-suspension is a separate step for *non-paused* Skyhooks, ordered after validation: invalidate suspended Jobs whose spec changed while paused, *then* clear `suspend` on the survivors. Clearing first would let one stale-spec attempt launch before validation catches it.
- **Everything else is indifferent**: existence gating counts the suspended Job, completion ignores it (Suspended is not terminal), and node state stays `in_progress` (guard (c) of the erroring evidence makes that hold).
- **Interrupts already fired**: suspension can't un-ring a reboot; on resume the replacement pod skips the interrupt via the resource-id flag and completes.
- **Legacy pods can't suspend** — for them pause keeps today's let-finish semantics until the upgrade window closes, so pause's stop-strength is operator-version-dependent (a CLI-docs note).

`disable` is unchanged: it skips a Skyhook from processing but has never claimed to halt in-flight work.

### TTL: outcome-based, set at completion

`ttlSecondsAfterFinished` is mutable and applies to both `Complete` and `Failed` Jobs. Jobs are created with it unset; the operator sets it in the same Update that writes `state-recorded`:

| Outcome | Operator env | Chart value (`controllerManager.manager.env.*`) | Default |
| --- | --- | --- | --- |
| Succeeded | `JOB_TTL_SUCCEEDED` | `jobTtlSucceeded` | `1h` |
| Failed | `JOB_TTL_FAILED` | `jobTtlFailed` | `24h` |

Failure logs thus outlive success logs. If the operator is down when a Job finishes, the TTL is simply set on resync — level-triggered, no timer lost. Where failure logs actually live: the archive pod for a genuinely-failing step, the `last-logs` annotation for hangs and never-started containers, and the agent's host-side logs (`SKYHOOK_LOG_DIR`) behind both.

Scale: succeeded pods are retained for the success window — roughly `nodes × packages × 3 stages` extra terminated pods at peak — plus at most two archived failed pods (the first and latest failures) per actively-failing stage. Keep `JOB_TTL_SUCCEEDED` short; note that kube-controller-manager's terminated-pod GC may reap retained *pods* (not Jobs) earlier on large clusters, which is exactly why the completion path has a no-child-pod fallback. Sizing guidance lives in [`operator_resources_at_scale.md`](../operator_resources_at_scale.md).

### Finished-Job lifecycle: naming and reruns

Job names are deterministic (same inputs as today's pod names), which makes creation idempotent — the name *is* the claim. But a finished Job keeps its name until TTL, so every "run this stage again" path must first clear the name. Five rules replace today's delete-on-success; "processed" means `Failed`, or `Complete` with `state-recorded`, and Job deletions are always **foreground** so a deterministic name frees only after its child pods are gone.

1. **Validation is scoped to unfinished Jobs.** The spec-mismatch / not-in-node-state / stage-mismatch checks apply only to unfinished Jobs; applying them to finished Jobs would delete every retained Job the moment the node progresses, gutting Goal 2.
2. **The rerun predicate.** A processed finished Job for stage `S` is deleted once node state no longer records `S` as done (entry absent, or progress at/before `S` without `S` complete — compared via the state machine's stage ordering, not string equality). **Park exception:** a `DeadlineExceeded` Job is *not* deleted while the entry sits at (`S`, `erroring`) — that pair is the parked state; every other value unparks it. Unprocessed `Complete` Jobs are never deleted except by the two sweeps.
3. **`AlreadyExists` on create is not blindly benign.** GET the Job: unfinished and matching → another pass won the race, return. Parked → return; the park is doing its job (this branch is what absorbs level-triggered recreate attempts on an erroring stage). Otherwise processed, or unfinished-but-mismatched → foreground-delete and requeue, without recording an `in_progress` that isn't happening. `Complete` but not yet marked → don't delete (that discards an unrecorded completion); let the completion path handle it.
4. **Orphaned-node sweep.** Jobs whose node no longer exists are deleted regardless of status — an unfinished one would otherwise churn PodGC↔replacement forever against the missing node, and a finished one has no node state left to claim it.
5. **Reboot-reset sweep.** When `REAPPLY_ON_REBOOT` resets a node's state, that node's Jobs for the Skyhook are deleted regardless of status, including unprocessed `Complete` ones — a completion from a previous boot must not land on freshly-reset state. Rule 2's protection of unprocessed completions yields to this sweep.

Two retention carve-outs follow and are accepted: a successful no-interrupt **uninstall** Job's completion *is* entry removal, so it satisfies the absent-entry branch immediately and gets no retention window; and because uninstall precedes apply in the ordering, starting an uninstall also clears the package's retained apply/config Jobs. Uninstall is the "remove all trace" operation, so prompt cleanup is coherent — and crash-loop *failures* of uninstall still retain logs (Active Job).

The existence gates move with these rules: the "is this stage in flight" check and the "wait until nothing is running" interrupt gate both count **unfinished Jobs only** (a retained Succeeded pod must not hold an interrupt hostage — node cordoned and drained — until its TTL expires), and config-update handling deletes the package's **unfinished or parked** Job rather than its child pod (which would just trigger a same-config replacement).

### Labels

The full resource id is routinely over the 63-character label limit, so it rides as an **annotation**; a numeric `skyhook.nvidia.com/generation` **label** carries the SCR generation instead (same-generation reruns exist — reboots and erroring retries don't bump it, though config updates do; the annotation is the exact provenance). Node names are legal up to 253 characters, so the `…/node` label is the node name when it fits and a stable hash otherwise; the operator computes the same transform on lookup, and `spec.template.spec.nodeName` always carries the authoritative full name. The issue's query goals hold with valid label values:

```bash
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init                              # all Jobs in one SCR
kubectl get jobs -l skyhook.nvidia.com/name=gpu-init,skyhook.nvidia.com/node=worker-7,skyhook.nvidia.com/generation=4
kubectl get jobs -l skyhook.nvidia.com/node=worker-7                              # everything touching a node
```

### RBAC

The operator ClusterRole gains `batch/jobs` (`get;list;watch;create;update;patch;delete`) and `core/pods/log` `get` (the deadline snapshot) — kubebuilder markers + `make manifests`, hand-mirrored into `chart/`. The role stays cluster-scoped, but the Jobs informer is namespace-scoped and all Job writes target the operator namespace. Pod verbs stay: the operator still reads child pods for restarts/container names, reads workload pods for drain, and drives legacy pods during the upgrade window.

## Upgrade and compatibility

No persisted schema change → **no migration shim**. In-flight raw pods from the previous version are distinguishable by the absence of the `batch.kubernetes.io/job-name` label. For **one minor release** the operator runs legacy-aware, then all four accommodations are removed together (release-notes entry on removal):

- **Completion:** the Pod watch keeps the old path (update state, delete pod) for legacy pods; Job-owned pods report only in-flight erroring/restarts.
- **Existence gating:** the in-flight and interrupt gates OR-in legacy pods, so the first post-upgrade reconcile doesn't create a duplicate Job executor for a stage already running as a legacy pod — nor let an interrupt cordon/drain under one.
- **Validation:** the validation pass keeps its legacy-pod sweep alongside the Job checks.
- **Config updates:** `HandleConfigUpdates` keeps direct deletion of legacy erroring pods — a legacy pod crash-looping on stale config has no Job to delete, and the existence gating would block a replacement, so a mid-window config update would otherwise deadlock.

New stages after the upgrade are Jobs immediately; nothing is proactively deleted. Legacy pod names equal new Job names byte-for-byte, but they are different kinds and Job child pods get a suffix, so there are no API name conflicts.

CLI: child pods inherit the full label set, so `package logs/status/rerun` label queries work unchanged against both operator generations, and post-completion `package logs` gets *better*. No CLI code change is expected; verification is part of #305.

## Rejected alternatives

`restartPolicy: Never` vs `OnFailure` for package Jobs is the central decision and is kept in full below; the rest reduce to a line each.

| Rejected | Why not |
| --- | --- |
| `podFailurePolicy` for ImagePullBackOff | An unpullable image never starts a container, so no exit-code or condition rule can ever match. The design *does* set `podFailurePolicy`, but only the `Ignore`-on-`DisruptionTarget` rule. Fast surfacing stays #306; the deadline + `last-logs` bound the exposure. |
| A separate JobReconciler with its own writes | A second reconciler doing read-modify-write against node state would race the global pass — per-controller concurrency limits don't serialize *across* controllers. Splitting the controller is its own design. |
| One long-lived Job (or JobSet) per rollout | `JobSet` targets coordinated parallel workloads, not a sequential per-stage lifecycle; a Job per stage keeps the state machine's granularity. Cross-stage grouping is served by labels. |
| Static TTL at creation | A single value can't distinguish success from failure, forcing a choice between losing failure logs early and retaining success pods too long. Mutability makes outcome-based TTL strictly better for one extra field in an Update the operator already makes. |
| `erroring` on disruption-failed Jobs | An earlier draft mapped Job `Failed` → `erroring`; that would count an invisible, self-healing vanished pod against DeploymentPolicy. `erroring` stays reserved for genuine step failures. |
| `backoffLimit: 0` with operator-driven recreation | With `OnFailure` it is broken outright (in-place restarts count toward the limit and kill the package on the first retry); with `Never` it moves the retry loop into operator code this migration is trying to delete. Unlimited backoff keeps retries and attempt accounting inside the Job controller. |
| Random Job name suffixes | Suffixes break creation idempotency (a crash between Create and the state write leaves two Jobs racing) and turn "does this stage run" into a list-and-filter. Deterministic names + the rerun predicate keep the level-triggered property. |

### `restartPolicy: OnFailure` for package Jobs (the earlier draft)

Two earlier revisions kept `OnFailure` everywhere for maximal parity with today's kubelet in-place crash loops. It was superseded by `Never` + the archive once the stage deadline existed, because the deadline had already removed `OnFailure`'s one decisive advantage — retry-forever-in-place — while its costs remained: an in-place restart destroys the previous attempt's container, so only the immediately-previous logs are reachable and everything dies at the deadline; backoff accounting under `OnFailure` sums restarts (a subtle trap); `podFailurePolicy` is forbidden, so disruptions can't be declaratively ignored; and no archive is possible. The remaining real costs of `Never` were measured and accepted — per-attempt init-copy re-runs (completed steps skip via host flag files), slightly slower attempt pacing, attempts counted from the Job's failed-pod count, and one honest edge (hard node crashes mint an archived attempt, guarded from flapping node state). Interrupt Jobs keep `OnFailure`, where in-place restart is load-bearing for reboot survival.

## Edge cases and correctness arguments

These are the load-bearing safety arguments, collected so the sections above can describe the happy path. All are the product of adversarial review and are retained deliberately.

**The two-write crash window and postcondition guards.** Recording a completion is a node-state write followed by the Job `state-recorded` marker — two objects, not atomic, ordered state-first so a crash can never mark a Job whose state is still `in_progress` (which would permanently lose the completion). A crash *between* the writes re-serves the event, and the node-state transitions are not idempotent, so before re-applying, the operator checks whether the transition's outcome already holds:

- normal stage → the entry records the stage complete;
- upgrade → stale-version entries gone and the entry complete;
- uninstall without interrupt → the entry is absent;
- uninstall with interrupt → the entry is at `uninstall-interrupt` or beyond, or absent;
- interrupt (the path that also promotes skipped packages) → the winning package's entry is at `interrupt`-complete or beyond, *and* no spec package is still at (`interrupt`, `skipped`).

If the postcondition already holds, the Job is only marked, not re-processed. Keying dedup by Job UID inside the node-state entry was rejected: the entry's map shape has been stable since v0.7.5, and older CLIs rewrite the whole value, silently dropping fields they don't know.

**Hard node crashes / unexpected reboots (no `DisruptionTarget`).** Two honest deltas from today. First, a crash lands one archived, counted attempt, and whether it can *briefly* flap node state depends on what the kubelet recorded: a `ContainerStatusUnknown` shape is guarded out, but a plain `terminated{137, Error}` passes the guards and shows `erroring` until the replacement attempt completes (self-healing via flag files; `REAPPLY_ON_REBOOT` deployments reset the node anyway). Second, a rebooting node can transiently reject the node-pinned replacement at kubelet admission (`OutOfPods` etc.); those Failed pods have no container statuses (never `erroring`) but do count an attempt and churn the archive slot. This replaces a *worse* latent behavior today, where an admission-rejected raw pod satisfies the in-flight check forever and wedges the stage.

**Erroring guards, in full.** A Failed package pod marks the node `erroring` only if (a) it carries no `DisruptionTarget` condition; (b) its step container terminated with a real verdict — nonzero exit including `OOMKilled`; `ContainerStatusUnknown` (the kubelet-couldn't-tell artifact of a node crash) and kubelet admission rejections (which carry a status reason and no container statuses) are skipped; and (c) its `DeletionTimestamp` is unset. Guard (c) is what keeps pause suspension, the rule-2/3 and config-update deletions, the sweeps, and manual `kubectl delete pod` silent — deadline `erroring` comes from the Job's `DeadlineExceeded` branch, not pod evidence, so nothing is lost. Today's `CrashLoopBackOff` waiting-state detection now applies only to interrupt Jobs (a `Never` pod fails once and goes terminal — it never enters `CrashLoopBackOff`).

**Pruner accounting safety.** The archive pruner deletes the middle attempts by normal deletion only, never force-delete: the Job-tracking finalizer is what guarantees a failure is counted and policy-classified before the pod is removed. The archives it keeps — the first and most recent genuine failures — exclude `DisruptionTarget` casualties so an ignored disruption can neither shadow a real failure nor suppress the deadline snapshot.

**Replacement-policy gate off.** `podReplacementPolicy` is beta and on by default across the supported range; if a cluster disables the gate, operator-initiated recreates stay safe (foreground deletion frees the name only after children are gone), and only Job-controller replacements can then briefly overlap a terminating predecessor — accepted for that non-default configuration, since the agent's flag files keep re-execution idempotent.

## References

- [`operator/internal/controller/skyhook_controller.go`](../../operator/internal/controller/skyhook_controller.go) — reconcile loop, `createPodFromPackage`, `PodExists`/`HasRunningPackages`, `ValidateRunningPackages`, `HandleConfigUpdates`, `UpdatePauseStatus`
- [`operator/internal/controller/pod_controller.go`](../../operator/internal/controller/pod_controller.go) — completion detection, in-flight erroring/restart reporting
- [`operator/internal/controller/cluster_state_v2.go`](../../operator/internal/controller/cluster_state_v2.go) — node-state snapshot, metrics
- [`operator/internal/controller/annotations.go`](../../operator/internal/controller/annotations.go) — package annotation round-trip
- [`operator/api/v1alpha1/skyhook_types.go`](../../operator/api/v1alpha1/skyhook_types.go) — `Package.stageTimeout`, webhook validation
- [issue #223](https://github.com/NVIDIA/nodewright/issues/223), sub-issues #299–#305; issue #306 (ImagePullBackOff surfacing)
