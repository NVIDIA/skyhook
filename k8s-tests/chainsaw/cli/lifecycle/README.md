# CLI Lifecycle Test

## Purpose

Validates all CLI lifecycle commands for controlling NodeWright processing state.

## Test Scenario

1. Reset state from previous runs
2. Create a nodewright and wait for it to start processing
3. Test pause command:
   - Run `nodewright pause <name>`
   - Assert the nodewright shows paused status
4. Test resume command:
   - Run `nodewright resume <name>`
   - Assert the nodewright resumes processing
5. Test disable command:
   - Run `nodewright disable <name>`
   - Assert the nodewright is disabled
6. Test enable command:
   - Run `nodewright enable <name>`
   - Assert the nodewright is enabled and processing
7. Test pause and disable together:
   - Apply both pause and disable to the nodewright
   - Assert both flags are set
   - Run `nodewright resume` - should only remove pause, not disable
   - Assert the nodewright is still disabled
   - Run `nodewright enable` - should remove disable
   - Assert the nodewright is fully enabled

## Key Features Tested

- `nodewright pause` - Pauses a NodeWright from processing
- `nodewright resume` - Resumes a paused NodeWright
- `nodewright disable` - Disables a NodeWright completely
- `nodewright enable` - Enables a disabled NodeWright
- Independence of pause and disable flags
- Resume only affects pause, not disable

## Files

- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - Test nodewright
- `assert-paused.yaml` - Paused state assertion
- `assert-resumed.yaml` - Resumed state assertion
- `assert-disabled.yaml` - Disabled state assertion
- `assert-enabled.yaml` - Enabled state assertion
- `assert-paused-and-disabled.yaml` - Both flags set assertion
- `assert-still-disabled.yaml` - Resume doesn't remove disable assertion
