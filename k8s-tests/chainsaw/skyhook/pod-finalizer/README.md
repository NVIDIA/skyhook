# Pod Finalizer Test

## Purpose

Validates that the operator correctly handles pod finalization and cleanup of node state when pods complete.

## Test Scenario

1. Clear any existing node annotations from previous runs
2. Create a pod that writes state to node annotations
3. Assert the node annotations are set correctly
4. Verify the pod finalizer behavior

## Key Features Tested

- Pod finalization
- Node annotation management
- State cleanup on pod completion

## Files

- `chainsaw-test.yaml` - Main test configuration
- `pod.yaml` - Pod definition with finalizer
- `node-assert.yaml` - Node state assertions
