# Local Development

## ctlptl Setup

The local development targets install the pinned `ctlptl` binary through `operator/deps.mk` into `operator/bin`. No global install is required when using the Makefile.

If you want a standalone `ctlptl` on your `PATH`, install the same version pinned in `operator/deps.mk`:

```bash
go install github.com/tilt-dev/ctlptl/cmd/ctlptl@v0.9.3
```

Homebrew is also available for manual use:

```bash
brew install tilt-dev/tap/ctlptl
```

Keep manual installs in sync with `CTLPTL_VERSION` in `operator/deps.mk`.

## Kubernetes Test Versions

`operator/k8s-test-versions.mk` is the source of truth for Kubernetes versions used by envtest, local kind clusters, and GitHub Actions test matrices.

- `ENVTEST_K8S_VERSION` controls `setup-envtest` binary assets under `operator/bin/k8s/`.
- `KIND_NODE_IMAGE_VERSION` controls the default `kindest/node` image for local kind clusters. `KIND_VERSION` remains as a backward-compatible alias for local overrides.
- `KIND_BINARY_VERSION` controls the Kind CLI/action version and is intentionally separate from Kubernetes node image versions.
- `CI_KIND_NODE_IMAGE_VERSIONS_JSON` and `CI_PRIMARY_KIND_NODE_IMAGE_VERSION` feed the GitHub Actions matrix.

`make unit-tests` installs the configured envtest assets and exports `KUBEBUILDER_ASSETS` for the Go test process. If you run `go test` or `ginkgo` directly, set `KUBEBUILDER_ASSETS` yourself or controller-runtime will use its default `/usr/local/kubebuilder` lookup path.

These versions often match, but they are allowed to diverge. Kind does not publish every Kubernetes patch version as a `kindest/node` image, so validate a new node image before using it:

```bash
cd operator
make validate-kind-node-image KIND_NODE_IMAGE_VERSION=1.35.0
```

When updating supported test versions, edit `operator/k8s-test-versions.mk` first, then run the validation target and the relevant tests.

## Local Cluster

Bring up the kind cluster and local registry:

```bash
cd operator
make create-kind-cluster
```

`make create-kind-cluster` renders `config/local-dev/ctlptl-config.yaml` into `reporting/ctlptl-config.yaml`, deletes any existing cluster defined by that rendered config, and then creates the kind cluster plus a local registry at `localhost:5005`. It also runs `make setup-kind-cluster`, which is idempotent and labels the test node and creates the `skyhook` namespace pull secret.

Do not run `ctlptl apply -f config/local-dev/ctlptl-config.yaml` directly; the tracked file is a Make-rendered template.

## Webhook Iteration

Webhook development uses the operator pod, so rebuilding and restarting the deployment is the main iteration loop:

```bash
cd operator
make rollout-local
```

The local registry removes the need for `kind load docker-image` or registry-pinned chart value edits while iterating on operator or webhook code.

## Teardown

Delete the cluster and registry with the rendered ctlptl config:

```bash
cd operator
make delete-kind-cluster
```
