# Release Validation

The manual pass run against a release candidate before it becomes a release. It is what
["Validate the RC"](release-process.md) means, and what the release checklist's *"The last RC
validated successfully"* box is signed against.

## Why this exists alongside CI

The chainsaw suite (`k8s-tests/chainsaw/`, see [CI test pools](ci-test-pools.md)) asserts fixed
shapes on every PR and is the gate for merging. It deliberately does not cover three things:

1. **Flows a user drives**, rather than a fixture: editing a package to unpark it, pausing mid-roll,
   changing a ConfigMap while a stage runs, deleting a node under a running package.
2. **Host-side effects.** Most automated tests run the `agentless` package image, which sleeps and
   exits. It proves the operator scheduled work; it does not prove a package *changed a host*.
3. **Upgrade from the previous release**, which by definition cannot be tested from one commit.

Everything below is hand-run and takes roughly half a day. Record evidence per case — for this
execution model the Job YAML plus the operator log slice usually reconstruct what happened.

## Setup

A kind cluster is sufficient for everything except Part 4's scale caveats.

```bash
cd operator
make create-kind-cluster                  # 1 control-plane + 2 workers, labels a worker
make install                              # CRDs
make build-cli                            # bin/nodewright, used by the lifecycle cases
../k8s-tests/operator-agent/setup.sh <worker> setup    # debugger pod, for host assertions
```

Run the operator either from the chart (preferred for an RC — it is the artifact under test) or
locally with `make run` when validating an unreleased commit. `make run` backgrounds the manager but
inherits the caller's stderr, so start it detached and redirect:

```bash
make kill
AGENT_IMAGE=<see the table below> make run > /tmp/nw-operator.log 2>&1 &
```

### The agent image has to match the fixture

**`AGENT_IMAGE` is not one setting for the whole run.** The operator injects it as the executor
beside every package, and the two fixture families need different ones — a mismatch fails every case
in the part, or worse, passes them while proving nothing:

| Part | Package fixture | `AGENT_IMAGE` |
|---|---|---|
| 1, 2, 5 | `skyhook/agentless` | `ghcr.io/nvidia/skyhook/agentless:6.2.0` — the `operator/Makefile` default, so no override needed |
| 3 | `skyhook-packages/shellscript` | `ghcr.io/nvidia/nodewright/agent:<version>` — the real agent |
| 4 | either | whatever the chart under test pins; override `controllerManager.manager.agent.*` (repository, tag, **and `digest: ""`**) if the fixture needs the other one |

So **restart the operator between Part 3 and the rest** — `make kill`, then `make run` with the other
value. The two failure modes look nothing alike, which is the tell:

- Real agent + an `agentless` package: every stage fails with
  `FileNotFoundError: .../config.json` — `agentless` ships no agent config for the real agent to read.
- `agentless` + a `shellscript` package: stages **pass** having executed nothing, because `agentless`
  sleeps and exits without entering the host mount. This is the one that silently invalidates Part 3.

**Confirm the injected image before asserting anything:**

```bash
grep -o '"AgentImage":"[^"]*"' /tmp/nw-operator.log     # local run
kubectl -n <ns> get deploy -o jsonpath='{..env}' | tr ',' '\n' | grep -i agent   # chart install
```

Host assertions go through the debugger pod:

```bash
kubectl exec <worker>-debugger -- chroot /host bash -c "<cmd>"
```

Paths: package scripts are copied to `/var/lib/skyhook/<cr>/<pkg>-<ver>-<uid>-<gen>/`, agent logs
land on the node at `/var/log/skyhook/<cr>/<pkg>/<ver>/*.log`, and per-package flags live under
`/var/lib/skyhook/<cr>/flags/`.

### Running it without wasting a day

Learned by running this end to end. None of it is product behaviour; all of it costs an hour if you
rediscover it.

**Batch the operator restarts.** Four cases need non-default operator settings, and a restart is a
minute each. Group them: everything on defaults first, then `JOB_TTL_SUCCEEDED=1m JOB_TTL_FAILED=3m`
for F5, then the real agent for Part 3 (with `JOB_TTL_SUCCEEDED=1m` again for H3), then Part 4's
chart installs. Note in the sign-off which cases ran under which settings.

**Run the destructive cases last within their part.** F6 deletes a node, and the only way back is a
cluster rebuild; do the rest of Part 2 first. Part 4 needs a rebuild between every case regardless,
since the upgrade is the thing under test.

**Check that `kubectl apply` succeeded before entering a wait loop.** The admission webhook is not
serving the instant `helm --wait` returns, and a rejected apply plus a wait loop burns the loop's
whole timeout on an object that was never created. Retry the apply until the object exists, then wait.

**Wait on the right signal.**

- Package work runs in **init containers**, so the pod sits `Pending` and never reaches `Running` —
  poll `Job.status.active` or the node-state annotation instead of pod phase.
- Do not poll CR `.status` immediately after an edit; it still reads the previous generation's value
  for a beat. Node state keyed by package is the reliable read.
- For an interrupt-in-flight case, confirm the CR actually reached `in_progress` before waiting for
  the interrupt stage — a CR that never started produces a fixture that looks like the case but is not.

**Read logs from every replica.** A chart install runs two replicas with leader election, and
`kubectl logs deploy/<name>` picks one — usually the wrong one. Use
`kubectl logs -l control-plane=controller-manager --prefix=false`, or a hold/sweep line will look
absent when it fired on the other pod.

**Budget the wall clock.** A Part 4 case is roughly cluster rebuild (2 min) + chart install (1 min) +
baseline to `complete` (1–2 min) + upgrade (1–2 min) before the first assertion. If your shell has a
command timeout, run those as separate steps rather than one script.

### Reset between cases

Deleting the CR is not enough — per-CR node metadata deliberately survives, and a leftover entry
silently changes the next case (an apply becomes an upgrade; a fresh package looks complete). Clear
all of it:

| Surface | Key |
|---|---|
| node annotations | `nodewright.nvidia.com/{nodeState,status,version,cordon}_<cr>` |
| node label | `nodewright.nvidia.com/status_<cr>` — mirrors the annotation, easy to miss |
| node label | the selector label you added to a **second** worker for L5 — leave it on and later single-node cases silently run twice |
| node taint / spec | the runtime-required taint; `unschedulable` if a case interrupted mid-flight |
| namespace | Jobs, their pods, and the per-node/package ConfigMaps |
| host (Part 3) | `/var/lib/skyhook/<cr>`, `/var/log/skyhook/<cr>`, and any marker file the case wrote |

Then assert the baseline is clean before starting the next case. Nearly every confusing result in
this kind of testing is the previous case's residue.

### Fixtures

Parts 1, 2 and 5 use `ghcr.io/nvidia/skyhook/agentless`, which honours `SLEEP_LEN` (seconds per step)
and `EXIT_CODE` (non-zero to fail) and needs no scripts. Part 3 uses
`ghcr.io/nvidia/skyhook-packages/shellscript`, whose ConfigMap keys are the scripts themselves.

Two fixture facts that cost time if you learn them the hard way:

- **The package `version` is the image tag.** A made-up version gets `ErrImagePull`, not a run.
  `agentless` publishes `1.2`, `1.2.3`, `1.2.5`, `1.2.6`, `1.3.2`, `2.0.0`, `2.0.1`, `2.1.4`,
  `3.2.1`, `3.2.3`, `3.3`, `5.4.3`, `6.0.0`, `6.2.0`; `shellscript` publishes `1.0.0`, `1.1.0`,
  `1.1.1`.
- **`SLEEP_LEN` is per step, not per stage.** A stage runs its step *and* its check, so a stage takes
  roughly twice the value.

---

## Part 1 — Package lifecycle

**L1 — one package, happy path.** One package, `SLEEP_LEN: 10`.
Expect: one Job per stage (apply, then config), `parallelism/completions: 1`,
`podReplacementPolicy: Failed`, per-attempt `spec.template.spec.activeDeadlineSeconds` set and **no**
Job-level `activeDeadlineSeconds`. At completion each Job carries `state-recorded: "true"` and the
success TTL, **its pod is retained**, and `kubectl logs` still returns output. Node state ends
`config/complete`; `kubectl nodewright node status` agrees.

**L2 — several packages with a dependency.** Use `k8s-tests/chainsaw/nodewright/simple-nodewright/`.
Expect: exactly one Job per (package, stage), distinct names, and the dependency reaching `complete`
before its dependents start. Check the label surface a user would query:
`-l nodewright.nvidia.com/{node,stage,name}`.

**L3 — two CRs targeting one node.** Different `priority` values.
Expect: at any instant only one CR has unfinished Jobs on the node; the lower `priority` number goes
first; both end `complete` with separate `nodeState_<cr>` annotations and no lost writes.

**L4 — interrupt grouping.** Use `k8s-tests/chainsaw/nodewright/interrupt-grouping/`.
Expect: **one** merged interrupt Job for the node (labelled `nodewright.nvidia.com/interrupt: "True"`),
not one per package; the node cordoned for its duration and uncordoned after; a package left
`skipped` during merging promoted to `complete` afterwards.

**L5 — two nodes and an interruption budget.** Label a second worker; `interruptionBudget: {count: 1}`
and a package with an interrupt.
Expect: never more than one node cordoned at a time — sample repeatedly rather than checking once —
one interrupt Job **per node**, and each node carrying its own `nodeState_<cr>`.

**L6 — upgrade, then downgrade.** Install a version, complete, bump it, then set it back.
Expect: the bump runs the **upgrade** stage (not apply) and removes the superseded entry, leaving one
entry. The downgrade keeps both entries, which [uninstall.md](../user-guide/uninstall.md) states is intentional for
`uninstall.enabled: false`. No Job survives naming a version no longer in spec.

## Part 2 — Failure handling and retention

**F1 — a package exhausts its retries.** `EXIT_CODE: 2`.
Expect: `status.failed` reaches `backoffLimit + 1`, condition `Failed`/`BackoffLimitExceeded`, the
failure TTL, and **at most two archive pods** (first genuine failure and most recent; the middle
attempts are pruned). Their logs still return the failure. The Job is not deleted and the stage does
not churn — re-check after two minutes. Then unpark it two ways, each of which must clear the
terminal Job and re-run the stage: edit the package spec, and `kubectl nodewright package rerun`.

**F2 — stage timeout.** `stageTimeout: 20s` with `SLEEP_LEN: 600`.
Expect: the attempt is killed at ~20s, not 600s; its pod is `Failed`/`DeadlineExceeded` **in place**
and still readable; the bound is per-attempt with no Job-level deadline. Raising `stageTimeout` and
re-applying clears the old Job and the new one carries the new value — the documented contract is
"applies to the next Job".

**F3 — attempts the kubelet refuses.** A package requesting more than the node has
(`resources` must set all four fields — see [resource-management.md](../operations/resource-management.md)).
Expect: pods fail immediately with `OutOfcpu`/`OutOfmemory` and **no container statuses**;
`status.failed` climbs; and node state stays `in_progress` and **never** `erroring` — a package that
never ran a line of script must not be blamed. The Job then goes terminal, is swept, and the stage is
recreated: an invisible self-heal worth watching once.

**F4 — a disruption costs nothing.** Evict a running attempt through the eviction subresource so
`DisruptionTarget` is actually set:
`kubectl create --raw /api/v1/namespaces/<ns>/pods/<pod>/eviction -f -`.
Expect: `status.failed` does **not** increase, a replacement attempt runs, the package never reads
`erroring`. (A plain `kubectl delete pod` is not a disruption and does spend an attempt — a useful
contrast.)

**F5 — retention follows the outcome.** One succeeding and one failing package, run with short TTLs
(`JOB_TTL_SUCCEEDED=1m JOB_TTL_FAILED=3m`; one minute is a hard floor the operator refuses to start
below).
Expect: TTL is absent until a Job finishes, then set from the outcome; succeeded Jobs and their pods
are collected first while the failed one remains; **node state is unchanged by collection**; and once
the failed Job is collected a fresh attempt appears — the slow-retry cadence, easy to mistake for
churn.

**F6 — a node disappears mid-run.** *Destructive — run it after the rest of Part 2; recovering the
node means rebuilding the cluster.* Delete a node while its package Job runs.
Expect: the Job is foreground-deleted within a reconcile or two with a log line naming the reason, no
replacement is created, the other nodes are unaffected, and the CR still reaches `complete`.

## Part 3 — Host effects (real agent)

Requires the real agent. Re-read the setup warning above before starting.

**H1 — a package changes the host.** `apply.sh` writes a file under `/etc`.
Expect: the file exists **on the node** with the expected content; the copy dir and `flags/` appear
under `/var/lib/skyhook/<cr>/`; the stage container's log shows the script's stdout.

**H2 — stages dispatch in order.** One package whose `apply.sh`, `config.sh` and `post_interrupt.sh`
each append their own name to one host file, with an interrupt configured.
Expect: the file's **line order** is `apply, config, post-interrupt` — the host is the witness, not
node-state bookkeeping — and the node is cordoned across the interrupt Job.
Note: step scripts use **underscores** (`apply_check.sh`, `post_interrupt.sh`), and there is no
package `interrupt.sh` — the interrupt container receives the operator's interrupt descriptor. A
script the agent cannot find is **not** an error: it logs `Could not find file ... was this in the
configmap?` and reports success, so a typo'd filename and an intentionally absent step look identical.

**H3 — host logs outlive the Job.** Re-run H1 with `JOB_TTL_SUCCEEDED=1m`.
Expect: after the Jobs and pods are collected, `/var/log/skyhook/<cr>/…/*.log` and the host change are
both still there. That file is the durable record; pod logs are not.

**H4 — re-execution semantics.** A step that appends a line, with a check that fails.
Expect: the script runs **once per attempt** — with the default `backoffLimit: 3` that is four
executions and four lines. The agent writes a completion flag but honours it only when the package
declares `Idempotence.Auto`; `shellscript` declares `Disabled`, and the agent logs
`Flag exists but idempotence is Idempotence.Disabled so running step.` Confirm the same on a
`package rerun`. **This is the case to run before shipping any package**: under Jobs, a
non-idempotent step is multiplied by the retry budget.

**H5 — uninstall reverses its change.** A package whose `apply.sh` writes a file and whose
`uninstall.sh` removes it, with `uninstall.enabled: true`.
Expect: flipping `uninstall.apply: true` runs an uninstall-stage Job, the file is **gone from the
node**, and the package entry leaves node state while its siblings are untouched. Then delete a CR
whose *only* package is uninstall-enabled and confirm **no** `nodewright.nvidia.com/*` annotation
survives on the node — that CR must also use `shellscript`, since Part 3 runs the real agent; an
`agentless` package here errors instead of completing and the annotations legitimately remain. Capture the uninstall output while the Job runs (see Known limitations).

## Part 4 — Upgrade from the previous release

Install the **previous** released chart, drive a CR to `complete`, then upgrade to the RC. Rebuild
the cluster between cases: the upgrade itself is the thing under test.

**U1 — a completed rollout does not re-run.** Expect: after the upgrade, no new Jobs or pods for any
package, node state unchanged, and the CR still `complete`. Also confirm the operator still runs in,
and creates work in, the namespace it was installed into.

**U2 — an in-flight rollout at upgrade.** Deliberately upgrade while a package is mid-stage. Confirm
the CR reads `in_progress` under the **previous** operator before upgrading; a CR that never started
migrates as an empty-status object, which is a different case entirely.
Expect: whatever the release's documented behaviour is — currently the operator holds rather than
taking the node over, requeueing until the in-flight work finishes or is removed. Record that it does
not take over a node another executor is still working.

**U3 — rollback to the previous release.** `helm rollback` to the pre-upgrade revision.
Expect: it completes and leaves a Ready operator; the CRDs and all CRs survive; the previous
operator resumes; **no package re-runs**; and node state is byte-identical to the pre-upgrade
snapshot. Then upgrade forward again and confirm it is clean a second time.
Note `helm rollback` with no revision targets the previous *deployed* revision — after a failed
attempt that is not the baseline. Pass the revision explicitly.

## Part 5 — Lifecycle controls

**C1 — pause, resume, disable.** With a long stage running, `kubectl nodewright pause`.
Expect: the Job gains `spec.suspend: true` and its running pod is deleted — a true stop, not just a
scheduling block — and no replacement appears while paused. `resume` starts a fresh pod. Then pause,
and in one edit add disable while removing pause: the Job must **stay** suspended. Re-enable and
unpause and the stage finishes normally.

**C2 — config change while a stage runs.** Edit a package's `configMap` with its config stage
in flight.
Expect: the in-flight Job is invalidated and foreground-deleted, and its replacement carries a
different `nodewright.nvidia.com/resource-id`. No orphaned pod survives. The rendered ConfigMap is
swapped once the stage completes across nodes, then the stage re-runs against the new content.

---

## Known limitations to expect

Not bugs to re-file — confirm they behave as described, and note any change:

- **A successful `uninstall` Job and its pod are deleted immediately**
  ([#443](https://github.com/NVIDIA/nodewright/issues/443)), so its output is not recoverable through
  Kubernetes. The agent's log on the node is the surviving copy.
- **An unpullable image reports `ContainerStatusUnknown`** on the archived attempt rather than
  `ImagePullBackOff`; the waiting reason is lost when the kubelet rewrites the status
  ([#306](https://github.com/NVIDIA/nodewright/issues/306)).

## Sign-off

Record per case: pass/fail, the build under test (`git_sha` from the operator's startup log, chart
and agent versions), and the evidence path. A failure blocks the release unless it is an already-known
limitation above, or is explicitly accepted and filed.

| Part | Cases | Operator settings | Result |
|---|---|---|---|
| 1 — lifecycle | L1–L6 | defaults, `agentless` agent | |
| 2 — failure handling | F1–F6 | defaults; F5 on `JOB_TTL_SUCCEEDED=1m JOB_TTL_FAILED=3m` | |
| 3 — host effects | H1–H5 | real agent; H3 adds `JOB_TTL_SUCCEEDED=1m` | |
| 4 — upgrade | U1–U3 | chart installs, rebuilt cluster per case | |
| 5 — lifecycle controls | C1–C2 | defaults, `agentless` agent | |

### Not covered here

Real reboots (kind nodes are containers — use `service`/`noop` interrupts), fleet scale, and image
provenance. Webhook rejection paths are only exercised under a chart install; `make run` defaults
`ENABLE_WEBHOOKS=false`.
