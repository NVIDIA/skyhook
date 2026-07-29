# Cleanup Pods Test

## Purpose

Validates that the operator correctly cleans up pods when a node's state is reset.

## Test Scenario

1. Create a nodewright with package dependencies (`bb` depends on `aa`)
2. Wait for the nodewright to complete
3. Trigger an update to force a config cycle on package `bb`
4. Once config is complete, update again to make `bb` error
5. Clear out the node annotation on `kind-worker` to trigger cleanup
6. Verify `bb`'s erroring config Job from before the reset does not survive on that node. This is
   the mechanism the rest of the test depends on: the Job is unfinished, so it is only reaped if
   the reset is actually observed, and while it lives `JobExists` gates the package
7. Verify the packages re-run from `apply` on that node, with fresh Job-owned pods

## Key Features Tested

- Executor cleanup after node state reset (Jobs; pods follow by ownership)
- Node state stays cleared after a reset: the pod watch reports in-flight erroring but must not
  write back an entry the reset removed
- Handling of erroring packages
- Package dependency handling during cleanup
- Orphan pod detection and removal

## Files

- `chainsaw-test.yaml` - Main test configuration with lifecycle assertions inline (pods, nodes, nodewright status) for sequential ordering
- `setup.yaml` - NodeWright resource definition with package dependencies
- `assert-setup-complete.yaml` - Assertion for initial setup completion
- `assert-config-complete.yaml` - Assertion for config cycle completion
- `force-config.yaml` - Update to trigger a config cycle
- `muck_up.yaml` - Update to make a package error
