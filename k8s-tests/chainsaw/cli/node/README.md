# CLI Node Test

## Purpose

Validates all CLI node commands for managing node state within a Skyhook.

## Test Scenario

1. Reset state from previous runs
2. Create a skyhook and wait for it to complete
3. Test node list command:
   - Run `skyhook node list <skyhook>`
   - Verify output shows targeted nodes
4. Test node status command:
   - Run `skyhook node status [node]`
   - Verify output shows Skyhook activity on nodes
5. Test node ignore command:
   - Run `skyhook node ignore <skyhook> <node>`
   - Assert the node is excluded from processing
6. Test node unignore command:
   - Run `skyhook node unignore <skyhook> <node>`
   - Assert the node is included back in processing
7. Test node reset command:
   - Run `skyhook node reset <skyhook> <node>`
   - Assert the package state is reset on the node

## Key Features Tested

- `skyhook node list` - Shows nodes targeted by a Skyhook
- `skyhook node status` - Shows Skyhook activity on nodes
- `skyhook node ignore` - Excludes a node from processing
- `skyhook node unignore` - Includes a node back in processing
- `skyhook node reset` - Resets package state on a node

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Test skyhook
- `assert-node-ignored.yaml` - Ignored state assertion
- `assert-node-unignored.yaml` - Unignored state assertion
- `assert-node-reset.yaml` - Reset state assertion
