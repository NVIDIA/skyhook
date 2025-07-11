#!/bin/bash -x

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


n_to_taint=$1
taint=$2
label_match=${3}
i=0

taint="${taint}-"
# taint_key=$(echo ${taint} | cut -d: -f 1| cut -d= -f1)

for node in $(kubectl get nodes -o name -l ${label_match} | sort); do
    if [ $n_to_taint != "all" ]; then
        if [ $i -ge $n_to_taint ]; then
            break
        fi
        i=$((i+1))
    fi
    kubectl taint nodes ${node} ${taint}
done
# Some tests remove the taint as a part of the test but still want to try to clean it up if
# the test fails. So we ignore the error here.
exit 0
