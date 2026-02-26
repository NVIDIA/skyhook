#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Scale test for skyhook-operator: scale EKS node group stepwise, run a minimal
# 10s Skyhook with exponential policy, record operator memory at each phase.
# Requires: kubectl, aws CLI, jq. Optional: metrics-server (for --metrics-source=kube).
set -euo pipefail

SKYHOOK_NAME="scale-test-skyhook"
POLICY_NAME="scale-test-policy"
NODE_GROUP_LABEL="eks.amazonaws.com/nodegroup"
OPERATOR_LABEL="control-plane=controller-manager"
ANNOTATION_PREFIX="skyhook.nvidia.com"
NODE_STATE_ANN="${ANNOTATION_PREFIX}/nodeState_${SKYHOOK_NAME}"
STATUS_ANN="${ANNOTATION_PREFIX}/status_${SKYHOOK_NAME}"
CORDON_ANN="${ANNOTATION_PREFIX}/cordon_${SKYHOOK_NAME}"
VERSION_ANN="${ANNOTATION_PREFIX}/version_${SKYHOOK_NAME}"
STATUS_LABEL="${ANNOTATION_PREFIX}/status_${SKYHOOK_NAME}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICY_YAML="${SCRIPT_DIR}/scale_test_policy.yaml"
SKYHOOK_YAML="${SCRIPT_DIR}/scale_test_skyhook.yaml"

# Defaults
NAMESPACE="${NAMESPACE:-skyhook-operator}"
METRICS_SOURCE="${METRICS_SOURCE:-kube}"
OUTPUT_FILE=""
CLEAR_ANNOTATIONS=1
FAKE_LABELS=0
DEFAULT_OUTPUT_PREFIX="scale_test_results"
NODE_READY_TIMEOUT=2400
SKYHOOK_COMPLETE_TIMEOUT=2400
ROLLOUT_START_TIMEOUT=120

usage() {
  cat <<EOF
Usage: $0 --cluster-name NAME --node-group NAME --start-size N --step N --final-size N [options]

Required:
  --cluster-name   EKS cluster name
  --node-group     EKS node group name (used as node selector)
  --start-size     Initial desired node count
  --step           Increment per iteration (e.g. 2 -> run at start, start+step, ... final)
  --final-size     Final desired node count (inclusive)

Options:
  --namespace           Operator namespace (default: skyhook-operator)
  --metrics-source      kube|prometheus (default: kube). kube uses kubectl top; prometheus uses operator :8080/metrics.
  --output               Write CSV results to this path (default: ./scale_test_results_<timestamp>.csv)
  --no-clear-annotations Do not remove skyhook annotations/labels from nodes (Skyhook/Policy CRs are still deleted)
  --fake-labels N        Add N fake labels to the node group at test start (skyhook/fake_1=1 .. skyhook/fake_N=N) to test operator scaling vs label count
  --help                 This help.

Prerequisites: kubectl context set to cluster, aws CLI, jq. For kube metrics: metrics-server.
EOF
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name)   CLUSTER_NAME="$2"; shift 2 ;;
    --node-group)    NODEGROUP="$2";    shift 2 ;;
    --start-size)    START_SIZE="$2";  shift 2 ;;
    --step)          STEP="$2";        shift 2 ;;
    --final-size)    FINAL_SIZE="$2";  shift 2 ;;
    --namespace)     NAMESPACE="$2";   shift 2 ;;
    --metrics-source)      METRICS_SOURCE="$2"; shift 2 ;;
    --output)              OUTPUT_FILE="$2"; shift 2 ;;
    --no-clear-annotations) CLEAR_ANNOTATIONS=0; shift ;;
    --fake-labels)         FAKE_LABELS="$2"; shift 2 ;;
    --help)                usage ;;
    *) echo "Unknown option: $1"; usage ;;
  esac
done

for v in CLUSTER_NAME NODEGROUP START_SIZE STEP FINAL_SIZE; do
  if [[ -z "${!v:-}" ]]; then
    echo "Missing required argument: --$(echo "$v" | tr '[:upper:]' '[:lower:]' | sed 's/_/-/g')"
    usage
  fi
done

if [[ "$METRICS_SOURCE" != "kube" && "$METRICS_SOURCE" != "prometheus" ]]; then
  echo "Invalid --metrics-source (must be kube or prometheus)"
  exit 1
fi

# Default output file so we always write measurements as we go
if [[ -z "${OUTPUT_FILE:-}" ]]; then
  OUTPUT_FILE="${DEFAULT_OUTPUT_PREFIX}_$(date +%Y%m%d-%H%M%S).csv"
  echo "Writing memory measurements to ${OUTPUT_FILE}"
fi

FIRST_RECORD=1
record_result() {
  local node_count="$1"
  local phase="$2"
  local memory_mb="$3"
  local ts
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if [[ $FIRST_RECORD -eq 1 ]]; then
    echo "node_count,phase,memory_mb,timestamp"
    FIRST_RECORD=0
  fi
  echo "${node_count},${phase},${memory_mb},${ts}"
  if [[ -n "${OUTPUT_FILE:-}" ]]; then
    if [[ ! -f "$OUTPUT_FILE" ]]; then
      echo "node_count,phase,memory_mb,timestamp" >> "$OUTPUT_FILE"
    fi
    echo "${node_count},${phase},${memory_mb},${ts}" >> "$OUTPUT_FILE"
  fi
}

get_memory_kube() {
  local max_mb=0
  local line raw m
  while read -r line; do
    [[ -z "$line" ]] && continue
    raw="$(echo "$line" | awk '{ print $NF }')"
    m=0
    if [[ "$raw" == *Gi ]]; then
      m="${raw%%Gi}"
      m=$((m * 1024))
    elif [[ "$raw" == *Mi ]]; then
      m="${raw%%Mi}"
    elif [[ "$raw" == *Ki ]]; then
      m="${raw%%Ki}"
      m=$((m / 1024))
    else
      [[ "$raw" =~ ^[0-9]+$ ]] && m=$((raw / 1024 / 1024))
    fi
    [[ -n "$m" && "$m" =~ ^[0-9]+$ && "$m" -gt "$max_mb" ]] && max_mb=$m
  done < <(kubectl top pod -n "$NAMESPACE" -l "$OPERATOR_LABEL" --no-headers 2>/dev/null)
  echo "$max_mb"
}

get_memory_prometheus() {
  local pod
  pod="$(kubectl get pod -n "$NAMESPACE" -l "$OPERATOR_LABEL" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [[ -z "$pod" ]]; then
    echo "0"
    return
  fi
  kubectl port-forward -n "$NAMESPACE" "pod/$pod" 18080:8080 &>/dev/null &
  local pf_pid=$!
  trap "kill $pf_pid 2>/dev/null || true" RETURN
  local mb=0
  local metrics
  for _ in 1 2 3 4 5; do
    sleep 2
    metrics="$(curl -s "http://127.0.0.1:18080/metrics" 2>/dev/null)"
    # Prefer process RSS (matches kubectl top / container memory); then Go sys; then heap alloc
    local line
    line="$(echo "$metrics" | grep -E '^process_resident_memory_bytes ' | head -1)"
    [[ -z "$line" ]] && line="$(echo "$metrics" | grep -E '^go_memstats_sys_bytes ' | head -1)"
    [[ -z "$line" ]] && line="$(echo "$metrics" | grep -E '^go_memstats_alloc_bytes ' | head -1)"
    if [[ -n "$line" ]]; then
      # Value may be integer or scientific (e.g. 3.982656e+06); awk handles both
      mb="$(echo "$line" | awk '{ v = $2 + 0; printf "%d", v / 1024 / 1024 }')"
      [[ -n "$mb" && "$mb" -ge 0 ]] 2>/dev/null && break
    fi
  done
  kill $pf_pid 2>/dev/null || true
  trap - RETURN
  echo "${mb:-0}"
}

get_memory_mb() {
  if [[ "$METRICS_SOURCE" == "prometheus" ]]; then
    get_memory_prometheus
  else
    get_memory_kube
  fi
}

# Apply N fake labels (skyhook/fake_1=1 .. skyhook/fake_N=N) to the node group and to all its nodes.
apply_fake_labels_to_node_group() {
  local n=$1
  [[ "$n" -le 0 ]] && return 0
  echo "Adding $n fake labels to node group (skyhook/fake_1=1 .. skyhook/fake_${n}=${n})..."
  local add_or_update=""
  local i=1
  while [[ $i -le $n ]]; do
    [[ -n "$add_or_update" ]] && add_or_update="${add_or_update},"
    add_or_update="${add_or_update}\"skyhook/fake_${i}\":\"${i}\""
    i=$((i + 1))
  done
  local labels_json="{\"addOrUpdateLabels\":{${add_or_update}}}"
  aws eks update-nodegroup-config \
    --cluster-name "$CLUSTER_NAME" \
    --nodegroup-name "$NODEGROUP" \
    --labels "$labels_json" \
    --output json >/dev/null 2>&1 || { echo "Warning: EKS node group label update failed (labels may still apply to existing nodes)"; }
  local nodes
  nodes="$(kubectl get nodes -l "${NODE_GROUP_LABEL}=${NODEGROUP}" --chunk-size=0 -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)"
  local label_args=""
  i=1
  while [[ $i -le $n ]]; do
    label_args="${label_args} skyhook/fake_${i}=${i}"
    i=$((i + 1))
  done
  for node in $nodes; do
    [[ -z "$node" ]] && continue
    kubectl label node "$node" $label_args --overwrite 2>/dev/null || true
  done
  echo "Fake labels applied."
}

scale_node_group() {
  local target=$1
  echo "Scaling node group to $target..."
  local out
  out="$(aws eks update-nodegroup-config \
    --cluster-name "$CLUSTER_NAME" \
    --nodegroup-name "$NODEGROUP" \
    --scaling-config "minSize=1,maxSize=$target,desiredSize=$target" \
    --output json 2>&1)" || { echo "$out"; return 1; }
  local update_id
  update_id="$(echo "$out" | jq -r '.update.id // empty')"
  if [[ -n "$update_id" ]]; then
    echo "EKS update started: $update_id (waiting for node count/Ready instead of update status)"
  fi
  # Do not block on describe-update: it can stay InProgress even when the node group
  # is already at desired size. Rely on wait_for_nodes_ready for the real condition.
}

wait_for_nodes_ready() {
  local target=$1
  echo "Waiting for $target nodes (label ${NODE_GROUP_LABEL}=${NODEGROUP}) to be Ready..."
  local waited=0
  while [[ $waited -lt $NODE_READY_TIMEOUT ]]; do
    local ready
    ready="$(kubectl get nodes -l "${NODE_GROUP_LABEL}=${NODEGROUP}" --chunk-size=0 -o json 2>/dev/null | jq '[.items[] | select(.status.conditions[]? | select(.type=="Ready" and .status=="True"))] | length')"
    local total
    total="$(kubectl get nodes -l "${NODE_GROUP_LABEL}=${NODEGROUP}" --chunk-size=0 --no-headers 2>/dev/null | wc -l)"
    if [[ "${total:-0}" -eq "$target" && "${ready:-0}" -eq "$target" ]]; then
      echo "All $target nodes Ready."
      return 0
    fi
    echo "  nodes: $ready/$target ready (total $total)"
    sleep 10
    waited=$((waited + 10))
  done
  echo "Timeout waiting for nodes."
  return 1
}

ensure_no_test_resources() {
  echo "Removing any existing scale-test Skyhook and Policy..."
  kubectl delete skyhook "$SKYHOOK_NAME" --ignore-not-found --timeout=30s 2>/dev/null || true
  kubectl delete deploymentpolicy "$POLICY_NAME" --ignore-not-found --timeout=30s 2>/dev/null || true
  if [[ "${CLEAR_ANNOTATIONS:-1}" -eq 1 ]]; then
    echo "Cleaning scale-test annotations/labels from nodes..."
    local nodes
    nodes="$(kubectl get nodes -l "${NODE_GROUP_LABEL}=${NODEGROUP}" --chunk-size=0 -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)"
    for node in $nodes; do
      for key in "$NODE_STATE_ANN" "$STATUS_ANN" "$CORDON_ANN" "$VERSION_ANN"; do
        kubectl annotate node "$node" "${key}"- --overwrite 2>/dev/null || true
      done
      kubectl label node "$node" "${STATUS_LABEL}-" --overwrite 2>/dev/null || true
    done
  fi
  sleep 5
}

apply_policy_and_skyhook() {
  sed "s/NODEGROUP_PLACEHOLDER/${NODEGROUP}/g" "$POLICY_YAML" | kubectl apply -f -
  sed "s/NODEGROUP_PLACEHOLDER/${NODEGROUP}/g" "$SKYHOOK_YAML" | kubectl apply -f -
}

wait_for_rollout_start() {
  echo "Waiting for rollout to start (nodesInProgress > 0)..."
  local waited=0
  while [[ $waited -lt $ROLLOUT_START_TIMEOUT ]]; do
    local in_progress
    in_progress="$(kubectl get skyhook "$SKYHOOK_NAME" -o jsonpath='{.status.nodesInProgress}' 2>/dev/null || echo "0")"
    if [[ "${in_progress:-0}" -gt 0 ]]; then
      echo "Rollout started (nodesInProgress=$in_progress)."
      return 0
    fi
    sleep 5
    waited=$((waited + 5))
  done
  echo "Rollout may not have started; continuing."
}

wait_for_skyhook_complete() {
  local target=$1
  echo "Waiting for Skyhook to complete on $target nodes..."
  local waited=0
  local next_record_threshold=1
  while [[ $waited -lt $SKYHOOK_COMPLETE_TIMEOUT ]]; do
    local complete_str
    complete_str="$(kubectl get skyhook "$SKYHOOK_NAME" -o jsonpath='{.status.completeNodes}' 2>/dev/null)"
    local count=0
    if [[ "$complete_str" =~ ^([0-9]+)/([0-9]+)$ ]]; then
      count="${BASH_REMATCH[1]}"
    fi
    if [[ "$count" -ge "$target" ]]; then
      echo "Skyhook complete on $count nodes."
      return 0
    fi
    if [[ "$count" -ge "$next_record_threshold" ]]; then
      echo "Measuring operator memory (while completing, $count nodes done)..."
      local mem_mb
      mem_mb="$(get_memory_mb)"
      echo "Memory (while completing at $count): ${mem_mb} Mi"
      record_result "$target" "while_completing" "$mem_mb"
      next_record_threshold=$((next_record_threshold * 2))
    fi
    local status
    status="$(kubectl get skyhook "$SKYHOOK_NAME" -o jsonpath='{.status.status}' 2>/dev/null)"
    echo "  complete: $complete_str (target=$target, status=$status)"
    sleep 10
    waited=$((waited + 10))
  done
  echo "Timeout waiting for Skyhook completion."
  return 1
}

cleanup_skyhook_and_nodes() {
  echo "Deleting Skyhook and Policy..."
  kubectl delete skyhook "$SKYHOOK_NAME" --ignore-not-found --timeout=60s 2>/dev/null || true
  kubectl delete deploymentpolicy "$POLICY_NAME" --ignore-not-found --timeout=30s 2>/dev/null || true
  if [[ "${CLEAR_ANNOTATIONS:-1}" -eq 1 ]]; then
    echo "Cleaning scale-test annotations/labels from nodes..."
    local nodes
    nodes="$(kubectl get nodes -l "${NODE_GROUP_LABEL}=${NODEGROUP}" --chunk-size=0 -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)"
    for node in $nodes; do
      for key in "$NODE_STATE_ANN" "$STATUS_ANN" "$CORDON_ANN" "$VERSION_ANN"; do
        kubectl annotate node "$node" "${key}"- --overwrite 2>/dev/null || true
      done
      kubectl label node "$node" "${STATUS_LABEL}-" --overwrite 2>/dev/null || true
    done
  fi
}

run_one_iteration() {
  local target=$1
  scale_node_group "$target"
  wait_for_nodes_ready "$target"

  ensure_no_test_resources
  sleep 10

  local mem_mb
  echo "Measuring operator memory (no skyhooks)..."
  mem_mb="$(get_memory_mb)"
  echo "Memory (no skyhooks): ${mem_mb} Mi"
  record_result "$target" "no_skyhooks" "$mem_mb"

  apply_policy_and_skyhook
  wait_for_rollout_start

  echo "Measuring operator memory (during rollout)..."
  mem_mb="$(get_memory_mb)"
  echo "Memory (during rollout): ${mem_mb} Mi"
  record_result "$target" "during_rollout" "$mem_mb"

  wait_for_skyhook_complete "$target"
  sleep 5

  echo "Measuring operator memory (after complete)..."
  mem_mb="$(get_memory_mb)"
  echo "Memory (after complete): ${mem_mb} Mi"
  record_result "$target" "after_complete" "$mem_mb"

  cleanup_skyhook_and_nodes
  sleep 10
}

# Main
echo "Scale test: cluster=$CLUSTER_NAME nodegroup=$NODEGROUP sizes $START_SIZE to $FINAL_SIZE step $STEP (metrics=$METRICS_SOURCE)"
if [[ "${FAKE_LABELS:-0}" -gt 0 ]]; then
  apply_fake_labels_to_node_group "$FAKE_LABELS"
fi
ensure_no_test_resources

target=$START_SIZE
while [[ $target -le $FINAL_SIZE ]]; do
  echo "========== Node count: $target =========="
  run_one_iteration "$target"
  target=$((target + STEP))
done

echo "Done. Results${OUTPUT_FILE:+ written to $OUTPUT_FILE}."
scale_node_group 1
