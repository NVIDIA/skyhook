#!/bin/bash -xe

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

## Rolls the release back to the pre-rename baseline and requires it to converge.
## This is the escape hatch docs/getting-started/migration.md offers, so it has to work
## with no manual steps: see #469.

RELEASE=$1
REVISION=$2
NAMESPACE=nodewright

if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else
    HELM=$(which helm)
fi

## The revision is explicit on purpose. `helm rollback` with no revision targets the
## previous *deployed* one, which after a failed rollback is the post-rename release
## rather than the baseline, so the command silently becomes a no-op you can run twice
## and conclude the rollback worked.
${HELM} history "${RELEASE}" -n "${NAMESPACE}"

## --wait, and no `|| true`: a rollback that leaves the release `failed` with no Ready
## operator is the bug under test, not an acceptable outcome to assert around.
${HELM} rollback "${RELEASE}" "${REVISION}" -n "${NAMESPACE}" --wait --timeout 5m

status=$(${HELM} status "${RELEASE}" -n "${NAMESPACE}")
echo "${status}"
if ! echo "${status}" | grep -q "STATUS: deployed"; then
    echo "ERROR: release is not 'deployed' after rollback"
    exit 1
fi
