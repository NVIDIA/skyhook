# Skyhook Kyverno Policies

This directory contains example [Kyverno](https://kyverno.io/) policies for Skyhook. These policies can be used to enforce security and best practices for Skyhook packages.

## Prerequisites

Before applying any policies, you need to have Kyverno installed in your cluster. You can install it using one of the following methods:

### Helm Installation (Recommended) 

```bash
helm repo add kyverno https://kyverno.github.io/kyverno/
helm install kyverno kyverno/kyverno -n kyverno --create-namespace
```

### Manual Installation

```bash
kubectl apply -f https://raw.githubusercontent.com/kyverno/kyverno/main/definitions/install.yaml
```

## Available Policies

### Restrict Package Images
The `disable_packages.yaml` policy demonstrates how to restrict which container images can be used in Skyhook packages. This is particularly useful for:
- Preventing the use of potentially dangerous images (e.g., those containing shell scripts)
- Enforcing the use of approved container registries
- Maintaining security standards across your cluster

To apply the policy:

```bash
kubectl apply -f disable_packages.yaml
```

The policy will prevent the creation of Skyhook resources that contain packages with restricted image patterns. Currently, it blocks:
- Images containing 'shellscript' anywhere in the image name
- Images from Docker Hub (matching 'docker.io/*')

## Testing the Policy

You can test the policy by trying to create a Skyhook resource with a restricted image. For example:

```yaml
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  labels:
    app.kubernetes.io/part-of: skyhook-operator
    app.kubernetes.io/created-by: skyhook-operator
  name: test-scr
spec:
  packages:
    shellscript:
      configMap:
        config.sh: |-
          #!/bin/bash
          echo "hello"
      image: shellscript
      version: 1.3.2

 # This will be blocked by the policy
```

The creation will be denied with an appropriate error message.

## Customizing Policies

The example policies are templates that you can modify to fit your security needs. Common customizations include:
- Adding additional restricted image patterns
- Modifying the validation rules
- Adjusting the failure action (warn vs enforce)

See the [Kyverno documentation](https://kyverno.io/docs/) for more details on policy customization.

