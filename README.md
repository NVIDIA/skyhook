# skyhook

**Skyhook** is a Kubernetes-aware package manager for cluster administrators to safely modify and maintain underlying host declaratively at scale.

## Why Skyhook?

Managing and updating Kubernetes clusters is challenging. While Kubernetes advocates treating compute as disposable, but certain scenarios make this difficult:

- **Updating hosts without re-imaging:**
  - Limited excess hardware/capacity for rolling replacements
  - Long node replacement times (example can be hours in some cloud providers)
- **OS image management:**
  - Maintain a common base image with workload-specific overlays instead of multiple OS images
- **Workload sensitivity:**
  - Some workloads can't be moved, are difficult to move, or take a long time to migrate

## What is Skyhook?

Skyhook functions like a package manager but for your entire Kubernetes cluster, with three main components:

1. **Skyhook Operator** - Manages installing, updating, and removing packages
2. **Skyhook Custom Resource (SCR)** - Declarative definitions of changes to apply
3. **Packages** - The actual modifications you want to implement

## Where and When to use Skyhook

Skyhook works in any Kubernetes environment (self-managed, on-prem, cloud) and shines when you need:

- Kubernetes-aware scheduling that protects important workloads
- Rolling or simultaneous updates across your cluster
- Declarative configuration management for host-level changes

## Benefits
 - **Native Kubernetes integration** - Packages are standard Kubernetes resources compatible with GitOps tools like ArgoCD, Helm, and Flux
 - **Autoscaling support** - Ensure newly created nodes are properly configured before schedulable
 - **First-class upgrades** - Deploys changes with minimal disruption, waiting for running workloads to complete when needed

## Key Features
- **Interruption Budget:** percent of nodes or count
- **Node Selectors:** selectors for which nodes to apply too (node labels)
- **Pod Non Interrupt Labels:**  labels for pods to **never** interrupt
- **Package Interrupt:** service (containerd, cron, any thing systemd), or reboot
- **Additional Tolerations:**  are tolerations added to the packages
- [**Runtime Required**](docs/runtime_required.md): requires node to come into the cluster with a taint, and will do work prior to removing custom taint.
- **Resource Management:** Skyhook uses Kubernetes [LimitRange](https://kubernetes.io/docs/concepts/policy/limit-range/) to set default CPU and memory requests/limits for all containers in its namespace. You can override these defaults per-package in your Skyhook CR. Strict validation is enforced: if you set any resource override, you must set all four fields (cpuRequest, cpuLimit, memoryRequest, memoryLimit), and limits must be >= requests. See [docs/resource_management.md](docs/resource_management.md) for details and examples.

## Pre-built Packages

There are a few pre-built generalist packages available at [NVIDIA/skyhook-packages](https://github.com/NVIDIA/skyhook-packages)

## Quick Start

### Install the operator
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

**For detailed information about our versioning strategy, git tagging conventions, and component release process, see [docs/versioning.md](docs/versioning.md) and [docs/release-process.md](docs/release-process.md).**

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

## Examples

See the [examples/](examples/) directory for sample manifests, usage patterns, and demo configurations to help you get started with Skyhook.

## Kyverno Policy Examples

See [docs/kyverno/README.md](docs/kyverno/README.md) for example Kyverno policies and guidance on restricting images or packages in Skyhook resources.

## [Skyhook-Operator](operator/README.md)
The operator is a kbuernetes operator that monitors cluster events and coordinates the installation and lifecycle of Skyhook packages.

## [Skyhook Agent](agent/README.md)
The agent is what does the operators work and is a separate container from the package. The agent knowns how to read a package (/skyhook_package/config.json) is what implements the [lifecycle](#stages) packages go though.

