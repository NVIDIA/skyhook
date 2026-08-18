# kubelet-refused

Validates that attempts the kubelet refuses to admit are not recorded as package failures.

## Why this needs a real cluster

Package pods carry `spec.nodeName`, so they bypass the scheduler entirely and kubelet admission is the only gate. A pod rejected there is `Failed` with **no container statuses at all** — it never ran a line of the package's script. That shape only exists on a real kubelet; a fake client has no admission step to fail.

This is the distinction `jobFailureIsGenuine` is built around. Getting it wrong parks a stage as `erroring` and points an operator at a package, when the thing that needs attention is the node's capacity.

## Test Scenario

1. Install a package requesting more CPU and memory than any node has, so every attempt is refused (resource overrides are all-four-fields-or-none — see [resource-management.md](../../../../docs/operations/resource-management.md))
2. Assert an attempt pod is `Failed` with reason `OutOfcpu`
3. Poll across the whole retry budget asserting node state **never** becomes `erroring`

Step 3 polls rather than asserting once: the claim is that the state never flips during any attempt, not that it happens to be right at one instant.
