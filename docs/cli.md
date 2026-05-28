# Skyhook CLI

kubectl plugin for managing Skyhook deployments, packages, and nodes.

## Overview

The Skyhook CLI (`kubectl skyhook`) provides SRE tooling for managing Skyhook operators and their packages across Kubernetes cluster nodes. It supports inspecting node/package state, forcing re-runs, managing node lifecycle, and retrieving logs.

## Compatibility

### Minimum Operator Version

The CLI requires **operator version v0.8.0 or later** for full functionality of all commands.

### Command Compatibility Matrix

| Command | v0.7.x and earlier | v0.8.0+ |
|---------|-------------------|---------|
| `version` | ✅ Full | ✅ Full |
| `node status` | ✅ Full | ✅ Full |
| `node list` | ✅ Full | ✅ Full |
| `node reset` | ✅ Full | ✅ Full |
| `node ignore/unignore` | ✅ Full | ✅ Full |
| `package status` | ✅ Full | ✅ Full |
| `package rerun` | ✅ Full | ✅ Full |
| `package logs` | ✅ Full | ✅ Full |
| `reset` | ✅ Full | ✅ Full |
| `reset --package` | ✅ Full (v0.7.5+) | ✅ Full |
| `update-state` | ✅ Full (v0.7.5+) — see note | ✅ Full — see note |
| `deployment-policy reset` | ❌ Not supported | ✅ Full |
| `pause` | ❌ Not supported | ✅ Full |
| `resume` | ❌ Not supported | ✅ Full |
| `disable` | ❌ Not supported | ✅ Full |
| `enable` | ❌ Not supported | ✅ Full |

> **Note on `update-state` and `reset --package`:** These commands edit the
> `skyhook.nvidia.com/nodeState_<skyhook>` annotation in-place. The
> annotation's `map[string]PackageStatus` shape has been stable since
> **operator v0.7.5**, and the CLI refuses to run against anything older.
> What *has* evolved across operator releases is the set of recognised
> stage and state values — for example `uninstall` and `uninstall-interrupt`
> were added in v0.16.0. Picking a stage/state your operator doesn't
> recognise will leave the package in a state the operator can't progress
> from. Confirm the stage/state values are valid for your operator before
> running these commands.

### Breaking Change: Pause/Disable Mechanism

In operator versions **v0.7.x and earlier**, pausing and disabling a Skyhook was done via spec fields:

```yaml
spec:
  pause: true  # Old method - no longer used by operator
```

Starting with **v0.8.0**, the operator uses **annotations** instead:

```yaml
metadata:
  annotations:
    skyhook.nvidia.com/pause: "true"
    skyhook.nvidia.com/disable: "true"
```

The CLI's `pause`, `resume`, `disable`, and `enable` commands set these annotations. If you're running an older operator (v0.7.x or earlier), these commands will appear to succeed but the operator won't recognize the annotations - you'll need to edit the Skyhook spec directly using `kubectl edit`.

## Installation

```bash
# Build from source
make build-cli

# Install as kubectl plugin
cp bin/kubectl-skyhook /usr/local/bin/

# Verify installation
kubectl skyhook version
```

## Usage Structure

### Basic Command Structure

```
kubectl skyhook [global-flags] <command> [subcommand] [flags] [arguments]
```

### Global Flags

- `-h, --help` - Show help for any command
- `--version` - Show version information
- `-n, --namespace` - Kubernetes namespace (default: "skyhook")
- `-o, --output` - Output format: table|json|yaml|wide
- `-v, --verbose` - Enable verbose output
- `--dry-run` - Preview changes without applying them
- `--kubeconfig` - Path to kubeconfig file

## Commands

### Version Command

Show plugin and operator versions.

```bash
# Show both plugin and operator versions
kubectl skyhook version

# Show only plugin version (no cluster query)
kubectl skyhook version --client-only

# With custom timeout
kubectl skyhook version --timeout 10s
```

### Pause/Resume Commands

Control Skyhook processing state.

> **Note:** Requires operator v0.8.0+. See [Compatibility](#compatibility) for details.

```bash
# Pause a Skyhook (stops processing new nodes)
kubectl skyhook pause my-skyhook
kubectl skyhook pause my-skyhook --confirm  # Skip confirmation

# Resume a paused Skyhook
kubectl skyhook resume my-skyhook
```

### Disable/Enable Commands

Completely disable or re-enable a Skyhook.

> **Note:** Requires operator v0.8.0+. See [Compatibility](#compatibility) for details.

```bash
# Disable a Skyhook completely
kubectl skyhook disable my-skyhook
kubectl skyhook disable my-skyhook --confirm

# Re-enable a disabled Skyhook
kubectl skyhook enable my-skyhook
```

### Reset Command

Reset all package state for a Skyhook, causing re-execution from the beginning.

```bash
# Reset all nodes for a Skyhook (also resets batch state by default)
kubectl skyhook reset gpu-init --confirm

# Preview changes without applying (dry-run)
kubectl skyhook reset gpu-init --dry-run

# Reset nodes only, preserve deployment policy batch state
kubectl skyhook reset gpu-init --skip-batch-reset --confirm

# Reset only a single package across all tracked nodes
kubectl skyhook reset gpu-init --package pkg1:1.0 --confirm --skip-batch-reset
```

| Flag | Description |
|------|-------------|
| `--confirm, -y` | Skip confirmation prompt |
| `--skip-batch-reset` | Skip resetting deployment policy batch state |
| `--package <name>[:<version>]` | Reset only this package's state on each node |

> **Note:** By default, `reset` also resets the deployment policy batch state so the next rollout starts from batch 1, and clears node ordering state (`NodeOrderOffset` and `NodePriority`) so `SKYHOOK_NODE_ORDER` restarts from `0`. Use `--skip-batch-reset` to preserve the existing batch and ordering state.

#### `--package <name>[:<version>]`

When `--package` is set, `reset` removes only the named package's entry from
each node's `nodeState` annotation instead of removing the whole annotation:

- If `<version>` is supplied (e.g. `pkg1:1.0`), only entries whose recorded
  version matches are removed; nodes with the package at a different version
  are left untouched and reported in the command output.
- If the package was the last entry in the annotation, the entire annotation
  is removed (matching the behavior of a full reset).
- Batch state is **deliberately not reset** on this path regardless of
  `--skip-batch-reset`: restarting the full rollout from batch 1 to recover a
  single package would be disproportionate. Pair with
  `kubectl skyhook deployment-policy reset` explicitly if you also want to
  restart batches.

### Update-State Command

Edit the recorded state of a single package on Skyhook-managed nodes.
`update-state` performs a surgical edit to the per-node
`skyhook.nvidia.com/nodeState_<skyhook>` annotation — replacing (or, with
`--add`, inserting) one entry in the `map[string]PackageStatus` value. It is
an **administrator escape hatch** for recovering from stuck rollouts and
deliberately does *not* validate that the requested `(stage, state)`
combination is one the operator could legally produce, nor does it gate
destructive stages (`uninstall`, `uninstall-interrupt`) behind extra
prompts.

```bash
kubectl skyhook update-state <skyhook-name> <package> <version> <stage> <state>
```

> **⚠️ Pause the Skyhook before running `update-state`.**
>
> `update-state` performs a read-modify-write on the node-state annotation and
> uses a merge patch with no resource-version check. If the operator
> reconciles the node between the CLI's read and write, the operator can
> immediately overwrite the manual edit — at best wasting the operation, at
> worst racing the operator into an inconsistent state. Always pause the
> Skyhook (`kubectl skyhook pause <name> --confirm`) before running this
> command, and resume only when finished.

```bash
# Mark pkg1@1.0 as complete on every node that already tracks this Skyhook
kubectl skyhook update-state gpu-init pkg1 1.0 config complete --confirm

# Same, but only on one node
kubectl skyhook update-state gpu-init pkg1 1.0 config complete --node worker-1 --confirm

# Narrow to a label-selected set of nodes
kubectl skyhook update-state gpu-init pkg1 1.0 interrupt in_progress -l role=gpu --confirm

# Preview the changes without writing them
kubectl skyhook update-state gpu-init pkg1 1.0 config erroring --dry-run
```

| Flag | Description |
|------|-------------|
| `--node` | Limit the update to specific node(s); repeat for multiple |
| `--selector, -l` | Limit the update to nodes matching a label selector |
| `--confirm, -y` | Skip confirmation prompt |
| `--add` | Create a fresh `nodeState` entry on nodes that do not yet have one for this package; requires `--node` or `--selector` |

The global `--dry-run` flag is honored: the command prints the set of nodes
it would patch and exits without writing.

By default `update-state` targets only nodes that already have a `nodeState`
entry for the named Skyhook. If `--node` names a node that does not exist
or has no state for the Skyhook, the command warns and skips that node
rather than failing.

#### `--add`

`--add` creates a fresh `nodeState` entry on nodes that do not yet have one
for the given `<package>@<version>` — useful for bootstrapping state on a
node the operator has not visited yet, or for re-creating an entry that was
manually deleted.

`--add` **requires** either `--node` or `--selector` so the scope of the
creation is explicit. Without one of these flags, `--add` would apply to
every node in the cluster that matches the Skyhook's selector, which is
far too broad for a creation operation — the CLI rejects this combination
with an error.

If a targeted node already has an entry for `<package>@<version>`, `--add`
warns and skips that node (use `update-state` without `--add` if you intend
to overwrite the existing entry).

### Deployment Policy Commands

Manage deployment policy batch state.

> **Note:** Requires operator v0.8.0+.

```bash
# Reset batch state for a Skyhook (starts rollout from batch 1)
kubectl skyhook deployment-policy reset gpu-init --confirm

# Preview what would be reset (dry-run)
kubectl skyhook deployment-policy reset gpu-init --dry-run

# Using the short alias
kubectl skyhook dp reset gpu-init --confirm
```

The `deployment-policy reset` command resets the batch processing state for all compartments in the specified Skyhook, including:

- Current batch number (reset to 1)
- Consecutive failure count
- Completed and failed node counts
- Stop flag
- Node ordering state (`NodeOrderOffset` and `NodePriority`) — `SKYHOOK_NODE_ORDER` restarts from `0`

| Flag | Description |
|------|-------------|
| `--confirm, -y` | Skip confirmation prompt |

**When to use**:

- After a rollout completes and you want to start a new rollout fresh
- When batch processing is stuck and needs to be reset
- Before re-running a rollout with the same deployment policy

See [Deployment Policy documentation](deployment_policy.md) for details on auto-reset configuration.

### Node Commands

Manage Skyhook nodes across the cluster.

```bash
# List all nodes targeted by a Skyhook
kubectl skyhook node list --skyhook my-skyhook
kubectl skyhook node list --skyhook my-skyhook -o json

# Show all Skyhook activity on specific node(s)
kubectl skyhook node status worker-1
kubectl skyhook node status worker-1 worker-2
kubectl skyhook node status "worker-.*"  # Regex pattern
kubectl skyhook node status worker-1 --skyhook my-skyhook  # Filter by Skyhook

# Reset all package state on node(s)
kubectl skyhook node reset worker-1 --skyhook my-skyhook --confirm
kubectl skyhook node reset "node-.*" --skyhook my-skyhook --dry-run

# Ignore/unignore nodes from processing
kubectl skyhook node ignore worker-1
kubectl skyhook node ignore "test-node-.*"
kubectl skyhook node unignore worker-1
```

#### Node Flags

| Command | Flag | Description |
|---------|------|-------------|
| `list` | `--skyhook` | Skyhook name (required) |
| `status` | `--skyhook` | Filter by Skyhook name |
| `reset` | `--skyhook` | Skyhook name (required) |
| `reset` | `--confirm, -y` | Skip confirmation prompt |

### Package Commands

Manage Skyhook packages.

```bash
# Query package status across nodes
kubectl skyhook package status my-package --skyhook my-skyhook
kubectl skyhook package status my-package --skyhook my-skyhook --node worker-1
kubectl skyhook package status my-package --skyhook my-skyhook -o wide

# Force package re-run on specific nodes
kubectl skyhook package rerun my-package --skyhook my-skyhook --node worker-1
kubectl skyhook package rerun my-package --skyhook my-skyhook --node "worker-.*" --confirm
kubectl skyhook package rerun my-package --skyhook my-skyhook --node worker-1 --stage config

# Get package logs
kubectl skyhook package logs my-package --skyhook my-skyhook
kubectl skyhook package logs my-package --skyhook my-skyhook --node worker-1
kubectl skyhook package logs my-package --skyhook my-skyhook --stage apply
kubectl skyhook package logs my-package --skyhook my-skyhook -f  # Follow
kubectl skyhook package logs my-package --skyhook my-skyhook --tail 100
```

#### Package Flags

| Command | Flag | Description |
|---------|------|-------------|
| `status` | `--skyhook` | Skyhook name (required) |
| `status` | `--node` | Filter by node pattern (repeatable) |
| `rerun` | `--skyhook` | Skyhook name (required) |
| `rerun` | `--node` | Node pattern (required, repeatable) |
| `rerun` | `--stage` | Re-run from stage: apply, config, interrupt, post-interrupt |
| `rerun` | `--confirm, -y` | Skip confirmation prompt |
| `logs` | `--skyhook` | Skyhook name (required) |
| `logs` | `--node` | Filter by node name |
| `logs` | `--stage` | Filter by stage |
| `logs` | `-f, --follow` | Follow log output |
| `logs` | `--tail` | Lines from end (-1 for all) |

## Help System

```bash
# General help
kubectl skyhook --help

# Command group help
kubectl skyhook node --help
kubectl skyhook package --help
kubectl skyhook deployment-policy --help

# Specific command help
kubectl skyhook node reset --help
kubectl skyhook package rerun --help
kubectl skyhook deployment-policy reset --help
```

## Common Usage Patterns

### Debugging a Failed Package

```bash
# 1. Check package status
kubectl skyhook package status my-package --skyhook my-skyhook -o wide

# 2. View logs for the failed package
kubectl skyhook package logs my-package --skyhook my-skyhook --node worker-1

# 3. Fix the issue, then force re-run
kubectl skyhook package rerun my-package --skyhook my-skyhook --node worker-1 --confirm
```

### Node Maintenance

```bash
# 1. Ignore node before maintenance
kubectl skyhook node ignore worker-1

# 2. Perform maintenance...

# 3. Unignore and reset to re-run all packages
kubectl skyhook node unignore worker-1
kubectl skyhook node reset worker-1 --skyhook my-skyhook --confirm
```

### Cluster-Wide Status Check

```bash
# List all nodes for a Skyhook
kubectl skyhook node list --skyhook my-skyhook

# Check status of all nodes
kubectl skyhook node status

# Check specific Skyhook across all nodes
kubectl skyhook node status --skyhook my-skyhook -o json
```

### Resetting a Rollout

```bash
# 1. Full reset: nodes + batch state (starts everything fresh)
kubectl skyhook reset my-skyhook --confirm

# 2. Or reset only batch state (keep node state, restart batch progression)
kubectl skyhook deployment-policy reset my-skyhook --confirm

# 3. Or reset only nodes (keep batch progression)
kubectl skyhook reset my-skyhook --skip-batch-reset --confirm
```

### Surgical Recovery

When a single package on a single node is wedged and a full reset is too
disruptive, pause the Skyhook, edit the recorded state directly, then
resume:

```bash
# 1. Pause so the operator doesn't clobber the manual edit
kubectl skyhook pause my-skyhook --confirm

# 2. Mark the wedged package as complete (or set whatever stage/state
#    you need to unblock the rollout)
kubectl skyhook update-state my-skyhook my-package 1.0 config complete \
    --node worker-1 --confirm

# 3. Resume processing
kubectl skyhook resume my-skyhook --confirm
```

For a single-package reset across all nodes (without disturbing the
deployment policy batch state), prefer `reset --package`:

```bash
kubectl skyhook reset my-skyhook --package my-package:1.0 --confirm
```

### Emergency Stop

> **Note:** Requires operator v0.8.0+. For older operators, use `kubectl edit skyhook my-skyhook` and set `spec.pause: true`.

```bash
# Pause all processing
kubectl skyhook pause my-skyhook --confirm

# Or disable completely
kubectl skyhook disable my-skyhook --confirm
```

## Output Formats

All status commands support multiple output formats:

```bash
# Table (default) - human-readable
kubectl skyhook node list --skyhook my-skyhook

# Wide - table with additional columns
kubectl skyhook node list --skyhook my-skyhook -o wide

# JSON - machine-readable
kubectl skyhook node list --skyhook my-skyhook -o json

# YAML - machine-readable
kubectl skyhook node list --skyhook my-skyhook -o yaml
```

## Architecture

### Package Structure

```
operator/cmd/cli/app/           # CLI commands
├── cli.go                      # Root command (NewSkyhookCommand)
├── version.go                  # Version command
├── reset.go                    # Reset command (nodes + batch state)
├── lifecycle.go                # Pause, resume, disable, enable commands
├── deploymentpolicy/           # Deployment policy subcommands
│   ├── deploymentpolicy.go     # Parent command
│   └── deploymentpolicy_reset.go  # Batch state reset
├── node/                       # Node subcommands
│   ├── node.go                 # Parent command
│   ├── node_list.go
│   ├── node_status.go
│   ├── node_reset.go
│   └── node_ignore.go          # Ignore and unignore commands
└── package/                    # Package subcommands
    ├── package.go              # Parent command
    ├── package_status.go
    ├── package_rerun.go
    └── package_logs.go

operator/internal/cli/          # Shared CLI utilities
├── client/                     # Kubernetes client wrapper
├── context/                    # CLI context and global flags
└── utils/                      # Shared utilities
```

### Command Creation Flow

```
main()
  └── cli.Execute()
      └── NewSkyhookCommand(ctx)
          ├── NewVersionCmd(ctx)
          ├── NewResetCmd(ctx)
          ├── NewPauseCmd(ctx)
          ├── NewResumeCmd(ctx)
          ├── NewDisableCmd(ctx)
          ├── NewEnableCmd(ctx)
          ├── deploymentpolicy.NewDeploymentPolicyCmd(ctx)
          │   └── NewResetCmd(ctx)
          ├── node.NewNodeCmd(ctx)
          │   ├── NewListCmd(ctx)
          │   ├── NewStatusCmd(ctx)
          │   ├── NewResetCmd(ctx)
          │   ├── NewIgnoreCmd(ctx)
          │   └── NewUnignoreCmd(ctx)
          └── pkg.NewPackageCmd(ctx)
              ├── NewStatusCmd(ctx)
              ├── NewRerunCmd(ctx)
              └── NewLogsCmd(ctx)
```

### Testing

```bash
# Run CLI tests
make test-cli

# Run all tests
make test
```
