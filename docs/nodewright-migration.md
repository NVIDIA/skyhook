# Migrating from Skyhook to NodeWright

> **STATUS: DRAFT.** This guide describes the in-progress `skyhook.nvidia.com` -> `nodewright.nvidia.com`
> API rename. It is written against the mirror-based upgrade flow. Do not treat version numbers here as
> final.
>
> **BREAKING CHANGE.** The primary CRD moves group and Kind: `skyhook.nvidia.com/v1alpha1 Skyhook` ->
> `nodewright.nvidia.com/v1alpha1 NodeWright`. `DeploymentPolicy` moves group (same Kind name). The
> on-node annotation/label/finalizer prefix moves `skyhook.nvidia.com/* -> nodewright.nvidia.com/*`, and
> the kubectl plugin is renamed `kubectl skyhook -> kubectl nodewright`.
>
> **Scope of the current release.** This guide is written against the full migration flow, and most of it
> has shipped. Available **now**: the `nodewright.nvidia.com` API group and CRDs, the mirror controller
> that pre-creates a `NodeWright`/`DeploymentPolicy` for each legacy object, new-group admission webhooks,
> legacy deprecation warnings, the primary reconciler reconciling `NodeWright`, the on-node
> annotation/label/condition **copy** to the `nodewright.nvidia.com/*` prefix on upgrade (so packages are
> **not** re-run), the deferred rollback-safe cleanup of the legacy-labeled package pods and per-node
> ConfigMaps (gated by `LEGACY_CLEANUP_DELAY`), the runtime migration hold that waits while any legacy
> `Skyhook` is non-complete, the `kubectl nodewright migrate`
> CLI command (the plugin binary is renamed to `nodewright`), and the Helm chart shipping both groups'
> CRDs and RBAC. **Not yet available** (planned for the removal release, do not rely on it today): the
> chart's pre-upgrade safety hook that refuses to drop the legacy CRD while legacy objects remain. Sections
> below flag this inline.

## TL;DR

1. **Upgrade the operator.** It automatically mirrors every existing `Skyhook` into a `NodeWright` (and
   each legacy `DeploymentPolicy` into a new-group `DeploymentPolicy`). You do **not** touch your CRs for
   this step. On upgrade the operator copies the per-node state (annotations, conditions) to the
   `nodewright.nvidia.com/*` prefix, so the migrated `NodeWright` picks up live node lifecycle position and
   **packages are not re-run**. The old `skyhook.nvidia.com/*` state is kept for a rollback window and
   pruned later (see [Rollback window](#rollback-window-legacy_cleanup_delay)).
2. **Rename your CRs** (in git for GitOps, or by re-applying for Helm/manual): change `apiVersion` and
   `kind`, keep the same `metadata.name`. (`kubectl nodewright migrate` generates the new manifests for
   you, or edit `apiVersion`/`kind` by hand.)
3. **Delete the old `Skyhook`/`DeploymentPolicy` CRs**, but only after confirming the corresponding
   `NodeWright` exists and is being reconciled (see [safe deletion](#safe-deletion-ordering)). Delete the
   legacy `DeploymentPolicy` objects too. The mirrored `NodeWright` objects are the real, reconciled
   resources and are never deleted by this step.

**Recommended runbook: migrate on a quiesced cluster.** In order: **quiesce** (every `Skyhook` `complete`,
including no `disabled`/`paused`/`blocked` ones, and no lifecycle changes in flight) -> **upgrade** the
operator -> **rename** the CRs atomically (steps 2 and 3 as one change: a rename = delete old + add new
together) -> **delete** the legacy CRs promptly. The sharp edges this guide warns about (a wedged migration
hold, non-propagated `pause`/`disable`, a GitOps fight during a non-atomic rename) all come from deviating
from that steady state.

## What the operator does automatically

On upgrade, a one-way, level-triggered **mirror controller** runs:

- For each legacy `Skyhook`/`DeploymentPolicy`, it creates the equivalent `nodewright.nvidia.com` object,
  stamped `nodewright.nvidia.com/mirrored-from: <legacy-name>`.
- It is **read-only on your legacy objects** (never writes to them), so GitOps tools see **no drift** on
  the CRs you still manage.
- It **backs off** if a `NodeWright` of that name already exists that it did not create (a user/GitOps
  managed object always wins).
- It **never deletes** a `NodeWright`, even when the legacy object is deleted; once mirrored, the
  `NodeWright` holds live workload state.

Legacy `Skyhook`/`DeploymentPolicy` writes return a deprecation warning pointing here.

**Operate the `NodeWright`, not the legacy `Skyhook`, during the bridge.** The mirror propagates only spec
changes (it reacts to a legacy object's `metadata.generation` advancing). The annotation-driven controls
`pause` and `disable` do **not** bump the generation, so toggling them on a legacy `Skyhook` will **not**
reach its `NodeWright`. Run every lifecycle operation (`pause`, `disable`, `resume`, `enable`) against the
`NodeWright` with `kubectl nodewright …`. Editing a legacy object's spec still works during the transition
but is discouraged (it re-syncs onto the `NodeWright` and can fight GitOps; see [Argo CD](#argo-cd-gitops)).

Alongside the mirror, the operator adopts the per-node state under the new prefix. It **copies** the legacy
per-node metadata (`nodeState_`, `status_`, `version_`, `cordon_` annotations, status labels, and node
conditions) to the `nodewright.nvidia.com/*` prefix and adds a `nodewright.nvidia.com` label to the
per-node ConfigMaps, so the migrated `NodeWright` picks up live lifecycle position and does **not** re-run
package stages. It does this **additively**: the legacy `skyhook.nvidia.com/*` copies, the pre-rename
package pods, and the legacy ConfigMap labels are **kept** so a rollback stays possible, and are pruned
later, not on upgrade (see [Rollback window](#rollback-window-legacy_cleanup_delay)).

Checking migration status: a `Skyhook` is migrated when a `NodeWright` of the same name with the
`nodewright.nvidia.com/mirrored-from` stamp exists:

```bash
kubectl get nodewrights.nodewright.nvidia.com -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.nodewright\.nvidia\.com/mirrored-from}{"\n"}{end}'
```

---

## Argo CD (GitOps)

The key property: because the operator **pre-creates** the `NodeWright`, when your git later declares it,
Argo finds it already present and **adopts** it (applies its tracking label, shows in-sync) rather than
creating a duplicate. This assumes Argo's default `label` resource-tracking method; if you run with
`annotation` or `annotation+label` tracking (`application.resourceTrackingMethod` in `argocd-cm`), the same
adoption still applies but via the tracking annotation. Orphaned-resource monitoring on the `AppProject`
is separate: it only controls warnings about untracked objects and does not affect adoption.

1. **Upgrade the operator** through its own Argo Application (bump the chart/image). The mirror imports
   your existing `Skyhook`s into `NodeWright`s. At this point the `NodeWright`s exist but are not part of
   any Argo Application (untracked) - Argo neither prunes nor flags them, and your `Skyhook` Application
   stays in-sync because the mirror never writes to your `Skyhook`s.

2. **Rename the CRs in git as a single commit.** For each `Skyhook`, change:
   - `apiVersion: skyhook.nvidia.com/v1alpha1` -> `nodewright.nvidia.com/v1alpha1`
   - `kind: Skyhook` -> `kind: NodeWright`
   - keep `metadata.name` identical.

   Same for `DeploymentPolicy` (only `apiVersion` changes; `kind` stays `DeploymentPolicy`). Run
   `kubectl nodewright migrate -f <old-manifest.yaml>` to generate the new manifests, or edit
   `apiVersion`/`kind` by hand. The commit should **remove** the old `Skyhook` manifest and **add** the
   `NodeWright` manifest so Argo prunes the old and adopts the new in one sync.

3. **Sync.** Argo adopts the existing `NodeWright` (in-sync, no re-create) and prunes the `Skyhook`. Once
   the `Skyhook` is gone, the mirror has nothing left to import for it.

**Do not** leave the old `Skyhook` in git after adding the `NodeWright`: in that window both the mirror
and Argo write the `NodeWright`, and editing it via git could be stomped by the mirror re-importing the
stale `Skyhook`. Renaming in one commit avoids this entirely.

If you keep CRDs in a separate Argo Application: during the transition it should track both CRDs. The
operator Helm chart ships both groups' CRDs and RBAC, so an operator upgrade installs the
`nodewright.nvidia.com` CRDs for you. Do not remove the `skyhook.nvidia.com` CRD until every `Skyhook` is
migrated and deleted (removing the CRD cascade-deletes any remaining `Skyhook`s). See
[Removal](#removal-future-release).

---

## Helm (`helm upgrade`)

Helm here refers to installing the **operator** via its chart. Your `Skyhook`/`NodeWright` **instances**
are usually authored separately (not part of the operator chart); adjust to your setup.

1. **`helm upgrade` the operator chart.** The mirror imports your existing `Skyhook`s into `NodeWright`s.
   The chart ships the CRDs and RBAC for **both** groups, so the upgrade installs the
   `nodewright.nvidia.com` CRDs for you. Per-node state is migrated on upgrade (see above), so packages are
   not re-run.

2. **Update your CR manifests to `NodeWright`** wherever you keep them, and apply. Run
   `kubectl nodewright migrate` to generate the new manifests, or edit `apiVersion`/`kind` by hand:
   ```bash
   kubectl nodewright migrate -f my-skyhook.yaml > my-nodewright.yaml
   kubectl apply -f my-nodewright.yaml
   ```
   Because the operator already created the `NodeWright` via the mirror, this apply is a no-op adoption of
   the existing object (same name, same spec) rather than a fresh create.

3. **Delete the old CRs** once you have confirmed the `NodeWright` exists and reconciles (see
   [safe deletion](#safe-deletion-ordering)):
   ```bash
   kubectl delete skyhook.skyhook.nvidia.com <name>
   kubectl delete deploymentpolicy.skyhook.nvidia.com <name>   # legacy DeploymentPolicies too
   ```
   The `NodeWright` remains (the mirror never deletes it).

**Planned, not yet in this release:** at the **removal** release the chart is intended to add a pre-upgrade
safety hook (`legacy.blockUpgradeIfPresent`) that **aborts the upgrade** if any legacy
`Skyhook`/`DeploymentPolicy` still exist, so you cannot accidentally drop the old CRD (and cascade-delete
live objects) before migrating. That hook is not implemented yet; until it ships, verify manually that no
legacy objects remain before removing the legacy CRD.

---

## Manual / kubectl

1. Upgrade the operator (apply the new manifests / image, or `helm upgrade` the chart, which ships both
   groups' CRDs and RBAC). The mirror imports your `Skyhook`s and the per-node state is migrated on
   upgrade.
2. Generate the new CRs with `kubectl nodewright migrate` (or write them by hand: change `apiVersion`/`kind`,
   keep `metadata.name`) and apply them:
   ```bash
   kubectl nodewright migrate > nodewrights.yaml   # reads legacy CRs from the cluster
   kubectl apply -f nodewrights.yaml               # adopts the mirrored objects
   ```
3. Delete the old CRs only after confirming the `NodeWright` reconciles (see
   [safe deletion](#safe-deletion-ordering)); delete legacy `DeploymentPolicy` objects too:
   ```bash
   kubectl delete skyhooks.skyhook.nvidia.com --all
   kubectl delete deploymentpolicies.skyhook.nvidia.com --all
   ```

---

## Safe deletion ordering

Delete a legacy `Skyhook` (or `DeploymentPolicy`) **only after** you have confirmed its `NodeWright`
counterpart exists and is being reconciled. The mirror never deletes a `NodeWright`, but deleting the
legacy object before the new one is live loses your intent. For each object:

1. Confirm the mirrored `NodeWright` exists and carries the `nodewright.nvidia.com/mirrored-from` stamp
   (see the status check above).
2. Confirm it is reconciling (its `.status` is populated and it is not erroring).
3. Then delete the legacy `Skyhook`/`DeploymentPolicy`.

A legacy `Skyhook` created by the pre-rename operator carries the old `skyhook.nvidia.com/skyhook`
finalizer, so `kubectl delete` briefly parks the object in `Terminating` until the mirror strips that
finalizer (the post-rename operator manages a finalizer only on `NodeWright`). This is automatic and
usually completes in seconds; you do not need to remove the finalizer by hand. It also means deletion
only completes while the operator is running, so delete legacy objects **before** you scale the operator
down for the final removal step.

Delete legacy `DeploymentPolicy` objects as part of the same pass: they move group too, and a stale legacy
`DeploymentPolicy` left behind will keep emitting deprecation warnings and block eventual removal of the
`skyhook.nvidia.com` CRD.

**Ordering within the pass matters:** the legacy `DeploymentPolicy` admission webhook rejects deletion
while any legacy `Skyhook` still references it. Delete the referencing `Skyhook`s **first**, then the
`DeploymentPolicy`. The Helm and Manual command sequences above already delete `skyhooks` before
`deploymentpolicies` for this reason.

---

## Prerequisite: all Skyhooks must be complete

**Perform the operator upgrade only when every `Skyhook` is `complete` with no nodes in progress**, i.e.
no package is mid-rollout. This is a **requirement**, not just a recommendation: the per-node state
migration renames the operator's package pods and per-node ConfigMaps, and the flow assumes there is no
in-flight package work to disrupt. Upgrading against idle Skyhooks avoids handing a migrated `NodeWright`
a stage that was mid-rollout.

Check before upgrading:

```bash
kubectl get skyhooks.skyhook.nvidia.com -o custom-columns=NAME:.metadata.name,STATUS:.status.status,INPROGRESS:.status.nodesInProgress
```

Proceed only when `STATUS` is `complete` and `INPROGRESS` is `0` for **all** Skyhooks.

**This includes `disabled`, `paused`, and `blocked` Skyhooks.** The runtime hold treats *any* non-`complete`
status as in-flight work, and after the upgrade the pre-rename operator is gone, so a frozen `disabled` or
`paused` status never advances on its own. A single such legacy `Skyhook` wedges the **entire** NodeWright
migration indefinitely, and the only symptom is a log line: the NodeWrights simply sit idle. Before
upgrading, enable-and-complete or delete any `disabled`/`paused`/`blocked` Skyhook (they show up in the
`STATUS` column above). If you have already upgraded and a migration is wedged, **delete** the offending
legacy `Skyhook` (its status cannot advance without the old operator) to clear the hold.

The operator enforces this at runtime as a fail-safe. On startup, while any legacy `Skyhook` reports a
status that is set and not `complete` (i.e. a rollout was in flight when you upgraded), the operator
**holds**: it does not take over any node, requeues every 20 seconds, and logs a warning naming the
non-complete Skyhooks. This prevents the new operator from double-driving a node the pre-rename operator
may still be mutating. It is a stop, not an auto-resume: because a legacy `Skyhook`'s status is frozen
once the operator stops reconciling that kind, the hold clears only when the legacy Skyhooks genuinely
read `complete`. If you upgraded mid-rollout, roll back to the pre-rename operator, let the rollout
finish, then upgrade again. A legacy `Skyhook` created *after* the upgrade (with no status, mirrored into
a `NodeWright`) does not trigger the hold.

## Rollback window (`LEGACY_CLEANUP_DELAY`)

The migration hold above keeps rollback safe while a rollout was *in flight* at upgrade; the rollback
window keeps it safe *after* a Skyhook has finished migrating. When the operator adopts a node it
**copies** the legacy `skyhook.nvidia.com/*` annotations, labels, and conditions to the
`nodewright.nvidia.com/*` prefix but **keeps the legacy copies**, and it leaves the pre-rename package pods
in place. So a rolled-back pre-rename operator still finds its state and resumes cleanly, and a bad upgrade
stays recoverable even after migration completes.

The legacy copies (and the old package pods, and the legacy ConfigMap labels) are pruned only once
`LEGACY_CLEANUP_DELAY` has elapsed since the Skyhook was adopted, tracked by a
`nodewright.nvidia.com/legacy-migrated-at` timestamp on the `NodeWright`. The delay defaults to **24h**
(`controllerManager.manager.env.legacyCleanupDelay` in the chart). Set it to `0` to prune immediately (no
rollback window), or longer to keep the door open longer. Pruning is the **point of no return**: after it
runs, rolling back to the pre-rename operator would re-apply every package from scratch.

This is transition-only behavior and is removed together with the `skyhook.nvidia.com` group at the
removal release.

## Downstream consumers (e.g. aicr)

Projects that ship or depend on Skyhook CRs need a coordinated update. For example
[aicr](https://github.com/NVIDIA/aicr) (Argo-based) references Skyhook in several places; the equivalent
changes for any such consumer are:

- **CRs you deploy** (e.g. aicr's `recipe.yaml` `Skyhook`): rename `apiVersion` + `kind` to
  `nodewright.nvidia.com/v1alpha1` / `NodeWright` (keep the name). Because the operator pre-creates the
  `NodeWright`, Argo adopts it on next sync.
- **CRD readiness waits / assertions** (e.g. chainsaw `assert-crds.yaml`, waits on
  `skyhooks.skyhook.nvidia.com` Established): point at `nodewrights.nodewright.nvidia.com`.
- **Cleanup tooling** referencing the `skyhook.nvidia.com` group: add/switch to `nodewright.nvidia.com`.

These land as a **companion PR in the consumer repo**, timed with (or shortly after) the operator upgrade.

## Verification checklist

- `kubectl get nodewrights.nodewright.nvidia.com` lists a `NodeWright` for each former `Skyhook`.
- Node state carried over: a node's annotations are copied under `nodewright.nvidia.com/` (the legacy
  `skyhook.nvidia.com/` keys remain during the rollback window) and no package re-ran.
- No drift reported by Argo on your (soon-to-be-deleted) `Skyhook` objects during the transition.
- After you delete the old CRs, the `NodeWright` objects remain and reconcile normally.
- Legacy `Skyhook`/`DeploymentPolicy` writes emit the deprecation warning.

## Removal (future release)

The legacy `skyhook.nvidia.com` group is kept for a multi-release migration window (targeting roughly two
releases, adoption-gated - not a fixed release number). In the removal release:

- The legacy admission webhook flips from warning to **denying** writes.
- The legacy CRDs are removed from the chart/manifests, sequenced behind a preflight that refuses removal
  while any legacy objects remain (the Helm/Argo pre-upgrade hook, planned above).
- Removing the `skyhook.nvidia.com` CRD cascade-deletes any remaining `Skyhook`s, so **migrate before the
  removal release**.

## FAQ

**Will my nodes reboot during the upgrade?** No (unless a package interrupt was mid-flight, and even then
the agent skips an already-completed interrupt). Per-node state is migrated and legacy workloads are
cleaned up, so packages are not re-run; still upgrade against idle, `complete` Skyhooks as required by the
prerequisite above.

**Does the operator write to my `Skyhook` objects?** No. The mirror is read-only on the source, so GitOps
tools see no drift. "Migrated" is derived from the existence of the stamped `NodeWright`.

**What if I create a new `Skyhook` after upgrading?** It still works (the CRD is served during the
transition) and gets mirrored, but you will get a deprecation warning. Prefer creating `NodeWright`
directly.

**Can I roll the operator back?** Yes, within the rollback window. The operator keeps the legacy node
state, package pods, and `Skyhook` CRs during the transition, so rolling back to a version that reconciles
`Skyhook` resumes cleanly. Two things close the door: the per-node prune after `LEGACY_CLEANUP_DELAY`
(default 24h; see [Rollback window](#rollback-window-legacy_cleanup_delay)), after which a rolled-back
operator sees fresh nodes and re-applies packages; and deleting the `Skyhook`s, which strands the
`NodeWright`s. Keep the `Skyhook`s (and raise `LEGACY_CLEANUP_DELAY`) until you are confident.
