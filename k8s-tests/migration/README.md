# Skyhook to NodeWright upgrade migration test

Upgrades a real cluster from the last pre-rename operator to the working tree and proves the migration adopts existing per-node state **without re-running packages**.

This is the only test in the repo that exercises state written by a *previous* operator. The chainsaw test `k8s-tests/chainsaw/nodewright/mirror-legacy-skyhook` applies a legacy `Skyhook` to an already-upgraded operator, which is a different and much weaker claim.

## Version pair

| | Chart | Operator image | CRDs served |
| --- | --- | --- | --- |
| From | `chart/v0.17.1` (git tag) | `ghcr.io/nvidia/nodewright/operator:v0.17.0` | `skyhook.nvidia.com` only |
| To | working tree `chart/` | locally built, pushed to the ctlptl registry | both groups |

v0.17.0 is the last operator that served only the legacy group, so this pin is correct permanently for this test's purpose and should not be advanced when newer operators ship.

The chart and image names moved to `nodewright` *before* the CRD rename, which is why the "old" side already says `nodewright`. Only the API group moved in the rename being tested here.

## Running it

```bash
cd operator
make create-kind-cluster   # must come FIRST
make migration-test        # depends on push-local-image
```

**Create the cluster before pushing the image.** `make create-kind-cluster` deletes and recreates the ctlptl registry along with the cluster, so anything pushed beforehand is gone. The symptom is `ImagePullBackOff` with `localhost:5005/skyhook-operator:testing: not found` and a helm upgrade that fails with the rollout stuck at `Updated: 1/2`.

The image under test is whatever `LOCAL_OPERATOR_IMG` points at; `run.sh` derives the repo and tag from it rather than hardcoding them, so overriding `LOCAL_REGISTRY` cannot leave the suite deploying a stale image from a previous build.

Or drive phases directly, which is what you want while debugging:

```bash
cd k8s-tests/migration
./run.sh                     # every phase
./run.sh --to-phase 4        # stop after the core assertion
./run.sh --from-phase 4      # resume, reusing the captured fingerprint
```

The cluster must be a clean pre-rename baseline. Phase 1 refuses to run if `nodewright.nvidia.com` CRDs already exist, because that would make every later assertion pass vacuously. A cluster that has run the normal e2e suite is **not** a valid baseline; recreate it.

## Phases

| Phase | What it does |
| --- | --- |
| 1 | Installs the pre-rename operator from the `chart/v0.17.1` tag |
| 2 | Runs a real package to `complete`, then captures the fingerprint |
| 3 | `helm upgrade` to the working-tree chart |
| 4 | Asserts state migrated and nothing re-ran |
| 5 | Asserts legacy objects went read-only |
| 6 | Follows `docs/getting-started/migration.md` verbatim |
| 7 | Asserts the prune after `LEGACY_CLEANUP_DELAY` |
| 8 | Asserts the migration hold |

## How "packages did not re-run" is proven

Not by pod comparison. The operator deletes package pods once a package completes, so steady state has zero and a before/after pod diff would be empty on both sides, which is vacuously green.

The real signals, captured by `capture_state` into `.fingerprint/`:

- **Agent log files.** The agent writes a new timestamped log per run (`shellscript_run.sh-<date>-<time>.log`). A re-run means a new file appears. This is the strongest signal.
- **`Hello, world!` count** across all logs, which the seeded `apply.sh` emits once per run.
- **Agent flag file paths and mtimes** under `/var/lib/skyhook/<name>/flags/`.
- **Node annotations, labels, and conditions**, compared by mapping the legacy prefix onto the new one.

Host paths stay under `/var/lib/skyhook` and `/var/log/skyhook` because the agent has not been renamed. That is correct; do not "fix" it.

## Reading a failed diff

`require_clean_diff` prints a unified diff before failing. Interpretation:

- **`annotations.expected` vs `annotations.actual`** differing means the migration did not carry a value across. That is a product bug.
- **`flags` or `logfiles` or `hellocount`** differing means a package re-ran. That is the headline bug this test exists to catch.
- **`annotations.legacy.before` vs `.after`** differing means the rollback window was violated: the legacy copies must survive until the prune.

Do not relax an assertion to get green. If one genuinely over-specifies, say so explicitly and record it as a known coverage gap.

### Nothing is excluded

Every captured key is compared strictly, `version_<name>` included.

An earlier draft excluded it, on the theory that the operator would bump its own "which version last touched this node" stamp across the upgrade. It does not, and there is a structural reason: `skyhookNode.Migrate` (`internal/wrapper/node.go`) only calls `SetVersion()` inside the `case "", "v0.5", "v0.6", "v0.7"` branch of its `version.MajorMinor(from)` switch. Upgrading from `v0.17.x` matches no case, so `SetVersion()` never runs on that path and the stamp is carried across verbatim like any other annotation.

If a future release adds a migration branch for the current version, this assertion will start failing. That is the correct outcome: it means the stamp really did change and someone should decide whether that is intended, rather than the test having quietly excluded it in advance.

## What this environment does NOT cover

Worth knowing before reading a green run as full coverage.

- **Node labels are dual-stamped.** `make setup-kind-cluster` labels `kind-worker` with *both*
  `skyhook.nvidia.com/test-node` and `nodewright.nvidia.com/test-node`. Real clusters usually carry only
  one. That means phase 6 cannot detect a rename that wrongly rewrites `nodeSelectors` keys: the node
  would match either way. Phase 6 compensates by asserting directly that the rename left the selector
  key alone, rather than inferring it from the rollout still working.
- **No interrupt.** The seeded package has no interrupt stage, so the node is never cordoned or drained
  and no runtime-required taint is ever applied. Phase 7's taint-preservation check is therefore vacuous
  here and says so in its output rather than implying coverage.
- **One node, one package.** Nothing here exercises batching, interruption budgets, `DeploymentPolicy`,
  or a multi-package dependency graph across a migration.
- **Upgrade only.** Rollback to the pre-rename operator is not exercised, only the invariants that make
  it possible (legacy annotations and pods surviving until prune).
- **Drain behavior during the rollback window is untested.** `drain.isSkyhookPackagePod` requires *both*
  the `nodewright.nvidia.com/name` and `.../package` labels, so a surviving pre-rename package pod is not
  recognized as a package pod during a drain. Reachable only if another NodeWright interrupts that node
  inside the window, which this suite never sets up.
- **The hold is exercised by restarting the operator, not by a genuine mid-upgrade rollout.** Phase 8
  parks a legacy `Skyhook` at `in_progress` and restarts the controller-manager, which reproduces the
  documented on-startup protection but not the harder case of an upgrade landing while packages are
  actually mutating a host. Note the hold is only evaluated inside the NodeWright reconcile, and a legacy
  status patch does not bump `generation`, so nothing queues a reconcile on its own; waiting for one
  instead of restarting makes the assertion flaky, not thorough.

## Fingerprint directory

`.fingerprint/` is gitignored working state. Delete it to force a clean capture; `--from-phase 4` reuses it.
