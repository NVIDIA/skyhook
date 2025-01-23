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