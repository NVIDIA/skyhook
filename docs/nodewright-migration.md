# Migrating from Skyhook to NodeWright

> **STATUS: DRAFT.** This guide describes the in-progress `skyhook.nvidia.com` -> `nodewright.nvidia.com`
> API rename. It is written against the mirror-based upgrade flow. Do not treat version numbers here as
> final.

> **BREAKING CHANGE.** The primary CRD moves group and Kind: `skyhook.nvidia.com/v1alpha1 Skyhook` ->
> `nodewright.nvidia.com/v1alpha1 NodeWright`. `DeploymentPolicy` moves group (same Kind name). The
> on-node annotation/label/finalizer prefix moves `skyhook.nvidia.com/* -> nodewright.nvidia.com/*`, and
> the kubectl plugin is renamed `kubectl skyhook -> kubectl nodewright`.

## TL;DR

1. **Upgrade the operator.** It automatically mirrors every existing `Skyhook` into a `NodeWright` (and
   each legacy `DeploymentPolicy` into a new-group `DeploymentPolicy`), and migrates the per-node state
   (annotations, package pods, ConfigMaps) to the new names. **No package re-runs, no reboots**, node
   lifecycle position is preserved. You do **not** touch your CRs for this step.
2. **Rename your CRs** (in git for GitOps, or by re-applying for Helm/manual): change `apiVersion` and
   `kind`, keep the same `metadata.name`. `kubectl nodewright migrate` generates the new manifests for you.
3. **Delete the old `Skyhook`/`DeploymentPolicy` CRs.** The mirrored `NodeWright` objects are the real,
   reconciled resources and are never deleted by this step.

Do steps 2 and 3 **as one change** (a rename = delete old + add new together). **Perform the operator
upgrade during a quiet window** with no active package rollout (see [Timing](#timing)).

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

Separately, the operator migrates per-node state to the new prefix (`nodeState_`, `status_`, `version_`,
`cordon_`, node conditions) and replaces the old `skyhook.nvidia.com/`-labeled package pods and per-node
ConfigMaps with new-labeled equivalents owned by the `NodeWright`. Legacy `Skyhook`/`DeploymentPolicy`
writes return a deprecation warning pointing here.

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

   Same for `DeploymentPolicy` (only `apiVersion` changes; `kind` stays `DeploymentPolicy`). Generate the
   new manifests with `kubectl nodewright migrate -f <old-manifest.yaml>` (or against the cluster). The
   commit should **remove** the old `Skyhook` manifest and **add** the `NodeWright` manifest so Argo
   prunes the old and adopts the new in one sync.

3. **Sync.** Argo adopts the existing `NodeWright` (in-sync, no re-create) and prunes the `Skyhook`. Once
   the `Skyhook` is gone, the mirror has nothing left to import for it.

**Do not** leave the old `Skyhook` in git after adding the `NodeWright`: in that window both the mirror
and Argo write the `NodeWright`, and editing it via git could be stomped by the mirror re-importing the
stale `Skyhook`. Renaming in one commit avoids this entirely.

If you keep CRDs in a separate Argo Application: it already tracks both CRDs during the transition (the
chart ships both). Do not remove the `skyhook.nvidia.com` CRD until every `Skyhook` is migrated and
deleted (removing the CRD cascade-deletes any remaining `Skyhook`s). See [Removal](#removal-future-release).

---

## Helm (`helm upgrade`)

Helm here refers to installing the **operator** via its chart. Your `Skyhook`/`NodeWright` **instances**
are usually authored separately (not part of the operator chart); adjust to your setup.

1. **`helm upgrade` the operator chart.** The transition-release chart ships **both** CRDs
   (`skyhook.nvidia.com` and `nodewright.nvidia.com`) and RBAC for both groups. The mirror imports your
   existing `Skyhook`s into `NodeWright`s and migrates node state, same as above.

2. **Update your CR manifests to `NodeWright`** wherever you keep them, and apply:
   ```bash
   kubectl nodewright migrate -f my-skyhook.yaml > my-nodewright.yaml   # rewrite apiVersion+kind
   kubectl apply -f my-nodewright.yaml
   ```
   Because the operator already created the `NodeWright` via the mirror, this apply is a no-op adoption of
   the existing object (same name, same spec) rather than a fresh create.

3. **Delete the old CRs:**
   ```bash
   kubectl delete skyhook.skyhook.nvidia.com <name>
   ```
   The `NodeWright` remains (the mirror never deletes it).

At the **removal** release, the chart adds a pre-upgrade safety hook (`legacy.blockUpgradeIfPresent`)
that **aborts the upgrade** if any legacy `Skyhook`/`DeploymentPolicy` still exist, so you cannot
accidentally drop the old CRD (and cascade-delete live objects) before migrating. The hook is
dual-annotated (`helm.sh/hook` + `argocd.argoproj.io/hook`) so it fires under both Helm and Argo.

---

## Manual / kubectl

1. Upgrade the operator (apply the new manifests / image). The mirror imports your `Skyhook`s.
2. Generate and apply the new CRs:
   ```bash
   kubectl nodewright migrate > nodewrights.yaml    # reads legacy CRs from the cluster
   kubectl apply -f nodewrights.yaml                # adopts the mirrored objects
   ```
3. Delete the old CRs: `kubectl delete skyhooks.skyhook.nvidia.com --all` (per-namespace/name as you prefer).

---

## Prerequisite: all Skyhooks must be complete

**Perform the operator upgrade only when every `Skyhook` is `complete` with no nodes in progress**, i.e.
no package is mid-rollout. This is a **requirement**, not just a recommendation: the migration renames the
operator's package pods and per-node ConfigMaps, and the flow assumes there is no in-flight package work to
disrupt. Node lifecycle state is preserved regardless, but upgrading while a rollout is active is
unsupported.

Check before upgrading:

```bash
kubectl get skyhooks.skyhook.nvidia.com -o custom-columns=NAME:.metadata.name,STATUS:.status.status,INPROGRESS:.status.nodesInProgress
```

Proceed only when `STATUS` is `complete` and `INPROGRESS` is `0` for **all** Skyhooks. The operator also
performs this check at upgrade time and warns (or blocks) if a rollout is in progress.

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
- Node state carried over: a node's annotations are re-keyed under `nodewright.nvidia.com/` and no package
  re-ran (package pods did not restart on nodes that were already complete).
- No drift reported by Argo on your (soon-to-be-deleted) `Skyhook` objects during the transition.
- After you delete the old CRs, the `NodeWright` objects remain and reconcile normally.
- Legacy `Skyhook`/`DeploymentPolicy` writes emit the deprecation warning.

## Removal (future release)

The legacy `skyhook.nvidia.com` group is kept for a multi-release migration window (targeting roughly two
releases, adoption-gated - not a fixed release number). In the removal release:

- The legacy admission webhook flips from warning to **denying** writes.
- The legacy CRDs are removed from the chart/manifests, sequenced behind a preflight that refuses removal
  while any legacy objects remain (the Helm/Argo pre-upgrade hook above).
- Removing the `skyhook.nvidia.com` CRD cascade-deletes any remaining `Skyhook`s, so **migrate before the
  removal release**.

## FAQ

**Will my nodes reboot during the upgrade?** No, unless a package interrupt was mid-flight at upgrade
time, and even then the agent skips an already-completed interrupt. Node state is preserved.

**Does the operator write to my `Skyhook` objects?** No. The mirror is read-only on the source, so GitOps
tools see no drift. "Migrated" is derived from the existence of the stamped `NodeWright`.

**What if I create a new `Skyhook` after upgrading?** It still works (the CRD is served during the
transition) and gets mirrored, but you will get a deprecation warning. Prefer creating `NodeWright`
directly.

**Can I roll the operator back?** During the transition release both groups are served, so a rollback to a
version that still reconciles `Skyhook` is possible while the `Skyhook` objects still exist. Once you have
deleted the `Skyhook`s and are on `NodeWright` only, rolling back to a Skyhook-only operator would strand
the `NodeWright`s. Keep the `Skyhook`s until you are confident.
