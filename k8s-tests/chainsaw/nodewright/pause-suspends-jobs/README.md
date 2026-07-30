# pause-suspends-jobs

Validates that pausing a NodeWright suspends its in-flight package Jobs, and that resuming restarts the interrupted stage.

## Why this needs a real cluster

The unit tests in `swap_test.go` cover the decision to set `spec.suspend` — which Jobs are selected, that finished and invalid Jobs are skipped, that it is idempotent. They run against a fake client, where **nothing acts on the field**. Everything the feature actually promises is Job-controller behavior and can only be observed here:

- suspension SIGTERMs the running pod
- node state stays `in_progress` rather than flipping to `erroring`
- resume starts a fresh pod that re-runs the stage

## Test Scenario

1. Install a package with `SLEEP_LEN=300` so its apply stage is still running when paused, and wait for a Running apply pod
2. Pause via the `nodewright.nvidia.com/pause` annotation
3. Verify the Job is `spec.suspend: true` (suspended, not deleted, so it can resume)
4. Verify no apply pod survives
5. **Verify node state is not `erroring`** — see below
6. Unpause, verify `spec.suspend: false` and a fresh apply pod appears

## The assertion that matters

Step 5 is the reason this test exists. A pod deleted by suspension is, to the pod watch, indistinguishable from a pod that failed — same absent container statuses, same terminal-looking state. Only the `DeletionTimestamp` guard in `PodReconcile` separates them. Without it, **every pause would mark its packages erroring**, which would in turn count against DeploymentPolicy failure budgets.

That guard is exercised by no other test, and a fake client cannot produce the situation: it has no Job controller to delete the pod in the first place.
