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

### Added

- New `migrate` command converts legacy `skyhook.nvidia.com` objects (`Skyhook`,
  `DeploymentPolicy`) into their `nodewright.nvidia.com` equivalents and prints
  them as a multi-document YAML stream on stdout, suitable for `kubectl apply -f`
  or GitOps. It reads from files or stdin with `-f/--filename` (offline, no
  cluster required) or, with no `-f`, lists the legacy objects from the current
  cluster. Server-managed fields (`status`, `resourceVersion`, `uid`,
  `creationTimestamp`) are stripped so the output is a clean, apply-able
  manifest.

### Changed

- Cluster-backed commands now run a preflight check on the served API groups.
  When the cluster serves only the legacy `skyhook.nvidia.com` group and not
  `nodewright.nvidia.com`, they fail fast with a clear, actionable error naming
  both groups and telling you to upgrade to a NodeWright-capable operator,
  instead of a confusing `NotFound`. The `migrate` command is exempt: it reads
  legacy objects by design.

- `reset` and `node reset` now select nodes with any resettable Skyhook
  metadata, including status, cordon, and drain-start metadata. Nodes with a
  malformed `nodeState` annotation are reset with an empty package state instead
  of being skipped.
