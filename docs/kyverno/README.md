# Kyverno Policy Examples for Skyhook

This directory contains example [Kyverno](https://kyverno.io/) policies for use with Skyhook. These are **not installed by default** and are provided as templates for users to adapt to their own security needs.

- `disable_packages.yaml`: Example policy to restrict or disable certain Skyhook packages/images.
- `skyhook-viewer-binding.yaml`: Example RBAC binding for Kyverno to view Skyhook resources.

**Note:**
- This directory was previously at the repo root and has been moved to `docs/kyverno/` for clarity.
- If you use these policies, ensure you enable the `skyhook-viewer-role` in your Helm values and bind Kyverno to that role.

See the main [README](../README.md) for more information about Skyhook. 