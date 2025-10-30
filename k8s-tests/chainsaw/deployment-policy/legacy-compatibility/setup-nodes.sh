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

echo "Cleaning up any existing labels from previous tests..."
kubectl label nodes --all skyhook.nvidia.com/test-node- --overwrite 2>/dev/null || true

echo "Labeling nodes for legacy compatibility test..."

# Get all worker nodes (excluding control-plane) and label the first 6
WORKERS=($(kubectl get nodes --no-headers -o custom-columns=NAME:.metadata.name | grep -v control-plane | sort))

# Label first 6 worker nodes for the test
echo "Labeling test nodes (first 6 workers)..."
for i in {0..5}; do
  if [ -n "${WORKERS[$i]}" ]; then
    kubectl label node/${WORKERS[$i]} skyhook.nvidia.com/test-node=skyhooke2e --overwrite
  fi
done

echo ""
echo "✓ Node labeling complete!"
echo ""
kubectl get nodes -L skyhook.nvidia.com/test-node --sort-by=.metadata.name | head -8
