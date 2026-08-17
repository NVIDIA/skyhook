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


# change to the script's directory
cd $(dirname $0)

DASHBOARD_DIR="dashboards"
OUTPUT_FILE="grafana-dashboards-configmap.yaml"
CONFIGMAP_NAME="grafana-dashboards"

echo "generating configmap from dashboards in $DASHBOARD_DIR..."

# ensure output directory exists
mkdir -p "$(dirname "$OUTPUT_FILE")"

# start the YAML
cat <<EOF > "$OUTPUT_FILE"
apiVersion: v1
kind: ConfigMap
metadata:
  name: $CONFIGMAP_NAME
  labels:
    grafana_dashboard: "1"
data:
EOF

# add each JSON file
for file in "$DASHBOARD_DIR"/*.json; do
  filename=$(basename "$file")
  echo "  $filename: |" >> "$OUTPUT_FILE"
  sed 's/^/    /' "$file" >> "$OUTPUT_FILE"
  echo "" >> "$OUTPUT_FILE"
done

echo "configmap written to $OUTPUT_FILE"
