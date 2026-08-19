# Installation

NodeWright is installed via Helm as an OCI artifact from GitHub Container Registry.

## Prerequisites

- Kubernetes cluster (tested on v1.30+)
- Helm 3.8+ installed (required for native OCI support)
- Container registry access credentials (if using private registries)

## Configure Image Pull Secrets (if needed)

If you're using private container registries, create the namespace and secret before installing so the release can reference it:

```bash
kubectl create namespace nodewright

kubectl create secret generic node-init-secret \
  --from-file=.dockerconfigjson=${HOME}/.docker/config.json \
  --type=kubernetes.io/dockerconfigjson \
  --namespace nodewright
```

NodeWright currently uses a single shared image pull secret for all packages, and agent/operator containers. If you need access to multiple registries, combine the credentials into one `dockerconfigjson` secret with multiple registry auths.

## Install NodeWright

```bash
# The chart is distributed as an OCI artifact on GitHub Container Registry.
# Helm 3.8+ supports OCI natively — no `helm repo add` needed.
helm install nodewright oci://ghcr.io/nvidia/nodewright/charts/nodewright \
  --version v0.18.0 \
  --namespace nodewright \
  --create-namespace \
  --set imagePullSecret=node-init-secret
```

Omit `--set imagePullSecret=node-init-secret` if you're pulling from public registries only.

> **Where things live:** chart at `oci://ghcr.io/nvidia/nodewright/charts/nodewright`, operator image at `ghcr.io/nvidia/nodewright/operator`, agent image at `ghcr.io/nvidia/nodewright/agent`.
> **Migrating from `helm repo add skyhook https://helm.ngc.nvidia.com/...`?** Run `helm repo remove skyhook` and use the OCI install above.

## Verify Installation

```bash
# Check that the operator is running
kubectl get pods -n nodewright

# Wait for the deployment to be available
kubectl wait --for=condition=Available deployment -l control-plane=controller-manager -n nodewright --timeout=300s

# Wait for the operator pod to be ready
kubectl wait --for=condition=Ready pod -l control-plane=controller-manager -n nodewright --timeout=300s

# Verify the CRDs are installed
kubectl get crd | grep nodewright
```

## Uninstalling

By default, the Helm chart includes a pre-delete hook that automatically cleans up all NodeWright and DeploymentPolicy resources before uninstalling:

```bash
helm uninstall nodewright --namespace nodewright
```

For more details on explicit package uninstall, see [Uninstall](../user-guide/uninstall.md).

## Related

- [Kubernetes support matrix](../operations/kubernetes-support.md)
- [Versioning](../operations/versioning.md)
- [Migration from Skyhook](migration.md)
