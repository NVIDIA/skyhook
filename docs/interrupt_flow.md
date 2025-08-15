# Interrupt Flow and Ordering

This document explains how Skyhook handles packages that require interrupts and the specific ordering of operations to ensure safe and reliable execution.

## Overview

When a package requires an interrupt (such as a reboot or service restart), Skyhook follows a specific sequence to ensure that workloads are safely evacuated from the node before any potentially disruptive operations occur.

## Interrupt Flow Sequence

### For packages WITH interrupts:

1. **Uninstall** (if downgrading) - Package uninstallation operations are executed.
2. **Cordon** - Node is marked as unschedulable to prevent new workloads from being scheduled
3. **Wait** - System waits for any conflicting workloads to naturally complete or be rescheduled
4. **Drain** - Remaining workloads are gracefully evicted from the node
5. **Apply** / **Upgrade** (if upgrading) - Package installation/upgrade operations are executed  
6. **Config** - Configuration and setup operations are performed
7. **Interrupt** - The actual interrupt operation (reboot, service restart, etc.) is executed
8. **Post-Interrupt** - Any cleanup or verification operations after the interrupt

### For packages WITHOUT interrupts:

1. **Uninstall** (if downgrading) - Package uninstallation operations are executed.
2. **Apply** / **Upgrade** (if upgrading) - Package installation/upgrade operations are executed
3. **Config** - Configuration and setup operations are performed

## Why This Order Matters

The **uninstall → cordon → wait → drain → apply/upgrade → config → interrupt** sequence is critical for several reasons:

### Safety First
- Workloads are safely removed before any potentially disruptive operations
- Prevents data loss or service interruption for running applications
- Ensures the node is in a clean state before package operations begin

### Use Cases
This ordering is particularly important for scenarios such as:

- **Kernel module changes**: Unloading kernel modules while workloads are present could cause system instability
- **GPU mode switching**: Changing GPU from graphics to compute mode requires exclusive access
- **Driver updates**: Hardware driver changes need exclusive access to the hardware
- **System reboots**: Obviously require all workloads to be evacuated first

### Example Scenario

Consider a package that needs to unload a kernel module, perform some operations, and then reboot:

```yaml
apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  name: gpu-mode-switch
spec:
  packages:
    gpu-driver:
      version: "1.0.0"
      image: "example/gpu-driver"
      interrupt:
        type: "reboot"
```

**Flow:**
1. **Cordon**: Node becomes unschedulable
2. **Wait**: Any non-interrupt workloads are given time to complete
3. **Drain**: Remaining workloads are evicted
4. **Apply**: GPU driver package operations run (unload old module, install new)
5. **Config**: Configuration files are updated
6. **Interrupt**: System reboots to complete the driver change
7. **Post-Interrupt**: Verification that the new driver is loaded correctly

## Technical Implementation

The interrupt flow is managed by the `ProcessInterrupt` and `EnsureNodeIsReadyForInterrupt` functions in the Skyhook controller, which:

- Check for conflicting workloads using label selectors
- Coordinate the cordon and drain operations
- Ensure the node is ready before proceeding with package operations
- Handle the timing and sequencing of all stages

## Best Practices

- Always test interrupt-enabled packages in non-production environments first
- Use appropriate `podNonInterruptLabels` selectors to identify important workloads that should block interrupts
- Consider the impact of node cordoning on cluster capacity
- Monitor package logs during interrupt operations for troubleshooting
- Use Grafana dashboards to monitor interrupt operations and track package state transitions across your cluster (see [docs/metrics/](metrics/) for dashboard setup and configuration)
