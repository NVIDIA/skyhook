# CLI Deployment Policy Reset Test

Tests the `deployment-policy reset` CLI command for manually resetting batch processing state.

## What This Test Validates

1. A NodeWright with a deployment policy completes rollout with batch state preserved (`currentBatch > 1`)
2. Running `nodewright deployment-policy reset` resets batch state to initial values (`currentBatch: 1`)

## Test Flow

1. **Setup**: Label 2 worker nodes, apply a DeploymentPolicy with exponential strategy and a NodeWright with `resetBatchStateOnCompletion: false` override
2. **Wait**: Rollout completes with batch state preserved (not auto-reset, `currentBatch > 1`)
3. **Reset**: Run `deployment-policy reset` via CLI
4. **Assert**: Batch state is reset (`currentBatch: 1`, counters zeroed)
5. **Cleanup**: Delete resources

## Running

```bash
cd k8s-tests/chainsaw/cli
chainsaw test --test-dir deployment-policy
```
