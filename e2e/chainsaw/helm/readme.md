## Helm Tests
This directory holds all the tests for the skyhook operator's helm chart. Right now this mainly ensures that tolerations set in the helm chart actually work and that the operator can be deployed successfully under another deployment name than skyhook-operator.

## Test Image
The image that is used by these tests should be `nvcr.io/nvidian/swgpu-baseos/skyhook-operator:test` since this will be built in CI every time a commit is pushed to Gitlab and will make sure that you current changes to the operator are compatible with the helm chart still.

**NOTE:** When you run the helm chart tests locally it may be using an outdated version of the test image since it hasn't been pushed and built by the CI. Be careful in the assumptions you make as your changes to the operator may pass the helm chart tests locally but fail in CI.