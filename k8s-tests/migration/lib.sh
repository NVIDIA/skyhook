#!/bin/bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

# Shared helpers for the Skyhook -> NodeWright upgrade migration test.
# Sourced by run.sh; not executable on its own.

NAMESPACE="${NAMESPACE:-skyhook}"
RELEASE="${RELEASE:-nodewright-operator}"
TEST_NAME="${TEST_NAME:-migration-upgrade-test}"
HOLD_NAME="${HOLD_NAME:-migration-hold-test}"
TEST_NODE="${TEST_NODE:-kind-worker}"
OLD_CHART_TAG="${OLD_CHART_TAG:-chart/v0.17.1}"

# The image under test is derived from LOCAL_OPERATOR_IMG, which is what
# `make push-local-image` actually pushed. Hardcoding a repo/tag here would let the
# Makefile push to one place while phase 3 deploys a stale image left in the
# registry from an earlier build - a silently green run against the wrong operator,
# which is the worst failure this suite could have.
LOCAL_OPERATOR_IMG="${LOCAL_OPERATOR_IMG:-localhost:5005/skyhook-operator:testing}"

# Split on the last colon AFTER the last slash. A naive ${VAR%:*} splits a tagless
# "localhost:5005/skyhook-operator" on the REGISTRY PORT, yielding repo "localhost"
# and tag "5005/skyhook-operator" - which is precisely the "silently green run
# against the wrong operator" this derivation exists to prevent. A tagless value is
# rejected rather than guessed, because the whole point is to deploy exactly the
# image `make push-local-image` pushed.
if [ -z "${NEW_IMAGE_REPO:-}" ] || [ -z "${NEW_IMAGE_TAG:-}" ]; then
	case "${LOCAL_OPERATOR_IMG##*/}" in
		*:*)
			NEW_IMAGE_REPO="${NEW_IMAGE_REPO:-${LOCAL_OPERATOR_IMG%:*}}"
			NEW_IMAGE_TAG="${NEW_IMAGE_TAG:-${LOCAL_OPERATOR_IMG##*:}}"
			;;
		*)
			echo "FAIL: LOCAL_OPERATOR_IMG ('$LOCAL_OPERATOR_IMG') has no tag; set an explicit repo:tag, or override NEW_IMAGE_REPO and NEW_IMAGE_TAG" >&2
			exit 1
			;;
	esac
fi

# shellcheck disable=SC2034  # consumed by run.sh, which sources this file
# The package seeded by skyhook.yaml. Kept here rather than repeated inline because
# the host paths, log greps, and ConfigMap names are all derived from them: bumping the
# version in skyhook.yaml without updating every literal would turn those lookups into
# silent misses rather than a clear failure.
PKG_NAME="${PKG_NAME:-shellscript}"
PKG_VERSION="${PKG_VERSION:-1.1.1}"
PKG_IMAGE="${PKG_IMAGE:-ghcr.io/nvidia/skyhook-packages/shellscript}"
# A version deliberately different from PKG_VERSION, used by phase 5 to attempt a real
# spec edit that the read-only webhook must reject.
PKG_EDIT_VERSION="${PKG_EDIT_VERSION:-1.1.2}"

# shellcheck disable=SC2034  # consumed by run.sh, which sources this file
LEGACY_PREFIX="skyhook.nvidia.com"
# shellcheck disable=SC2034  # consumed by run.sh, which sources this file
NEW_PREFIX="nodewright.nvidia.com"

SUITE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SUITE_DIR/../.." && pwd)"
FINGERPRINT_DIR="${FINGERPRINT_DIR:-$SUITE_DIR/.fingerprint}"

KUBECTL="${KUBECTL:-kubectl}"
# Prefer the version-pinned helm the Makefile downloads over whatever is on PATH.
if [ -z "${HELM:-}" ] && [ -x "$REPO_ROOT/operator/bin/helm" ]; then
	HELM="$REPO_ROOT/operator/bin/helm"
else
	HELM="${HELM:-helm}"
fi

log_phase() {
	echo
	echo "=============================================================="
	echo "PHASE $1: $2"
	echo "=============================================================="
}

log() { echo "  -> $*"; }

fail() {
	echo
	echo "FAIL: $*" >&2
	exit 1
}

# materialize_old_chart <git-tag> <dest-dir>
#
# Extracts the chart from a released tag WITHOUT touching the working tree.
# The predecessor of this suite (time_travel_tests/migration_0.5.0) used
# `git checkout <tag>`, which rewrote the working tree and left the repo on a
# dangling branch when it failed. git archive has no such failure mode.
materialize_old_chart() {
	local tag="$1" dest="$2"
	# Check the tag resolves first: the default CI checkout is shallow and tagless,
	# and `git archive` on a missing ref otherwise fails with a bare "not a valid
	# object name" that gives no hint about the actual cause.
	git -C "$REPO_ROOT" rev-parse --verify --quiet "$tag^{commit}" >/dev/null \
		|| fail "tag $tag is not present in this checkout; fetch tags with full history (actions/checkout needs fetch-depth: 0 and fetch-tags: true)"
	git -C "$REPO_ROOT" archive "$tag" chart/ | tar -x -C "$dest" \
		|| fail "could not materialize chart from $tag"
	[ -f "$dest/chart/Chart.yaml" ] || fail "no Chart.yaml after extracting $tag"
}

# wait_for <description> <timeout-seconds> <command...>
#
# Deadline is wall-clock, not iteration count: each poll costs a kubectl round-trip
# on top of the sleep, so counting iterations would both overrun the stated timeout
# and understate how long the wait actually took.
wait_for() {
	local desc="$1" timeout="$2"
	shift 2
	local start=$SECONDS
	local deadline=$((SECONDS + timeout))
	while [ "$SECONDS" -lt "$deadline" ]; do
		if "$@" >/dev/null 2>&1; then
			log "$desc: ok ($((SECONDS - start))s)"
			return 0
		fi
		sleep 1
	done
	fail "timed out after ${timeout}s waiting for: $desc"
}

# node_annotations <node> <prefix> [substring-filter]
#
# Emits sorted key=value lines, suitable for diff. The substring filter exists
# because a shared e2e cluster accumulates unrelated per-skyhook annotations;
# scoping to this test's own name keeps the fingerprint honest.
#
# autoTaint_ is matched unconditionally: its suffix is the runtime-required TAINT
# key, not the Skyhook name, so a name-only filter would silently drop it from both
# sides of every comparison and quietly stop covering it the moment someone adds a
# runtime-required package to the seed.
node_annotations() { node_metadata "$1" annotations "$2" "${3:-}"; }

# node_labels <node> <prefix> [substring-filter]
node_labels() { node_metadata "$1" labels "$2" "${3:-}"; }

# node_conditions <node> <prefix> [substring-filter]
node_conditions() { node_metadata "$1" conditions "$2" "${3:-}"; }

# node_metadata <node> <annotations|labels|conditions> <prefix> [substring-filter]
#
# One implementation for all three so the prefix filter, the name filter, and the
# sort cannot drift apart between copies. The two underlying shapes are normalised
# to key/value pairs up front: annotations and labels are string maps under
# .metadata, while conditions are an array under .status whose .type and .status
# play exactly the same roles.
node_metadata() {
	local node="$1" kind="$2" prefix="$3" filter="${4:-}"
	$KUBECTL get node "$node" -o json \
		| jq -r --arg kind "$kind" --arg p "$prefix/" --arg f "$filter" '
			(if $kind == "conditions"
			 then (.status.conditions // []) | map({key: .type, value: .status})
			 else (.metadata[$kind] // {}) | to_entries
			 end)
			| .[]
			| select(.key | startswith($p))
			| select($f == "" or (.key | contains($f)) or (.key | contains("autoTaint_")))
			| "\(.key)=\(.value)"' \
		| sort
}

# on_node <command>
#
# Runs a command in the host root namespace via the privileged debugger pod that
# k8s-tests/operator-agent/setup.sh provisions.
on_node() {
	$KUBECTL exec "${TEST_NODE}-debugger" -- chroot /host bash -c "$1"
}

# nodewright_annotation <name> <annotation-key>
#
# Read via jq, not jsonpath: the annotation keys contain dots
# (nodewright.nvidia.com/...) which jsonpath treats as path separators unless
# escaped, and an unescaped lookup silently returns empty rather than erroring.
nodewright_annotation() {
	$KUBECTL get nodewright "$1" -o json \
		| jq -r --arg k "$2" '.metadata.annotations[$k] // ""'
}

# restart_operator <why>
#
# Rolls the controller-manager and waits for it to be ready again. Used to force a
# from-scratch re-evaluation when no cluster event would otherwise queue a reconcile.
restart_operator() {
	log "restarting the operator $1"
	$KUBECTL rollout restart deployment/nodewright-controller-manager -n "$NAMESPACE" >/dev/null \
		|| fail "could not restart the operator"
	$KUBECTL rollout status deployment/nodewright-controller-manager -n "$NAMESPACE" --timeout=300s >/dev/null \
		|| fail "the operator did not become ready after restart"
}

# operator_logs_since_restart
#
# Logs from the controller-manager pods running RIGHT NOW, resolved by name.
#
# WHY not `kubectl logs -l ... --since-time=<stamp>`: the controllers log
# "Starting workers" BEFORE the pod reports ready, so a stamp taken after
# `rollout status` returns filters that line out and the wait can never succeed.
# Stamping before the restart instead lets a terminating old pod's output into
# the window. Resolving the current pods by name sidesteps both: a freshly
# rolled pod's log contains only post-restart lines, with no time filter needed.
operator_logs_since_restart() {
	local pod
	for pod in $($KUBECTL get pods -n "$NAMESPACE" -l control-plane=controller-manager \
		--field-selector=status.phase=Running -o name 2>/dev/null); do
		$KUBECTL logs -n "$NAMESPACE" "$pod" -c manager 2>/dev/null || true
	done
}

# operator_started_workers: true once a post-restart pod has started its controllers.
# Passed to wait_for by name so it runs in this shell; a `bash -c` wrapper would not
# see these functions.
operator_started_workers() {
	operator_logs_since_restart | grep -q 'Starting workers'
}

# operator_logged_hold_for <skyhook-name>: true once the migration hold has fired
# naming that object. Reads the post-restart pods by name for the same reason
# operator_logs_since_restart exists, and additionally because `kubectl logs -l`
# silently caps output at ten lines per pod.
operator_logged_hold_for() {
	operator_logs_since_restart | grep 'holding NodeWright reconcile' | grep -q "$1"
}

# configmap_uid <fingerprint-file> <configmap-name>
#
# Extracts a ConfigMap's uid from a capture, or empty if absent.
#
# WHY a helper rather than an inline `x=$(grep ... | sed ...)`: under `set -e` with
# `pipefail`, a non-matching grep makes the whole substitution fail and errexit kills
# the script immediately, so the carefully worded fail() the caller wrote right
# afterwards never runs. Absence is a legitimate answer here (it is exactly what the
# cascade-delete assertion is looking for), so it must come back as empty, not as an
# abort with no message.
configmap_uid() {
	local file="$1" name="$2"
	[ -f "$file" ] || return 0
	grep "^$name " "$file" 2>/dev/null | sed 's/.* uid=\([^ ]*\).*/\1/' || true
}

# require_clean_diff <before-file> <after-file> <message>
#
# The emptiness guard is the point: `diff` of two empty files is clean, so a capture
# that silently stopped producing output would turn every comparison into a green
# no-op. No comparison in this suite legitimately expects an empty baseline, so an
# empty one always means the capture broke, not that nothing changed.
require_clean_diff() {
	local before="$1" after="$2" message="$3"
	[ -s "$before" ] \
		|| fail "$message: baseline $(basename "$before") is empty, so this comparison would be vacuous"
	if ! diff -u "$before" "$after"; then
		fail "$message"
	fi
	log "$message: unchanged (as expected)"
}
