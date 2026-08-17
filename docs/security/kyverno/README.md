# Kyverno Policy Examples for NodeWright

This directory contains example [Kyverno](https://kyverno.io/) policies for use with NodeWright. These are **not installed by default** and are provided as templates for users to adapt to their own security needs.

- `disable_packages.yaml`: Example policy to restrict or disable certain NodeWright packages/images.
- `skyhook-viewer-binding.yaml`: Example RBAC binding for Kyverno to view NodeWright resources.

**Note:**

- This directory was previously at the repo root and has been moved to `docs/kyverno/` for clarity.
- If you use these policies, ensure you enable the `skyhook-viewer-role` in your Helm values and bind Kyverno to that role.

See the main [README](../README.md) for more information about NodeWright. 
