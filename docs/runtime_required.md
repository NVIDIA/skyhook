# What it is

Runtime required is a special mode that packages can be run in. This mode is for when a set of Packages must complete before any other workloads are allowed to run on the node.

## How to use it

## Pre-requisites

1. A node MUST join the cluster with a pre define taint
1. That same taint must be set as the chart value `controllerManager.manager.env.runtimeRequiredTaint`
    1. The default value for this taint is `skyhook.nvidia.com=runtime-required:NoSchedule`

## Required NodeWrights

Once the pre-requisites are satisfied any NodeWright Custom Resource (CR) may be marked with `runtimeRequired: true`. This flag indicates that all packages within this CR must complete
before the nodes that it targets are considered available for general use.

## Auto-tainting new nodes

**Recommended:** The preferred approach is to taint nodes as they are added to the cluster (e.g., via your infrastructure provisioner or node bootstrap configuration). Pre-tainting eliminates any race condition between a node becoming schedulable and the operator applying the taint.

If you cannot control node tainting at provisioning time, `autoTaintNewNodes` provides a fallback. Note that there is a small window between when a node joins the cluster and when the operator's reconcile loop applies the taint, during which workloads could theoretically be scheduled on the node.

To enable, set `autoTaintNewNodes: true` alongside `runtimeRequired: true` on the NodeWright CR:

```yaml
spec:
  runtimeRequired: true
  autoTaintNewNodes: true
```

When enabled, the operator automatically applies the runtime-required taint to nodes that:

1. Match the NodeWright's node selector
2. Do not already have the runtime-required taint
3. Have no `nodewright.nvidia.com/*` annotations (i.e., have never been touched by the NodeWright operator)

A node is considered "new" if it has no NodeWright annotations. This works for both initial cluster setup (day 0) and nodes joining an existing cluster (day 2+). Nodes that have already been processed by NodeWright (and had their taint removed after completion) will not be re-tainted because they retain their NodeWright annotations.

**Exception: reboot with `REAPPLY_ON_REBOOT=true`.** When the operator is configured with `REAPPLY_ON_REBOOT=true` and a NodeWright has both `runtimeRequired: true` and `autoTaintNewNodes: true`, a node whose boot ID changes is treated as new for taint purposes. The runtime-required taint is re-applied alongside the state reset in the same atomic operation, ensuring no workloads can schedule on the rebooted node before NodeWright finishes re-applying. The taint is removed again by the normal completion path once all runtime-required NodeWrights finish on that node.

## What runtimeRequired: true will NOT do

1. Without `autoTaintNewNodes: true`, it will NOT add the taint to any nodes targeted by a CR with `runtimeRequired: true`

## Details

## When is the runtime-required taint removed from a node

The taint is removed from a node when all CRs with `runtimeRequired: true` that target that node are complete **on that specific node**.

**Important**: Taint removal is per-node, not per-NodeWright. This means:

- Node A's taint is removed when all runtime-required NodeWrights complete on Node A
- Node A does NOT wait for Node B to complete those same NodeWrights
- If Node B is stuck or failing, Node A can still have its taint removed and become available

This per-node behavior prevents deadlocks where a few bad nodes would block all other healthy nodes from becoming available.

## What happens when the taint is removed

1. The node becomes available for general workload scheduling (pods without the runtime-required toleration can now be scheduled on it).

## Why would you use runtime required

This is useful when you want to gate other work behind the successful completion of some set of NodeWright Packages. This can be for security reasons or for scheduling.

**NOTE:** No additional toleration is required; NodeWright automatically tolerates this (`runtimeRequiredTaint`) taint.
