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
> **Migrating from `helm repo add skyhook https://helm.ngc.nvidia.com/...`?** Run `helm repo remove skyhook` and use the OCI install above. If you want to keep the existing in-cluster release name (e.g. `skyhook`), substitute it for `nodewright` in the `helm install` command — the chart works either way.
>
> **Already installed in the `skyhook` namespace?** Stay there. The documented namespace for **new** installs moved from `skyhook` to `nodewright`, but Kubernetes namespaces cannot be renamed in place and Helm cannot move a release between namespaces, so there is nothing to migrate and no deadline. `kubectl nodewright` finds the operator in either namespace automatically. See [Install namespace](migration.md#install-namespace-skyhook---nodewright).

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

The pre-delete hook will:

- Delete all NodeWright resources
- Delete all DeploymentPolicy resources
- Complete quickly if no resources exist
- Wait for finalizers to be processed if resources exist
- Proceed with uninstall even if cleanup times out (job deadline: 2 minutes)

### Cleanup options

To disable automatic cleanup and manage resources manually:

```bash
helm install nodewright oci://ghcr.io/nvidia/nodewright/charts/nodewright \
  --namespace nodewright --set cleanup.enabled=false
```

To adjust the cleanup job timeout:

```bash
helm install nodewright oci://ghcr.io/nvidia/nodewright/charts/nodewright \
  --namespace nodewright --set cleanup.jobTimeoutSeconds=180
```

### Manual cleanup

If you disabled automatic cleanup, or need to clean up by hand:

```bash
# Delete all NodeWright and DeploymentPolicy resources first
kubectl delete nodewrights.nodewright.nvidia.com --all
kubectl delete deploymentpolicies.nodewright.nvidia.com --all

# Then uninstall the chart
helm uninstall nodewright --namespace nodewright
```

**Why cleanup matters:** uninstalling while NodeWright CRs with finalizers still exist can leave resources in a broken state that causes problems on reinstall.

For more details on explicit package uninstall, see [Uninstall](../user-guide/uninstall.md).

## Related

- [Kubernetes support matrix](../operations/kubernetes-support.md)
- [Versioning](../operations/versioning.md)
- [Migration from Skyhook](migration.md)
