# Release Notes

Human-authored highlights, behavior changes, and upgrade steps for the Helm chart.
For the full commit-level log see CHANGELOG.md.

## Unreleased

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

- Remove any overrides under `controllerManager.kubeRbacProxy`; that values
  block no longer exists. Metrics remain available on HTTPS port `8443`.
  Scrapers that used the chart's former unauthenticated HTTP `:8080` service
  port must switch to `:8443`, trust the metrics serving certificate (or opt
  into TLS verification skipping), and provide a Kubernetes bearer token.

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
