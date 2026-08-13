# NodeWright Helm Chart

NodeWright was developed for modifying the underlying host OS in Kubernetes clusters. Think of it as a package manager like apt/yum for linux but for whole cluster management. The package manager (NodeWright Operator) manages the lifecycle (install/configure/uninstall/upgrade) of the packages (NodeWright Custom Resource, often CR for short). It is Kubernetes aware, making cluster modifications easy. This enables NodeWright to schedule updates around important workloads and do rolling updates. It can be used in any cluster environment: self-managed clusters, on-prem clusters, cloud clusters, etc. 

## Benefits

 - The requested changes (the Packages) are native Kubernetes resources they can be combined and applied with common tools like ArgoCD, Helm, Flux etc. This means that all the tooling to manage applications can package customizations right alongside them to get applied, removed and upgraded as the applications themselves are.
 - Autoscaling: with NodeWright, if you want to enable autoscaling on your cluster but need to modify all Nodes added to a cluster, you need something that is Kubernetes-aware. NodeWright has a feature to make sure your nodes are ready before they enter the cluster.
 - Upgrades are first class: with NodeWright you can make deploy changes to your cluster and can wait for running workloads to finish before applying changes.

## Key Features

- **interruptionBudget:** percent of nodes or count
- **nodeSelectors:** selectors for which nodes to apply too (node labels)
- **podNonInterruptLabels:**  labels for pods to **never** interrupt
- **package interrupt:** service (containerd, cron, any thing systemd), or reboot
- **config interrupt:** service, or reboot when a certain key's value changes in the configmap
- **configMap:** per package
- **env vars:** per package
- **additionalTolerations:**  are tolerations added to the packages
- [**runtimeRequired**](../docs/runtime_required.md): requires node to come into the cluster with a taint, and will do work prior to removing custom taint.

## Important Chart Settings

Settings | Description | Default |
---| --- | --- |
| controllerManager.tolerations | add tolerations to the controller manager pod | [] |
| controllerManager.selectors | add node selectors to the controller manager pod | {} |
| controllerManager.nodeAffinity.matchExpressions | advanced node affinity expressions for the controller manager pod. Cannot be used together with `controllerManager.selectors` — choose one. | [] |
| controllerManager.manager.env.copyDirRoot | Directory for which the operator will work from on the host. Some environments may require this to be set to a specific directory. | /var/lib/skyhook |
| controllerManager.manager.env.agentLogRoot | Directory the agent will write logs to on the host. Some environments may require this to be set to a specific directory. | /var/log/skyhook |
| webhook.enable | Enable the webhook setup in the operator controller. Default is "true" and is required for production. | "true" |
| controllerManager.manager.env.leaderElection | Enable leader election for the operator controller. Default is "true" and is required for production. | "true" |
| controllerManager.manager.env.logLevel | Log level for the operator controller. If you want more or less logs, change this value to "debug" or "error". | "info" |
| controllerManager.manager.env.reapplyOnReboot | Reapply the packages on reboot. This is useful for systems that are read-only. | "false" |
| controllerManager.manager.env.runtimeRequiredTaint | This feature assumes nodes are added to the cluster with `--register-with-taints` kubelet flag. This taint is assumed to be on all new nodes; NodeWright pods tolerate it, and the operator removes it from a node once every `runtimeRequired: true` NodeWright targeting that node has completed on it (completion on other nodes does not affect removal). During the rename deprecation window the operator additionally tolerates and removes the legacy `skyhook.nvidia.com=runtime-required:NoSchedule` taint, but never applies it. | nodewright.nvidia.com=runtime-required:NoSchedule | 
| controllerManager.manager.image.repository | Where to get the image from | "ghcr.io/nvidia/nodewright/operator" |
| controllerManager.manager.image.tag | what version of the operator to run | defaults to appVersion |
| controllerManager.manager.image.digest | content-addressable pin for the operator image. If set, the digest determines the pulled image. If both tag and digest are provided, the digest takes precedence; the rendered image may include `tag@digest` but the digest controls selection. | "" |
| controllerManager.manager.agent.repository | Where to get the image from | "ghcr.io/nvidia/nodewright/agent" |
| controllerManager.manager.agent.tag | what version of the agent to run | defaults to the current latest, but is not latest example v6.1.5 |
| controllerManager.manager.agent.digest | content-addressable pin for the agent image. Same precedence rules as above: if both tag and digest are provided, the digest controls which image is pulled. | "" |
| imagePullSecret | the secret used to pull the operator controller image, agent image, and package images. | "" |
| useHostNetwork | run the operator pods with `hostNetwork: true`. Required in environments where the apiserver is only reachable on the host network. | false |
| estimatedPackageCount | estimated number of packages to be installed on the cluster, this is used to calculate the resources for the operator controller. | 1 |
| estimatedNodeCount | estimated number of nodes in the cluster, this is used to calculate the resources for the operator controller | 1 |
| rbac.createSkyhookViewerRole | create a `ClusterRole` that grants read-only access to NodeWright and DeploymentPolicy resources. Aggregate-bind to your own users/groups. | false |
| rbac.createSkyhookEditorRole | create a `ClusterRole` that grants read/write access to NodeWright and DeploymentPolicy resources. Aggregate-bind to your own users/groups. | false |
| limitRange.default | namespace-wide default CPU/memory **limits** applied to every container that doesn't set its own. Set both `default` and `defaultRequest` (or omit `limitRange` entirely to disable). See [../docs/resource_management.md](../docs/resource_management.md). | cpu: 500m, memory: 512Mi |
| limitRange.defaultRequest | namespace-wide default CPU/memory **requests** applied to every container that doesn't set its own. | cpu: 250m, memory: 256Mi |
| cleanup.enabled | Automatically delete all NodeWright and DeploymentPolicy resources during helm uninstall. Recommended to prevent orphaned CRs. | true |
| cleanup.jobTimeoutSeconds | Hard deadline for the entire cleanup job during uninstall. The job will be killed if it exceeds this time. | 120 |

### NOTES

- **estimatedPackageCount** and **estimatedNodeCount** are used to size the resource requirements. Default setting should be good for nodes > 1000 and packages 1-2 or nodes > 500 and packages >= 4. If your approaching this size deployment it would make sense to set these. You can also override them by explicitly with `controllerManager.manager.resources` the values file has an example.
- **runtimeRequired**: If your systems nodes have this taint make sure to add the toleration to the controllerManager.tolerations
- **CRD**: This project currently has one CRD and its not managed the ["recommended" way](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/). Its part of the templates. Meaning it will be updated with the `helm upgrade`. We decided it was better do it this way for this project. Doing it either way has consequences and this route has worked well for upgrades so far our deployments.
- **Image pinning (tag vs digest)**: You can set either an image tag or a digest. If both are set, the digest is prioritized; the tag is ignored for selection and may appear as `tag@digest` only for readability. This applies to both operator and agent images.

## Upgrade Notes

### Breaking Change: imagePullSecret Default Changed

**Previous behavior:** The `imagePullSecret` value defaulted to `node-init-secret`. If this secret didn't exist, kubelet logs would show errors about the missing secret.

**New behavior:** The `imagePullSecret` value now defaults to empty (`""`). No imagePullSecrets will be added to pods unless explicitly configured.

**Migration:** If you rely on `node-init-secret` for pulling images from private registries, you must now explicitly set `imagePullSecret` in your values:

```yaml
imagePullSecret: "node-init-secret"
```

If you use public images (default operator `ghcr.io/nvidia/nodewright/operator` and agent `ghcr.io/nvidia/nodewright/agent`), no action is needed.

### Resource Management

NodeWright uses Kubernetes LimitRange to set default CPU/memory requests/limits for all containers in the namespace. You can override these per-package in your NodeWright CR. Strict validation is enforced. See [../docs/resource_management.md](../docs/resource_management.md) for details and examples.

## Versioning

This Helm chart follows independent versioning from the operator and agent components. The chart's `appVersion` field specifies the recommended stable operator version that provides a good default for installations. See [../docs/versioning.md](../docs/versioning.md) for more details on versioning.

### Chart Version vs App Version

- **Chart version** (`version` in Chart.yaml): Tracks changes to chart templates, values, and configuration (NOTE: agent version in set in the values.)
- **App version** (`appVersion` in Chart.yaml): Recommended stable operator version for this chart release

## Uninstalling

### Automatic Cleanup (Default Behavior)

By default, the Helm chart includes a pre-delete hook that automatically cleans up all NodeWright and DeploymentPolicy custom resources before uninstalling. This prevents orphaned resources that could cause issues during reinstallation.

```bash
# Uninstall with automatic cleanup (default)
helm uninstall nodewright --namespace skyhook
```

The pre-delete hook will:

- Delete all NodeWright resources cluster-wide
- Delete all DeploymentPolicy resources cluster-wide
- Wait for finalizers to be processed
- Proceed with uninstall even if cleanup times out (job deadline: 2 minutes, configurable via `cleanup.jobTimeoutSeconds`)

### Disabling Automatic Cleanup

If you need to preserve NodeWright resources during uninstall (e.g., for backup/migration scenarios), disable the cleanup feature:

```yaml
# values.yaml
cleanup:
  enabled: false
```

When disabled, you must manually delete resources before uninstalling to avoid issues:

```bash
# Manual cleanup when automatic cleanup is disabled
kubectl delete skyhooks --all
kubectl delete deploymentpolicies --all
helm uninstall nodewright --namespace skyhook
```

### Configuring Timeout Values

For large clusters or when resources have complex finalizers, you may need to adjust the job timeout:

```yaml
# values.yaml
cleanup:
  enabled: true
  jobTimeoutSeconds: 180  # 3 minutes total job deadline
```

**Note:** The job will be killed if it exceeds `jobTimeoutSeconds`. The default of 120 seconds (2 minutes) should be sufficient for most clusters.
