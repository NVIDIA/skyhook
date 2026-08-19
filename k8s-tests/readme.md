# E2E Tests

Most of these are E2E tests using a declarative framework called [Chainsaw](https://github.com/kyverno/chainsaw). You define YAML files to create resources and assert that expected state is achieved. The `migration` suite is the one exception; see below.

## Test Suites

| Suite | Directory | Framework | Description |
|-------|-----------|-----------|-------------|
| **NodeWright** | [chainsaw/nodewright/](./chainsaw/nodewright/) | Chainsaw | Core operator functionality tests |
| **Helm** | [chainsaw/helm/](./chainsaw/helm/) | Chainsaw | Helm chart deployment and configuration tests |
| **Deployment Policy** | [chainsaw/deployment-policy/](./chainsaw/deployment-policy/) | Chainsaw | Deployment policy and rollout strategy tests |
| **CLI** | [chainsaw/cli/](./chainsaw/cli/) | Chainsaw | kubectl-nodewright CLI command tests |
| **Migration** | [migration/](./migration/) | bash | Upgrade from the last pre-rename release and assert the Skyhook to NodeWright migration preserves node state |

For more information on each suite, refer to their respective README files.

**Why the migration suite is not Chainsaw.** Its central assertion is that a value is *unchanged across an operator upgrade*, which means capturing state before the upgrade and comparing after. Chainsaw asserts that the cluster matches a declared shape at a point in time; it has no way to carry a captured value across steps and diff against it. That suite is driven by [migration/run.sh](./migration/run.sh) instead, and is run by `make migration-test` rather than `make e2e-tests`. Add new tests to a Chainsaw suite unless you need that same before/after comparison.

## Creating a New Test

To create a new test:

1. Make a new folder in the appropriate test suite directory
2. Add a file named `chainsaw-test.yaml` with the test configuration
3. Add a `README.md` documenting the test (see standards below)
4. Add any additional YAML files needed (nodewright definitions, assertions, etc.)

## Test Documentation Standards

All E2E tests must have a `README.md` file in their test directory. This provides better GitHub visibility (READMEs auto-display in the web UI) and clearer documentation.

### README Structure

Each test README should include:

- **Test Name** as H1 heading
- **Purpose** section explaining what is being tested
- **Test Scenario** describing the test flow/steps
- **Key Features Tested** as bullet points
- **Files** section listing test files and their purpose
- **Notes** for special considerations (if applicable)

### Example README

See [helm-node-affinity-test/README.md](./chainsaw/helm/helm-node-affinity-test/README.md) for a good example.

Basic structure:
```markdown
# Test Name

## Purpose
Brief description of what this test validates.

## Test Scenario
1. Step one
2. Step two
3. Assert expected state

## Key Features Tested
- Feature A
- Feature B

## Files
- `chainsaw-test.yaml` - Main test configuration
- `nodewright.yaml` - NodeWright resource definition
- `assert.yaml` - State assertions

## Notes
- Any special considerations or requirements
```

### What NOT to Do

- **Do not** use `spec.description` in `chainsaw-test.yaml` files for documentation
- **Do not** use long YAML comments to document tests
- Keep YAML files focused on test configuration only
- Keep the README as the single source of documentation

## Manual Testing

Due to some limitations it can be hard to test circumstances where a node will be removed from a cluster. The operator performs cleanup on node removal (removing orphan configmaps) which makes automated testing difficult. For these scenarios, test manually:

1. Run `make create-kind-cluster` and wait for local cluster to be brought up
2. Run `make install` to install the nodewright CRD into the cluster
3. Use VSCode's debugger to run the operator with your local cluster
4. Use `kubectl apply -f k8s-tests/chainsaw/nodewright/simple-nodewright/nodewright.yaml` to define a nodewright
5. Use `kubectl` or `k9s` to overview the state of the nodewright and its resources. Look for the configmap named `{nodewright.Name}-{node.Name}-metadata`
6. Remove the node with `kubectl delete node {node.Name}`
7. Verify the configmap `{nodewright.Name}-{node.Name}-metadata` no longer exists
8. Note: Kind doesn't autoscale, so you'll need to rebuild the cluster to continue testing (`make create-kind-cluster`)

## Agentless Test Image

The tests use a test container image from `containers/agentless` which sleeps briefly and returns, simulating the nodewright agent. Since the operator enforces strict versioning (no `latest` tag), only predefined semantic or calendar versions can be used. See `containers/agentless/versions.sh` for available versions.
