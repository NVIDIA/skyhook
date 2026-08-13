# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the operator.
For the full commit-level log see CHANGELOG.md.

## Unreleased

### Other Changes

- **The documented install namespace for new deployments is now `nodewright`, not
  `skyhook`.** The kustomize overlay moved from `skyhook-operator-system` to
  `nodewright-operator-system`, and the operator's `NAMESPACE` env default (used only
  when nothing sets it, such as a bare binary or `make run`) moved from `skyhook` to
  `nodewright`.

  **Existing installs need no action, and there is no deadline.** Namespaces cannot be
  renamed in place and Helm cannot move a release between namespaces, so an install in
  `skyhook` stays supported indefinitely. The chart sources the namespace from
  `.Release.Namespace` throughout and installs into any namespace, so `helm upgrade`
  against a `skyhook`-namespace release is unaffected. The chart always sets `NAMESPACE`
  explicitly, so the default change is invisible to chart users.

  `kubectl nodewright` discovers the operator's namespace rather than assuming one, so
  it keeps working against a `skyhook`-namespace install. See
  [docs/nodewright-migration.md](../docs/nodewright-migration.md#install-namespace-skyhook---nodewright).

### Deprecations

- **The `skyhook_*` metrics are deprecated in favour of `nodewright_*`, and the
  `skyhook_name` series label in favour of `nodewright_name`.** Both sets are now
  published side by side.

  **No action is required at upgrade time, and nothing breaks.** Metric names and
  label keys are the identifiers users bake into Grafana dashboards, Prometheus
  alerting rules, and recording rules, so the rename is a dual-publish rather than
  an in-place swap: every `skyhook_*` series continues to be exported with the same
  value and the same remaining labels as its `nodewright_*` twin.

  Migrating a query is a two-token swap, for example
  `skyhook_node_status_count{skyhook_name="x"}` becomes
  `nodewright_node_status_count{nodewright_name="x"}`. The `skyhook_*` help text in
  `/metrics` names its replacement and the removal release, so the exposition itself
  documents the mapping.

  **The legacy set is removed in operator v0.20.0**, the same release that removes
  the legacy `skyhook.nvidia.com` API group, so there is one deadline rather than
  two. Update dashboards and alerts before then. See
  [docs/metrics/README.md](../docs/metrics/README.md) and the
  [migration guide](../docs/nodewright-migration.md#metrics).

  Note this roughly doubles the operator's exported series count for the duration of
  the window; see [docs/operator_resources_at_scale.md](../docs/operator_resources_at_scale.md).
  If you have already migrated your dashboards, or never consumed the legacy names,
  set `PUBLISH_LEGACY_METRICS=false` (chart:
  `controllerManager.manager.env.publishLegacyMetrics`) to drop the deprecated half
  immediately. It defaults to `true`, so upgrading without setting it keeps the
  compatibility window.

### Breaking Changes

- **The default runtime-required taint key moves from `skyhook.nvidia.com` to
  `nodewright.nvidia.com`.** `RUNTIME_REQUIRED_TAINT` now defaults to
  `nodewright.nvidia.com=runtime-required:NoSchedule`.

  **This is a coordinated change, not a silent default bump.** The taint key is a
  contract with infrastructure the operator cannot see: cluster autoscaler and
  Karpenter node pools, machine/node templates, `--register-with-taints` kubelet
  arguments, and tolerations on your own workloads all name it. A node that comes
  up carrying a key the operator does not recognise is never untainted, and sits
  unschedulable — a cluster-down failure mode for anyone using
  `autoTaintNewNodes`, not a cosmetic break.

  For the deprecation window the operator therefore recognises **both** keys. It
  **applies** only the configured taint, but **tolerates** and **removes** the
  legacy `skyhook.nvidia.com=runtime-required:NoSchedule` taint as well, and
  treats a node already carrying either key as gated so `autoTaintNewNodes` does
  not stamp a second taint. Existing taints on existing nodes are not rewritten.
  A cluster whose provisioner still applies the legacy key keeps working with no
  change on your side.

  **What you must do:** update the taint key in your autoscaler / node-pool /
  machine-template configuration, and in any workload tolerations that name it,
  before operator v0.20.0, after which the legacy key is neither tolerated nor
  removed. To stay on the old key in the meantime, set it explicitly via
  `controllerManager.manager.env.runtimeRequiredTaint`. See
  [docs/runtime_required.md](../docs/runtime_required.md#taint-key-rename-skyhooknvidiacom---nodewrightnvidiacom).

- **The primary CRD is renamed from `Skyhook` (`skyhook.nvidia.com/v1alpha1`) to
  `NodeWright` (`nodewright.nvidia.com/v1alpha1`), and `DeploymentPolicy` moves to
  the same `nodewright.nvidia.com` group.** This is the operator side of the
  Skyhook to NodeWright rename.
  - **Migration is automatic for the common case.** The transition release serves
    both API groups. A mirror controller imports every existing `skyhook.nvidia.com`
    `Skyhook`/`DeploymentPolicy` into its `nodewright.nvidia.com` equivalent and
    reconciles the NodeWright as the source of truth. Per-node package state is
    **copied** to the new prefix (`skyhook.nvidia.com/*` node annotations, labels,
    and conditions gain `nodewright.nvidia.com/*` equivalents) while the legacy
    keys are **kept** for a rollback window, so **packages are not re-run** and
    in-progress rollouts keep their position. The legacy copies are pruned only
    after `LEGACY_CLEANUP_DELAY` (default 24h) has elapsed.
  - **Legacy `Skyhook`/`DeploymentPolicy` objects become read-only once migrated:**
    the admission webhook rejects spec and `pause`/`disable` edits (deletions and
    identical re-applies are still allowed) and emits a deprecation warning.
    Operate the mirrored `NodeWright` instead; to move your manifests forward,
    change `apiVersion` to `nodewright.nvidia.com/v1alpha1` (and, for `Skyhook`,
    `kind` to `NodeWright`) and re-apply.
  - **Upgrade during a quiet window:** perform the upgrade only when every Skyhook
    is `complete` with no nodes in progress. In-flight stages resume idempotently
    (no double reboot), but upgrading idle avoids even the benign package-pod
    restart.
  - The legacy `skyhook.nvidia.com` group remains available for a two-minor-release
    migration window spanning v0.18.x and v0.19.x, and is removed in operator v0.20.0.
  - **See [docs/nodewright-migration.md](../docs/nodewright-migration.md)** for the
    full migration guide, the pre-upgrade check, and a verification checklist.

### Bug Fixes

- **The migration now touches only the keys the operator owns, not the whole
  `skyhook.nvidia.com/` prefix.** The metadata prefix is the product's domain name,
  so users legitimately carry their own node labels and annotations under it. The
  migration previously copied *every* prefixed key to `nodewright.nvidia.com/*` and
  then deleted every prefixed key at prune time, which silently duplicated user data
  into the operator's namespace and destroyed the originals 24h after an upgrade. It
  now migrates only the operator's own keys (`nodeState_`, `status_`, `version_`,
  `cordon_`, `drainStart_`, `autoTaint_` annotations; the `status_` and
  operator-defined `ignore` labels; the `<name>/NotReady` and `<name>/Erroring`
  conditions) and leaves everything else under the prefix untouched.

- **Deleting a migrated legacy `Skyhook` no longer cascade-deletes the
  `NodeWright`'s ConfigMaps.** The migration relabelled the pre-rename ConfigMaps but
  left their `ownerReferences` pointing at the legacy `Skyhook`, so the documented
  "delete the old CRs" step garbage-collected the package and per-node metadata
  ConfigMaps out from under the live `NodeWright`. The operator recreated the package
  ConfigMap, but a package pod scheduled in that window would reference a missing
  one. The converge now re-parents these ConfigMaps onto the `NodeWright`.

- **Upgrading from a pre-rename operator no longer wedges on the package
  ConfigMap.** The migration relabelled only the per-node metadata ConfigMaps
  (`skyhook.nvidia.com/skyhook-node-meta`) and missed the package ConfigMaps,
  which carry `skyhook.nvidia.com/name`. Because the post-rename reconciler lists
  package ConfigMaps by the new `nodewright.nvidia.com/name` label, it could not
  see the existing one, tried to create it, and failed permanently with
  `configmaps "<name>-<package>-<version>" already exists` - leaving the
  `NodeWright` stuck short of `complete` behind an exponential backoff. Both
  legacy label keys now converge (and both are dropped at prune time).

- **Package `image` must be a bare registry/repository reference; inline tags
  and digests are now rejected.** `image` carries neither a tag nor a digest:
  `version` supplies the tag (and is the operator's ordering key for
  upgrade/downgrade detection) and `containerSHA` pins the exact bytes the
  kubelet pulls.
  - An inline digest (`image: repo@sha256:...`) was previously accepted but
    never deployable: with `containerSHA` unset the operator appended the
    version tag to an image that already had a digest, producing an unpullable
    reference like `repo@sha256:...:1.2.2`. It is now rejected with a message
    pointing at `containerSHA`.
  - **Behavior change:** an inline *tag* (`image: repo:1.2.2`) was previously
    absorbed silently as a leftover migration off the pre-semver scheme. That
    migration is complete and has been removed, so an inline tag is now rejected
    the same way a digest is, even when it matches `version`. If your manifests
    still embed a tag in `image`, drop it (the package `version` becomes the
    tag). Existing stored CRs are unaffected: their tags were already stripped
    on write by the old migration, so only re-applying a manifest that still
    carries an inline tag will be rejected.
  - `image` is also validated by the CRD schema (apiserver-enforced) to be
    non-empty with no whitespace, and `containerSHA`, when set, must be a
    well-formed digest (`sha256:` followed by 64 lowercase hex characters).

- **Reapply-on-reboot dropped on busy nodes.** With `REAPPLY_ON_REBOOT=true`, a
  reboot of a node under heavy controller churn (frequent pod/label/annotation
  updates) could be detected and then silently lost: the per-node state reset
  was persisted with a full `Update` that lost an optimistic-concurrency race,
  yet the node's boot id was advanced anyway, marking the reboot handled. The
  node kept its stale `complete` state and the package was never reapplied
  (`unknown -> complete`, no pod). The reset now persists via a strategic-merge
  `Patch` (not resourceVersion-gated, like the rest of the reconcile), and the
  boot id is advanced only after that write succeeds, so a failed reset leaves
  the reboot pending to be retried. Also fixes `Reset()` deleting the cordon
  annotation with a key missing the Skyhook name.

### New Features

- Add `spec.drainConfig` so interrupt drains can tune eviction, direct deletion,
  emptyDir handling, unmanaged-pod handling, DaemonSet skipping, timeout, and
  grace-period behavior.

### Changed

- **Metrics now use controller-runtime's built-in authentication and
  authorization filter.** The operator serves TLS metrics directly on `:8443`
  with controller-runtime's in-memory self-signed certificate. The manager
  performs TokenReview and SubjectAccessReview calls in process; the metrics
  reader role and `/metrics` authorization contract are unchanged. Scrapers
  must disable certificate verification because the per-pod certificate has no
  stable, published CA.

- Align the default interrupt drain pod filter with Kubernetes drain semantics:
  already-terminating pods and mirror/static pods are skipped, unschedulable
  tolerations use Kubernetes `ToleratesTaint` matching, and DaemonSet pods are
  identified from the controller owner reference instead of the previous owner
  reference count heuristic.

## operator/v0.16.1 - 2026-05-22

### Bug Fixes

- **Webhook serving-cert bootstrap deadlock on major upgrade.** During an
  upgrade from older versions, an old-version leader holding the main
  reconcile lease could deadlock the new webhook's serving-cert bootstrap
  and prevent the new controller from coming up. The bootstrap now runs
  under its own dedicated `ctrl.Manager` with a separate leader-election
  lease (`nodewright-webhook-bootstrap.nvidia.com`), so it no longer
  contends with the main reconcile lease and the upgrade proceeds even
  while the old leader still holds the primary lease (#243).

## operator/v0.16.0 - 2026-05-19 — Explicit Uninstall

Introduces an opt-in declarative uninstall workflow and reworks how downgrades
and CR deletion behave. Affects the Operator, Webhook, and CRD.

### New Features

- Add a standard `Ready` condition to Skyhook status for native Kubernetes wait and GitOps health tooling.

### New behavior

- **`uninstall.enabled` / `uninstall.apply` on each package.** Setting
  `apply: true` (requires `enabled: true`) triggers an uninstall pod on every
  target node, running `uninstall.sh` / `uninstall-check.sh` from the package's
  ConfigMap (or the agentless equivalent) with the full package configuration
  (env, resources, volumes).
- **Interrupt after uninstall.** Packages with an `interrupt:` block (reboot,
  service restart, etc.) now run that interrupt *after* the uninstall pod
  completes, via a new `StageUninstallInterrupt` stage on `PackageStatus`. The
  new stage is distinct from the install-cycle `StageInterrupt` so the two can
  never be confused.
- **Finalizer-driven cleanup on CR delete.** Deleting a `Skyhook` CR now blocks
  on uninstall completion for every `enabled: true` package before the
  finalizer clears. Uncordon, labels, annotations, and per-node ConfigMaps are
  cleaned up automatically.
- **`UninstallInProgress` and `UninstallFailed` status conditions** report the
  state of in-flight uninstall work.
- **`Blocked` status condition** is emitted when a package depends on another
  package that is currently uninstalling (DAG dependency safety).
- **Spec-change pod recreation.** Editing an explicit-uninstall package's
  ConfigMap or env while the uninstall pod is failing causes the operator to
  recreate the pod with the new config — fixes can be rolled forward without
  manual pod deletion, even on a CR that is being deleted.

### Removed / changed behavior

- **Removing a package from `spec.packages` no longer triggers an uninstall.**
  For `enabled: false` (or unset) packages, the package's entry is **left in
  the node state annotation** (`skyhook.nvidia.com/nodeState_<name>`) — no
  uninstall pod runs and nothing on the node is cleaned up, so the persistent
  state entry signals to operators that the package's files are still on the
  node. For `enabled: true` packages, the webhook now **rejects** removal
  until the package has been explicitly uninstalled on all nodes.
- **Downgrades are gated.** The webhook rejects a version downgrade unless the
  OLD spec already had `uninstall.apply: true` AND the package is absent from
  every tracked node's state. The old "downgrade auto-triggers an uninstall
  pod" path is removed. For `enabled: false` packages, downgrades are accepted
  but the old version's node-state entry is preserved (D2 semantics: absent =
  cleanly uninstalled; non-absent = not cleanly uninstalled, just superseded).
  Upgrades are unchanged.
- **`apply: true` with `enabled: false`** is rejected by the webhook.

### Deprecations

- Deprecated prefixed Skyhook status condition types such as `skyhook.nvidia.com/Ready`, `skyhook.nvidia.com/Transition`, and `skyhook.nvidia.com/TaintNotTolerable`; bare condition types such as `Ready` and `TaintNotTolerable` are now emitted alongside the legacy names for one release.

### Migration

See [`docs/uninstall.md`](../docs/uninstall.md) for the API reference, workflow
examples, cancellation semantics, webhook rules, and migration guidance from
the previous remove-from-spec behavior.
