# Legacy Compatibility Test

## Purpose

Validates backwards compatibility with the legacy `interruptionBudget` field for existing customer configurations.

## Test Scenario

1. Reset state from previous runs
2. Create a Skyhook using the legacy `interruptionBudget` (count: 3) instead of `deploymentPolicy`
3. Verify that:
   - A synthetic `__default__` compartment is automatically created
   - The budget ceiling is respected (max 3 nodes in progress at a time)
4. Assert the skyhook completes successfully with legacy configuration

## Key Features Tested

- Backwards compatibility with `interruptionBudget` field
- Automatic `__default__` compartment creation
- Budget enforcement with legacy configuration
- Migration path for existing configurations

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Skyhook with legacy interruptionBudget
- `assert-default-compartment.yaml` - Compartment creation assertions
