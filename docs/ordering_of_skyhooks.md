# Ordering of Skyhooks
## What
Skyhooks are applied in a repeatable and specific order based on their `priority` field. Each custom resource supports a `priority` field which is a non-zero positive integer. Skyhooks will be processed in order starting from 1, any Skyhooks with the same `priority` will be processed by sorting them by their `metadata.name` field.

**NOTE**: Any Skyhook which does NOT provide a `priority` field will be assigned a priority value of 200.

## Per-Node Ordering

**Important**: Priority ordering is enforced **per-node**, not globally across all nodes. This means:
- Node A can proceed to Skyhook 2 as soon as Skyhook 1 completes on Node A
- Node A does NOT wait for Node B to complete Skyhook 1
- If Node B is stuck on Skyhook 1, Node A can still progress through all its skyhooks

This per-node behavior prevents deadlocks where a few stuck/bad nodes would block all other healthy nodes from progressing through their skyhook sequence.

### Example
With two nodes (A, B) and two skyhooks (priority 1 and priority 2):
- Node A completes Skyhook 1 → Node A immediately starts Skyhook 2
- Node B is still processing Skyhook 1 → Node B shows "waiting" status on Skyhook 2
- Node A completes Skyhook 2 → Node A is fully complete
- Node B eventually completes Skyhook 1 → Node B starts Skyhook 2

## Flow Control Annotations

Two flow control features can be set in the annotations of each skyhook:
 * `skyhook.nvidia.com/disable`: bool. When `true` it will skip this Skyhook from processing and continue with any other ones further down the priority order.
 * `skyhook.nvidia.com/pause`: bool. When `true` it will NOT process this Skyhook and it WILL NOT continue to process any Skyhook's after this one on that node. This will effectively stop all application of Skyhooks starting with this one. NOTE: This ability used to be on the Skyhook spec itself as the `pause` field and has been moved here to be consistent with `disable` and to avoid incrementing the generation of a Skyhook Custom Resource instance when changing it.

## Why
This solves a few problems:

The first is to to better support debugging. Prior to this it was impossible to know the order Skyhooks would get applied to nodes as they would all run in parallel. This can, and has, lead to issues debugging a problem as it isn't deterministic. Now every node will always receive updates in the same order as every other node. Additionaly, this removes the possiblility of conflicts between Skyhooks by heaving each one run in order.

The second is to provide the ability for complex tasks to be sequenced. This comes up when needing to apply different sets of work to different node groups in a particular order.

The third is to provide the community a way to bucket Skyhooks according to where they might live in a stream of updates and therefore better coordinate work without explicit communication. We propose the following buckets:
 * 1 - 99 for initialization and infrastucture work
    * install security or monitoring tools
 * 100 - 199 for configuration work
    * configuring ssh access
 * 200+ for final user level configuration
    * applying tuning for workloads