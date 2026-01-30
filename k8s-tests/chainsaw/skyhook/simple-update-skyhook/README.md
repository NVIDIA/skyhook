# Simple Update Skyhook Test

## Purpose

Validates that updating a skyhook triggers the appropriate re-processing of packages.

## Test Scenario

1. Reset state from previous runs
2. Apply an initial skyhook configuration
3. Wait for the skyhook to complete
4. Update the skyhook with new configuration
5. Verify the update triggers re-processing
6. Assert the final state reflects the updated configuration

## Key Features Tested

- Skyhook update handling
- Package re-processing on update
- State transition during updates
- Final state consistency

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Initial skyhook definition
- `update.yaml` - Updated skyhook configuration
- `assert.yaml` - Initial state assertions
- `assert-update.yaml` - Post-update state assertions
