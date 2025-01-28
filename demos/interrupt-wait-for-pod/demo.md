Bring up node group with name: `demo` and 3 nodes

Label two of them so workload can go on
```
kubectl label node/ip-10-255-9-251.us-west-2.compute.internal demo=user-workload
```

Show which nodes have labels
```
for node in $(kubectl get nodes | cut -d" " -f 1); do echo $node; kubectl label --list node/${node} | grep demo; done
```

Apply workload
```
kubectl apply -f demo/workload.yaml
```

Apply SCR
```
kubectl apply -f demo/scr.yaml
```

Wait for work to finish on the one node that doesn't have workload pods on it.
Note how it DOES NOT taint/schedule skyhook work on nodes WITH workload pods.

Show status in Skyhook SCR that it is complete on the one node but unkown on other two and overall state is unkown.

Remove workload pods
```
kubectl delete -f demo/workload.yaml
```

Note how it now DOES drain, taint and schedule onto other nodes.

Status in SCR is now complete on ALL nodes and complete overall.