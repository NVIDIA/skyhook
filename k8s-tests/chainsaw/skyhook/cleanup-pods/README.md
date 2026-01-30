# Cleanup Pods Test

## Purpose

Validates that the operator correctly cleans up pods when a node's state is reset.

## Test Scenario

1. Create a skyhook with package dependencies (A depends on B)
2. Wait for the skyhook to complete
3. Trigger an update to force a config cycle on package B
4. Once config is complete, update again to make the package error
5. Clear out the node annotation to trigger cleanup
6. Verify that pods that should not be running are removed

## Key Features Tested

- Pod cleanup after node state reset
- Handling of erroring packages
- Package dependency handling during cleanup
- Orphan pod detection and removal

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Skyhook resource definition
- `assert.yaml` - State assertions
