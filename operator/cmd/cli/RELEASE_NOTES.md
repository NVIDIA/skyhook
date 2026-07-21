# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the CLI.
For the full commit-level log see CHANGELOG.md.

## Unreleased

### Breaking Changes

- The plugin is now invoked as `kubectl nodewright` (binary `kubectl-nodewright`)
  instead of `kubectl skyhook`. Reinstall the plugin under the new name; update any
  scripts or aliases that call `kubectl skyhook`.
- The CLI now targets the `nodewright.nvidia.com` API group and the `NodeWright`
  kind, and it reads and writes `nodewright.nvidia.com/*` node annotations. It no
  longer works against operators that only serve the legacy `skyhook.nvidia.com`
  group and `Skyhook` kind. Migration: upgrade the operator to a
  NodeWright-capable version before using this CLI; an older Skyhook-only
  operator is unsupported.

### Changed

- Commands that read or write `NodeWright` resources now run a preflight check on
  the served API groups. When the cluster serves only the legacy
  `skyhook.nvidia.com` group and not `nodewright.nvidia.com`, they fail fast with
  a clear, actionable error naming both groups and telling you to upgrade to a
  NodeWright-capable operator, instead of a confusing `NotFound`. `version` does
  not run the preflight.

- `reset` and `node reset` now select nodes with any resettable Skyhook
  metadata, including status, cordon, and drain-start metadata. Nodes with a
  malformed `nodeState` annotation are reset with an empty package state instead
  of being skipped.
