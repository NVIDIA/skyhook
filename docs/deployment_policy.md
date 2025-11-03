# Deployment Policy and Compartments

Deployment Policy provides fine-grained control over how Skyhook rolls out updates across your cluster by defining **compartments** — groups of nodes selected by labels — with different rollout strategies and budgets.

---

## Overview

A **DeploymentPolicy** is a Kubernetes Custom Resource that separates rollout configuration from the Skyhook Custom Resource, allowing you to:
- Reuse the same policy across multiple Skyhooks
- Apply different strategies to different node groups (e.g., production vs. test)
- Control rollout speed and safety with configurable thresholds

---

## Basic Structure

```yaml
apiVersion: skyhook.nvidia.com/v1alpha1
kind: DeploymentPolicy
metadata:
  name: my-policy
  namespace: skyhook
spec:
  # Default applies to nodes that don't match any compartment
  default:
    budget:
      percent: 100  # or count: N
    strategy:
      fixed:        # or linear, exponential
        initialBatch: 1
        batchThreshold: 100
        failureThreshold: 3
        safetyLimit: 50

  # Compartments define specific node groups
  compartments:
  - name: production
    selector:
      matchLabels:
        env: production
    budget:
      count: 2
    strategy:
      exponential:
        initialBatch: 1
        growthFactor: 2
        batchThreshold: 100
        failureThreshold: 1
        safetyLimit: 50
```

---

## Core Concepts

### Compartments
A named group of nodes selected by labels with:
- **Selector**: Kubernetes `LabelSelector` to match nodes
- **Budget**: Maximum nodes in progress at once (count or percent)
- **Strategy**: Rollout pattern (fixed, linear, or exponential)

### Budgets
Defines the ceiling for concurrent nodes:
- **Count**: Fixed number (e.g., `count: 3`)
- **Percent**: Percentage of matched nodes (e.g., `percent: 25`)

**Rounding for Percent**: `ceiling = max(1, int(matched_nodes × percent / 100))`
- Always rounds **down**
- Minimum is **1** (unless 0 nodes match)

**Examples**:
| Matched Nodes | Percent | Ceiling |
|---------------|---------|---------|
| 10 | 25% | 2 |
| 10 | 30% | 3 |
| 5 | 10% | 1 (rounds down from 0.5, then max(1, 0)) |
| 100 | 1% | 1 |

---

## Rollout Strategies

### Fixed Strategy
Constant batch size throughout the rollout.

```yaml
strategy:
  fixed:
    initialBatch: 5      # Always process 5 nodes
    batchThreshold: 100  # Require 100% success
    failureThreshold: 3  # Stop after 3 consecutive failures
    safetyLimit: 50      # Apply failure threshold only below 50% progress
```

**Use when**: You want predictable, safe rollouts.

---

### Linear Strategy
Increases by delta on success, decreases on failure.

```yaml
strategy:
  linear:
    initialBatch: 1
    delta: 1             # Increase by 1 each success
    batchThreshold: 100
    failureThreshold: 3
    safetyLimit: 50
```

**Progression** (delta=1): `1 → 2 → 3 → 4 → 5`

**Use when**: You want gradual ramp-up with slowdown on failures.

---

### Exponential Strategy
Multiplies by growth factor on success, divides on failure.

```yaml
strategy:
  exponential:
    initialBatch: 1
    growthFactor: 2      # Double on success
    batchThreshold: 100
    failureThreshold: 2
    safetyLimit: 50
```

**Progression** (factor=2): `1 → 2 → 4 → 8 → 16`

**Use when**: You want fast rollouts in large clusters with high confidence.

---

## Strategy Parameters

All strategies share these parameters:

- **`initialBatch`** (≥1): Starting number of nodes (default: 1)
- **`batchThreshold`** (1-100): Minimum success percentage to continue (default: 100)
- **`failureThreshold`** (≥1): Max consecutive failures before stopping (default: 3)
- **`safetyLimit`** (1-100): Progress threshold for failure handling (default: 50)

### Safety Limit Behavior

**Before safetyLimit** (e.g., < 50% progress):
- Failures count toward `failureThreshold`
- Batch sizes slow down (linear/exponential)
- Reaching `failureThreshold` stops the rollout

**After safetyLimit** (e.g., ≥ 50% progress):
- Rollout continues despite failures
- Batch sizes don't slow down
- Assumes rollout is "safe enough" to complete

**Rationale**: Early failures indicate a problem. Late failures are less critical since most nodes are updated.

---

## Selectors and Node Matching

Compartments use standard Kubernetes label selectors:

### Match Labels
```yaml
selector:
  matchLabels:
    env: production
    tier: frontend
```

---

## Overlapping Selectors

When a node matches **multiple compartments**, the operator uses a **safety heuristic** to choose the safest one.

### Tie-Breaking Algorithm (3 levels)

1. **Strategy Safety**: Prefer safer strategies
   - **Fixed** (safest) > **Linear** > **Exponential** (least safe)

2. **Effective Ceiling**: If strategies are the same, prefer smaller ceiling
   - Smaller ceiling = fewer nodes at risk

3. **Lexicographic**: If still tied, alphabetically by compartment name
   - Ensures deterministic behavior

### Example

```yaml
compartments:
- name: us-west
  selector:
    matchLabels:
      region: us-west
  budget:
    count: 5          # Ceiling = 5
  strategy:
    exponential: {}

- name: production
  selector:
    matchLabels:
      env: production
  budget:
    count: 2          # Ceiling = 2 (smaller)
  strategy:
    exponential: {}

- name: critical
  selector:
    matchLabels:
      priority: critical
  budget:
    count: 1
  strategy:
    fixed: {}         # Fixed (safest)
```

**Node with labels** `region=us-west, env=production, priority=critical`:
- Matches all three compartments
- **Winner**: `critical` (fixed strategy is safest)

**Node with labels** `region=us-west, env=production`:
- Matches `us-west` and `production`
- Both use exponential strategy
- **Winner**: `production` (ceiling=2 is smaller than ceiling=5)

---

## Using with Skyhooks

Reference a policy by name:

```yaml
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  name: my-skyhook
spec:
  deploymentPolicy: my-policy  # References DeploymentPolicy
  nodeSelectors:
    matchLabels:
      workload: gpu
  packages:
    # ...
```

**Behavior**:
- DeploymentPolicy must be in the **same namespace**
- Each node is assigned to a compartment based on selectors
- Nodes not matching any compartment use the `default` settings

---

## Migration from InterruptionBudget

The legacy `interruptionBudget` field is still supported but **DeploymentPolicy is recommended**.

### Before
```yaml
spec:
  interruptionBudget:
    percent: 25
```

### After
```yaml
# 1. Create DeploymentPolicy
apiVersion: skyhook.nvidia.com/v1alpha1
kind: DeploymentPolicy
metadata:
  name: legacy-equivalent
  namespace: skyhook
spec:
  default:
    budget:
      percent: 25
    strategy:
      fixed:
        initialBatch: 1
        batchThreshold: 100
        failureThreshold: 3
        safetyLimit: 50
```

```yaml
# 2. Update Skyhook
spec:
  deploymentPolicy: legacy-equivalent
  # Remove interruptionBudget field
```

---

## Monitoring

Deployment Policy rollout behavior is exposed via Prometheus metrics. See [Metrics documentation](metrics/README.md) for details.

---

## Examples

See `/operator/config/samples/deploymentpolicy_v1alpha1_deploymentpolicy.yaml` for a complete sample showing:
- Critical nodes (count=1, fixed strategy)
- Production nodes (count=3, linear strategy)
- Staging nodes (percent=33, exponential strategy)
- Test nodes (percent=50, fast exponential)

---

