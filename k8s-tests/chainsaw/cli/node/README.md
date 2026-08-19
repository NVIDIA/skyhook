# CLI Node Test

## Purpose

Validates all CLI node commands for managing node state within a NodeWright.

## Test Scenario

1. Reset state from previous runs
2. Create a nodewright and wait for it to complete
3. Test node list command:
   - Run `nodewright node list <nodewright>`
   - Verify output shows targeted nodes
4. Test node status command:
   - Run `nodewright node status [node]`
   - Verify output shows NodeWright activity on nodes
5. Test node ignore command:
   - Run `nodewright node ignore <nodewright> <node>`
   - Assert the node is excluded from processing
6. Test node unignore command:
   - Run `nodewright node unignore <nodewright> <node>`
   - Assert the node is included back in processing
7. Test node reset command:
   - Run `nodewright node reset <nodewright> <node>`
   - Assert the package state is reset on the node

## Key Features Tested

- `nodewright node list` - Shows nodes targeted by a NodeWright
- `nodewright node status` - Shows NodeWright activity on nodes
- `nodewright node ignore` - Excludes a node from processing
- `nodewright node unignore` - Includes a node back in processing
- `nodewright node reset` - Resets package state on a node

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - Test nodewright
- `assert-node-ignored.yaml` - Ignored state assertion
- `assert-node-unignored.yaml` - Unignored state assertion
- `assert-node-reset.yaml` - Reset state assertion
