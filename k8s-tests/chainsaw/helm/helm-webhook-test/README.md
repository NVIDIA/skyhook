# Helm Webhook Test

## Purpose

Validates that the admission webhooks correctly validate NodeWright and DeploymentPolicy resources.

## Test Scenario

1. Reset state and install the operator
2. Test invalid resource rejection:
   - Invalid NodeWright (bad dependencies) should be rejected
   - Invalid DeploymentPolicy (bad config) should be rejected
3. Test policy reference validation:
   - NodeWright with non-existent policy reference should be rejected
   - NodeWright with valid policy reference should be accepted
   - Updating NodeWright to non-existent policy should be rejected
4. Test policy deletion protection:
   - Deleting DeploymentPolicy in use should be rejected
   - Deleting DeploymentPolicy after NodeWright removed should succeed

## Key Features Tested

- Validating webhook for NodeWright resources
- Validating webhook for DeploymentPolicy resources
- Package dependency validation
- Policy reference validation
- Policy deletion protection

## Files

- `chainsaw-test.yaml` - Main test configuration
- `values.yaml` - Helm values for webhook configuration
- `invalid-nodewright.yaml` - NodeWright with invalid dependencies
- `invalid-deploymentpolicy.yaml` - Invalid DeploymentPolicy
- `valid-deploymentpolicy.yaml` - Valid DeploymentPolicy
- `nodewright-valid-policy.yaml` - NodeWright with valid policy reference
- `nodewright-missing-policy.yaml` - NodeWright with missing policy reference
- `assert-*.yaml` - State assertions
