# Skyhook Versioning Strategy

Skyhook uses independent versioning for three components, all following [Semantic Versioning](https://semver.org/):

1. **Operator** - Kubernetes operator that manages Skyhook resources
2. **Agent** - Container that executes package operations on nodes  
3. **Chart** - Helm chart for deploying the operator
   1. NOTE: the versioning of the chat also includes versioning of the expected agent version. 
   2. NOTE: If changing either the operator or agent this will generally include a chart release too.

## Git Tagging Convention

```bash
operator/v{version}    # Operator releases
agent/v{version}       # Agent releases  
chart/v{version}       # Chart releases
```

## Component Versioning

### Operator & Agent
- **Independent versioning** with their own release cycles
- **Semantic versioning**: MAJOR.MINOR.PATCH
- **Compatibility**: Maintained through well-defined interfaces

### Helm Chart  
- **Independent from operator/agent** (starting at v0.8.0 these will start to diverge in version number)
- **Chart version** (`version`): Tracks chart template/config changes
- **App version** (`appVersion`): Recommended stable operator version

## Chart Behavior

### Chart.yaml
example:
```yaml
version: v1.0.0        # Chart version (independent)
appVersion: v0.7.0   # Recommended operator version
```

### Image Tag Defaults
```yaml
# values.yaml
image:
  tag: ""  # Empty = defaults to Chart.AppVersion

# Template renders as:
image: "ghcr.io/nvidia/skyhook/operator:0.7.0"
```

## Quick Reference

```bash
# Check deployed versions
kubectl get deployment -n skyhook -o jsonpath='{.items[0].spec.template.spec.containers[0].image}'
helm list -n skyhook

# Override operator version
helm install skyhook ./chart --set controllerManager.manager.image.tag="0.8.0"
```

## Release Process

For step-by-step instructions on how to release components, see [release-process.md](release-process.md).

**CI/CD triggers on git tags:**
- `operator/vx.y.z` → publishes operator image
- `agent/vx.y.z` → publishes agent image  
- `chart/vx.y.z` → publishes helm chart

**Chart versioning:**
- **PATCH**: Bug fixes, docs
- **MINOR**: New features, config options  
- **MAJOR**: Breaking changes to chart, or compatibility with agent or operator.

