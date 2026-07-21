# Shared Cordon Ownership

## Purpose

Verify that a node selected by two interrupt-enabled Skyhooks stays cordoned while either Skyhook still owns a cordon annotation.

## Test Scenario

1. Label the e2e test node with a test-specific selector and start a non-evictable blocker pod on it.
2. Create `shared-cordon-slow`, which cordons the node and waits because its `podNonInterruptLabels` match the blocker pod.
3. Create `shared-cordon-fast`, which also requires an interrupt and completes while `shared-cordon-slow` still owns the cordon.
4. Assert that the fast Skyhook removed only its own cordon annotation and the node remains unschedulable.
5. Delete the blocker pod, wait for the slow Skyhook to complete, and assert the node is schedulable with no `cordon_*` annotations.

## Key Features Tested

- Shared node cordon ownership across multiple Skyhooks
- Cordon annotation cleanup for the completing Skyhook only
- Final uncordon after all Skyhook-owned cordons are released

## Files

- `chainsaw-test.yaml` - Main test configuration
- `blocker-pod.yaml` - Pod that holds the slow Skyhook at the pre-interrupt wait
- `nodewright-fast.yaml` - Fast interrupt-enabled Skyhook
- `nodewright-slow.yaml` - Slow interrupt-enabled Skyhook blocked by `podNonInterruptLabels`

## Notes

- The blocker pod tolerates `node.kubernetes.io/unschedulable`, so the fast Skyhook's drain ignores it while the slow Skyhook still treats it as non-interrupt work.
