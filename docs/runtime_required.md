# What it is

Runtime required is a special mode that packages can be run in. This mode is for when a set of Packages must complete before any other workloads are allowed to run on the node.

## How to use it

## Pre-requisites

1. The node MUST carry the runtime-required taint. There are two ways to get there, and only the first requires the node to join already tainted:
    1. **Pre-taint at provisioning (recommended).** The node joins the cluster already carrying the taint, via the kubelet `--register-with-taints` flag or your provisioner. Nothing can schedule on the node before the taint exists.
    1. **`autoTaintNewNodes: true` (fallback).** The operator applies the taint itself to new nodes matching the NodeWright's selector, so the node does **not** have to join with it. Use this when you cannot control tainting at provisioning time, and note the race window described in [Auto-tainting new nodes](#auto-tainting-new-nodes).
1. The operator MUST recognise that taint. It recognises two:
    1. the taint set as the chart value `controllerManager.manager.env.runtimeRequiredTaint`, which defaults to `nodewright.nvidia.com=runtime-required:NoSchedule`
    1. for the deprecation window only, the legacy `skyhook.nvidia.com=runtime-required:NoSchedule` taint, whatever the chart value is

Recognition matters for both paths: a taint the operator does not recognise is never removed, so the node stays unschedulable. `autoTaintNewNodes` applies only the configured taint, never the legacy one.

So during the deprecation window the node taint and the chart value do **not** have to match: a cluster whose nodes still join with the legacy key keeps working without touching the chart, and a cluster on the new key works out of the box. You only need to set `runtimeRequiredTaint` explicitly if your nodes join with some **other** key. See [Taint key rename](#taint-key-rename-skyhooknvidiacom---nodewrightnvidiacom) for what to migrate and by when.

## Taint key rename: `skyhook.nvidia.com` -> `nodewright.nvidia.com`

The default runtime-required taint key moved from `skyhook.nvidia.com` to `nodewright.nvidia.com` as part of the Skyhook -> NodeWright rename. **This is a coordinated change, not a cosmetic default bump**: the taint key is named by infrastructure the operator cannot see, and a node carrying a key the operator does **not** recognise is never untainted, so it sits unschedulable. The deprecation window below exists precisely so that moving the default does not create that situation: during the window the legacy key is still recognised, so it is not a "wrong" key.

During the deprecation window the operator:

- **applies** only the configured taint (`runtimeRequiredTaint`, defaulting to the `nodewright.nvidia.com` key)
- **tolerates** both the configured taint and the legacy `skyhook.nvidia.com=runtime-required:NoSchedule` taint, so package pods still schedule onto legacy-tainted nodes
- **removes** both from a node once every `runtimeRequired: true` NodeWright targeting it has completed on it
- treats a node that already carries either taint as already gated, so `autoTaintNewNodes` does not stamp a second taint on it

That means a cluster whose provisioner still applies the legacy key keeps working with no change, and a cluster on the new key works immediately.

### What you need to update

Anywhere outside the operator that names the taint key:

- **Cluster autoscaler / Karpenter node pool definitions** and any `--register-with-taints` kubelet arguments, so new nodes come up carrying the new key
- **Machine or node templates** in your provisioning stack
- **Tolerations on your own workloads** that were written against the legacy key

If you cannot change your provisioning config yet, pin the old behavior explicitly instead of relying on the deprecation window:

```yaml
controllerManager:
  manager:
    env:
      runtimeRequiredTaint: skyhook.nvidia.com=runtime-required:NoSchedule
```

### Deprecation window

Legacy-key toleration and removal ship as a transition-only shim and are removed together with the legacy `skyhook.nvidia.com` API group in **operator v0.20.0** (see [nodewright-migration.md](nodewright-migration.md#removal-operator-v0200)). The rename ships in v0.18.0, so you have the v0.18.x and v0.19.x lines to migrate.

**Migrate your provisioning config before v0.20.0.** From that release on, a node carrying only the legacy taint is never untainted by the operator and package pods do not tolerate it, so such nodes stay unschedulable.

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

**NOTE:** No additional toleration is required; NodeWright automatically tolerates the configured (`runtimeRequiredTaint`) taint, and for the deprecation window the legacy `skyhook.nvidia.com` taint as well.
