# Operator Resources at Scale

As cluster size and package count increase the operator requires more CPU and Memory to efficiently operate. This is especially true for memory as the operator can being to OOM at large cluster sizes.

# Scaling Equations

The following have been validated on cluster size up to 1k nodes.

For the equations below the variables are as follows:

 * `N` : number of nodes
 * `P` : number of packages

## memory
| item    | function |
----------|-----------	
| request |	max(256Mi, N * 0.45) |
| limit   |	max(N * .8 * max(P * 0.4, 1), 512) |
	
## cpu
| item    | function |
----------|-----------	
| request |	max(500m, $limit/2) |
| limit	 | max(N*1.6 * max(P * 0.4, 1), 1000) |

# Helm chart

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