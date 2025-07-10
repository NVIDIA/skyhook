#!/bin/bash

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




node_name=$1
action=$2

setup() {
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
    name: ${node_name}-debugger
    namespace: default
spec:
    nodeName: ${node_name}
    
    tolerations:
        # Tolerate the this taint so it wont be drained by operator
        - key: node.kubernetes.io/unschedulable
          operator: "Exists"
        # Tolerate the skyhook unschedulable taint so it wont go away during interrupt tests
        - key: skyhook.nvidia.com/unschedulable
          operator: "Exists"
    containers:
        - name: debugger
          image: ubuntu:22.04
          command:
            - sleep
            - "infinity"
          imagePullPolicy: IfNotPresent
          securityContext: 
            privileged: true
          volumeMounts:
            - mountPath: /host
              name: host-root 
    volumes:
        - hostPath:
            path: /
            type: ""
          name: host-root
EOF

    while true; do
        status=$(kubectl get pod -n default ${node_name}-debugger -o jsonpath="{.status.phase}")
        if [ "$status" == "Running" ]; then
            break
        fi
        echo "Waiting for ${node_name}-debugger to be running"
        sleep 1
    done
}

teardown() {
    kubectl delete pod -n default ${node_name}-debugger
}

case $action in
    setup)
        setup
        ;;
    teardown)
        teardown
        ;;
esac
