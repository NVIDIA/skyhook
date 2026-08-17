# Installation

NodeWright is installed via Helm as an OCI artifact from GitHub Container Registry.

For prerequisites, install commands, and configuration options, see the [project README](../../README.md#installation-via-helm).

## Quick install

```bash
helm install nodewright oci://ghcr.io/nvidia/nodewright/charts/nodewright \
  --version v0.17.1 \
  --namespace nodewright \
  --create-namespace
```

## Related

- [Kubernetes support matrix](../operations/kubernetes-support.md)
- [Versioning](../operations/versioning.md)
- [Uninstall](../user-guide/uninstall.md)
