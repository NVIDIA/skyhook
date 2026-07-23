# Simple Update NodeWright Test

## Purpose

Validates that updating a nodewright triggers the appropriate re-processing of packages.

## Test Scenario

1. Reset state from previous runs
2. Apply an initial nodewright configuration
3. Wait for the nodewright to complete
4. Update the nodewright with new configuration
5. Verify the update triggers re-processing
6. Assert the final state reflects the updated configuration

## Key Features Tested

- NodeWright update handling
- Package re-processing on update
- State transition during updates
- Final state consistency

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - Initial nodewright definition
- `update.yaml` - Updated nodewright configuration
- `assert.yaml` - Initial state assertions
- `assert-update.yaml` - Post-update state assertions
