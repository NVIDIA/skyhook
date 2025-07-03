#!/bin/bash -e

# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


## this test is to verify the migration from 0.4.0 to 0.5.0 works as expected

## NOTE: making this a script to be more reproducible, but today making this work in CI, might be hard
## so for now starting here and will move to a chainsaw in another ticket

## steps:
## 1. install cert-manager
## 2. install 0.4.0 of the operator by using git to checkout the v0.4.0 tag
## 3. run a chainsaw test to create a few resources, but tell it to skip delete
## 4. upgrade the operator to 0.5.0 (which is current HEAD, but will need to be updated to tag once 0.5.0 is released)
## 5. clean up the resources created

PROJECT_ROOT=${1:-../../../} ## pass in the root of the repo, run from here
CLEANUP=${2:-false} ## pass in false to keep the resources created
OPERATOR_NAME=skyhook-operator

if [ -n "$GITLAB_CI" ]; then
    CHAINSAW=/workspace/bin/chainsaw
    HELM=/workspace/bin/helm
else 
    CHAINSAW=$(which chainsaw)
    HELM=$(which helm)
fi

# install cert-manager
${PROJECT_ROOT}/e2e/chainsaw/helm/install-cert-manager.sh v1.16.2

# install 0.4.0 of the operator
git checkout v0.4.0
${HELM} install ${OPERATOR_NAME} ${PROJECT_ROOT}/chart -n skyhook --set controllerManager.manager.image.tag=v0.4.0

# run chainsaw test
## NOTE: the directory struture was different back then
${CHAINSAW} test --test-dir ${PROJECT_ROOT}/e2e/chainsaw/simple-update-skyhook --skip-delete --exec-timeout 30s

# upgrade the operator to 0.5.0
git checkout migration_support_v2 ## TODO: update this to the tag once 0.5.0 is released
${HELM} upgrade ${OPERATOR_NAME} ${PROJECT_ROOT}/chart -n skyhook --set controllerManager.manager.image.tag=test ## todo update once 0.5.0 is released

# clean up
if [ "$CLEANUP" = true ]; then
    ${HELM} uninstall ${OPERATOR_NAME} -n skyhook
    ${PROJECT_ROOT}/e2e/chainsaw/helm/uninstall-cert-manager.sh
fi
