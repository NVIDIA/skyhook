#!/bin/bash

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














## prints out all resources that are namespaced
## can be helpful for debuging

ns=${1:-skyhook}

for resource in $(kubectl api-resources --verbs=list --namespaced -o name); do 
    echo "resource: ${resource}"
    kubectl get --show-kind -n ${ns} ${resource}
done