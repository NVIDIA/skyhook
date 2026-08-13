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

- **The operator pod now contains only the `manager` container.** Secure metrics
  are served directly by controller-runtime on port `8443`; the separate metrics
  proxy image, resources, values, and RBAC objects have been removed. Existing
  Prometheus clients should continue using a bearer token authorized for the
  `skyhook-operator-metrics-reader` ClusterRole.

- **Maintenance jobs moved off `bitnami/kubectl` to `alpine/kubectl`.** The
  webhook and skyhook cleanup pre-delete hooks and the new selector-migration
  pre-upgrade hook now use a maintained, versioned, multi-arch `alpine/kubectl`
  image, pinned by tag and digest. The previous image's free versioned tags were
  withdrawn on 2025-08-28, leaving only an unversioned `:latest` with no
  security maintenance. If you had overridden `webhook.removalImage` to point at
  a private mirror, remirror from the new source (#207).

### Upgrade notes

- **Run `helm upgrade` only when no package work is in flight.** The operator
  that precedes this chart executes packages as raw pods, and the two execution
  models are never allowed to overlap: the new operator withholds all
  reconciliation while any pre-rename `Skyhook` is still rolling out. Upgrading
  mid-rollout therefore leaves that node **cordoned and stalled** until you
  `helm rollback` and let the rollout finish, or delete the legacy `Skyhook`.
  Confirm every `Skyhook` reads `complete` first, and do not unpause or enable a
  migrated `NodeWright` until its pre-upgrade package pods are gone. See the
  operator release notes and
  [docs/nodewright-migration.md](../docs/nodewright-migration.md).
- **A crash-looping package now gives up after ~70 seconds instead of retrying
  for up to an hour.** `jobBackoffLimit` bounds retries per stage; if you rely on
  a package self-healing through transient environment problems, raise it before
  upgrading. See the operator release notes for the full behavior change.
- **`jobTtlSucceeded` / `jobTtlFailed` must be at least `"1m"`.** Setting `"0"`
  to disable retention makes the operator fail validation at startup.
- Remove any overrides under `controllerManager.kubeRbacProxy`; that values
  block no longer exists. Metrics remain available on HTTPS port `8443`.
  If you override `controllerManager.manager.env.metricsPort`, update it from
  `:8080` to `:8443`; the value remains supported, but the Service now targets
  `8443` exclusively.
  Scrapers that used the chart's former unauthenticated HTTP `:8080` service
  port must switch to `:8443`, disable TLS certificate verification, and
  provide a Kubernetes bearer token authorized for the `/metrics`
  non-resource URL. controller-runtime generates an in-memory self-signed
  certificate for each operator pod, so there is no stable CA to trust.
  The chart no longer adds `prometheus.io/*` discovery annotations by default,
  because standard annotation-based jobs cannot supply that authentication or
  trust configuration and would continuously report a failed target. Configure
  an authenticated endpoints-discovery job as shown in
  `docs/metrics/prometheus_values.yaml` instead.

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
