# Multi-Compartment Test

## Purpose

Validates deployment policy with multiple compartments using exponential strategy, verifying compartment assignment, budget enforcement, and batch progression.

## Test Scenario

1. Reset state and label nodes with compartment assignments
2. Create a DeploymentPolicy with multiple compartments
3. Apply a skyhook using the policy
4. Verify:
   - Nodes are assigned to correct compartments based on labels
   - Each compartment follows exponential batch progression (1, 2, 4, 8...)
   - Budget is enforced per compartment
5. Assert the skyhook completes with all compartments processed

## Key Features Tested

- Multiple compartment configuration
- Label-based compartment assignment
- Exponential rollout strategy
- Per-compartment budget enforcement
- Parallel compartment processing

## Files

- `chainsaw-test.yaml` - Main test configuration
- `deployment-policy.yaml` - Policy with multiple compartments
- `skyhook.yaml` - Skyhook using the policy
- `assert-compartments.yaml` - Compartment assignment assertions
- `assert-batch-*.yaml` - Batch progression assertions
- `assert-complete.yaml` - Final completion assertions

## Notes

- Requires the 15-node test cluster (`make create-deployment-policy-cluster`)
