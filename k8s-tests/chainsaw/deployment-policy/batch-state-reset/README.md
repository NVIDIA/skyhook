# Batch State Reset Test

This test validates the deployment policy batch state reset functionality, which allows rollouts to start fresh instead of continuing from previous scaled-up batch sizes.

## Overview

When a deployment policy uses exponential or linear strategies, the batch size grows as the rollout progresses (e.g., 1→2→4→8 nodes). The batch state reset feature ensures that subsequent rollouts start from the initial batch size rather than continuing where the previous rollout left off.

## What This Test Validates

### 1. Auto-Reset on Completion
When a Skyhook rollout completes (Status→Complete), batch state is automatically reset if `resetBatchStateOnCompletion` is enabled.

**Expected behavior:**
- Batch state resets to initial values (CurrentBatch=1, CompletedNodes=0)
- All compartments are reset
- Configuration can enable/disable this behavior

### 2. Configuration Precedence
Tests that Skyhook-level configuration overrides DeploymentPolicy-level configuration.

**Precedence order:**
1. `Skyhook.spec.deploymentPolicyOptions.resetBatchStateOnCompletion` (highest)
2. `DeploymentPolicy.spec.resetBatchStateOnCompletion`
3. Default: `true` (safe by default)

**Test scenario:**
- DeploymentPolicy has `resetBatchStateOnCompletion: true`
- Skyhook overrides with `resetBatchStateOnCompletion: false`
- Result: Batch state is NOT reset (Skyhook override takes precedence)

## Test Architecture

The test uses a single deployment policy with multiple Skyhooks to validate different scenarios:

- **test-batch-reset-policy**: DeploymentPolicy with auto-reset enabled
- **test-auto-reset-enabled**: Skyhook that inherits policy config (tests auto-reset)
- **test-override-disabled**: Skyhook that overrides to disable reset (tests precedence)

Each scenario validates:
1. Rollout completes successfully
2. Batch state is in the expected state (reset or preserved)
3. Configuration is respected

> **Note:** Manual CLI reset (`deployment-policy reset`) is tested separately in `k8s-tests/chainsaw/cli/deployment-policy/`.

## Node Setup

The test requires 8 nodes labeled with:
- `tier: "1"` - Compartment selector
- `skyhook.nvidia.com/test-node: batch-reset-test` - Test isolation

## Test Flow

1. **Setup**: Label nodes and apply deployment policy
2. **Test Auto-Reset**: Deploy skyhook with reset enabled, verify batch state resets on completion
3. **Test Precedence**: Deploy skyhook with reset disabled (override), verify batch state preserved
4. **Cleanup**: Remove resources and node labels

## Expected Results

### Auto-Reset Enabled (Inherits Policy)
After rollout completes:
```yaml
compartmentStatuses:
  tier-1:
    batchState:
      currentBatch: 1
      consecutiveFailures: 0
      completedNodes: 0
      failedNodes: 0
      shouldStop: false
      lastBatchSize: 0
      lastBatchFailed: false
```

### Override Disabled (Precedence Test)
After rollout completes:
```yaml
compartmentStatuses:
  tier-1:
    batchState:
      currentBatch: >=1  # Preserved, not reset
      # Other fields may reflect actual rollout progress
```

## Running the Test

```bash
# From operator directory
make deployment-policy-tests

# Or run this specific test
cd k8s-tests/chainsaw/deployment-policy
chainsaw test --test-dir batch-state-reset
```

## Why This Matters

Without batch state reset:
- Subsequent rollouts would start with large batch sizes from the previous rollout
- A rollout that reached batch size 32 would start the next rollout at batch 32
- This defeats the purpose of cautious initial batches (1→2→4...)

With batch state reset:
- Every rollout starts conservatively from the initial batch size
- Provides consistent, predictable rollout behavior
- Reduces risk when deploying new package versions

## Related Features

- **Auto-reset on version change**: Batch state also resets when package versions change (tested separately)
- **Rollout strategies**: Exponential, linear, and fixed strategies all benefit from batch state reset
- **Multi-compartment policies**: Each compartment's batch state is reset independently
