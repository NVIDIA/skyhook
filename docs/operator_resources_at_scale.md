# Operator Resources at Scale

As cluster size and package count increase the operator requires more CPU and Memory to efficiently operate. This is especially true for memory as the operator can being to OOM at large cluster sizes.

## Scaling Equations

The following have been validated on cluster size up to 1k nodes.

For the equations below the variables are as follows:

 * `N` : number of nodes
 * `P` : number of packages

## memory

| item    | function |
----------|----------- 
| request | max(256Mi, N * 0.45) |
| limit   | max(N *.8* max(P * 0.4, 1), 512) |
 
## cpu

| item    | function |
----------|----------- 
| request | max(500m, $limit/2) |
| limit  | max(N*1.6* max(P * 0.4, 1), 1000) |

## Exported metric series during the metrics deprecation window

Between operator v0.18.0 and v0.20.0 every metric is published twice, once under the current
`nodewright_*` name and once under the deprecated `skyhook_*` name (see
[docs/metrics/README.md](metrics/README.md)). That roughly doubles the operator's exported series count
for the duration of the window.

This affects the Prometheus side rather than the operator: series count drives scrape payload size and
Prometheus storage, and the equations above are driven by node and package count, not by series count.
No change to the requests/limits below is needed. The legacy half disappears in v0.20.0.

If the extra series are unwelcome and your dashboards and alerts already use the `nodewright_*` names,
set `PUBLISH_LEGACY_METRICS=false` (chart: `controllerManager.manager.env.publishLegacyMetrics`) to
unregister the deprecated collectors at startup and halve the count immediately.

## Helm chart

The chart is already setup with the above equations. You can use the `estimatedPackageCount` and `estimatedNodeCount`. The default values in the chart of:
```
limits:
  cpu: 1000m
  memory: 512Mi
requests:
  cpu: 500m
  memory: 256Mi
```
Is sufficient to get to ~800 nodes and 1 - 3 packages or ~500 nodes a 4+ packages.
