# What it is

Runtime required is a special mode that packages can be run in. This mode is for when a set of Packages must complete before any other workloads are allowed to run on the node.

# How to use it

## Pre-requisites
1. A node MUST join the cluster with a pre define taint
1. That same taint must be set as the chart value `controllerManager.manager.env.runtimeRequiredTaint`
    1. The default value for this taint is `skyhook.nvidia.com=runtime-required:NoSchedule`

## Required Skyhooks

Once the pre-requisites are satisfied any Skyhook Custom Resource (SCR) may be marked with `runtimeRequired: true`. This flag indicates that all packages within this SCR must complete
before the nodes that it targets are considered available for general use.

## What runtimeRequired: true will NOT do
1. It will NOT add the taint to any nodes targeted by a SCR with `runtimeRequired: true`

# Details
## When is the runtime-required taint removed from a node
The taint is removed from a node when all SCRs with `runtimeRequired: true` that target that node are complete **on that specific node**.

**Important**: Taint removal is per-node, not per-skyhook. This means:
- Node A's taint is removed when all runtime-required skyhooks complete on Node A
- Node A does NOT wait for Node B to complete those same skyhooks
- If Node B is stuck or failing, Node A can still have its taint removed and become available

This per-node behavior prevents deadlocks where a few bad nodes would block all other healthy nodes from becoming available.

## What happens when the taint is removed
1. The node becomes available for general workload scheduling (pods without the runtime-required toleration can now be scheduled on it).

# Why would you use runtime required
This is useful when you want to gate other work behind the successful completion of some set of Skyhook Packages. This can be for security reasons or for scheduling.

**NOTE:** No additional toleration is required, skyhook auto tolerates this (env:`runtimeRequiredTaint`) taint.