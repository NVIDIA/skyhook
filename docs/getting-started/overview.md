# NodeWright Overview

NodeWright is a Kubernetes-aware package manager for cluster administrators to safely modify and maintain underlying hosts declaratively at scale.

For a full introduction — what NodeWright is, why it exists, where it fits, and its key features — see the [project README](../../README.md#what-is-nodewright).

## Components

1. **NodeWright Operator** — manages installing, updating, and removing packages
2. **NodeWright Custom Resource** — declarative definitions of changes to apply
3. **Packages** — the actual modifications you want to implement

Pre-built packages are available at [NVIDIA/skyhook-packages](https://github.com/NVIDIA/skyhook-packages).

## Next steps

- [Installation](installation.md) — install NodeWright via Helm
- [Migration from Skyhook](migration.md) — rename transition guide
- [CLI reference](../user-guide/cli.md) — `kubectl nodewright` commands
