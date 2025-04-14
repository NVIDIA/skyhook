#!/bin/bash -x

# 
# LICENSE START
#
#    Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.
#
# LICENSE END
# 

## helper script
## clears labels and annotate from nodes with the prefix for tests

## Usage:
## ./rest_test.sh <skyhook-name> [selector]
## Examples:
##   ./rest_test.sh my-skyhook                                              # uses default selector
##   ./rest_test.sh my-skyhook "skyhook.nvidia.com/test-node=skyhooke2e"   # label selector
##   ./rest_test.sh my-skyhook "node-role.kubernetes.io/control-plane notin ()"  # expression selector

## the name of the skyhook to reset
skyhook="$1"

## the selector to use for finding nodes (defaults to test node selector)
selector="${2:-skyhook.nvidia.com/test-node=skyhooke2e}"

for node in $(kubectl get nodes -l "$selector" -o name); do
    kubectl annotate ${node} skyhook.nvidia.com/nodeState_${skyhook}-
    kubectl annotate ${node} skyhook.nvidia.com/status_${skyhook}-
    kubectl annotate ${node} skyhook.nvidia.com/cordon_${skyhook}-
    kubectl annotate ${node} skyhook.nvidia.com/version_${skyhook}-
    kubectl label ${node} skyhook.nvidia.com/status_${skyhook}-
done