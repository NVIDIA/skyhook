# Node Affinity Test

This test validates the node affinity configuration feature for the Skyhook Operator Helm chart.

## Test Overview

The test demonstrates that the `controllerManager.nodeAffinity.matchExpressions` configuration in the Helm chart works correctly by:

1. **Phase 1**: Installing the operator with node affinity expressions that target non-existent labels
   - Uses `values-no-match.yaml` with expressions targeting `skyhook.nvidia.com/test-node=fooboar` 
   - Verifies that pods remain in `Pending` state and cannot be scheduled
   - Validates the affinity expressions are correctly applied to the pod spec

2. **Phase 2**: Adding the required labels to nodes and updating the configuration
   - Adds `skyhook.nvidia.com/test-node=skyhooke2e` label to all nodes
   - Updates the deployment to use `values-match.yaml` with expressions targeting the existing label
   - Verifies that pods are now scheduled and running successfully

## Files

- `chainsaw-test.yaml` - The main test configuration
- `values-no-match.yaml` - Helm values with node affinity targeting non-existent labels
- `values-match.yaml` - Helm values with node affinity targeting existing labels
- `assert-no-schedule.yaml` - Assertion to verify pods are not scheduled
- `assert-scheduled.yaml` - Assertion to verify pods are scheduled and running

## Node Affinity Configuration

The test uses the following node affinity expressions:

```yaml
controllerManager:
  nodeAffinity:
    matchExpressions:
    - key: node-role.kubernetes.io/control-plane
      operator: DoesNotExist
    - key: skyhook.nvidia.com/test-node
      operator: In
      values:
      - skyhooke2e
```

## Running the Test

This test is designed to be run with Chainsaw in a Kind cluster. It will:

1. Create a Kind cluster (if not already present)
2. Run the test scenarios
3. Clean up labels and resources

The test validates that the Helm chart correctly translates the `nodeAffinity.matchExpressions` configuration into proper Kubernetes node affinity rules in the deployment template.

## Handling Selectors vs NodeAffinity

The Helm chart enforces a clear separation between simple selectors and advanced node affinity:

### Validation Behavior
- **Cannot use both** `selectors` and `nodeAffinity.matchExpressions` together
- The chart will fail with an error if both are defined
- This prevents conflicting or confusing node selection rules

### Error Message
```
Error: Cannot specify both controllerManager.selectors and controllerManager.nodeAffinity.matchExpressions. 
Use nodeAffinity.matchExpressions for complex node selection or selectors for simple key-value matching.
```

### Examples

**Simple selector (uses nodeSelector):**
```yaml
controllerManager:
  selectors:
    dedicated: system-workload
```

**Advanced node affinity (uses nodeAffinity):**
```yaml
controllerManager:
  nodeAffinity:
    matchExpressions:
    - key: node-role.kubernetes.io/control-plane
      operator: DoesNotExist
    - key: skyhook.nvidia.com/test-node
      operator: In
      values:
      - skyhooke2e
```

**Invalid (will cause error):**
```yaml
controllerManager:
  selectors:
    dedicated: system-workload
  nodeAffinity:
    matchExpressions:
    - key: node-role.kubernetes.io/control-plane
      operator: DoesNotExist
```

### Usage Recommendations
- Use `selectors` for simple key-value node selection
- Use `nodeAffinity.matchExpressions` for complex node selection with operators like `In`, `NotIn`, `Exists`, `DoesNotExist`
- Choose one approach - they cannot be mixed 