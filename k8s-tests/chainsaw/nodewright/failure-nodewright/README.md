# Failure NodeWright Test

## Purpose

Validates that the operator correctly handles package failures and reports the error state.

## Test Scenario

1. Reset state from previous runs
2. Apply a nodewright with a package configured to fail
3. Verify the node and nodewright report the error status
4. Assert error conditions are properly set

## Key Features Tested

- Package failure handling
- Error status propagation
- Node condition reporting for errors
- NodeWright error state

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - NodeWright with failing package
- `assert.yaml` - Error state assertions
