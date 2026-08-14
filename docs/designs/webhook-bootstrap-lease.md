# Webhook bootstrap lease

This document describes why the operator's webhook cert bootstrap runs under a
**dedicated** leader-election lease, separate from the main reconcile lease, and
what guarantees that design does (and does not) provide.

## Background: the v0.7.x → v0.15.x deadlock

Starting in v0.8.0 the operator self-bootstraps its admission-webhook TLS:
on startup the leader generates a CA + serving cert, writes `Secret/webhook-cert`
in the operator namespace, and patches the `caBundle` of the
`MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`. Earlier
versions (≤ v0.7.x) relied on a different bootstrap model and have no knowledge
of `webhook-cert`.

On an in-place rollout from v0.7.x to a self-bootstrapping version, the cluster
can enter the following dead state:

1. The new pod (v0.15.x) starts as a **follower**: the leader-election lease
   is still held by an old v0.7.x pod.
2. The new pod's webhook controller is gated on leader election, so it never
   creates `webhook-cert`. Its readiness probe (which waits for the secret)
   never goes green.
3. The old leader has no concept of `webhook-cert` and never creates it. The
   old leader also has nothing that would cause it to release the lease.
4. Meanwhile, Argo/Helm applying the new manifests has reset the
   `MutatingWebhookConfiguration` `caBundle` to empty, so the apiserver no
   longer trusts the v0.7.x pods either. With `failurePolicy: Fail`, all
   CREATE/UPDATE on NodeWright CRs fail with
   `x509: certificate signed by unknown authority`.

Net result: the new pod waits forever for a secret that requires leadership to
create, the old pod holds leadership forever because the new pod never goes
Ready, and Argo cannot reconcile any further NodeWright config changes.

The only field operations that broke the cycle were manual:

- Delete the old-version pods to force a lease handover, *or*
- Delete the lease object directly (`Lease/3c22c1ae.nvidia.com`).

## Design: a second lease, owned by a second manager

The webhook bootstrap is now driven by a **dedicated `ctrl.Manager`** with its
own leader-election lease name (`nodewright-webhook-bootstrap.nvidia.com`), separate from the
main reconcile manager's lease (`3c22c1ae.nvidia.com`).

```
┌────────────────────────────────────────────────────────────────────────┐
│ operator process (one per pod)                                         │
│                                                                        │
│  ┌────────────────────────────────────┐                                │
│  │ reconcile manager                  │  lease: 3c22c1ae.nvidia.com    │
│  │  - SkyhookReconciler (leader-gated)│                                │
│  │  - SecretCertWatcher (every pod)   │                                │
│  │  - webhook server (every pod)      │                                │
│  │  - readyz / healthz                │                                │
│  └────────────────────────────────────┘                                │
│                                                                        │
│  ┌────────────────────────────────────┐                                │
│  │ webhook bootstrap manager          │  lease: nodewright-webhook-bootstrap.nvidia.com │
│  │  - WebhookController (leader-gated)│                                │
│  │     creates webhook-cert Secret    │                                │
│  │     patches caBundle on            │                                │
│  │     {Validating,Mutating}WebhookConfiguration │                     │
│  └────────────────────────────────────┘                                │
└────────────────────────────────────────────────────────────────────────┘
```

`SecretCertWatcher` (`NeedLeaderElection() == false`) continues to run on the
reconcile manager on every pod. It watches the namespace's Secrets via the
reconcile manager's cache and syncs `webhook-cert` to the local disk so this
pod's webhook server can serve TLS. That part is unchanged.

Both managers share the same `rest.Config` and `scheme` and start under the
same signal handler via `errgroup.WithContext`. If either manager exits with
an error the process exits non-zero.

### Invariants this preserves

- **Single writer to `webhook-cert` and to the webhook configuration caBundle.**
  Exactly one pod holds `nodewright-webhook-bootstrap.nvidia.com` at a time. There is no
  split-brain risk between operator versions that both understand this lease.
- **Webhook server runs on every pod.** The webhook server, its readiness
  probe, and `SecretCertWatcher` are all on the reconcile manager, not gated on
  the bootstrap lease — so all pods can serve admission traffic as soon as
  `webhook-cert` exists locally.
- **`failurePolicy: Fail` on the admission webhooks remains in effect.** We
  do not need to relax admission to make this work.

### What this fixes

Any *future* upgrade discontinuity between two versions that both understand
the dedicated bootstrap lease: a new pod can win
`nodewright-webhook-bootstrap.nvidia.com` and complete the cert bootstrap independently of
who holds the main reconcile lease. Readiness flips green, Service endpoints
rotate, the rollout completes.

### Corollary: never look the webhook configurations up by name

The dedicated lease only helps when a new pod can *win* it. It cannot help when the
old pod is alive, holding the bootstrap lease, and stuck — which is exactly what a
name-based lookup of the webhook configurations produces the moment those objects are
renamed:

1. The chart renames `{Validating,Mutating}WebhookConfiguration`.
2. The old pod's `WebhookController` hard-errors on `... not found`, so its readiness
   probe never goes green.
3. The rolling update will not terminate an un-Ready old pod, so the old pod keeps
   renewing the bootstrap lease.
4. The new pod stays a follower, never mints the cert, never patches the caBundle.
   `helm upgrade` fails with `Pending termination`, and the release stays wedged.

The operator therefore selects these objects by the
`nodewright.nvidia.com/webhook-config` label (filtered to configurations whose
`clientConfig` dials its own webhook Service, since the caBundle it injects only
signs that Service's certificate) and patches every match, so a rename is
invisible to it. **Do not reintroduce a name-based lookup.** The chart must keep
applying that label.

This makes *future* renames safe. It could not save the one upgrade that introduced it
(the operator already running was the name-based one), so the chart's
`selectorMigration` pre-upgrade hook detects a pre-label-discovery operator — the live
Deployment has no `WEBHOOK_SERVICE_NAME` env var — and deletes the Deployment so Helm
recreates it.

### What this does NOT fix

This design **cannot** retroactively fix the v0.7.x → v0.15.x upgrade
specifically, because v0.7.x has no knowledge of the new lease. Operators
on v0.7.x still need the manual workaround:

```bash
# The operator's namespace. A v0.7.x install predates the skyhook -> nodewright
# namespace default, so it is almost certainly still `skyhook`; `nodewright` is
# the default only for installs created after that change.
NS=skyhook

kubectl -n "$NS" get pods                                                   # identify old-version pods
kubectl -n "$NS" delete pod <old-pod-1> <old-pod-2>                         # free the lease
# Or, equivalently:
kubectl -n "$NS" delete lease 3c22c1ae.nvidia.com

# Verify recovery:
# The object names below are the pre-rename ones, which is what a v0.7.x-era install has.
# On a chart from the resource-name rename onward they are nodewright-*; select the webhook
# configuration by label instead of by name there:
#   kubectl get mutatingwebhookconfiguration -l nodewright.nvidia.com/webhook-config
kubectl -n "$NS" get secret webhook-cert -w
kubectl get mutatingwebhookconfiguration skyhook-operator-mutating-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | wc -c                # must be > 0
kubectl -n "$NS" rollout status deploy/skyhook-operator-controller-manager
```

The runbook above should be added to release notes for any future major
operator version that changes the webhook bootstrap model.

## Implementation notes

- Lease names live as constants in `operator/cmd/manager/main.go`:
  `reconcileLeaseID`, `webhookBootstrapLeaseID`. **Do not rename them** without
  a coordinated upgrade plan — renaming the bootstrap lease re-introduces
  exactly the deadlock this design avoids.
- The bootstrap manager is only constructed when `ENABLE_WEBHOOKS=true`.
- The bootstrap manager disables its own metrics endpoint
  (`metricsserver.Options{BindAddress: "0"}`) and does not bind a health-probe
  port; the reconcile manager already owns those.
- The bootstrap manager runs its own cache, which adds informers for
  `Secret/webhook-cert` and the two webhook configurations only. The reconcile
  manager's cache also watches Secrets via `SecretCertWatcher`. Two
  Secret informers in the operator namespace is the cost of the split and is
  considered acceptable.
- `WebhookSecretReadyzCheck` reads through the bootstrap manager's client and
  is registered on the reconcile manager's health-probe server. Cache-sync
  races during startup naturally result in NotReady, which is the desired
  behavior.
