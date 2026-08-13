# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the operator.
For the full commit-level log see CHANGELOG.md.

## Unreleased

### Breaking Changes

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
  - The legacy `skyhook.nvidia.com` group remains available for a multi-release,
    adoption-gated migration window and is removed in a later release.
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

- **Package stages now execute as `batch/v1` Jobs**, one per (NodeWright,
  package, stage, node), instead of operator-managed raw pods. The lifecycle
  state machine is unchanged — stages, Status/State derivation, interrupt /
  cordon / drain sequencing and DeploymentPolicy all behave as before — but the
  executor is now a Job, so `kubectl logs job/<name>` works during and after a
  run and finished work is retained instead of deleted on completion.
- **Failure logs survive the failure.** Each genuinely failing stage keeps up to
  two full-log archive pods (its first failure and its most recent), including an
  attempt killed by its own `stageTimeout` — the deadline fails the pod in place
  rather than deleting it, so its logs are read like any other failed attempt's.
  An interrupt Job, which keeps a whole-stage deadline, is the case where the pod
  *is* deleted; there the operator falls back to a best-effort ~16KiB log tail in
  the Job's `nodewright.nvidia.com/last-logs` annotation.
- **New per-package `stageTimeout`** bounds the wall-clock runtime of one attempt
  at each of a package's stages. A hung stage (stuck script, unpullable image,
  dead registry) is killed and retried instead of sitting `in_progress`
  indefinitely. Unset uses the operator default; `"0"` removes the time bound.
- **`pause` is now a true stop.** The pause annotation cascades to `spec.suspend`
  on the NodeWright's unfinished Jobs, so the stage that is *currently executing*
  is stopped rather than being allowed to finish. Resuming re-runs that stage
  from the start of its current phase; the agent skips already-completed steps.
  `disable` is unchanged and still does not stop in-flight work.
- Add `spec.drainConfig` so interrupt drains can tune eviction, direct deletion,
  emptyDir handling, unmanaged-pod handling, DaemonSet skipping, timeout, and
  grace-period behavior.

### Changed

- **A failing stage now gives up after a bounded number of retries.** Package
  stage Jobs run with `backoffLimit` = `JOB_BACKOFF_LIMIT` (default 3), so a
  stage runs at most four times before it is surfaced as `erroring` and left
  alone until a `package rerun`/`reset`, a config or spec change, or the failed
  Job's TTL expires. **This shortens self-healing from up to an hour to roughly
  70 seconds** for a package that previously rode out transient environment
  problems (a registry blip, a mount not up yet) by crash-looping until it
  succeeded. If you depend on that behavior, raise `JOB_BACKOFF_LIMIT` (chart
  value `controllerManager.manager.env.jobBackoffLimit`).
  - Not every failed attempt counts against the package: pods lost to eviction,
    preemption or node shutdown are ignored, and attempts the kubelet refused to
    admit spend the budget but never mark a package `erroring` on their own.
- **Finished Jobs are retained and then cleaned up on a timer**, controlled by
  `JOB_TTL_SUCCEEDED` (default 1h) and `JOB_TTL_FAILED` (default 24h) — failure
  logs outlive success logs. Both have a hard minimum of one minute; the
  operator refuses to start below it, so `"0"` is not a way to disable retention.
- Editing `stageTimeout`, `JOB_STAGE_TIMEOUT` or `JOB_BACKOFF_LIMIT` applies to
  the *next* stage Job, not one already running (a Job's pod template is
  immutable). To apply a new value to work already under way, clear that Job
  with `kubectl nodewright package rerun <package>`. Editing the **package
  itself** does clear a stage that has already given up, so fixing a broken
  image or config takes effect without waiting for the failure TTL.
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
