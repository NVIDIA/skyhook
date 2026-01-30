# CLI Tests

This directory contains end-to-end tests for the `kubectl-skyhook` CLI plugin. These tests validate that all CLI commands work correctly against a real Kubernetes cluster.

## Prerequisites

The CLI tests require:
1. A running Kind cluster with the skyhook operator installed
2. Nodes labeled with `skyhook.nvidia.com/test-node=skyhooke2e`
3. The `skyhook` CLI binary built with coverage enabled

## Tests

| Test | Description |
|------|-------------|
| [lifecycle](./lifecycle/) | Pause, resume, disable, and enable commands |
| [node](./node/) | Node list, status, ignore, unignore, and reset commands |
| [package](./package/) | Package status, logs, and rerun commands |
| [reset](./reset/) | Skyhook reset command |

## CLI Commands Tested

### Lifecycle Commands
- `skyhook pause <skyhook>` - Pauses a Skyhook from processing
- `skyhook resume <skyhook>` - Resumes a paused Skyhook
- `skyhook disable <skyhook>` - Disables a Skyhook completely
- `skyhook enable <skyhook>` - Enables a disabled Skyhook

### Node Commands
- `skyhook node list <skyhook>` - Shows nodes targeted by a Skyhook
- `skyhook node status [node]` - Shows Skyhook activity on nodes
- `skyhook node ignore <skyhook> <node>` - Excludes a node from processing
- `skyhook node unignore <skyhook> <node>` - Includes a node back in processing
- `skyhook node reset <skyhook> <node>` - Resets package state on a node

### Package Commands
- `skyhook package status <skyhook> <package>` - Shows package status across nodes
- `skyhook package logs <skyhook> <package>` - Retrieves logs from package pods
- `skyhook package rerun <skyhook> <package>` - Forces a package to re-run

### Reset Command
- `skyhook reset <skyhook>` - Resets all nodes for a Skyhook

## Running the Tests

```bash
# Run all CLI tests
make cli-e2e-tests

# Run a specific test
cd k8s-tests/chainsaw/cli
chainsaw test --test-dir lifecycle
```
