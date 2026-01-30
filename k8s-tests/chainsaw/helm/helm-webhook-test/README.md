# Helm Webhook Test

## Purpose

Validates that the admission webhooks correctly validate Skyhook and DeploymentPolicy resources.

## Test Scenario

1. Reset state and install the operator
2. Test invalid resource rejection:
   - Invalid Skyhook (bad dependencies) should be rejected
   - Invalid DeploymentPolicy (bad config) should be rejected
3. Test policy reference validation:
   - Skyhook with non-existent policy reference should be rejected
   - Skyhook with valid policy reference should be accepted
   - Updating Skyhook to non-existent policy should be rejected
4. Test policy deletion protection:
   - Deleting DeploymentPolicy in use should be rejected
   - Deleting DeploymentPolicy after Skyhook removed should succeed

## Key Features Tested

- Validating webhook for Skyhook resources
- Validating webhook for DeploymentPolicy resources
- Package dependency validation
- Policy reference validation
- Policy deletion protection

## Files

- `chainsaw-test.yaml` - Main test configuration
- `values.yaml` - Helm values for webhook configuration
- `invalid-skyhook.yaml` - Skyhook with invalid dependencies
- `invalid-deploymentpolicy.yaml` - Invalid DeploymentPolicy
- `valid-deploymentpolicy.yaml` - Valid DeploymentPolicy
- `skyhook-valid-policy.yaml` - Skyhook with valid policy reference
- `skyhook-missing-policy.yaml` - Skyhook with missing policy reference
- `assert-*.yaml` - State assertions
