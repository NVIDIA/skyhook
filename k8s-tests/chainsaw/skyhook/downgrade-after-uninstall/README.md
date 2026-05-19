# Downgrade After Uninstall Test

## Purpose

Verifies the webhook's downgrade gate for `uninstall.enabled: true` packages:
a version downgrade is rejected unless the package has been explicitly
uninstalled first. This test exercises the happy path — once the uninstall
has completed on every tracked node, the same CR can be downgraded and the
new (older) version installs fresh.

Pairs with the `downgrade-enabled-false-preserves-state` test, which covers
the complementary case where no uninstall is required.

## Test Scenario

1. Install the package at v2.1.4 with `uninstall.enabled: true`, wait for
   complete.
2. Flip `uninstall.apply: true`. Wait until the package is absent from node
   state (`nodeState_<name>: '{}'`) — uninstall complete per D2 semantics.
3. Update the spec to v1.2.3. The webhook accepts the downgrade because the
   package is absent from all tracked nodes. The new version installs and
   reaches `stage=config, state=complete`.
4. Flip `uninstall.apply: true` on v1.2.3 and wait for the package to be
   absent from node state again. This is purely operational — it leaves the
   CR with no installed packages so chainsaw's automatic CLEANUP can delete
   it without waiting on the delete-time uninstall finalizer.

## Key Features Tested

- Webhook downgrade gate requires `uninstall.apply=true` AND full uninstall
  on every node before allowing the version change
- Happy path: downgrade succeeds once the gate conditions are met
- Fresh install of the older version after uninstall
- D2 semantic: `nodeState` absence = uninstalled

## Files

- `chainsaw-test.yaml` — Main test: install v2 → uninstall v2 → downgrade to v1 → uninstall v1
- `skyhook.yaml` — Initial v2.1.4 Skyhook, `uninstall.enabled: true`
- `update-trigger-uninstall.yaml` — Patch setting `uninstall.apply: true` on v2.1.4
- `update-downgrade.yaml` — Patch changing version to v1.2.3
- `update-trigger-uninstall-v1.yaml` — Patch setting `uninstall.apply: true` on v1.2.3 to clear node state before cleanup
