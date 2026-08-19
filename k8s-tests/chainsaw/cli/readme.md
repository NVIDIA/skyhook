# CLI Tests

This directory contains end-to-end tests for the `kubectl-nodewright` CLI plugin. These tests validate that all CLI commands work correctly against a real Kubernetes cluster.

## Prerequisites

The CLI tests require:

1. A running Kind cluster with the nodewright operator installed
2. Nodes labeled with `nodewright.nvidia.com/test-node=skyhooke2e`
3. The `nodewright` CLI binary built with coverage enabled

## Tests

| Test | Description |
|------|-------------|
| [lifecycle](./lifecycle/) | Pause, resume, disable, and enable commands |
| [node](./node/) | Node list, status, ignore, unignore, and reset commands |
| [package](./package/) | Package status, logs, and rerun commands |
| [reset](./reset/) | NodeWright reset command |
| [deployment-policy](./deployment-policy/) | Deployment policy batch state reset command |

## CLI Commands Tested

### Lifecycle Commands

- `nodewright pause <nodewright>` - Pauses a NodeWright from processing
- `nodewright resume <nodewright>` - Resumes a paused NodeWright
- `nodewright disable <nodewright>` - Disables a NodeWright completely
- `nodewright enable <nodewright>` - Enables a disabled NodeWright

### Node Commands

- `nodewright node list <nodewright>` - Shows nodes targeted by a NodeWright
- `nodewright node status [node]` - Shows NodeWright activity on nodes
- `nodewright node ignore <nodewright> <node>` - Excludes a node from processing
- `nodewright node unignore <nodewright> <node>` - Includes a node back in processing
- `nodewright node reset <nodewright> <node>` - Resets package state on a node

### Package Commands

- `nodewright package status <nodewright> <package>` - Shows package status across nodes
- `nodewright package logs <nodewright> <package>` - Retrieves logs from package pods
- `nodewright package rerun <nodewright> <package>` - Forces a package to re-run

### Reset Command

- `nodewright reset <nodewright>` - Resets all nodes for a NodeWright

### Deployment Policy Commands

- `nodewright deployment-policy reset <nodewright>` - Resets batch processing state for a NodeWright

## Running the Tests

```bash
# Run all CLI tests
make cli-e2e-tests

# Run a specific test
cd k8s-tests/chainsaw/cli
chainsaw test --test-dir lifecycle
```
