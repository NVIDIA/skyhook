# disruption-casualty

Validates that an attempt lost to a disruption costs no retry budget and is not recorded as a package failure.

## Why this needs a real cluster

The Job's `podFailurePolicy` ignores failures carrying the `DisruptionTarget` condition, so a drain, a preemption or a node scale-down replaces the attempt for free. Only the **eviction API** sets that condition — a plain `kubectl delete pod` does not — and only a real apiserver implements the eviction subresource. There is nothing to assert against a fake client.

Without the rule, routine cluster maintenance walks healthy packages into `erroring` by exhausting budgets they never spent on a real failure.

## Test Scenario

1. Install a package with a long-running apply stage and wait for an attempt to be genuinely executing
2. Evict its pod through `POST /api/v1/namespaces/<ns>/pods/<pod>/eviction`
3. Poll asserting `status.failed` stays empty and node state never becomes `erroring`
4. Assert a replacement attempt exists, so step 3 did not pass by there being no pods at all

Note the eviction has to go through `kubectl create --raw`; `kubectl create -f` on an `Eviction` object fails with `no matches for kind "Eviction" in version "policy/v1"`, because eviction is a subresource rather than a namespaced object.
