# Deployment Policy Tests

This directory contains end-to-end tests for the Skyhook deployment policy feature. These tests validate rollout strategies, compartment management, and budget enforcement for controlled node deployments.

## Test Cluster Requirements

These tests require a **15-node Kind cluster** to properly validate deployment policies at scale. The cluster configuration is defined in `kind-config.yaml`.

To create the test cluster:
```bash
make create-deployment-policy-cluster
```

## Tests

| Test | Description |
|------|-------------|
| [batch-state-reset](./batch-state-reset/) | Auto-reset on completion and config precedence (CLI reset tested in `cli/deployment-policy/`) |
| [legacy-compatibility](./legacy-compatibility/) | Backwards compatibility with legacy `interruptionBudget` |
| [linear-strategy](./linear-strategy/) | Linear ramp-up rollout strategy with incremental batch growth |
| [multi-compartment](./multi-compartment/) | Multiple compartments with exponential strategy |
| [overlapping-selectors](./overlapping-selectors/) | Compartment selector precedence and node assignment |

## Key Concepts

### Deployment Policy
A custom resource that defines how nodes should be grouped into compartments and how packages should be rolled out to them.

### Compartments
Logical groups of nodes with their own rollout strategy and budget. Nodes are assigned to compartments based on label selectors.

### Rollout Strategies
- **Fixed**: Process a fixed number of nodes per batch
- **Exponential**: Double the batch size with each iteration (1, 2, 4, 8...)
- **Linear**: Increase batch size linearly (delta-based growth)

## Running the Tests

```bash
# Run all deployment policy tests
make deployment-policy-tests

# Run a specific test
cd k8s-tests/chainsaw/deployment-policy
chainsaw test --test-dir linear-strategy
```

## Helper Scripts

- `label-nodes.sh` - Labels nodes with compartment assignments for testing
- `kind-config.yaml` - Kind cluster configuration with 15 worker nodes
