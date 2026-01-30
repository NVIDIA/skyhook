# Interrupt Test

## Purpose

Validates the interrupt feature of the skyhook operator, including pod draining, waiting, and interrupt budget enforcement.

## Test Scenario

1. Create pods on the node before applying the skyhook:
   - An invalid package pod
   - A pod to wait on
   - A pod to drain
2. Apply a skyhook that requires interrupting these workloads
3. Verify interrupt behavior:
   - Pods are drained according to the interrupt configuration
   - Wait-for pods block progress until ready
   - Invalid pods are handled correctly
4. Assert packages complete successfully after interrupts

## Key Features Tested

- Interrupt (drain) functionality
- Wait-for pod support
- Agent image override (`agentImageOverride`)
- Interruption budgets
- Package dependencies (`dependsOn`)

## Files

- `chainsaw-test.yaml` - Main test configuration
- `skyhook.yaml` - Skyhook with interrupt configuration
- `pre-pods.yaml` - Pods to create before the skyhook
- `assert*.yaml` - State assertions

## Notes

- This test cannot run concurrently with other tests because it can cause race conditions where other skyhooks make the node unschedulable
