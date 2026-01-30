# Helm Chart Test

## Purpose

Validates that the Helm chart deploys correctly with custom configurations, including custom deployment names and tolerations.

## Test Scenario

1. Reset state from previous runs
2. Install the Helm chart with custom values:
   - Different deployment name than `skyhook-operator`
   - Custom tolerations
3. Verify the operator is scheduled correctly
4. Apply a skyhook and verify it completes
5. Assert metrics and state are correct

## Key Features Tested

- Custom deployment name support
- Toleration configuration via Helm values
- Operator deployment and scheduling
- End-to-end skyhook processing with Helm-deployed operator

## Files

- `chainsaw-test.yaml` - Main test configuration
- `values.yaml` - Helm values with custom configuration
- `skyhook.yaml` - Test skyhook
- `deployment-policy.yaml` - Test deployment policy
- `assert-*.yaml` - State assertions
