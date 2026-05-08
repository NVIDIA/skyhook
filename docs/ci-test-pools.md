# CI test pools

The skyhook chainsaw e2e suite (`k8s-tests/chainsaw/skyhook/`) is split into labeled pools that run as parallel CI jobs to keep wall-clock per matrix row under ~18 min.

## Pools

Every test under `k8s-tests/chainsaw/skyhook/*/chainsaw-test.yaml` has a top-level `metadata.labels.pool: <name>`:

- `core` — fast smoke and feature tests; **also the fallback for any
  test missing a `pool:` label**.
- `interrupt` — cordon/drain/taint/runtime-required flows.
- `uninstall` — uninstall, upgrade, downgrade lifecycle.
- `lifecycle` — pause/disable/delete/finalizer/state.

## Running locally

```bash
# All tests (default — same as before pools existed):
make e2e-tests

# A single pool:
POOL=interrupt make e2e-tests
```

The `POOL` variable is honored by `operator/Makefile`'s `e2e-tests`
target. `POOL=core` uses a `pool notin (...)` selector so unlabeled
tests still run.

## Adding a new pool

Three-file change, no branch-protection edits:

1. Add `pool: <newpool>` to the relevant `chainsaw-test.yaml` files' `metadata.labels` block.
2. Append `<newpool>` to the `pool:` matrix list in `.github/workflows/operator-ci.yaml`.
3. Append `<newpool>` to `POOL_NON_DEFAULT` in `operator/Makefile` so the `core` pool's `notin (...)` selector excludes it.

Worst case if step 3 is forgotten: the new pool's tests run twice (once in their pool, once in `core`). Noisy but not silently broken.

## Code coverage

Per-row coverage artifacts are named `coverage-<test-suite>[-<pool>]-k8s-<version>`. The `upload-coverage` job globs `coverage-*` and concatenates the per-row `cover.out` files (Go covdata `mode: set` format auto-deduplicates lines), producing a single combined profile uploaded to Coveralls — equivalent to the pre-pool combined profile.

## Gate jobs

Every workflow that gates merge publishes a check named **`ci-gate`**.
GitHub composes same-named required checks across workflows, so branch
protection on `main` requires only the single `ci-gate` name and every
`ci-gate` that posts must pass.

Workflows publishing `ci-gate`:

- `lint-ci.yaml` — runs on every PR (no path filter). Guarantees a
  `ci-gate` is always posted, even on doc-only changes.
- `operator-ci.yaml`, `agent-ci.yaml` — wrapper job that depends on the
  matrix and image-build jobs. Posts `ci-gate` only when the workflow's
  paths trigger.
- `commit-linting.yaml`, `security-checkov.yaml`,
  `agentless-container.yaml` — single-job workflows wrap their worker
  in a `ci-gate` job for uniformity.

The `if: always()` + `jq -e '... result == "success"'` pattern is the
standard fix for GitHub Actions' "skipped == green" pitfall. Each gate's
`needs:` only lists jobs that always run — conditionally-skipped jobs
(`create-manifest`/`upload-coverage` on fork PRs / tag builds) are
deliberately excluded so legitimate skips don't fail the gate.
