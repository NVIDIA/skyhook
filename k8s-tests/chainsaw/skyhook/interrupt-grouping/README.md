# Interrupt Grouping Test

## Purpose

Validates that interrupts are correctly grouped around services or reboots, ensuring there is one interrupt per group with correct priority.

## Test Scenario

1. Apply a skyhook with multiple packages that have different interrupt configurations
2. Verify that interrupts are grouped correctly:
   - Service-related interrupts are grouped together
   - Reboot-related interrupts are grouped together
3. Assert the priority ordering within groups is correct
4. Verify only one interrupt runs per group

## Key Features Tested

- Interrupt grouping by type (service vs reboot)
- Interrupt priority ordering
- Single interrupt execution per group
- Multiple package interrupt coordination

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Skyhook with grouped interrupt packages
- `assert*.yaml` - State assertions for interrupt grouping
