# Demo: waiting for workload pods before an interrupt

This example demonstrates `podNonInterruptLabels` — the mechanism that stops
NodeWright from disrupting workloads you care about.

A package that declares an `interrupt` needs the node cordoned and drained before
it runs. `podNonInterruptLabels` puts a **barrier in front of that drain**: while
any pod matching those labels is still running on a node, NodeWright will not
drain it and will not start the interrupt. It waits.

Note the barrier sits *after* the cordon, not before it. A waiting node is
already cordoned, so nothing new schedules onto it while NodeWright waits for
your workload to finish — it just is not evicted.

The point of the demo is the contrast: two workloads run on the same nodes, one
matching the labels and one not, and only the matching one holds work back.

## What is in this directory

| File | What it is |
|---|---|
| `scr.yaml` | The NodeWright Custom Resource (NCR). One package with `interrupt: reboot`, and `podNonInterruptLabels` matching `app: nodewright-demo-workload`. |
| `workload.yaml` | A DaemonSet whose pods carry `app: nodewright-demo-workload` — **matches** the barrier, so it blocks. Pinned to nodes labelled `demo=user-workload`. |
| `non-workload.yaml` | A ReplicaSet whose pods carry `app: nodewright-demo-non-workload` — **does not match**, so it gets drained like any other pod. |

The NCR selects nodes with `eks.amazonaws.com/nodegroup: demo`. That label is
EKS-specific; on another provider, change `spec.nodeSelectors` to something your
nodes actually carry.

## Setup

Bring up a node group named `demo` with 3 nodes, then label two of them so the
blocking workload has somewhere to land:

```bash
kubectl label node/<node-1> demo=user-workload
kubectl label node/<node-2> demo=user-workload
```

Confirm which nodes got the label:

```bash
for node in $(kubectl get nodes -o name); do
  echo "$node"; kubectl label --list "$node" | grep demo
done
```

You should end up with two labelled nodes and one unlabelled.

## Run it

### 1. Start the workloads

```bash
kubectl apply -f examples/interrupt-wait-for-pod/workload.yaml
kubectl apply -f examples/interrupt-wait-for-pod/non-workload.yaml
```

The DaemonSet lands only on the two labelled nodes. The ReplicaSet spreads across
all three.

### 2. Apply the NCR

```bash
kubectl apply -f examples/interrupt-wait-for-pod/scr.yaml
```

### 3. Watch the third node finish, and the other two wait

```bash
kubectl get nodewright demo -w
```

Work completes on the **one node without workload pods**. The other two sit
waiting: the DaemonSet pod on each matches `podNonInterruptLabels`, so the drain
never starts. Note that the non-workload ReplicaSet pods on that third node were
drained without complaint — they do not match the barrier.

Where to look:

```bash
# Per-node status: complete on one node, still in progress on the other two
kubectl get nodewright demo -o jsonpath='{.status.nodeStatus}' | jq

# Overall status stays short of complete while any node is unfinished
kubectl get nodewright demo -o jsonpath='{.status.status}'

# Cordon state. Selected nodes are cordoned up front, including the two that
# are waiting — the barrier holds back the drain, not the cordon.
kubectl get nodes

# The package pods the operator created, and which node each is pinned to
kubectl get pods -n nodewright -o wide -l nodewright.nvidia.com/name=demo
```

### 4. Remove the blocking workload

```bash
kubectl delete -f examples/interrupt-wait-for-pod/workload.yaml
```

Once those pods are gone, the barrier lifts. NodeWright now cordons, drains and
interrupts the remaining two nodes, and the NCR reaches `complete` on all three.

```bash
kubectl get nodewright demo -o jsonpath='{.status.status}'
```

## Where the state actually lives

The NCR's `status` is the operator's published summary. The authoritative
per-package record is an annotation on each **node**:

```bash
kubectl get node <node-name> \
  -o jsonpath='{.metadata.annotations.nodewright\.nvidia\.com/nodeState_demo}' | jq
```

If a node is not progressing and you want to know why, the NCR's conditions are
usually the fastest answer — they surface ignored nodes, untolerated taints, and
a missing deployment policy:

```bash
kubectl get nodewright demo -o jsonpath='{.status.conditions}' | jq
```

## Clean up

```bash
kubectl delete -f examples/interrupt-wait-for-pod/scr.yaml
kubectl delete -f examples/interrupt-wait-for-pod/non-workload.yaml
kubectl delete -f examples/interrupt-wait-for-pod/workload.yaml   # if still present
```

## Related

- [Interrupt Flow](../../docs/architecture/interrupt-flow.md) — the full cordon → drain → interrupt sequence, and how `podNonInterruptLabels` differs from the configurable drain
- [The NodeWright Custom Resource](../../docs/user-guide/custom-resource.md) — every field used here
- [Lifecycle of a NodeWright](../../docs/architecture/lifecycle.md) — what the operator does between apply and complete
