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

set -e

# Usage: label-nodes.sh <operation> <node_range> <label1=value1> [label2=value2] ...
# Examples:
#   label-nodes.sh add 0-4 priority=critical skyhook.nvidia.com/test-node=skyhooke2e
#   label-nodes.sh remove 0-14 priority env region
#   label-nodes.sh clean-all skyhook.nvidia.com/test-node

OPERATION=$1
shift

if [ "$OPERATION" = "clean-all" ]; then
  LABEL_PREFIX=$1
  echo "Cleaning all labels matching: $LABEL_PREFIX"
  kubectl label nodes --all "${LABEL_PREFIX}-" --overwrite 2>/dev/null || true
  echo "✓ Cleanup complete"
  exit 0
fi

NODE_RANGE=$1
shift

# Get all worker nodes (excluding control-plane)
WORKERS=($(kubectl get nodes --no-headers -o custom-columns=NAME:.metadata.name | grep -v control-plane | sort))

# Parse node range
if [[ $NODE_RANGE == *-* ]]; then
  START=$(echo $NODE_RANGE | cut -d'-' -f1)
  END=$(echo $NODE_RANGE | cut -d'-' -f2)
else
  START=$NODE_RANGE
  END=$NODE_RANGE
fi

# Validate we have enough nodes
if [ ${#WORKERS[@]} -lt $((END + 1)) ]; then
  echo "ERROR: Need at least $((END + 1)) worker nodes for this operation"
  echo "Found: ${#WORKERS[@]} workers"
  exit 1
fi

case "$OPERATION" in
  add)
    LABELS="$@"
    echo "Adding labels to nodes [$START-$END]: $LABELS"
    for i in $(seq $START $END); do
      if [ -n "${WORKERS[$i]}" ]; then
        kubectl label node ${WORKERS[$i]} $LABELS --overwrite
      fi
    done
    ;;
  remove)
    LABELS_TO_REMOVE=""
    for label in "$@"; do
      LABELS_TO_REMOVE="$LABELS_TO_REMOVE ${label}-"
    done
    echo "Removing labels from nodes [$START-$END]: $@"
    for i in $(seq $START $END); do
      if [ -n "${WORKERS[$i]}" ]; then
        kubectl label node ${WORKERS[$i]} $LABELS_TO_REMOVE --overwrite 2>/dev/null || true
      fi
    done
    ;;
  *)
    echo "ERROR: Unknown operation: $OPERATION"
    echo "Usage: $0 {add|remove|clean-all} ..."
    exit 1
    ;;
esac

echo "✓ Operation complete"

