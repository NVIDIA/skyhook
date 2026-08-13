# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the Helm chart.
For the full commit-level log see CHANGELOG.md.

## Unreleased

### New Features

- **Package stage execution moved to `batch/v1` Jobs**, with four new operator
  env values under `controllerManager.manager.env`:

  | Value | Default | What it does |
  | --- | --- | --- |
  | `jobStageTimeout` | `"1h"` | Default per-attempt deadline for a package stage, when the package sets no `stageTimeout` of its own. `"0"` removes the time bound. |
  | `jobBackoffLimit` | `"3"` | Retries after the first attempt before a stage is surfaced as `erroring` — so at most four attempts. |
  | `jobTtlSucceeded` | `"1h"` | How long a succeeded stage Job (and its logs) is kept. Minimum `"1m"`. |
  | `jobTtlFailed` | `"24h"` | How long a failed stage Job is kept, so failure logs outlive success logs. Minimum `"1m"`. |

  The chart also gains `batch/job` RBAC for the operator; no values change is
  required to upgrade.

### Bug Fixes

- **Helm upgrade no longer fails on the immutable Deployment selector after the
  skyhook -> nodewright rename.** A release first installed from a pre-rename
  chart (chart name `skyhook-operator`) could not be upgraded, because the
  rename changed `app.kubernetes.io/name` inside the controller-manager
  Deployment's `spec.selector`, which Kubernetes forbids mutating
  (`spec.selector: field is immutable`). The chart now sources the selector from
  a single shared helper (`chart.managerSelectorLabels`) and ships a
  `pre-upgrade` hook (`selectorMigration.enabled`, default `true`) that inspects
  the live Deployment and deletes it only when its selector does not match the
  desired one, so Helm can recreate it. The hook is a no-op (no downtime) on
  normal upgrades and is scoped to the controller-manager Deployment via its own
  narrowly-permissioned ServiceAccount (#285).

### Other Changes

- **Maintenance jobs moved off `bitnami/kubectl` to `alpine/kubectl`.** The
  webhook and skyhook cleanup pre-delete hooks and the new selector-migration
  pre-upgrade hook now use a maintained, versioned, multi-arch `alpine/kubectl`
  image, pinned by tag and digest. The previous image's free versioned tags were
  withdrawn on 2025-08-28, leaving only an unversioned `:latest` with no
  security maintenance. If you had overridden `webhook.removalImage` to point at
  a private mirror, remirror from the new source (#207).

### Upgrade notes

- **A crash-looping package now gives up after ~70 seconds instead of retrying
  for up to an hour.** `jobBackoffLimit` bounds retries per stage; if you rely on
  a package self-healing through transient environment problems, raise it before
  upgrading. See the operator release notes for the full behavior change.
- **`jobTtlSucceeded` / `jobTtlFailed` must be at least `"1m"`.** Setting `"0"`
  to disable retention makes the operator fail validation at startup.
- No action is required: the selector-migration hook runs automatically on
  `helm upgrade` and only recreates the Deployment when its selector is stale.
  To perform the recreate manually instead, set `selectorMigration.enabled=false`
  and, before upgrading, delete the Deployment so the upgrade recreates it:

  ```bash
  # Simplest: brief control-plane gap, but clean (the pod is removed too).
  kubectl delete deployment <release>-controller-manager -n <namespace>
  ```

  For no control-plane gap, orphan the pod instead, then remove the leftover
  old pod once the new one is Ready (two managers run briefly; leader election
  keeps only one active):

  ```bash
  kubectl delete deployment <release>-controller-manager -n <namespace> --cascade=orphan
  ```
