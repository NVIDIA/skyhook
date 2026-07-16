# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the CLI.
For the full commit-level log see CHANGELOG.md.

## Unreleased

### Breaking Changes

- The CLI now targets the `nodewright.nvidia.com` API group and the `NodeWright`
  kind, and it reads and writes `nodewright.nvidia.com/*` node annotations. It no
  longer works against operators that only serve the legacy `skyhook.nvidia.com`
  group and `Skyhook` kind. Migration: upgrade the operator to a
  NodeWright-capable version before using this CLI; an older Skyhook-only
  operator is unsupported.

### Changed

- `reset` and `node reset` now select nodes with any resettable Skyhook
  metadata, including status, cordon, and drain-start metadata. Nodes with a
  malformed `nodeState` annotation are reset with an empty package state instead
  of being skipped.
