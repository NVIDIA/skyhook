# stage-timeout

Validates that a package's `stageTimeout` is enforced per attempt, and that no deadline caps the stage as a whole.

## Why this needs a real cluster

Unit tests cover the arithmetic — that `stageTimeout` lands on the pod template and that `0` omits the field. Nothing in a fake client runs the clock. The claim that matters to a user is that a stage which hangs is *killed*, and only the kubelet does that.

The Job-level `activeDeadlineSeconds` was removed deliberately (#402): a whole-stage ceiling interacts badly with the retry budget, because a stage that has retried is closer to its ceiling through no fault of the attempt currently running. Its absence is asserted here so a future change cannot quietly reintroduce it.

## Test Scenario

1. Install a package with `stageTimeout: 20s` whose step sleeps for 600s
2. Assert the Job's **pod template** carries `activeDeadlineSeconds: 20`
3. Assert the **Job** carries no `activeDeadlineSeconds` of its own
4. Assert the attempt pod ends `Failed` with reason `DeadlineExceeded`

The gap between 20s and 600s is the assertion: a pod reporting `DeadlineExceeded` cannot have run to completion, so the test proves the deadline fired without measuring wall-clock.
