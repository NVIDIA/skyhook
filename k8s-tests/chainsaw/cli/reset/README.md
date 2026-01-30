# CLI Reset Test

## Purpose

Validates the skyhook reset command for resetting all nodes in a Skyhook.

## Test Scenario

1. Create a skyhook and wait for it to complete
2. Disable the skyhook to prevent re-processing after reset
3. Test reset command:
   - Run `skyhook reset <name>`
   - Assert all nodes are reset to initial state
4. Verify node annotations are cleared

## Key Features Tested

- `skyhook reset` - Resets all nodes for a Skyhook
- Node state cleanup
- Annotation removal

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Test skyhook
- `assert-skyhook-complete.yaml` - Initial completion assertion
- `assert-skyhook-disabled.yaml` - Disabled state assertion
- `assert-nodes-reset.yaml` - Reset state assertion
