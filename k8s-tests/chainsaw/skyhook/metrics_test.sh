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


# Usage: ./metrics_test.sh <metric_name>
# Env vars:
#   TIMEOUT: total time to wait (default: 10s)
#   PERIOD: time between checks (default: 1s)

set -euo pipefail

METRIC_NAME=$1
if [[ -z "$METRIC_NAME" ]]; then
  echo "Usage: $0 <metric_name> <metric_key> <metric_value>"
  exit 2
fi

METRIC_KEY=$2
if [[ -z "$METRIC_KEY" ]]; then
  echo "Usage: $0 <metric_name> <metric_key> <metric_value>"
  exit 2
fi

METRIC_VALUE=$3
if [[ -z "$METRIC_VALUE" ]]; then
  echo "Usage: $0 <metric_name> <metric_key> <metric_value>"
  exit 2
fi

TIMEOUT="${TIMEOUT:-10}"
PERIOD="${PERIOD:-1}"

start_time=$(date +%s)
end_time=$(( $(date +%s) + TIMEOUT ))

while true; do
  #curl -s http://127.0.0.1:8080/metrics | grep "^$METRIC_NAME"
  if curl -s http://127.0.0.1:8080/metrics | grep "^$METRIC_NAME" | grep $METRIC_KEY | grep -q "$METRIC_VALUE\$"; then
    exit 0
  fi
  now=$(date +%s)
  if (( now >= end_time )); then
    echo "Metric $METRIC_NAME not found after $TIMEOUT seconds"
    exit 1
  fi
  sleep "$PERIOD"
done
