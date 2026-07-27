# Delete NodeWright Test

## Purpose

Validates that deleting a NodeWright resource properly cleans up all associated resources and metrics.

## Test Scenario

1. Reset state from previous runs
2. Apply a nodewright with multiple packages (dexter, spencer, foobar)
3. Wait for the nodewright to complete
4. Verify all metrics are present:
   - Node target count
   - Node status count
   - Package state counts
   - Package stage counts
   - Rollout metrics
5. Delete the nodewright
6. Verify all metrics are cleaned up

## Key Features Tested

- NodeWright deletion
- Metrics cleanup after deletion
- Resource cleanup (configmaps, owner references)
- Multiple package handling

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - NodeWright resource definition
- `assert.yaml` - State assertions
