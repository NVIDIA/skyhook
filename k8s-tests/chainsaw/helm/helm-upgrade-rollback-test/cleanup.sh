#!/bin/bash -x

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

## Best-effort teardown, deliberately without `-e`. This test is expected to fail at
## the rollback step until #469 is fixed, and a failed test that leaves the release
## behind breaks every later test in this directory: they install under different
## release names into the same namespace, and the kept CRDs stay stamped with this
## one's release, which fails the next install with "invalid ownership metadata".
##
## The plain uninstall is not enough on its own here: a wedged release can leave helm
## unable to delete it cleanly, and then uninstall-helm-chart.sh exits before it gets
## to the CRDs it normally removes for exactly this reason.

RELEASE=$1
NAMESPACE=nodewright

if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else
    HELM=$(which helm)
fi

../uninstall-helm-chart.sh "${RELEASE}"
${HELM} uninstall "${RELEASE}" -n "${NAMESPACE}" --no-hooks --ignore-not-found

kubectl delete crd nodewrights.nodewright.nvidia.com \
    deploymentpolicies.nodewright.nvidia.com --ignore-not-found --timeout=120s

exit 0
