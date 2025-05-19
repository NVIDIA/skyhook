# Skyhook Operator Kustomize Config

This directory contains Kustomize manifests and overlays used for development and testing of the Skyhook operator.

**Note:**
- The only recommended installation method for production or end users is the [Helm chart](../../chart/README.md).
- The Kustomize setup here is primarily for local development, CI, and testing.
- If you are installing Skyhook in a real cluster, please use the Helm chart.

## config

Almost everything in this directory is generated code from comments in the code. Currently the only exception is the `samples` directory.


The following make commands up date these config files.
```
❯ make help
...

Development
  manifests        Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
  generate         Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.

...
```