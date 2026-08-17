# NodeWright (formerly Skyhook)

[![Pipeline Status](https://github.com/NVIDIA/nodewright/actions/workflows/operator-ci.yaml/badge.svg)](https://github.com/NVIDIA/nodewright/actions/workflows/operator-ci.yaml)
[![Coverage Status](https://coveralls.io/repos/github/NVIDIA/nodewright/badge.svg)](https://coveralls.io/github/NVIDIA/nodewright)

**NodeWright** is a Kubernetes-aware package manager for cluster administrators to safely modify and maintain underlying host declaratively at scale.

> **Note:** NodeWright is being renamed from Skyhook, and the rename has now landed for the core surfaces. The Helm chart, operator image, CLI (`kubectl nodewright`), and the CRDs (`nodewright.nvidia.com/v1alpha1`, Kind `NodeWright`; `DeploymentPolicy` moves to the same group) are published under `nodewright`. Existing `skyhook.nvidia.com`/`Skyhook` resources keep working during the transition: the operator auto-imports them to NodeWright and preserves per-node state (no package re-run), and legacy writes emit a deprecation warning. The agent image and the default install namespace (`skyhook`) still use `skyhook` for now. See the [migration guide](docs/getting-started/migration.md).
>
> **Distribution change (v0.16.0+):** NodeWright is now distributed exclusively through GitHub Container Registry (`ghcr.io`) — both the container images and the Helm chart (as an OCI artifact). Publication to `nvcr.io` / the NGC Helm repository (`helm.ngc.nvidia.com`) is paused and is planned to return in a future release. **Existing users installing from NGC need to switch to the OCI install below.** See [Distribution: ghcr.io only (for now)](docs/contributing/release-process.md#distribution-ghcrio-only-for-now) for the full story.

## Why NodeWright?

Managing and updating Kubernetes clusters is challenging. While Kubernetes advocates treating compute as disposable, but certain scenarios make this difficult:

- **Updating hosts without re-imaging:**
  - Limited excess hardware/capacity for rolling replacements
  - Long node replacement times (example can be hours in some cloud providers)
- **OS image management:**
  - Maintain a common base image with workload-specific overlays instead of multiple OS images
- **Workload sensitivity:**
  - Some workloads can't be moved, are difficult to move, or take a long time to migrate

## What is NodeWright?

NodeWright functions like a package manager but for your entire Kubernetes cluster, with three main components:

1. **NodeWright Operator** - Manages installing, updating, and removing packages
2. **NodeWright Custom Resource** - Declarative definitions of changes to apply
3. **Packages** - The actual modifications you want to implement

## Where and When to use NodeWright

NodeWright works in any Kubernetes environment (self-managed, on-prem, cloud) and shines when you need:

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
- [**Runtime Required**](docs/user-guide/runtime-required.md): requires node to come into the cluster with a taint, and will do work prior to removing custom taint.
- **Resource Management:** NodeWright uses Kubernetes [LimitRange](https://kubernetes.io/docs/concepts/policy/limit-range/) to set default CPU and memory requests/limits for all containers in its namespace. You can override these defaults per-package in your NodeWright CR. Strict validation is enforced: if you set any resource override, you must set all four fields (cpuRequest, cpuLimit, memoryRequest, memoryLimit), and limits must be >= requests. See [docs/operations/resource-management.md](docs/operations/resource-management.md) for details and examples.
- [**Explicit Uninstall**](docs/user-guide/uninstall.md): controlled, explicit uninstall of packages from nodes with `uninstall.enabled` and `uninstall.apply` fields, webhook guards, finalizer-driven cleanup on CR deletion, and cancel support.

## Pre-built Packages

There are a few pre-built generalist packages available at [NVIDIA/skyhook-packages](https://github.com/NVIDIA/skyhook-packages)

## Installation via Helm

Install NodeWright quickly using Helm without downloading the repository:

### Prerequisites

- Kubernetes cluster (tested on v1.30+)
- Helm 3.x installed
- Container registry access credentials (if using private registries)

### Install NodeWright

```bash
# The chart is distributed as an OCI artifact on GitHub Container Registry.
# Helm 3.8+ supports OCI natively — no `helm repo add` needed.
helm install nodewright oci://ghcr.io/nvidia/nodewright/charts/nodewright \
  --version v0.17.1 \
  --namespace nodewright \
  --create-namespace
```

> **Where things live:** chart at `oci://ghcr.io/nvidia/nodewright/charts/nodewright`, operator image at `ghcr.io/nvidia/nodewright/operator`, agent image at `ghcr.io/nvidia/nodewright/agent`. NGC / `nvcr.io` distribution is paused — see [docs/contributing/release-process.md#distribution-ghcrio-only-for-now](docs/contributing/release-process.md#distribution-ghcrio-only-for-now).
>
> **Migrating from `helm repo add skyhook https://helm.ngc.nvidia.com/...`?** Run `helm repo remove skyhook` and use the OCI install above. If you also want to keep the existing in-cluster release name (e.g. `skyhook`), substitute it for `nodewright` in the `helm install` command — the chart works either way.
>
> **Already installed in the `skyhook` namespace?** Stay there. The documented namespace for **new** installs moved from `skyhook` to `nodewright`, but Kubernetes namespaces cannot be renamed in place and Helm cannot move a release between namespaces, so there is nothing to migrate and no deadline. `kubectl nodewright` finds the operator in either namespace automatically. See [docs/getting-started/migration.md#install-namespace](docs/getting-started/migration.md#install-namespace-skyhook---nodewright).

### Configure Image Pull Secrets (if needed)

If you're using private container registries, create the necessary secrets:

```bash
kubectl create secret generic node-init-secret \
  --from-file=.dockerconfigjson=${HOME}/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson \
  --namespace nodewright
```

**Note:** NodeWright currently uses a single shared image pull secret for all packages, and agent/operator containers. If you need access to multiple registries, combine the credentials into one `dockerconfigjson` secret with multiple registry auths.

### Verify Installation

```bash
# Check that the operator is running
kubectl get pods -n nodewright

# or Wait for the deployment to be available first
kubectl wait --for=condition=Available deployment -l control-plane=controller-manager -n nodewright --timeout=300s

# Then wait for the operator pod to be ready
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n nodewright --timeout=300s

# Verify the Ready condition
kubectl get pods -l control-plane=controller-manager -n nodewright -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}'

# Verify the CRDs are installed
kubectl get crd | grep nodewright

# Verify packages are working
kubectl apply -f - <<EOF
apiVersion: nodewright.nvidia.com/v1alpha1
kind: NodeWright
metadata:
  name: nodewright-sample
spec:
  nodeSelectors:
    matchExpressions:
      - key: node-role.kubernetes.io/control-plane
        operator: DoesNotExist
  packages:
    something-important:
      version: 1.1.1
      image: ghcr.io/nvidia/skyhook-packages/shellscript
      configMap:
        apply.sh: |-
          #!/bin/bash
          echo "hello world" > /skyhook-hello-world
          sleep 10
        apply_check.sh: |-
          #!/bin/bash
                     cat /skyhook-hello-world | wc -l | grep -q 1
           sleep 10
EOF

# Wait for the NodeWright to complete
kubectl wait --for=jsonpath='{.status.status}'=complete nodewright/nodewright-sample --timeout=300s

# Check the status
kubectl describe nodewright nodewright-sample
```

### Uninstalling

**Automatic Cleanup (Default):** By default, the Helm chart includes a pre-delete hook that automatically cleans up all NodeWright and DeploymentPolicy resources before uninstalling:

```bash
# Uninstall the chart (cleanup happens automatically)
helm uninstall nodewright --namespace nodewright
```

The pre-delete hook will:

- Delete all NodeWright resources
- Delete all DeploymentPolicy resources  
- Complete quickly if no resources exist
- Wait for finalizers to be processed if resources exist
- Proceed with uninstall even if cleanup times out (job deadline: 2 minutes)

**Configuration Options:**

To disable automatic cleanup and manage resources manually:

```bash
helm install nodewright ./chart --namespace nodewright --set cleanup.enabled=false
```

To adjust the job timeout:

```bash
helm install nodewright ./chart --namespace nodewright \
  --set cleanup.jobTimeoutSeconds=180
```

**Manual Cleanup (if needed):**

If you disabled automatic cleanup or need to clean up resources manually:

```bash
# Delete all NodeWright resources first
kubectl delete nodewrights.nodewright.nvidia.com --all
kubectl delete deploymentpolicies.nodewright.nvidia.com --all

# Delete all DeploymentPolicy resources
kubectl delete deploymentpolicies --all

# Then uninstall the chart
helm uninstall nodewright --namespace nodewright
```

**Why cleanup matters:** If you uninstall while NodeWright CRs with finalizers still exist, it can leave resources in a broken state that may cause reinstall issues.

## Monitoring and Troubleshooting

### Watch NodeWright apply packages

```
kubectl get pods -w -n nodewright
```
There will be a pod for each lifecycle stage (apply, config, etc.) per package per node matching the selector.

### Check NodeWright resource status

```bash
# Check overall status
kubectl get nodewrights

# Get detailed status of a specific NodeWright
kubectl describe nodewright <nodewright-name>
```
The Status will show the overall package status as well as the status of each node

### Check node annotations for package state

```bash
# View node state annotations for a specific NodeWright
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{": "}{.metadata.annotations.nodewright\.nvidia\.com/nodeState_<nodewright-name>}{"\n"}{end}'
```

### Stages

The operator will apply steps in a package throughout different lifecycle stages. This ensures that the right steps are applied in the right situations and in the correct order.

- Upgrade: This stage runs whenever a package's version is upgraded in the NodeWright CR.
- Uninstall: This stage runs only when explicitly requested — either by setting `uninstall.apply: true` on a package with `uninstall.enabled: true`, or during NodeWright CR deletion (finalizer-driven) for `uninstall.enabled: true` packages. See [Explicit Uninstall](docs/user-guide/uninstall.md).
- Apply: This stage will always be ran at least once.
- Config: This stage will run when a configmap is changed and on the first SCR application.
- Interrupt: This stage will run when a package has an interrupt defined or a key's value in a packages configmap changes which has a config interrupt defined.
- Post-Interrupt: This stage will run when a package's interrupt has finished.

The stages are applied in this order:

**Without Interrupts:**

- Uninstall -> Apply -> Config (No Upgrade)
- Upgrade -> Config (With Upgrade)

**With Interrupts:**
For packages that require interrupts, the node is first cordoned and drained to ensure workloads are safely evacuated before package operations begin:

- Uninstall -> Apply -> Config -> Interrupt -> Post-Interrupt (No Upgrade)
- Upgrade -> Config -> Interrupt -> Post-Interrupt (With Upgrade)

This ensures that when operations like kernel module unloading or system reboots are required, they happen after workloads have been safely removed and any necessary pre-interrupt package operations have completed.

**NOTE**: Uninstall is explicit. Removing a package from the SCR does **not** automatically run the uninstall stage. For packages with `uninstall.enabled: true`, the webhook rejects removal until `uninstall.apply: true` has been set and the uninstall has completed on all nodes; packages with `uninstall.enabled: false` (or unset) can be removed without an uninstall pod, and their prior node state is preserved as a marker. See [Explicit Uninstall](docs/user-guide/uninstall.md).

**Semantic versioning is strictly enforced in the operator** in order to support upgrade and uninstall. Semantic versioning allows the
operator to know which way the package is going while also enforcing best versioning practices.

**For detailed information about our versioning strategy, git tagging conventions, and component release process, see [docs/operations/versioning.md](docs/operations/versioning.md) and [docs/contributing/release-process.md](docs/contributing/release-process.md).**

**For definitions of Status, State, and Stage concepts used throughout the operator, see [docs/architecture/operator-status.md](docs/architecture/operator-status.md).**

## Packages

Part of how the operator works is the [NodeWright agent](agent/README.md). Packages have to be created in way so the operator knows how to use them. This is where the agent comes into play, more on that later. A package is a container that meets these requirements:

- Container shall have `bash`, so needs to be at least something like busybox/alpine
- Config that is valid, jsonschema is used to valid this config. The agent has a tool build in to valid the config. This tool should be used to test packages before publishing.
- The file system structure needs to adhere to:

```
/skyhook-package
├── skyhook_dir/{steps}
├── root_dir/{static files}
└── config.json
```

When a NodeWright's `configMap` is set, the operator projects each key as an individual file at `/skyhook-package/configmaps/<key>`. These files overlay any files the package image baked in under that directory rather than replacing the whole directory: a package can ship default files there and a user's `configMap` overrides only the keys it supplies. (Because the files are mounted via `subPath`, live ConfigMap edits are not propagated into a running pod, but package pods are recreated per stage and on version bumps.)

## Examples

See the [examples/](examples/) directory for sample manifests, usage patterns, and demo configurations to help you get started with NodeWright.

## Kyverno Policy Examples

See [docs/security/kyverno/README.md](docs/security/kyverno/README.md) for example Kyverno policies and guidance on restricting images or packages in NodeWright resources.

## [NodeWright Operator](operator/README.md)

The operator is a Kubernetes operator that monitors cluster events and coordinates the installation and lifecycle of NodeWright packages.

## [NodeWright Agent](agent/README.md)

The agent is what does the operator's work and is a separate container from the package. The agent knows how to read a package (/skyhook_package/config.json) and implements the [lifecycle](#stages) packages go through.

## [NodeWright CLI](docs/user-guide/cli.md)

A kubectl plugin for managing NodeWright deployments, packages, and nodes. Provides SRE tooling for inspecting node/package state, forcing re-runs, managing node lifecycle, and retrieving logs.

### Quick Install

```bash
# Build from source
cd operator
make build-cli

# Install as kubectl plugin
cp bin/nodewright /usr/local/bin/kubectl-nodewright # or another directory in $PATH with write access

# Verify installation
kubectl nodewright version
```

See the [full CLI documentation](docs/user-guide/cli.md) for detailed usage and examples.

## Contributing

- Start here: [CONTRIBUTING.md](CONTRIBUTING.md)
- Code of Conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)

## Governance & Maintainers

- Governance: [GOVERNANCE.md](GOVERNANCE.md) - roles, how decisions get made, how maintainers join and leave
- Maintainers: [MAINTAINERS.md](MAINTAINERS.md) - current roster and how to become one
- Review ownership: [`.github/CODEOWNERS`](.github/CODEOWNERS)

## Security

Please report security vulnerabilities through [NVIDIA's Security Vulnerability Form](https://www.nvidia.com/object/submit-security-vulnerability.html). Do **not** file public GitHub issues for security reports. See [SECURITY.md](SECURITY.md) for details.

## Support

- **Support Level:** Maintained
- **How to get help:** [GitHub Issues](https://github.com/NVIDIA/nodewright/issues)
- See [SUPPORT.md](SUPPORT.md) for more details.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
