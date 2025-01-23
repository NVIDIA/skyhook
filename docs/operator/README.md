# skyhook-operator
[![pipeline status](https://gitlab-master.nvidia.com/dgx/infra/skyhook-operator/badges/main/pipeline.svg)](https://gitlab-master.nvidia.com/dgx/infra/skyhook-operator/-/commits/main) [![coverage report](https://gitlab-master.nvidia.com/dgx/infra/skyhook-operator/badges/main/coverage.svg)](https://gitlab-master.nvidia.com/dgx/infra/skyhook-operator/-/commits/main)

Skyhook was developed for modifying the underlying host OS in Kubernetes clusters. Think of it as a package manager like apt/yum for linux but for whole cluster management. The package manager (Skyhook Operator) manages the lifecycle (install/configure/uninstall/upgrade) of the packages (Skyhook Custom Resource, often SCR for short). It is Kubernetes aware, making cluster modifications easy. This enables Skyhook to schedule updates around important workloads and do rolling updates. It can be used in any cluster environment: self-managed clusters, on-prem clusters, cloud clusters, etc. 

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

### Usage Example Custom Resource

Usage once operator is installed looks like this, you can apply skyhook packages. In this example we are applying 2 (nvssh, and bcp).

```yaml
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  name: skyhook-example
spec:
  additionalTolerations:
    - key: nvidia.com/gpu
      operator: Exists
  nodeSelectors:
    matchLabels:
      agentpool: gpu
  podNonInterruptLabels:
    matchLabels:
      key: value
  interruptionBudget: 
    percent: 33
  packages:
    nvssh:
      version: 2024.05.10
      image: nvcr.io/nvidian/swgpu-baseos/nvssh:2024.05.10
      configInterrupts:
        nvssh_vars.sh:
          type: service
          services: [cron]
      configMap:
        nvssh_vars.sh: |-
          #!/bin/bash
          nvssh_allowed_roles=access-azure-nv-ngc-prod-dgxc-admin
          nvssh_allowed_sudo_roles=access-azure-nv-ngc-prod-dgxc-admin
          echo $0
    bcp:
      version: 2024.05.13
      image: nvcr.io/nvidian/swgpu-baseos/bcp:2024.05.13
      env:
        - name: CSP
          value: azure
      interrupt: 
        type: service
        services: [containerd]
```

Packages can depend on each other, so if you needed bcp to be installed before nvssh you can define that like this:

```yaml
    nvssh:
      ...
      dependsOn: 
        bcp: "3.0"
    bcp:
      ...
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

***NOTE:*** The stages that are actually applied can change depending on the configuration of the package and current state. See [Skyhook 2.0](https://docs.google.com/document/d/1rnU0fs4MoWi9NfD2ff1hZMWRJKNQ58HeKob34EjS92M/) for more info.

## Getting Started

## Dependencies
To install skyhook operator you need to first install [cert manager](https://cert-manager.io/). Its pretty easy to install, and does not need much tweeking or any depending on the install method used. There are two different ways of deployment:
  - [static kubectl apply](https://cert-manager.io/docs/installation/)
  - [helm](https://cert-manager.io/docs/installation/helm/)

## Packages
Part of how the operator works is the [skyhook-agent](#skyhook-agent). Packages have to be created in way so the operator knows how to use them. This is where the agent comes into play, more on that later. A package is a container that meets these requirements:

- Container shall have `bash`, so needs to be at least something like busybox/alpine
- Config that is valid, jsonschema is used to valid this config. The agent has a tool build in to valid the config. This tool should be used to test packages before publishing.
- The file system structure needs to adhere to:
```
/skyhook-package
├── skyhook_dir/{steps}
├── root_dir/{static files}
└── config.json
```

## [Skyhook Agent](agent/README.md)
The agent is what does the operators work and is a separate container from the package. The agent knowns how to read a package (/skyhook_package/config.json) is what implements the [lifecycle](#stages) packages go though.

## Development

### Prerequisites
- go version v1.23.4+
- docker version 17.03+ or podman 4.9.4+ (project makefile kind of assumes podman)
- kubectl version v1.27.3+.
- Access to a Kubernetes v1.27+ cluster. (we test on 1.27, should work on older if needed, just not tested.)


**Install the CRDs into the cluster:**
```sh
make install
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Run the Operator**
```sh
make run ## will run in background process, not in kubernetes
make kill ## kills background process
```

or you can build and run this way
```sh
make build
./bin/manager
```

**Create instances of your solution**
You can apply the [examples](./config/samples/) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**NOTE:** You may need to run [kind in experimental mode](https://kind.sigs.k8s.io/docs/user/rootless/#creating-a-kind-cluster-with-rootless-podman) when using `make` create-kind-cluster. Run `make --help` for more information on all potential `make` targets
```bash
❯ make help

Usage:
  make <target>

General
  help             Display this help.
  clean            Clears out the local build folder

Development
  manifests        Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
  generate         Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
  generate-mocks   Generate code for interface mocking
  license-report   Run run license report
  license-check    Run go-licenses check against code.
  license-fmt      Run add license header to code.
  fmt              Run go fmt against code.
  vet              Run go vet against code.
  test             Run all tests.
  watch-tests      watch unit tests and auto run on changes.
  unit-tests       Run unit tests.
  e2e-tests        Run end to end tests.
  create-kind-cluster  deletes and creates a new kind cluster. versions is set via KIND_VERSION
  podman-create-machine  creates a podman machine
  lint             Run golangci-lint linter & yamllint
  lint-fix         Run golangci-lint linter and perform fixes
  create-dashboard  create kubernetes dashboard for local testing
  access-dashboard  portforwards and gets token for dashboard for local testing

Build
  build            Build manager binary.
  run              Run a controller from your host.
  docker-build     Build docker image with the manager.

Deployment
  install          Install CRDs into the K8s cluster specified in ~/.kube/config.
  uninstall        Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
  deploy           Deploy controller to the K8s cluster specified in ~/.kube/config.
  generate-helm    Generates new helm chart using helmify. Be-careful, this can break things, it overwrites files, make sure to look at you git diff.
  undeploy         Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.

Build Dependencies
  install-deps     Install all dependencies
  golangci-lint    Download golangci locally if necessary. 
  kustomize        Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
  controller-gen   Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
  envtest          Download envtest-setup locally if necessary.
  gocover-cobertura  Download gocover-cobertura locally if necessary.
  ginkgo           Download ginkgo locally if necessary.
  mockery          Download mockery locally if necessary.
  chainsaw         Download chainsaw locally if necessary.
  helm             Download helm locally if necessary.
  helmify          Download helmify locally if necessary.
  go-license       Download  go-license locally if necessary.
  go-licenses      Download  go-licenses locally if necessary.
```

# Deployment

The helm repos are here:
- [Prod - https://helm.ngc.nvidia.com/nv-ngc-devops/skyhook-operator](https://helm.ngc.nvidia.com/nv-ngc-devops/skyhook-operator)
- [Dev - https://helm.ngc.nvidia.com/nvidian/swgpu-baseos/skyhook-operator](https://helm.ngc.nvidia.com/nvidian/swgpu-baseos/skyhook-operator)

Operator containers:
- [Prod - nvcr.io/nv-ngc-devops/skyhook-operator](https://nvcr.io/nv-ngc-devops/skyhook-operator)
- [Dev - nvcr.io/nvidian/swgpu-baseos/skyhook-operator](https://nvcr.io/nvidian/swgpu-baseos/skyhook-operator)

## Deploy from main

If you want to test the helm chart, this is how you can deploy it from the repo.

```
## setup namespace "skyhook"
kubectl create namespace skyhook --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic node-init-secret --from-file=.dockerconfigjson=${HOME}/.config/containers/auth.json --type=kubernetes.io/dockerconfigjson -n  skyhook

## install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.2/cert-manager.yaml

## install operator
helm install skyhook-operator ./chart --namespace skyhook
```

to remove operator from a cluster:
```
## remove operator
helm uninstall skyhook-operator --namespace skyhook

## delete CRD
make uninstall
```

**NOTE**: because there is a finalizer on it you need to need to delete the SCRs before uninstalling the CRD or operator. If you remove the operator first, delete the CRD or SCR can hang trying to finalize. Easiest way to fix is re install the operator. You can clean up by hand, but could be some work. cleaning up: configmaps, uncording nodes, removing taints, and deleting running pods


## Helm Chart and General Config infra
This repo uses kubebuilder for scaffolding this project, and lots of functions in the makefile are hooked up to this as well. Kubebuilder for the most part manages the [config](./config) directory which uses kustomize heavily. To convert this structure into helm, we using a tool call helmify. Its a generic tool that converts this kubebuilder kustomize into a helm chart. It does not do everything, just a lot of it. So once you call `make generate-helm` you might need to make some additional changes or revert some of its changes. At some point we might want to stop using it if if becomes more work, but for now it does a pretty good job keep the two different ways of managing config in sync. 

Common work flow looks like: change something that requires updating config. First do `make manifest` and `make generate`. This will alter the config based on comments let in code. Example would be in the CRD `//+kubebuilder:validation:Enum=service;reboot`. Changes to the CRD will require those 2 make functions( well depending what you change might need just one). Then to keep in sync do `make generate-helm` which ask `are you sure` this is because it might not do exactly what you think. To keep in sync you need to say Y, and edit the chart as needed or do it by hand. Depending on what you did. Might make sense to do this in a clean git state so you can see what its doing. Mostly making it sound scarier then it is, just be warned.
