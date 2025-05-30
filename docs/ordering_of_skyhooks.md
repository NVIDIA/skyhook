# Ordering of Skyhooks
## What
With v0.8.0 Skyhooks now always get applied in a repeatable and specific order. This also means that all Skyhooks will now be sequential, though packages within a Skyhook can be parallel. Each custom resource now supports a `priority` field which is a non-zero positive integer. Skyhooks will be processed in order starting from 0, any Skyhooks with the same `priority` will be processed by sorting them by their `metadata.name` field.

**NOTE**: Any Skyhook which does NOT provide a `priority` field will be assigned a priority value of 200.

Two additional flow control features have been added with this and can be set in the annotations of each skyhook:
 * `skyhook.nvidia.com/disable`: bool. When `true` it will skip this Skyhook from processing and continue with any other ones further down the priority order.
 * `skyhook.nvidia.com/pause`: bool. When `true` it will NOT process this Skyhook and it WILL NOT continue to process any Skyhook's after this one. This will effectively stop all application of Skyhooks starting with this one. NOTE: This ability used to be on the Skyhook spec itself as the `pause` field and has been moved here to be consistent with `disable` and to avoid incrementing the generation of a Skyhook Custom Resource instance when changing it.

## Why
This solves a few problems:

The first is to to better support debugging. Prior to this it was impossible to know the order Skyhooks would get applied to nodes as they would all run in parallel. This can, and has, lead to issues debugging a problem as it isn't deterministic. Now every node will always receive updates in the same order as every other node. Additionaly, this removes the possiblility of conflicts between Skyhooks by heaving each one run in order.

The second is to provide the ability for complex tasks to be sequenced. This comes up when needing to apply different sets of work to different node groups in a particular order.

The third is to provide the community a way to bucket Skyhooks according to where they might live in a stream of updates and therefore better coordinate work without explicit communication. We propose the following buckets:
 * 0 - 99 for initialization and infrastucture work
    * install security or monitoring tools
 * 100 - 199 for configuration work
    * configuring ssh access
 * 200+ for final user level configuration
    * applying tuning for workloads