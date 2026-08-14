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

- **The `--namespace` default moved from `skyhook` to `nodewright`, and the CLI now
  discovers the operator's namespace instead of assuming it.** The documented install
  namespace moved with the rename, but a namespace cannot be renamed in place, so
  installs that predate the rename legitimately stay in `skyhook` forever. A blind
  default change would have answered "not found" for every one of them.

  With no `--namespace`, the CLI now checks `nodewright`, then `skyhook` (printing a
  one-line note to stderr when the legacy namespace is what answered), then sweeps
  cluster-wide for an operator in some other namespace, and only then falls back to
  `nodewright` so the command's own lookup reports the specific miss. An explicit
  `--namespace` is always used verbatim, with no discovery and no note.

  Within the set of operators this CLI supports at all (NodeWright-capable ones, per
  the Breaking Changes above), the namespace change introduces **no** new
  incompatibility: the namespace is only used to locate the operator Deployment and
  package pods, and the `NodeWright` and `DeploymentPolicy` CRDs are cluster-scoped,
  so no command changes which CRs it sees. This says nothing about Skyhook-only
  operators, which remain unsupported for unrelated reasons. See
  [docs/cli.md](../../../docs/cli.md#namespace-resolution) for the full resolution
  order and the situation matrix.

  The cluster-wide sweep needs cluster-scoped Deployment list permission. Users
  without it lose only that last discovery step; the `nodewright`/`skyhook` probes
  and explicit `--namespace` are unaffected.

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
