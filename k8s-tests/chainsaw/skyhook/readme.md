## Skyhook tests

This directory holds all the end to end tests for the skyhook operator itself. These tests ensure that the operator is working as expected and walks through the entire Skyhook and SCR workflows to do so.

## Agentless Test Image Versions

In `containers/agentless` there is a test container which is used in the e2e tests which sleeps for a little bit and then returns and is used to simulate the skyhook agent container running. Since the operator enforces strict versioning latest can not be used but instead semantic or calendar versioning has to be used. With this being said the test containers are built and tagged with a preset of valid semantic and calendar versions. Only these versions can be used in the e2e tests otherwise you will get an image-pull error as that image tag won't exist. These pre-defined versions can be found and adjusted at `containers/agentless/versions.sh`. For more info on how to update these versions refer to the comment in `versions.sh`.