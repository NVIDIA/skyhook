# Config Skyhook Test

## Purpose

Validates that configuration changes work correctly for the operator, including config interrupts and merging behavior.

## Test Scenario

1. Apply a simple skyhook definition and verify packages start applying
2. Update two packages before the skyhook finishes:
   - One with a package interrupt
   - One without a package interrupt
   - Both on a configmap key with a config interrupt defined
3. Assert that config interrupts are merged and run for both packages
4. Verify the package without an interrupt doesn't hang when config changes occur
5. Update the same two packages again on a key with a config interrupt
6. Assert that config, interrupt, and post-interrupt stages run correctly
7. Update one more time on a key without a config interrupt defined
8. Verify only the config step runs

## Key Features Tested

- Configuration changes during package execution
- Config interrupt merging
- Package interrupt and config interrupt interaction
- Handling packages without interrupts during config changes
- Post-interrupt stage execution

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Initial skyhook definition
- `update*.yaml` - Various update configurations
- `assert*.yaml` - State assertions for each phase
