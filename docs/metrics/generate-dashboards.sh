#!/bin/bash

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