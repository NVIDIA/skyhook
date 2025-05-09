# Skyhook Helm Chart
Skyhook was developed for modifying the underlying host OS in Kubernetes clusters. Think of it as a package manager like apt/yum for linux but for whole cluster management. The package manager (Skyhook Operator) manages the lifecycle (install/configure/uninstall/upgrade) of the packages (Skyhook Custom Resource, often SCR for short). It is Kubernetes aware, making cluster modifications easy. This enables Skyhook to schedule updates around important workloads and do rolling updates. It can be used in any cluster environment: self-managed clusters, on-prem clusters, cloud clusters, etc. 

### Benefits
 - The requested changes (the Packages) are native Kubernetes resources they can be combined and applied with common tools like ArgoCD, Helm, Flux etc. This means that all the tooling to manage applications can package customizations right alongside them to get applied, removed and upgraded as the applications themselves are.
 - Autoscaling, with skyhook if you want to enable autoscaling on your cluster but need to modify all Nodes added to a cluster, you need something that is kubernetes aware. Skyhook as feature to make sure you nodes are ready before then enter the cluster.
 - Upgrades are first class, with skyhook you can make deploy changes to your cluster and can wait for running workloads to finish before applying changes.

### Key Features
- **interruptionBudget:** percent of nodes or count
- **nodeSelectors:** selectors for which nodes to apply too (node labels)
- **podNonInterruptLabels:**  labels for pods to **never** interrupt
- **package interrupt:** service (containerd, cron, any thing systemd), or reboot
- **config interrupt:** service, or reboot when a certain key's value changes in the configmap
- **configMap:** per package
- **env vars:** per package
- **additionalTolerations:**  are tolerations added to the packages
- [**runtimeRequired**](docs/runtime_required.md): requires node to come into the cluster with a taint, and will do work prior to removing custom taint.

## Important Chart Settings
Settings | Description | Default |
---| --- | --- |
| controllerManager.tolerations | add tolerations to the controller manager pod | [] |
| controllerManager.selectors | add node selectors to the controller manager pod | {} |
| controllerManager.manager.env.copyDirRoot | Directory for which the operator will work from on the host. Some environments may require this to be set to a specific directory. | /tmp |
| webhooks.enable | Enable the webhook setup in the operator controller. Default is "true" and is required for production. | "true" |
| controllerManager.manager.env.leaderElection | Enable leader election for the operator controller. Default is "true" and is required for production. | "true" |
| controllerManager.manager.env.logLevel | Log level for the operator controller. If you want more or less logs, change this value to "debug" or "error". | "info" |
| controllerManager.manager.env.reapplyOnReboot | Reapply the packages on reboot. This is useful for systems that are read-only. | "false" |
| controllerManager.manager.env.runtimeRequiredTaint | This feature assumes nodes are added to the cluster with `--register-with-taints` kubelet flag. This taint is assume to be all new nodes, and skyhook pods will tolerate this taint, and remove it one the nodes packages are complete. | skyhook.nvidia.com=runtime-required:NoSchedule | 
| imagePullSecret | the secret used to pull the operator controller image, agent image, and package images. | node-init-secret |
| estimatedPackageCount | estimated number of packages to be installed on the cluster, this is used to calculate the resources for the operator controller. | 1 |
| estimatedNodeCount | estimated number of nodes in the cluster, this is used to calculate the resources for the operator controller | 1 |

### NOTES
- **estimatedPackageCount** and **estimatedNodeCount** are used to size the resource requirements. Default setting should be good for nodes > 1000 and packages 1-2 or nodes > 500 and packages >= 4. If your approaching this size deployment it would make sense to set these. You can also override them by explicitly with `controllerManager.manager.resources` the values file has an example.
- **runtimeRequired**: If your systems nodes have this taint make sure to add the toleration to the controllerManager.tolerations
- **CRD**: This project currently has one CRD and its not managed the ["recommended" way](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/). Its part of the templates. Meaning it will be updated with the `helm upgrade`. We decided it was better do it this way for this project. Doing it either way has consequences and this route has worked well for upgrades so far our deployments.