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
## When is a node considered ready
When all of the following is true per node:
1. All SCRs with `runtimeRequired: true` are complete (ie complete on all nodes)

## What happens happens when a node is considered ready
1. The runtime required taint is removed from the node if it exists.

# Why would you use runtime required
This is useful when you want to gate other work behind the successful completion of some set of Skyhook Packages. This can be for security reasons or for scheduling.

**NOTE:** No additional toleration is required, skyhook auto tolerates this (env:`runtimeRequiredTaint`) taint.