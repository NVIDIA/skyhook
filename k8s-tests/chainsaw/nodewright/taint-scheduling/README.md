# Taint Scheduling Test

## Purpose

Validates that the operator correctly handles node taints and can schedule packages on tainted nodes using tolerations.

## Test Scenario

1. Reset state from previous runs
2. Add a taint to the test nodes (`nvidia.com/gpu:NoSchedule`)
3. Apply a nodewright without the required toleration
4. Verify the nodewright is blocked (status: blocked)
5. Update the nodewright with the required toleration
6. Verify the nodewright completes successfully

## Key Features Tested

- Node taint handling
- NodeWright blocked status when tolerations are missing
- Additional tolerations (`additionalTolerations`) configuration
- Package scheduling on tainted nodes

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - Initial nodewright without toleration
- `update-nodewright.yaml` - NodeWright with toleration added
- `assert.yaml` - Blocked state assertion
- `assert-update.yaml` - Completed state assertion
