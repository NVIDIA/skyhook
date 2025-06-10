# Metrics
The current metrics supplied by the Operator are intended to be sufficient to determine the state of application of a Skyhook Custom Resource within a cluster. These metrics are defined at [internal/controller/metrics.go](operator/internal/controller/metrics.go). There are three primary flavors:
 * `skyhook_node_*` : Which give the count of nodes in given state for a SCR. Tags are:
    * `skyhook_name` : The name of the Skyhook Custom Resource
 * `skyhook_[complete|disabled|pause]_count` : A 1 or 0 for if a given SCR is in this state
 * `skyhook_package_*` : Can be used to track the progress of packages through a deployment. Tags:
    * `skyhook_name` : The of the SCR the package belongs to
    * `package_name` : The name of the package
    * `package_version`: The version of the package
 * `skyhook_package_stage_count` : Allows the tracking of a package's lifecyle. Tags:
    * `skyhook_name` : The of the SCR the package belongs to
    * `package_name` : The name of the package
    * `package_version`: The version of the package
    * `stage` : One of apply, conifg, interrupt, post_interrupt, uninstall
* `skyhook_package_restart-count`: Sum of restarts across all nodes for this package
    * `skyhook_name` : The of the SCR the package belongs to
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
kubectl port-forward svc/skyhook-operator-controller-manager-metrics-service -n skyhook  8080:8443
```

# Visualization
The makefile provides the `metrics` command which will install prometheus and grafana as a starting point for visualization.

## Prometheus Configuration
Use the file [prometheus_values.yaml](prometheus_values.yaml) as an example of configuring a scraper job for Skyhook. Note: This can be used directly with the prometheus community chart:
```bash
helm install prometheus prometheus-community/prometheus -f ../docs/metrics/prometheus_values.yaml
```

## Grafana configuration
After the chart is installed connect to the grafana instance and configure the prometheus datasource. An example that will work with the Makefile commands in operator is included here at [granfa_values.yaml](grafana_values.yaml)

