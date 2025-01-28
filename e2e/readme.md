## chainsaw
These are E2E tests using a declarative framework, called [chainsaw](https://github.com/kyverno/chainsaw). You define yaml files to create resources and assert state is archived.


To create a new test, make a new folder. Add a file named chainsaw-test.yaml and then added any additional files needed to create a test. [hello-world](./chainsaw/hello-world/) is an example test that creates a configmap and asserts that it was created.

There are two classifications of e2e tests in this project one for the skyhook operator and another for the helm chart. You can see the corresponding tests for these in the subdirectories `helm` and `skyhook`. For more information on these refer to their respective README in both of those directories.

## manual testing
Due to some limitatations it can be hard to test circumstances where a node will be removed from a cluster, and there are some things that the operator does on the removal of a node (remove orphan config maps for the removed node) which makes testing this difficult. This means you will have to test manually. One way to do this is to use `make create-kind-cluster` to bring up a local kind cluster then use `kubectl delete kind-worker` to bring down the node when you are ready. If you have a node pool in a CSP you can also use `kubectl` in order to test this functionality. For demonstration purposes the process for removing a node and testing whether or not an orphan configmap is cleaned up or not will be overviewed:

1. Run `make create-kind-cluster`, and wait for local cluster to be brought up.
2. Run `make install` to install the skyhook CRD into the cluster.
3. Use vscode's debugger in order to run the operator with your local cluster.
4. Use `kubectl apply -f e2e/chainsaw/simple-skyhook/skyhook.yaml` to define a skyhook.
5. You can then use `kubectl` or `k9s` in order to overview the state of the skyhook as well as it's resources. In this instance we are looking for the configmap named `{skyhook.Name}-{node.Name}-metadata`. 
6. Now that we can see that the configmap exists we can remove the node to see if the configmap will be deleted accordingly. We'll do so with the command `kubectl delete node {node.Name}`.
7. Now check to see whether the configmap named `{skyhook.Name}-{node.Name}-metadata` still exists and if it doesn't then everything is working.
8. Unfortunately kind doesn't autoscale meaning that a new node won't be brought up. This means that you will most likely need to build a new cluster if you plan to continue testing or use `make test` to run the rest of the tests which you can do with the same make command specified in step 1 `make create-kind-cluster`.