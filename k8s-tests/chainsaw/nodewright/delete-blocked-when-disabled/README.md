# Delete Blocked When Disabled Test

## Purpose

Verifies the NodeWright finalizer refuses to proceed with cleanup when the CR is
disabled and at least one `uninstall.enabled: true` package is still tracked
in `nodeState`. `processSkyhooksPerNode` skips disabled NodeWrights, so uninstall
pods never get created; without this guard, the finalizer would silently drop
on-host state the user explicitly asked to be cleaned.

Symmetric to `../delete-blocked-when-paused/` — same block, different reason
string, different remediation (re-enable vs. unpause).

## Test Scenario

1. Install a NodeWright with one `uninstall.enabled: true` package, wait for
   `status: complete`.
2. Annotate the NodeWright with `nodewright.nvidia.com/disable=true`.
3. Issue `kubectl delete nodewright --wait=false`.
4. Assert the CR is still present (`deletionTimestamp != null`) with a
   `DeletionBlocked` condition
   (`status=True`, `reason=DisabledWithPendingUninstall`).
5. Assert a Warning event was recorded on the NodeWright with
   `reason=DeletionBlocked`.
6. Remove the disable annotation. The finalizer now drives uninstall to
   completion and the CR disappears.

## Key Features Tested

- Finalizer DeletionBlocked guard for disabled + pending uninstall
- DeletionBlocked condition with `reason=DisabledWithPendingUninstall`
- Warning event emission for operator visibility
- Condition cleared on re-enable so the finalizer can proceed

## Files

- `chainsaw-test.yaml` — Main test: install → disable+delete+assert → re-enable+drain
- `nodewright.yaml` — One `uninstall.enabled: true` package
