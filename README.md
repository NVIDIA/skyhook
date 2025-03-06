# skyhook

Skyhook was developed for modifying the underlying host OS in Kubernetes clusters. Think of it as a package manager like apt/yum for linux but for whole cluster management. The package manager (Skyhook Operator) manages the lifecycle (install/configure/uninstall/upgrade) of the packages (Skyhook Custom Resource, often SCR for short). It is Kubernetes aware, making cluster modifications easy. This enables Skyhook to schedule updates around important workloads and do rolling updates. It can be used in any cluster environment: self-managed clusters, on-prem clusters, cloud clusters, etc.

## Quick Start

### Install the operator
  1. Install cert-manager `kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.2/cert-manager.yaml`
  1. Create a secret for the operator to pull images `kubectl create secret generic node-init-secret --from-file=.dockerconfigjson=${HOME}/.config/containers/auth.json --type=kubernetes.io/dockerconfigjson -n  skyhook`
  1. Install the operator `helm install skyhook ./chart --namespace skyhook`

### Install a package
Example package using shellscript, put this in a file called `demo.yaml` and apply it with `kubectl apply -f demo.yaml`
```
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  labels:
    app.kubernetes.io/part-of: skyhook-operator
    app.kubernetes.io/created-by: skyhook-operator
  name: demo
spec:
  nodeSelectors:
    matchLabels:
      skyhook.nvidia.com/test-node: demo
  packages:
    tuning:
      version: 1.1.0
      image: ghcr.io/nvidia/skyhook-packages/shellscript
      configMap:
        apply.sh: |-
            #!/bin/bash
            echo "hello world" > /skyhook-hello-world
            sleep 5
        apply_check.sh: |-
            #!/bin/bash
            cat /skyhook-hello-world
            sleep 5
        config.sh: |-
            #!/bin/bash
            echo "a config is run" >> /skyhook-hello-world
            sleep 5
        config_check.sh: |-
            #!/bin/bash
            grep "config" /skyhook-hello-world
            sleep 5
```

### Watch Skyhook apply the package
```
kubectl get pods -w -n skyhook
```
There will a pod for each lifecycle stage (apply, config) in this case.

### Check the package
```
kubectl describe skyhooks.skyhook.nvidia.com/demo
```
The Status will show the overall package status as well as the status of each node

### Check the annotations on the node using the label
```
kubectl get nodes -o jsonpath='{range .items[?(@.metadata.labels.skyhook\.nvidia\.com/test-node=="demo")]}{.metadata.annotations.skyhook\.nvidia\.com/nodeState_demo}{"\n"}{end}'
```
  
## Benefits
 - The requested changes (the Packages) are native Kubernetes resources they can be combined and applied with common tools like ArgoCD, Helm, Flux etc. This means that all the tooling to manage applications can package customizations right alongside them to get applied, removed and upgraded as the applications themselves are.
 - Autoscaling, with skyhook if you want to enable autoscaling on your cluster but need to modify all Nodes added to a cluster, you need something that is kubernetes aware. Skyhook as feature to make sure you nodes are ready before then enter the cluster.
 - Upgrades are first class, with skyhook you can make deploy changes to your cluster and can wait for running workloads to finish before applying changes.

## Key Features
- **interruptionBudget:** percent of nodes or count
- **nodeSelectors:** selectors for which nodes to apply too (node labels)
- **podNonInterruptLabels:**  labels for pods to **never** interrupt
- **package interrupt:** service (containerd, cron, any thing systemd), or reboot
- **config interrupt:** service, or reboot when a certain key's value changes in the configmap
- **configMap:** per package
- **env vars:** per package
- **additionalTolerations:**  are tolerations added to the packages
- [**runtimeRequired**](docs/runtime_required.md): requires node to come into the cluster with a taint, and will do work prior to removing custom taint.

### Stages
The operator will apply steps in a package throughout different lifecycle stages. This ensures that the right steps are applied in the right situations and in the correct order.
- Upgrade: This stage will be ran whenever a package's version is upgraded in the SCR.
- Uninstall: This stage will be ran whenever a package's version is downgraded or it's removed from the SCR.
- Apply: This stage will always be ran at least once.
- Config: This stage will run when a configmap is changed and on the first SCR application. 
- Interrupt: This stage will run when a package has an interrupt defined or a key's value in a packages configmap changes which has a config interrupt defined.
- Post-Interrupt: This stage will run when a package's interrupt has finished.

The stages are applied in this order:

- Uninstall -> Apply -> Config -> Interrupt -> Post-Interrupt (No Upgrade)
- Upgrade -> Config -> Interrupt -> Post-Interrupt (With Upgrade)

**Semantic versioning is strictly enforced in the operator** in order to support upgrade and uninstall. Semantic versioning allows the 
operator to know which way the package is going while also enforcing best versioning practices.

## Packages
Part of how the operator works is the [skyhook-agent](agent/README.md). Packages have to be created in way so the operator knows how to use them. This is where the agent comes into play, more on that later. A package is a container that meets these requirements:

- Container shall have `bash`, so needs to be at least something like busybox/alpine
- Config that is valid, jsonschema is used to valid this config. The agent has a tool build in to valid the config. This tool should be used to test packages before publishing.
- The file system structure needs to adhere to:
```
/skyhook-package
├── skyhook_dir/{steps}
├── root_dir/{static files}
└── config.json
```

## Example Kyverno Policy

This repository includes an example Kyverno policy that demonstrates how to restrict the images that can be used in Skyhook packages. While this is not a complete policy, it serves as a template that end users can modify to fit their security needs.

The policy prevents the creation of Skyhook resources that contain packages with restricted image patterns. Specifically, it blocks:
- Images containing 'shellscript:' anywhere in the image name
- Images from Docker Hub (matching 'docker.io/*')

If you are going to use kyverno make sure to turn on the creation of the skyhook-viewer-role in the values file for the operator. (rbac.createSkyhookViewerRole: true) and then bind kyverno to that role. Example policy:
```
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kyverno-skyhook-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: skyhook-viewer-role
subjects:
- kind: ServiceAccount
  name: kyverno-reports-controller
  namespace: kyverno
```

## [Skyhook-Operator](operator/README.md)
The operator is a kbuernetes operator that monitors cluster events and coordinates the installation and lifecycle of Skyhook packages.

## [Skyhook Agent](agent/README.md)
The agent is what does the operators work and is a separate container from the package. The agent knowns how to read a package (/skyhook_package/config.json) is what implements the [lifecycle](#stages) packages go though.

