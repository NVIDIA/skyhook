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

## Package execution Jobs

Package stages run as `batch/v1` Jobs — one per (NodeWright, package, stage, node), each `parallelism: 1`,
`completions: 1`. A package that does not interrupt runs two of them per node (apply, config); one that
interrupts runs four (apply, config, interrupt, post-interrupt), and uninstall/upgrade add their own.

Two things follow for sizing.

**Cache.** The operator watches Jobs through a **namespace-scoped** informer (only the operator's own
namespace), so its cache holds every package-stage Job in that namespace — in flight *and* retained.
Package pods stay cached cluster-wide, because drain has to see every pod on a node, so Jobs are added
to the pod cache rather than replacing it.

**Retention.** A finished Job is not deleted immediately: `ttlSecondsAfterFinished` is set by outcome
from `jobTtlSucceeded` (1h default) and `jobTtlFailed` (24h default), both with a hard floor of one
minute. The retained population is therefore roughly the number of stages that completed inside those
windows — bounded above by `N × P × stages-per-package` for a rollout that finishes inside the TTL, and
decaying as the TTLs expire. Each retained Job also keeps at most **two** failed child pods (the first
genuine failure and the most recent); successful child pods are deleted with their Job.

If the operator's memory is the constraint at scale, `jobTtlSucceeded` is the first lever — successful
stages are the bulk of the population and the least interesting to keep. Note that the equations above
were derived before package execution moved to Jobs and have not been re-measured with the Jobs informer
in the cache, so treat them as a floor.
