#!/bin/bash -eox pipefail

# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

## helper script
## clears labels and annotate from nodes with the prefix "skyhook.nvidia.com"
## note, a lot of tests have a label setup to target, so you might need to put that back
## example:
## ❯ kubectl label node/kind-worker skyhook.nvidia.com/test-node=skyhooke2e

for node in $(kubectl get nodes -o name); do
    for anno in $(kubectl annotate --list ${node}); do
        [[ ${anno} =~ (^skyhook.nvidia.com\/.*)=.* ]] && kubectl annotate ${node} ${BASH_REMATCH[1]}-
    done
    for label in $(kubectl label --list ${node}); do
        [[ ${label} =~ (^skyhook.nvidia.com\/.*)=.* ]] && kubectl label ${node} ${BASH_REMATCH[1]}-
    done
done
