# Release Validation

The short manual pass run against a release candidate, covering what the automated suites do not. It
is what ["Validate the RC"](release-process.md) means, and what the release checklist's *"The last RC
validated successfully"* box is signed against.

## Run the automated suites first

Most of the product is already covered, and re-doing it by hand is wasted time. Before anything below:

```bash
cd operator
make test                 # unit + e2e pools + cli-e2e + helm + operator-agent
make create-kind-cluster  # must precede the next one; it recreates the local registry
make migration-test       # upgrade from the last pre-rename operator
```

CI runs the same set on every PR (`e2e` × core/interrupt/uninstall/lifecycle, `deployment-policy`,
`cli-e2e`, `unit-tests`, `helm-tests`, `migration`), so on a clean RC these should all be green
already. What they cover, so you do not re-test it:

| Behaviour | Where |
|---|---|
| Job shape, one Job per (package, stage), retention | `nodewright/simple-nodewright`, `hello-world` |
| `dependsOn` ordering; strict ordering across CRs | `nodewright/depends-on`, `strict-order` |
| Interrupt grouping, cordon/drain, budgets | `nodewright/interrupt-grouping`, `interrupt`, `drain-config` |
| Upgrade and downgrade paths | `nodewright/package-upgrade`, `downgrade-after-uninstall`, `downgrade-enabled-false-preserves-state` |
| Uninstall in all its shapes | `nodewright/explicit-uninstall`, `uninstall-on-delete`, `uninstall-mixed-packages`, `uninstall-cancel` |
| Failure, retry budget, timed-out Job shape | `nodewright/failure-nodewright` |
| Pause suspending a running Job | `nodewright/pause-suspends-jobs` |
| Config change while a stage runs | `nodewright/config-nodewright` |
| Package scripts running **on the host** via the real agent | `k8s-tests/operator-agent/` |
| **Upgrade from the previous release without re-running packages** | `make migration-test` |
| Migration hold on an in-flight rollout | `k8s-tests/migration/skyhook-hold.yaml` |
| `helm upgrade` then `helm rollback` across the rename | `helm/helm-upgrade-rollback-test` |

## What is left, and why it is manual

The eight cases below are either uncovered outright, or covered only as a *shape* where the thing
worth checking is *behaviour over time* — a distinction chainsaw is bad at and a human watching a
cluster is good at. Half a day, not a full one.

### Setup

```bash
make install && make build-cli
../k8s-tests/operator-agent/setup.sh <worker> setup   # debugger pod, for case 4 only
make kill
make run > /tmp/nw-operator.log 2>&1 &                # detached: make run inherits your stderr
```

`AGENT_IMAGE` must match the package fixture, and the mismatch is silent in one direction:

| Fixture | `AGENT_IMAGE` |
|---|---|
| `skyhook/agentless` (cases 1, 2, 3, 5, 6, 7, 8) | `ghcr.io/nvidia/skyhook/agentless:6.2.0` — the `operator/Makefile` default |
| `skyhook-packages/shellscript` (case 4) | `ghcr.io/nvidia/nodewright/agent:<version>` — the real agent |

Real agent + an `agentless` package fails loudly on a missing `config.json`. `agentless` + a
`shellscript` package **passes having executed nothing** — that is the one that invalidates case 4.
Confirm before asserting: `grep -o '"AgentImage":"[^"]*"' /tmp/nw-operator.log`.

Reset between cases: delete the CR **and** clear the per-CR node annotations, the mirrored
`status_<cr>` label, any cordon or runtime-required taint, the namespace's Jobs/pods/ConfigMaps, and
(case 4) `/var/lib/skyhook/<cr>`, `/var/log/skyhook/<cr>` and the marker file. Assert a clean baseline
before starting the next one — most confusing results are the previous case's residue.

Fixture notes: the package `version` **is** the image tag (a made-up version gets `ErrImagePull`, not
a run), and `SLEEP_LEN` is per *step*, so a stage takes roughly twice the value.

---

## 1. Attempts the kubelet refuses are not the package's failure

**Not covered anywhere.** Ask for more than the node has (`resources` needs all four fields — see
[resource-management.md](../operations/resource-management.md)) so every attempt is rejected before a
container starts.

Expect pods `OutOfcpu`/`OutOfmemory` with **no container statuses**, `status.failed` climbing to
`backoffLimit + 1`, and node state that stays `in_progress` and **never** `erroring` — a package that
never ran a line of script must not be blamed for failing. Then watch the Job go terminal, get swept,
and the stage recreated: an invisible self-heal that only shows up if you are watching.

## 2. A disruption costs no retry budget

**Not covered anywhere.** Evict a running attempt through the eviction **subresource** so
`DisruptionTarget` is actually set — `kubectl create -f` on an `Eviction` fails with
`no matches for kind "Eviction" in version "policy/v1"`:

```bash
NS=nodewright
POD=$(kubectl -n "$NS" get pods --no-headers | grep -v Completed | head -1 | awk '{print $1}')
cat > /tmp/eviction.json <<JSON
{"apiVersion":"policy/v1","kind":"Eviction","metadata":{"name":"$POD","namespace":"$NS"}}
JSON
kubectl create --raw "/api/v1/namespaces/$NS/pods/$POD/eviction" -f /tmp/eviction.json
```

A successful call returns `"code": 201`. Expect `status.failed` **not** to increase, a replacement
attempt to run, and the package never to read `erroring`. Contrast: a plain `kubectl delete pod` is
not a disruption and does spend an attempt.

## 3. A node disappearing mid-run

**Not covered.** (`nodewright/cleanup-pods` sweeps orphaned *pods* after a state reset, which is a
different path.) *Destructive — run it last; recovering the node means rebuilding the cluster.*

Delete a node while its package Job is running. Expect the Job foreground-deleted within a reconcile
or two, with an operator log line naming the reason; no replacement created; other nodes unaffected;
and the CR still reaching `complete`.

## 4. Re-execution semantics of a real package

**Not covered** — and the one to run before shipping any package. Needs the real agent and a
`shellscript` package whose `apply.sh` appends a line to a host file, with an `apply_check.sh` that
fails.

Expect the script to run **once per attempt** — four executions and four lines under the default
`backoffLimit: 3`. The agent writes a completion flag but honours it only when the package declares
`Idempotence.Auto`; `shellscript` declares `Disabled`, and the agent logs
`Flag exists but idempotence is Idempotence.Disabled so running step.` Confirm the same on a
`kubectl nodewright package rerun`. **Under Jobs, a non-idempotent step is multiplied by the retry
budget** — this is the fact package authors most need to know, and nothing else demonstrates it.

## 5. Retention actually collects, by outcome

`nodewright/failure-nodewright` asserts the TTL **value** on a timed-out Job. What it cannot show is
the cluster acting on it. Run one succeeding and one failing package with
`JOB_TTL_SUCCEEDED=1m JOB_TTL_FAILED=3m` (or `controllerManager.manager.env.{jobTtlSucceeded,jobTtlFailed}`
on a chart install; one minute is a hard floor the operator refuses to start below).

Expect the TTL absent until a Job finishes then set from the outcome; the succeeded Jobs **and their
pods** collected first while the failed one remains; **node state unchanged by collection**; and after
the failed Job is collected, a fresh attempt appearing — the slow-retry cadence, easily mistaken for
churn.

## 6. Disable must not resume a paused stage

`nodewright/pause-suspends-jobs` covers pause and resume; the disable interaction has no coverage and
is the shape a past regression turned on. With a stage running: pause, then in **one** edit add the
disable annotation and remove the pause. The Job must **stay** suspended. Re-enable and unpause, and
the stage finishes normally.

## 7. A stage timeout kills on the clock

`nodewright/simple-nodewright` asserts `stageTimeout` is *set*; nothing checks it *fires*. Use
`stageTimeout: 20s` with `SLEEP_LEN: 600` and time it.

Expect the attempt killed at ~20s rather than 600s, its pod `Failed`/`DeadlineExceeded` **in place**
and still readable, and the bound per-attempt with no Job-level deadline. Then raise `stageTimeout`,
re-apply, and confirm the old Job is cleared and the new one carries the new value.

## 8. Node state survives a rollback

`helm/helm-upgrade-rollback-test` proves the rollback *mechanism* — CRDs, webhooks and resource names
survive. It does not drive a package first, so it cannot speak to state.

Install the previous release, drive a CR to `complete`, snapshot the node's `nodewright.nvidia.com/*`
annotations, upgrade to the RC, then `helm rollback`. Expect the previous operator to come up Ready,
the CR to still read `complete`, **no package to re-run**, and the annotations to be byte-identical to
the snapshot. Note `helm rollback` with no revision targets the previous *deployed* revision — after a
failed attempt that is not the baseline, so pass the revision explicitly.

---

## Known limitations to expect

Confirm rather than re-file; a change in either is itself a signal.

- A **successful** `uninstall` Job and its pod are deleted immediately
  ([#443](https://github.com/NVIDIA/nodewright/issues/443)), so its output is unrecoverable through
  Kubernetes. The agent's log on the node survives.
- An unpullable image reports `ContainerStatusUnknown` on the archived attempt rather than
  `ImagePullBackOff` ([#306](https://github.com/NVIDIA/nodewright/issues/306)).

## Sign-off

Record the build under test (`git_sha` from the operator's startup log, plus chart and agent
versions), pass/fail per case, and the evidence path. A failure blocks the release unless it is a
known limitation above, or is explicitly accepted and filed.

| # | Case | Operator settings | Result |
|---|---|---|---|
| 1 | kubelet-refused attempts | defaults | |
| 2 | disruption costs no budget | defaults | |
| 3 | node disappears mid-run (destructive, last) | defaults | |
| 4 | re-execution semantics | **real agent** | |
| 5 | retention collects by outcome | `JOB_TTL_SUCCEEDED=1m JOB_TTL_FAILED=3m` | |
| 6 | disable must not resume | defaults | |
| 7 | stage timeout fires on the clock | defaults | |
| 8 | node state survives a rollback | chart install, previous release | |

### Not covered anywhere

Real reboots (kind nodes are containers — use `service`/`noop` interrupts), fleet scale, and image
provenance. Webhook rejection paths need a chart install; `make run` defaults `ENABLE_WEBHOOKS=false`.
