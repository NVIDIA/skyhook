# Delete Skyhook Test

## Purpose

Validates that deleting a Skyhook resource properly cleans up all associated resources and metrics.

## Test Scenario

1. Reset state from previous runs
2. Apply a skyhook with multiple packages (dexter, spencer, foobar)
3. Wait for the skyhook to complete
4. Verify all metrics are present:
   - Node target count
   - Node status count
   - Package state counts
   - Package stage counts
   - Rollout metrics
5. Delete the skyhook
6. Verify all metrics are cleaned up

## Key Features Tested

- Skyhook deletion
- Metrics cleanup after deletion
- Resource cleanup (configmaps, owner references)
- Multiple package handling

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Skyhook resource definition
- `assert.yaml` - State assertions
