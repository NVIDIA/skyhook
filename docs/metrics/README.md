# Metrics
The current metrics supplied by the Operator are intended to be sufficient to determine the state of application of a Skyhook Custom Resource within a cluster. These metrics are defined at [internal/controller/metrics.go](../../operator/internal/controller/metrics.go). 

## Skyhook Status Metrics
 * `skyhook_status` : Binary metric indicating the status of the Skyhook Custom Resource (1 if in that status, 0 otherwise). Tags:
    * `skyhook_name` : The name of the Skyhook Custom Resource
    * `status` : One of complete, disabled, paused

## Node Metrics
 * `skyhook_node_status_count` : Number of nodes in the cluster by status for the Skyhook Custom Resource. Tags:
    * `skyhook_name` : The name of the Skyhook Custom Resource
    * `status` : One of complete, in_progress, erroring, blocked, waiting
 * `skyhook_node_target_count` : Total number of nodes targeted by this Skyhook Custom Resource. Tags:
    * `skyhook_name` : The name of the Skyhook Custom Resource

## Package Metrics
 * `skyhook_package_state_count` : Number of nodes in the cluster by state for this package. Tags:
    * `skyhook_name` : The name of the SCR the package belongs to
    * `package_name` : The name of the package
    * `package_version`: The version of the package
    * `state` : One of complete, in_progress, skipped, erroring, unknown
 * `skyhook_package_stage_count` : Number of nodes in the cluster by stage for this package. Tags:
    * `skyhook_name` : The name of the SCR the package belongs to
    * `package_name` : The name of the package
    * `package_version`: The version of the package
    * `stage` : One of apply, config, interrupt, post_interrupt, uninstall, upgrade
 * `skyhook_package_restarts_count`: Number of restarts for this package on this node. Tags:
    * `skyhook_name` : The name of the SCR the package belongs to
    * `package_name` : The name of the package
    * `package_version`: The version of the package

Note: When a Skyhook is deleted all metrics for that Skyhook are no longer reported.

# Testing
See the script [metrics_test.py](../../k8s-tests/chainsaw/skyhook/metrics_test.py) that will let you test of exists or absence of metrics based on name and labels. The metrics endpoint can also be hit directly at:
```bash
curl http://localhost:8080/metrics
```
Or you can port forward to it in kubernetes if installed via the chart
```bash
kubectl port-forward svc/skyhook-operator-controller-manager-metrics-service -n skyhook  8080:8080
```

# Visualization
The makefile provides the `metrics` command which will install prometheus and grafana as a starting point for visualization.

## Prometheus Configuration
### Scrape directly
Use the file [prometheus_values.yaml](prometheus_values.yaml) as an example of configuring a scraper job for Skyhook. Note: This can be used directly with the prometheus community chart:
```bash
helm install prometheus prometheus-community/prometheus -f ../docs/metrics/prometheus_values.yaml
```

### Auto discovery
The current values file for the operator helm chart sets prometheus auto-discovery annotations for the http endpoint. Copied below:
```
metricsService:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
    prometheus.io/scheme: "http"
  ports:
  - name: metrics
    port: 8443
    targetPort: 8443
    protocol: TCP
  - name: metrics-http
    port: 8080
    targetPort: 8080
    protocol: TCP
  type: ClusterIP
```
To change to https set the scheme to `https` and the port to `8443` but you will need to set the prometheus auto-discovery to allow insecure tls. See the scrape directly example for known working values.



## Grafana configuration
After the chart is installed connect to the grafana instance and configure the prometheus datasource. An example that will work with the Makefile commands in operator is included here at [granfa_values.yaml](grafana_values.yaml)

