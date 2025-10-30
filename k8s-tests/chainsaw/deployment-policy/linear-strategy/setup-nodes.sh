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

echo "Labeling nodes for linear strategy test..."

# Get all worker nodes (nodes without control-plane role)
WORKERS=$(kubectl get nodes --no-headers | grep -v "control-plane" | awk '{print $1}')
WORKER_ARRAY=($WORKERS)

if [ ${#WORKER_ARRAY[@]} -lt 8 ]; then
  echo "ERROR: Need at least 8 worker nodes for this test"
  echo "Found: ${#WORKER_ARRAY[@]} workers"
  exit 1
fi

# Label all 8 nodes as production tier
echo "Labeling production nodes (0-7)..."
for i in {0..7}; do
  kubectl label node ${WORKER_ARRAY[$i]} tier=production skyhook.nvidia.com/test-node=skyhooke2e --overwrite
done

echo ""
echo "✓ Node labeling complete!"
echo ""
kubectl get nodes -L tier,skyhook.nvidia.com/test-node --no-headers | grep production | sort
