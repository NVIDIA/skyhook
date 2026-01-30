# Linear Strategy Test

## Purpose

Validates deployment policy with linear ramp-up strategy, verifying incremental batch growth and capacity limits.

## Test Scenario

1. Reset state and label nodes for testing
2. Create a DeploymentPolicy with linear strategy
3. Apply a skyhook using the policy
4. Verify batch progression:
   - Batch 1: Initial nodes processed
   - Batch 2: Incremental growth (delta-based)
   - Batch 3: Continued linear growth
   - Batch 4: Remaining nodes up to capacity limit
5. Assert the skyhook completes with all nodes processed

## Key Features Tested

- Linear rollout strategy
- Delta-based batch growth
- Capacity limit enforcement
- Batch progression metrics

## Files

- `chainsaw-test.yaml` - Main test configuration
- `deployment-policy.yaml` - Policy with linear strategy
- `skyhook.yaml` - Skyhook using the policy
- `assert-batch-*.yaml` - Batch progression assertions
- `assert-compartment.yaml` - Compartment state assertions
- `assert-complete.yaml` - Final completion assertions

## Notes

- Requires the 15-node test cluster (`make create-deployment-policy-cluster`)
