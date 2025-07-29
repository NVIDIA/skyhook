# Operator Status, State, and Stage Definitions

This document provides concise definitions for the status, state, and stage concepts used throughout the Skyhook operator to track package operations and node lifecycle management.

## Key Relationships

- **Status** reflects the overall health and progress of nodes and the Skyhook resource
- **State** tracks the execution status of individual package operations
- **Stage** defines the specific lifecycle phase a package is currently in

- A node's Status is derived from the collective States of its packages
- Stages progress sequentially, with State indicating success/failure at each stage
- All stages except for interrupts include validation checks that must succeed for progression

## Usage in Operations

- **Monitoring**: Use Status for high-level health checks and dashboards
- **Debugging**: Examine State and Stage for detailed package-level troubleshooting  
- **Automation**: State transitions trigger the next appropriate Stage in the lifecycle
- **Scheduling**: Status values like `blocked` and `paused` control operation scheduling and dependencies 

## Status

**Scope**: Applied to the overall Skyhook resource and individual nodes  
**Purpose**: High-level operational status indicating the current condition

| Status | Definition |
|--------|------------|
| `complete`    | All operations have finished successfully |
| `blocked`     | Operations are prevented from proceeding due to taint toleration issues |
| `waiting`     | Queued for execution but not yet started |
| `disabled`    | Execution is disabled but will continue for other Skyhooks |
| `paused`      | Execution is paused for this and all other Skyhooks supposed to be executed after this one |
| `in_progress` | Currently executing operations |
| `erroring`    | Experiencing failures or errors |
| `unknown`     | Status cannot be determined or is uninitialized |

## State

**Scope**: Applied to individual packages within a node  
**Purpose**: Current execution state of a specific package operation

| State | Definition |
|-------|------------|
| `complete`    | Package operation has finished successfully |
| `in_progress` | Package is actively running (pod has started) |
| `skipped`     | Package/stage was intentionally bypassed in the lifecycle |
| `erroring`    | Package operation is experiencing failures |
| `unknown`     | Package state cannot be determined or is uninitialized |

## Stage

**Scope**: Applied to individual packages  
**Purpose**: Indicates which phase of the package installation/management process is currently executing

| Stage | Definition |
|-------|------------|
| `uninstall` & `uninstall-check`           | Removal of the package |
| `upgrade`   & `upgrade-check`             | Package version update operations |
| `apply`     & `apply-check`               | Initial installation/deployment of the package |
| `config`    & `config-check`              | Configuration and setup operations |
| `interrupt`                               | Execution of interrupt operations (e.g., reboots, service restarts) |
| `post-interrupt` & `post-interrupt-check` | Operations that run after interrupt completion |

**NOTE**: All stages except for interrupts include validation checks that must succeed for progression

## Stage Flow

The typical stage progression depends on whether the package has interrupts:

### Without Interrupts:
```
uninstall → apply → config
upgrade → config
```

### With Interrupts:
```
uninstall → apply → config → interrupt → post-interrupt
upgrade → config → interrupt → post-interrupt
```