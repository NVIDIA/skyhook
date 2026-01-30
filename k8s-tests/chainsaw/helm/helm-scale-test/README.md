# Helm Scale Test

## Purpose

Validates that the Helm chart correctly handles resource scaling and override configurations.

## Test Scenario

1. Install the Helm chart with scaled resource configuration
2. Assert the resources are set correctly
3. Update with resource override values
4. Assert the override resources are applied

## Key Features Tested

- Resource scaling via Helm values
- Resource override configuration
- Deployment resource requests and limits
- Helm upgrade with different values

## Files

- `chainsaw-test.yaml` - Main test configuration
- `values-scale.yaml` - Helm values with scaled resources
- `vaules-override-resources.yaml` - Helm values with resource overrides
- `assert-scaled-resources.yaml` - Scaled resource assertions
- `assert-override-resources.yaml` - Override resource assertions
