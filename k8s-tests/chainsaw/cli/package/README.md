# CLI Package Test

## Purpose

Validates all CLI package commands for inspecting and managing package state.

## Test Scenario

1. Reset state from previous runs
2. Create a skyhook and wait for it to complete
3. Test package status command:
   - Run `skyhook package status <skyhook> <package>`
   - Verify output shows package status across nodes
4. Test package logs command:
   - Run `skyhook package logs <skyhook> <package>`
   - Verify logs are retrieved from package pods
5. Test package rerun command:
   - Run `skyhook package rerun <skyhook> <package>`
   - Assert the package is re-run on the node

## Key Features Tested

- `skyhook package status` - Shows package status across nodes
- `skyhook package logs` - Retrieves logs from package pods
- `skyhook package rerun` - Forces a package to re-run

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Test skyhook
- `assert-complete.yaml` - Initial completion assertion
- `assert-package-rerun.yaml` - Rerun state assertion
