# Shared Cordon Ownership

## Purpose

Verify that a node selected by two interrupt-enabled NodeWrights stays cordoned while either NodeWright still owns a cordon annotation.

## Test Scenario

1. Label the e2e test node with a test-specific selector and start a non-evictable blocker pod on it.
2. Create `shared-cordon-slow`, which cordons the node and waits because its `podNonInterruptLabels` match the blocker pod.
3. Create `shared-cordon-fast`, which also requires an interrupt and completes while `shared-cordon-slow` still owns the cordon.
4. Assert that the fast NodeWright removed only its own cordon annotation and the node remains unschedulable.
5. Delete the blocker pod, wait for the slow NodeWright to complete, and assert the node is schedulable with no `cordon_*` annotations.

## Key Features Tested

- Shared node cordon ownership across multiple NodeWrights
- Cordon annotation cleanup for the completing NodeWright only
- Final uncordon after all NodeWright-owned cordons are released

## Files

- `chainsaw-test.yaml` - Main test configuration
- `blocker-pod.yaml` - Pod that holds the slow NodeWright at the pre-interrupt wait
- `nodewright-fast.yaml` - Fast interrupt-enabled NodeWright
- `nodewright-slow.yaml` - Slow interrupt-enabled NodeWright blocked by `podNonInterruptLabels`

## Notes

- The blocker pod tolerates `node.kubernetes.io/unschedulable`, so the fast NodeWright's drain ignores it while the slow NodeWright still treats it as non-interrupt work.
