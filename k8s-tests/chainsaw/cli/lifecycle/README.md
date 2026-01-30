# CLI Lifecycle Test

## Purpose

Validates all CLI lifecycle commands for controlling Skyhook processing state.

## Test Scenario

1. Reset state from previous runs
2. Create a skyhook and wait for it to start processing
3. Test pause command:
   - Run `skyhook pause <name>`
   - Assert the skyhook shows paused status
4. Test resume command:
   - Run `skyhook resume <name>`
   - Assert the skyhook resumes processing
5. Test disable command:
   - Run `skyhook disable <name>`
   - Assert the skyhook is disabled
6. Test enable command:
   - Run `skyhook enable <name>`
   - Assert the skyhook is enabled and processing
7. Test pause and disable together:
   - Apply both pause and disable to the skyhook
   - Assert both flags are set
   - Run `skyhook resume` - should only remove pause, not disable
   - Assert the skyhook is still disabled
   - Run `skyhook enable` - should remove disable
   - Assert the skyhook is fully enabled

## Key Features Tested

- `skyhook pause` - Pauses a Skyhook from processing
- `skyhook resume` - Resumes a paused Skyhook
- `skyhook disable` - Disables a Skyhook completely
- `skyhook enable` - Enables a disabled Skyhook
- Independence of pause and disable flags
- Resume only affects pause, not disable

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Test skyhook
- `assert-paused.yaml` - Paused state assertion
- `assert-resumed.yaml` - Resumed state assertion
- `assert-disabled.yaml` - Disabled state assertion
- `assert-enabled.yaml` - Enabled state assertion
- `assert-paused-and-disabled.yaml` - Both flags set assertion
- `assert-still-disabled.yaml` - Resume doesn't remove disable assertion
