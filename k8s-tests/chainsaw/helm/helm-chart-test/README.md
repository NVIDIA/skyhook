# Helm Chart Test

## Purpose

Validates that the Helm chart deploys correctly with custom configurations, including custom deployment names, tolerations, and automatic cleanup on uninstall.

## Test Scenario

1. Reset state from previous runs
2. Install the Helm chart with a "bad" node taint and verify pods don't schedule
3. Change to a "good" node taint that matches configured tolerations
4. Reinstall the Helm chart (tests uninstall + reinstall flow)
5. Verify the operator is scheduled correctly with tolerations
6. Apply a DeploymentPolicy and Skyhook
7. Verify the Skyhook completes successfully
8. Uninstall the Helm chart (verifies pre-delete hook cleans up Skyhook/DeploymentPolicy resources automatically)

## Key Features Tested

- Custom deployment name support
- Toleration configuration via Helm values
- Operator deployment and scheduling with node taints
- End-to-end skyhook processing with Helm-deployed operator
- Automatic cleanup of Skyhook and DeploymentPolicy resources during helm uninstall
- Pre-delete hook tolerates node taints (can schedule on same nodes as operator)

## Files

- `chainsaw-test.yaml` - Main test configuration
- `values.yaml` - Helm values with custom configuration
- `skyhook.yaml` - Test skyhook
- `deployment-policy.yaml` - Test deployment policy
- `assert-*.yaml` - State assertions
