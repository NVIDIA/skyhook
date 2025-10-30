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

# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

echo "Labeling nodes for overlapping selectors test..."

# Get all worker nodes
WORKERS=$(kubectl get nodes --no-headers | grep -v "control-plane" | awk '{print $1}')
WORKER_ARRAY=($WORKERS)

if [ ${#WORKER_ARRAY[@]} -lt 6 ]; then
  echo "ERROR: Need at least 6 worker nodes for this test"
  echo "Found: ${#WORKER_ARRAY[@]} workers"
  exit 1
fi

# Node 0-1: Only region=us-west (matches us-west compartment)
echo "Labeling us-west-only nodes (0-1)..."
for i in {0..1}; do
  kubectl label node ${WORKER_ARRAY[$i]} region=us-west skyhook.nvidia.com/test-node=skyhooke2e --overwrite
done

# Node 2-3: Only env=production (matches production compartment)
echo "Labeling production-only nodes (2-3)..."
for i in {2..3}; do
  kubectl label node ${WORKER_ARRAY[$i]} env=production skyhook.nvidia.com/test-node=skyhooke2e --overwrite
done

# Node 4-5: BOTH region=us-west AND env=production (OVERLAPPING - should match prod-us-west compartment)
echo "Labeling overlapping nodes (4-5) with BOTH labels..."
for i in {4..5}; do
  kubectl label node ${WORKER_ARRAY[$i]} region=us-west env=production skyhook.nvidia.com/test-node=skyhooke2e --overwrite
done

echo ""
echo "✓ Node labeling complete!"
echo ""
echo "Node distribution:"
echo "  Nodes 0-1: region=us-west only"
echo "  Nodes 2-3: env=production only"
echo "  Nodes 4-5: region=us-west + env=production (overlapping)"
echo ""
kubectl get nodes -L region,env,skyhook.nvidia.com/test-node --no-headers | grep skyhooke2e | sort
