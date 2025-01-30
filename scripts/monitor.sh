#!/bin/bash

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


if [ -f $PWD/monitor.txt ]; then
    echo "cpu"
    cut -d" " -f1 ~/git_repos/dgx/infra/skyhook-operator/monitor.txt |sed 's/m//g'| sort -rn | head -5
    echo "memory"
    cut -d" " -f2 ~/git_repos/dgx/infra/skyhook-operator/monitor.txt |sed 's/Mi//g'| sort -rn | head -5
    rm $PWD/monitor.txt
fi


pods=$(kubectl get pods -n skyhook-operator | grep skyhook-operator-controller-manager | awk '{print $1}')
while true; do
    for pod in ${pods}; do
        kubectl top pod $pod -n skyhook-operator --no-headers | tr -s ' ' | cut -d" " -f2,3 >> $PWD/monitor.txt
    done
    sleep 2
done
