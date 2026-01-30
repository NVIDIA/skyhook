# Overlapping Selectors Test

## Purpose

Validates deployment policy with overlapping compartment selectors, verifying that nodes are assigned to the most specific match.

## Test Scenario

1. Reset state and label nodes with overlapping labels
2. Create a DeploymentPolicy with compartments that have overlapping selectors
3. Apply a skyhook using the policy
4. Verify:
   - Nodes matching multiple compartments are assigned correctly
   - Assignment is based on most specific match (most labels matched)
   - No node is assigned to multiple compartments
5. Assert batch progression and completion

## Key Features Tested

- Overlapping compartment selectors
- Most-specific-match assignment algorithm
- Label count-based tie-breaking
- Deterministic node-to-compartment assignment

## Files

- `chainsaw-test.yaml` - Main test configuration
- `deployment-policy.yaml` - Policy with overlapping selectors
- `skyhook.yaml` - Skyhook using the policy
- `assert-compartments.yaml` - Compartment assignment assertions
- `assert-batch-*.yaml` - Batch progression assertions
- `assert-complete.yaml` - Final completion assertions

## Notes

- Requires the 15-node test cluster (`make create-deployment-policy-cluster`)
