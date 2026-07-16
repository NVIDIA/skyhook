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
> **Scope of the current release.** This guide is written against the full migration flow, but not all of
> it has shipped yet. Available **now**: the `nodewright.nvidia.com` API group and CRDs, the mirror
> controller that pre-creates a `NodeWright`/`DeploymentPolicy` for each legacy object, new-group admission
> webhooks, legacy deprecation warnings, the primary reconciler reconciling `NodeWright`, the on-node
> annotation/label/condition re-keying to the `nodewright.nvidia.com/*` prefix on upgrade, the runtime
> migration hold that waits while any legacy `Skyhook` is non-complete, and the `migrate` CLI command
> (`kubectl skyhook migrate`, pending the plugin binary rename). **Not yet available** (planned, do not rely
> on it today): the Helm chart's `nodewright.nvidia.com` CRDs/RBAC and its pre-upgrade safety hook, the
> plugin binary rename, and safe-deletion tooling. Sections below flag these inline; where a step depends
> on an unshipped piece, rewrite the CRs by hand for now.

## TL;DR

1. **Upgrade the operator.** It automatically mirrors every existing `Skyhook` into a `NodeWright` (and
   each legacy `DeploymentPolicy` into a new-group `DeploymentPolicy`). You do **not** touch your CRs for
   this step. **Planned, not yet in this release:** migration of the per-node state (annotations, package
   pods, ConfigMaps) to the new names. Until that ships, the `NodeWright` starts from a fresh per-node
   state, so **do not** assume "no package re-runs" yet; treat the current mirror as a way to stand up the
   new objects, not to hand off live node lifecycle position.
2. **Rename your CRs** (in git for GitOps, or by re-applying for Helm/manual): change `apiVersion` and
   `kind`, keep the same `metadata.name`. (`kubectl nodewright migrate` will generate the new manifests for
   you once it ships; until then, edit `apiVersion`/`kind` by hand.)
3. **Delete the old `Skyhook`/`DeploymentPolicy` CRs**, but only after confirming the corresponding
   `NodeWright` exists and is being reconciled (see [safe deletion](#safe-deletion-ordering)). Delete the
   legacy `DeploymentPolicy` objects too. The mirrored `NodeWright` objects are the real, reconciled
   resources and are never deleted by this step.

Do steps 2 and 3 **as one change** (a rename = delete old + add new together). **Perform the operator
upgrade during a quiet window** with no active package rollout (see [the prerequisite](#prerequisite-all-skyhooks-must-be-complete)).

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

**Planned, not yet in this release:** separate migration of per-node state to the new prefix
(`nodeState_`, `status_`, `version_`, `cordon_`, node conditions) and replacement of the old
`skyhook.nvidia.com/`-labeled package pods and per-node ConfigMaps with new-labeled equivalents owned by
the `NodeWright`. Until that lands, the mirror creates the `NodeWright` object but does **not** carry over
node lifecycle position, so a migrated `NodeWright` may re-run package stages on its nodes.

Checking migration status: a `Skyhook` is migrated when a `NodeWright` of the same name with the
`nodewright.nvidia.com/mirrored-from` stamp exists:

```bash
kubectl get nodewrights.nodewright.nvidia.com -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.nodewright\.nvidia\.com/mirrored-from}{"\n"}{end}'
```

---

## Argo CD (GitOps)

The key property: because the operator **pre-creates** the `NodeWright`, when your git later declares it,
Argo finds it already present and **adopts** it (applies its tracking label, shows in-sync) rather than
creating a duplicate.

1. **Upgrade the operator** through its own Argo Application (bump the chart/image). The mirror imports
   your existing `Skyhook`s into `NodeWright`s. At this point the `NodeWright`s exist but are not part of
   any Argo Application (untracked) - Argo neither prunes nor flags them, and your `Skyhook` Application
   stays in-sync because the mirror never writes to your `Skyhook`s.

2. **Rename the CRs in git as a single commit.** For each `Skyhook`, change:
   - `apiVersion: skyhook.nvidia.com/v1alpha1` -> `nodewright.nvidia.com/v1alpha1`
   - `kind: Skyhook` -> `kind: NodeWright`
   - keep `metadata.name` identical.

   Same for `DeploymentPolicy` (only `apiVersion` changes; `kind` stays `DeploymentPolicy`). Edit
   `apiVersion`/`kind` by hand for now; `kubectl nodewright migrate -f <old-manifest.yaml>` will generate
   the new manifests once that command ships. The commit should **remove** the old `Skyhook` manifest and
   **add** the `NodeWright` manifest so Argo prunes the old and adopts the new in one sync.

3. **Sync.** Argo adopts the existing `NodeWright` (in-sync, no re-create) and prunes the `Skyhook`. Once
   the `Skyhook` is gone, the mirror has nothing left to import for it.

**Do not** leave the old `Skyhook` in git after adding the `NodeWright`: in that window both the mirror
and Argo write the `NodeWright`, and editing it via git could be stomped by the mirror re-importing the
stale `Skyhook`. Renaming in one commit avoids this entirely.

If you keep CRDs in a separate Argo Application: during the transition it should track both CRDs. **Note:**
shipping the `nodewright.nvidia.com` CRDs and RBAC in the operator Helm chart is **not yet done** in this
release; until it lands, apply the new-group CRDs from `operator/config/crd/bases/` yourself. Do not remove
the `skyhook.nvidia.com` CRD until every `Skyhook` is migrated and deleted (removing the CRD
cascade-deletes any remaining `Skyhook`s). See [Removal](#removal-future-release).

---

## Helm (`helm upgrade`)

Helm here refers to installing the **operator** via its chart. Your `Skyhook`/`NodeWright` **instances**
are usually authored separately (not part of the operator chart); adjust to your setup.

1. **`helm upgrade` the operator chart.** The mirror imports your existing `Skyhook`s into `NodeWright`s.
   **Not yet in this release:** the chart does **not** yet ship the `nodewright.nvidia.com` CRDs and RBAC
   for both groups; apply the new-group CRDs from `operator/config/crd/bases/` yourself until that lands.
   Per-node state migration is likewise not yet implemented (see above).

2. **Update your CR manifests to `NodeWright`** wherever you keep them, and apply. Edit `apiVersion`/`kind`
   by hand for now; the `kubectl nodewright migrate` helper below is not yet available:
   ```bash
   # once shipped: kubectl nodewright migrate -f my-skyhook.yaml > my-nodewright.yaml
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

1. Upgrade the operator (apply the new manifests / image). The mirror imports your `Skyhook`s. Apply the
   `nodewright.nvidia.com` CRDs from `operator/config/crd/bases/` yourself: the chart does not ship them
   yet.
2. Write the new CRs by hand (change `apiVersion`/`kind`, keep `metadata.name`) and apply them. The
   `kubectl nodewright migrate` generator is planned but not yet available:
   ```bash
   # once shipped: kubectl nodewright migrate > nodewrights.yaml   # reads legacy CRs from the cluster
   kubectl apply -f nodewrights.yaml                                # adopts the mirrored objects
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

Delete legacy `DeploymentPolicy` objects as part of the same pass: they move group too, and a stale legacy
`DeploymentPolicy` left behind will keep emitting deprecation warnings and block eventual removal of the
`skyhook.nvidia.com` CRD.

---

## Prerequisite: all Skyhooks must be complete

**Perform the operator upgrade only when every `Skyhook` is `complete` with no nodes in progress**, i.e.
no package is mid-rollout. This is a **requirement**, not just a recommendation: once the per-node state
migration ships it will rename the operator's package pods and per-node ConfigMaps, and the flow assumes
there is no in-flight package work to disrupt. Even in this release, where node state is not yet migrated,
upgrading against idle Skyhooks avoids a migrated `NodeWright` re-running a stage that was mid-rollout.

Check before upgrading:

```bash
kubectl get skyhooks.skyhook.nvidia.com -o custom-columns=NAME:.metadata.name,STATUS:.status.status,INPROGRESS:.status.nodesInProgress
```

Proceed only when `STATUS` is `complete` and `INPROGRESS` is `0` for **all** Skyhooks.

The operator enforces this at runtime as a fail-safe. On startup, while any legacy `Skyhook` reports a
status that is set and not `complete` (i.e. a rollout was in flight when you upgraded), the operator
**holds**: it does not take over any node, requeues every 20 seconds, and logs a warning naming the
non-complete Skyhooks. This prevents the new operator from double-driving a node the pre-rename operator
may still be mutating. It is a stop, not an auto-resume: because a legacy `Skyhook`'s status is frozen
once the operator stops reconciling that kind, the hold clears only when the legacy Skyhooks genuinely
read `complete`. If you upgraded mid-rollout, roll back to the pre-rename operator, let the rollout
finish, then upgrade again. A legacy `Skyhook` created *after* the upgrade (with no status, mirrored into
a `NodeWright`) does not trigger the hold.

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
- (Planned, once per-node state migration ships) node state carried over: a node's annotations are re-keyed
  under `nodewright.nvidia.com/` and no package re-ran. This does **not** hold in the current release.
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

**Will my nodes reboot during the upgrade?** Once per-node state migration ships, no (unless a package
interrupt was mid-flight, and even then the agent skips an already-completed interrupt). In the current
release node state is **not** migrated, so a mirrored `NodeWright` can re-run package stages on its nodes;
upgrade against idle, `complete` Skyhooks to minimize this.

**Does the operator write to my `Skyhook` objects?** No. The mirror is read-only on the source, so GitOps
tools see no drift. "Migrated" is derived from the existence of the stamped `NodeWright`.

**What if I create a new `Skyhook` after upgrading?** It still works (the CRD is served during the
transition) and gets mirrored, but you will get a deprecation warning. Prefer creating `NodeWright`
directly.

**Can I roll the operator back?** During the transition release both groups are served, so a rollback to a
version that still reconciles `Skyhook` is possible while the `Skyhook` objects still exist. Once you have
deleted the `Skyhook`s and are on `NodeWright` only, rolling back to a Skyhook-only operator would strand
the `NodeWright`s. Keep the `Skyhook`s until you are confident.
