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


# Skyhook -> NodeWright upgrade migration test.
#
# Upgrades a real cluster from the last pre-rename operator (chart/v0.17.1, the last
# release serving only skyhook.nvidia.com) to the working tree, and proves the
# migration adopts existing per-node state without re-running packages. Phases run in
# order against ONE cluster, which must be a clean pre-rename baseline: phase 1
# refuses to run if nodewright CRDs already exist, because that would make every
# later assertion pass vacuously. See README.md for what this does and does not cover.
#
# Usage:
#   ./run.sh                        run every phase
#   ./run.sh --from-phase 4         resume at phase 4 (state from a prior run is reused)
#   ./run.sh --to-phase 2           stop after phase 2

set -euo pipefail

SUITE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$SUITE_DIR/lib.sh"

FROM_PHASE=1
TO_PHASE=8

while [ $# -gt 0 ]; do
	case "$1" in
		--from-phase) FROM_PHASE="$2"; shift 2 ;;
		--to-phase) TO_PHASE="$2"; shift 2 ;;
		*) fail "unknown argument: $1" ;;
	esac
done

FIRST_PHASE=1
LAST_PHASE=8

## Validate the range rather than trusting it. An out-of-order or out-of-range pair
## (--from-phase 9, or --from-phase 4 --to-phase 2) would otherwise skip every phase
## and still exit 0 - a green run that tested nothing, which is the exact failure
## this suite exists to catch. PHASES_RUN is the belt-and-braces check on the same
## thing: main() refuses to report success if nothing actually executed.
for v in "$FROM_PHASE" "$TO_PHASE"; do
	case "$v" in
		''|*[!0-9]*) fail "phase must be an integer, got '$v'" ;;
	esac
	if [ "$v" -lt "$FIRST_PHASE" ] || [ "$v" -gt "$LAST_PHASE" ]; then
		fail "phase $v is out of range ($FIRST_PHASE..$LAST_PHASE)"
	fi
done
[ "$FROM_PHASE" -le "$TO_PHASE" ] \
	|| fail "--from-phase $FROM_PHASE is after --to-phase $TO_PHASE; nothing would run"

PHASES_RUN=0
should_run() {
	if [ "$1" -ge "$FROM_PHASE" ] && [ "$1" -le "$TO_PHASE" ]; then
		PHASES_RUN=$((PHASES_RUN + 1))
		return 0
	fi
	return 1
}

OLD_CHART_DIR="${OLD_CHART_DIR:-$SUITE_DIR/.old-chart}"

#######################################################################
# Phase 1: install the pre-rename operator
#######################################################################
phase_1() {
	log_phase 1 "install the pre-rename operator ($OLD_CHART_TAG)"

	rm -rf "$OLD_CHART_DIR"
	mkdir -p "$OLD_CHART_DIR"
	materialize_old_chart "$OLD_CHART_TAG" "$OLD_CHART_DIR"
	log "materialized $(grep -E '^(name|version|appVersion)' "$OLD_CHART_DIR/chart/Chart.yaml" | tr '\n' ' ')"

	# A cluster carrying nodewright CRDs before the upgrade would make every
	# later assertion pass vacuously, so refuse to run against one.
	if $KUBECTL get crd nodewrights.nodewright.nvidia.com >/dev/null 2>&1; then
		fail "nodewrights.nodewright.nvidia.com CRD already exists; this cluster is not a pre-rename baseline"
	fi
	if $KUBECTL get crd deploymentpolicies.nodewright.nvidia.com >/dev/null 2>&1; then
		fail "deploymentpolicies.nodewright.nvidia.com CRD already exists; this cluster is not a pre-rename baseline"
	fi
	log "no nodewright CRDs present (clean pre-rename baseline)"

	# Do not override the image: the v0.17.1 chart pins the v0.17.0 operator by
	# digest, and using its own default is what a real user would get.
	$HELM upgrade --install "$RELEASE" "$OLD_CHART_DIR/chart" \
		-n "$NAMESPACE" --create-namespace \
		--wait --timeout 5m \
		|| fail "helm install of the pre-rename chart failed"

	$KUBECTL get crd skyhooks.skyhook.nvidia.com >/dev/null \
		|| fail "legacy Skyhook CRD missing after install"
	$KUBECTL get crd deploymentpolicies.skyhook.nvidia.com >/dev/null \
		|| fail "legacy DeploymentPolicy CRD missing after install"

	local images
	images=$($KUBECTL get deploy -n "$NAMESPACE" -o jsonpath='{.items[*].spec.template.spec.containers[*].image}')
	log "running images: $images"
	echo "$images" | grep -q 'operator' \
		|| fail "no operator image found in namespace $NAMESPACE"

	log "phase 1 complete"
}

#######################################################################
# Phase 2: seed a real package on the old operator and fingerprint it
#######################################################################

# capture_state <suffix>
#
# Snapshots everything the "packages did not re-run" claim rests on.
# Package pods are deliberately NOT captured: the operator deletes them once a
# package completes, so steady state has zero and a before/after pod diff would
# be empty on both sides, i.e. vacuously green. The re-run signal is host state
# (the agent writes a NEW timestamped log file per run) plus a pod-count check
# taken during the upgrade window.
capture_state() {
	local s="$1"

	## Phases 4, 6, and 7 all read host state, but only phase 2 originally created the
	## debugger pod. Re-applying here (it is an idempotent kubectl apply) means a
	## --from-phase run after the pod was reaped fails with a clear message instead of
	## dying inside on_node.
	"$REPO_ROOT/k8s-tests/operator-agent/setup.sh" "$TEST_NODE" setup >/dev/null \
		|| fail "could not ensure the debugger pod on $TEST_NODE"
	node_annotations "$TEST_NODE" "$LEGACY_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/annotations.legacy.$s"
	node_annotations "$TEST_NODE" "$NEW_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/annotations.new.$s"
	node_labels "$TEST_NODE" "$LEGACY_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/labels.legacy.$s"
	node_labels "$TEST_NODE" "$NEW_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/labels.new.$s"
	node_conditions "$TEST_NODE" "$LEGACY_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/conditions.legacy.$s"
	node_conditions "$TEST_NODE" "$NEW_PREFIX" "$TEST_NAME" > "$FINGERPRINT_DIR/conditions.new.$s"

	on_node "find /var/lib/skyhook/$TEST_NAME/flags -type f -printf '%p %T@\n' 2>/dev/null | sort" \
		> "$FINGERPRINT_DIR/flags.$s"
	on_node "ls -1 /var/log/skyhook/$TEST_NAME/$PKG_NAME/$PKG_VERSION/ 2>/dev/null | sort" \
		> "$FINGERPRINT_DIR/logfiles.$s"
	on_node "cat /var/log/skyhook/$TEST_NAME/$PKG_NAME/$PKG_VERSION/*.log 2>/dev/null | grep -c 'Hello, world!' || true" \
		> "$FINGERPRINT_DIR/hellocount.$s"

	## Name alone would not show a delete-and-recreate, and the bug this suite found
	## was a ConfigMap bug, so capture identity (uid) and the labels the migration is
	## supposed to converge.
	$KUBECTL get configmaps -n "$NAMESPACE" -o json \
		| jq -r --arg n "$TEST_NAME" '
			.items[]
			| select(.metadata.name | startswith($n))
			| "\(.metadata.name) uid=\(.metadata.uid) labels=\(.metadata.labels // {} | to_entries | sort_by(.key) | map("\(.key)=\(.value)") | join(","))"' \
		| sort > "$FINGERPRINT_DIR/configmaps.$s"
}

phase_2() {
	log_phase 2 "seed a real package on the pre-rename operator and fingerprint it"

	"$REPO_ROOT/k8s-tests/operator-agent/setup.sh" "$TEST_NODE" setup >/dev/null \
		|| fail "could not start the debugger pod"

	$KUBECTL apply -f "$SUITE_DIR/skyhook.yaml" >/dev/null

	wait_for "legacy Skyhook reaches complete" 300 \
		bash -c "$KUBECTL get skyhook.skyhook.nvidia.com $TEST_NAME -o jsonpath='{.status.status}' | grep -qx complete"

	capture_state before

	# An empty fingerprint would make every phase-4 diff pass vacuously. This is
	# the single most likely way this test could silently stop testing anything.
	[ -s "$FINGERPRINT_DIR/annotations.legacy.before" ] \
		|| fail "no legacy node annotations captured; the old operator wrote no state"
	[ -s "$FINGERPRINT_DIR/flags.before" ] \
		|| fail "no agent flag files captured; the package never ran on the host"
	grep -qE '^[1-9]' "$FINGERPRINT_DIR/hellocount.before" \
		|| fail "package never logged 'Hello, world!'; it did not actually run"
	if [ -s "$FINGERPRINT_DIR/annotations.new.before" ]; then
		fail "nodewright-prefixed annotations exist before the upgrade; cluster is not a clean baseline"
	fi

	log "legacy annotations: $(wc -l < "$FINGERPRINT_DIR/annotations.legacy.before" | tr -d ' ')"
	log "agent flag files:   $(wc -l < "$FINGERPRINT_DIR/flags.before" | tr -d ' ')"
	log "package log files:  $(wc -l < "$FINGERPRINT_DIR/logfiles.before" | tr -d ' ')"
	log "'Hello, world!' x   $(tr -d ' \n' < "$FINGERPRINT_DIR/hellocount.before")"
	log "phase 2 complete"
}

#######################################################################
# Phase 3: upgrade to the working-tree operator
#######################################################################
phase_3() {
	log_phase 3 "upgrade to the working-tree chart ($NEW_IMAGE_REPO:$NEW_IMAGE_TAG)"

	$HELM upgrade "$RELEASE" "$REPO_ROOT/chart" \
		-n "$NAMESPACE" \
		--set "controllerManager.manager.image.repository=$NEW_IMAGE_REPO" \
		--set "controllerManager.manager.image.tag=$NEW_IMAGE_TAG" \
		--set "controllerManager.manager.image.digest=" \
		--wait --timeout 5m \
		|| fail "helm upgrade to the working-tree chart failed"

	$KUBECTL get crd nodewrights.nodewright.nvidia.com >/dev/null \
		|| fail "nodewright CRD not installed by the upgrade"
	log "nodewright CRDs installed"
	log "phase 3 complete"
}

#######################################################################
# Phase 4: prove the migration adopted state and re-ran nothing
#######################################################################
phase_4() {
	log_phase 4 "assert node state migrated and no package re-ran"

	wait_for "NodeWright appears" 120 \
		bash -c "$KUBECTL get nodewright $TEST_NAME >/dev/null 2>&1"
	wait_for "NodeWright reaches complete" 300 \
		bash -c "$KUBECTL get nodewright $TEST_NAME -o jsonpath='{.status.status}' | grep -qx complete"

	local mirrored
	mirrored=$(nodewright_annotation "$TEST_NAME" "$NEW_PREFIX/mirrored-from")
	[ "$mirrored" = "$TEST_NAME" ] \
		|| fail "NodeWright is not stamped mirrored-from=$TEST_NAME (got '$mirrored')"
	log "NodeWright stamped mirrored-from=$mirrored"

	capture_state after

	## --- the migrated state must carry the pre-upgrade values ------------
	## Every key is compared strictly, version_ included. version_ is NOT excluded even
	## though it is the operator's own version stamp: skyhookNode.Migrate only calls
	## SetVersion() for MajorMinor(from) in {"", v0.5, v0.6, v0.7}, and this upgrade is
	## from v0.17.x, so nothing rewrites it and it must survive verbatim. If a future
	## release adds a migration branch for the current version this will fail, which is
	## the right outcome: better to re-decide then than to pre-exclude it now.
	sed -e "s|^$LEGACY_PREFIX/|$NEW_PREFIX/|" \
		"$FINGERPRINT_DIR/annotations.legacy.before" | sort > "$FINGERPRINT_DIR/annotations.expected"
	sort "$FINGERPRINT_DIR/annotations.new.after" > "$FINGERPRINT_DIR/annotations.actual"
	require_clean_diff "$FINGERPRINT_DIR/annotations.expected" "$FINGERPRINT_DIR/annotations.actual" \
		"migrated node annotations carry the pre-upgrade values"

	sed -e "s|^$LEGACY_PREFIX/|$NEW_PREFIX/|" "$FINGERPRINT_DIR/labels.legacy.before" | sort \
		> "$FINGERPRINT_DIR/labels.expected"
	require_clean_diff "$FINGERPRINT_DIR/labels.expected" "$FINGERPRINT_DIR/labels.new.after" \
		"migrated node labels carry the pre-upgrade values"

	sed -e "s|^$LEGACY_PREFIX/|$NEW_PREFIX/|" "$FINGERPRINT_DIR/conditions.legacy.before" | sort \
		> "$FINGERPRINT_DIR/conditions.expected"
	require_clean_diff "$FINGERPRINT_DIR/conditions.expected" "$FINGERPRINT_DIR/conditions.new.after" \
		"migrated node conditions carry the pre-upgrade values"

	## --- the legacy copies must survive the rollback window --------------
	require_clean_diff "$FINGERPRINT_DIR/annotations.legacy.before" "$FINGERPRINT_DIR/annotations.legacy.after" \
		"legacy annotations kept for the rollback window"
	require_clean_diff "$FINGERPRINT_DIR/labels.legacy.before" "$FINGERPRINT_DIR/labels.legacy.after" \
		"legacy labels kept for the rollback window"
	require_clean_diff "$FINGERPRINT_DIR/conditions.legacy.before" "$FINGERPRINT_DIR/conditions.legacy.after" \
		"legacy conditions kept for the rollback window"

	## --- nothing re-ran on the host --------------------------------------
	require_clean_diff "$FINGERPRINT_DIR/flags.before" "$FINGERPRINT_DIR/flags.after" \
		"agent completion flags untouched"
	require_clean_diff "$FINGERPRINT_DIR/logfiles.before" "$FINGERPRINT_DIR/logfiles.after" \
		"no new package log file (a re-run writes a new timestamped log)"
	require_clean_diff "$FINGERPRINT_DIR/hellocount.before" "$FINGERPRINT_DIR/hellocount.after" \
		"apply.sh did not run again"

	## --- the ConfigMaps the migration must converge, not recreate ---------
	## This is the surface the wedge bug lived on: the package ConfigMap
	## (<name>-<package>-<version>) carries only the legacy label until the migration
	## relabels it, and an unconverged one made the reconciler try to Create over an
	## existing object forever.
	[ -s "$FINGERPRINT_DIR/configmaps.before" ] || fail "no ConfigMaps captured before the upgrade"
	local pkgCM="$TEST_NAME-$PKG_NAME-$PKG_VERSION"
	grep -q "^$pkgCM " "$FINGERPRINT_DIR/configmaps.after" \
		|| fail "the package ConfigMap $pkgCM is missing after the upgrade"
	grep "^$pkgCM " "$FINGERPRINT_DIR/configmaps.after" | grep -q "$NEW_PREFIX/name=$TEST_NAME" \
		|| fail "the package ConfigMap was not converged to the $NEW_PREFIX/name label; the reconciler will try to recreate it"
	grep "^$pkgCM " "$FINGERPRINT_DIR/configmaps.after" | grep -q "$LEGACY_PREFIX/name=$TEST_NAME" \
		|| fail "the package ConfigMap lost its legacy label during the rollback window"

	local uidBefore uidAfter
	uidBefore=$(configmap_uid "$FINGERPRINT_DIR/configmaps.before" "$pkgCM")
	uidAfter=$(configmap_uid "$FINGERPRINT_DIR/configmaps.after" "$pkgCM")
	[ -n "$uidBefore" ] && [ "$uidBefore" = "$uidAfter" ] \
		|| fail "the package ConfigMap was recreated across the upgrade (uid $uidBefore -> $uidAfter), not adopted"
	log "package ConfigMap converged in place (same uid, both labels present)"

	## --- the rollback-window stamp ---------------------------------------
	local stamp
	stamp=$(nodewright_annotation "$TEST_NAME" "$NEW_PREFIX/legacy-migrated-at")
	[ -n "$stamp" ] || fail "NodeWright carries no legacy-migrated-at stamp; the rollback window is not tracked"
	log "legacy-migrated-at=$stamp"

	log "phase 4 complete"
}

#######################################################################
# Phase 5: the legacy object is read-only once migrated
#######################################################################
phase_5() {
	log_phase 5 "assert legacy Skyhook writes are rejected"

	## A steady-state GitOps sync re-applies the same manifest and must keep working.
	$KUBECTL apply -f "$SUITE_DIR/skyhook.yaml" >/dev/null \
		|| fail "an identical re-apply of the legacy Skyhook was rejected; steady-state GitOps would break"
	log "identical re-apply accepted"

	## A real spec edit must be rejected, and specifically BY THE MIGRATION WEBHOOK.
	## Without the second check, a rejection for an unrelated reason (a schema error
	## in the patch, say) would read as a pass.
	## Built with jq from the shared package constants rather than hardcoded, so the
	## edit always targets the package the manifest actually seeds. jq also handles the
	## quoting, which hand-escaped JSON inside a shell string does badly.
	local patch err
	patch=$(jq -nc --arg n "$PKG_NAME" --arg v "$PKG_EDIT_VERSION" --arg i "$PKG_IMAGE" \
		'{spec: {packages: {($n): {version: $v, image: $i}}}}')
	if err=$($KUBECTL patch skyhook.skyhook.nvidia.com "$TEST_NAME" --type=merge \
		-p "$patch" 2>&1); then
		fail "legacy Skyhook spec edit was ACCEPTED; the read-only webhook is not enforcing"
	fi
	echo "$err" | grep -qi 'read-only' \
		|| fail "legacy edit was rejected, but not by the read-only migration webhook: $err"
	log "spec edit rejected as read-only"

	## The deprecation warning is asserted on this same rejected write rather than on a
	## re-apply: kubectl computes an empty patch for an identical object, prints
	## "unchanged" and never contacts the apiserver, so no admission webhook runs and no
	## warning is produced. That is kubectl behavior, not a missing warning.
	echo "$err" | grep -qi 'deprecated' \
		|| fail "no deprecation warning emitted on a legacy write: $err"
	log "deprecation warning emitted"

	log "phase 5 complete"
}

#######################################################################
# Phase 6: follow docs/nodewright-migration.md verbatim
#######################################################################
phase_6() {
	log_phase 6 "follow the migration guide's own commands verbatim"

	## This is the doc's own command, run as written (docs/cli.md, "Migrating manifests
	## to NodeWright"). It rewrites apiVersion and kind ONLY. A blanket
	## s|skyhook\.nvidia\.com/|...|g would also rewrite the nodeSelectors key, which
	## names the user's own node label rather than anything the operator owns.
	sed -e '/^ *apiVersion:/ s|skyhook\.nvidia\.com/|nodewright.nvidia.com/|' \
		-e 's|^\( *kind: *\)Skyhook[[:space:]]*$|\1NodeWright|' \
		"$SUITE_DIR/skyhook.yaml" > "$SUITE_DIR/nodewright.yaml"
	grep -q 'kind: NodeWright' "$SUITE_DIR/nodewright.yaml" || fail "sed did not produce a NodeWright"
	grep -q "$LEGACY_PREFIX/test-node" "$SUITE_DIR/nodewright.yaml" \
		|| fail "the rename rewrote the nodeSelector key; it must touch only apiVersion and kind"
	log "renamed manifest generated; nodeSelector left untouched"

	## The doc claims this apply is "a no-op adoption of the existing object (same name,
	## same spec)". Capture the spec either side of the apply and hold it to that.
	$KUBECTL get nodewright "$TEST_NAME" -o json | jq -S '.spec' > "$FINGERPRINT_DIR/spec.beforeAdopt"

	$KUBECTL apply -f "$SUITE_DIR/nodewright.yaml" >/dev/null \
		|| fail "applying the renamed manifest failed"

	## Scope the count to this test's own name. A bare count also sees the NodeWright
	## the mirror creates for phase 8's hold object, which would make --from-phase 6
	## after a full run fail with a misleading "duplicated instead of adopting".
	local count
	count=$($KUBECTL get nodewrights.nodewright.nvidia.com -o name \
		| grep -c "/$TEST_NAME\$" || true)
	[ "$count" = "1" ] || fail "expected exactly 1 NodeWright named $TEST_NAME after adoption, got $count (the apply duplicated instead of adopting)"

	$KUBECTL get nodewright "$TEST_NAME" -o json | jq -S '.spec' > "$FINGERPRINT_DIR/spec.afterAdopt"
	require_clean_diff "$FINGERPRINT_DIR/spec.beforeAdopt" "$FINGERPRINT_DIR/spec.afterAdopt" \
		"applying the renamed manifest was a true no-op adoption (spec unchanged)"
	log "apply adopted the mirrored object (still exactly 1 NodeWright)"

	## The pre-rename finalizer must be released automatically. kubectl's own --timeout
	## is used rather than timeout(1), which is GNU coreutils and absent on macOS.
	$KUBECTL delete skyhook.skyhook.nvidia.com "$TEST_NAME" --timeout=180s >/dev/null \
		|| fail "legacy Skyhook deletion hung; the pre-rename finalizer was not released"
	log "legacy Skyhook deleted, finalizer released automatically"

	$KUBECTL get nodewright "$TEST_NAME" >/dev/null \
		|| fail "the NodeWright did not survive deletion of its legacy source"
	wait_for "NodeWright still reconciles after the legacy source is gone" 180 \
		bash -c "$KUBECTL get nodewright $TEST_NAME -o jsonpath='{.status.status}' | grep -qx complete"

	capture_state after6
	require_clean_diff "$FINGERPRINT_DIR/logfiles.before" "$FINGERPRINT_DIR/logfiles.after6" \
		"no package re-ran during the rename and delete"

	## Deleting the legacy Skyhook must NOT take the NodeWright's ConfigMaps with it.
	## The converge re-parents them onto the NodeWright precisely so this delete does
	## not cascade; before that fix the package ConfigMap came back with a new uid (a
	## recreate, not an adoption) and the per-node metadata one did not come back
	## promptly, leaving a window where a package pod would see a missing ConfigMap.
	local pkgCM="$TEST_NAME-$PKG_NAME-$PKG_VERSION"
	local uidPre uidPost
	uidPre=$(configmap_uid "$FINGERPRINT_DIR/configmaps.after" "$pkgCM")
	uidPost=$(configmap_uid "$FINGERPRINT_DIR/configmaps.after6" "$pkgCM")
	[ -n "$uidPre" ] || fail "no package ConfigMap uid captured before the legacy delete"
	[ -n "$uidPost" ] \
		|| fail "deleting the legacy Skyhook cascade-deleted the package ConfigMap; ownerReferences were not re-parented to the NodeWright"
	[ "$uidPre" = "$uidPost" ] \
		|| fail "the package ConfigMap was recreated across the legacy delete (uid $uidPre -> $uidPost), so the delete cascaded"
	log "package ConfigMap survived the legacy Skyhook delete (same uid; ownership re-parented)"

	log "phase 6 complete"
}

#######################################################################
# Phase 7: prune once the rollback window is collapsed
#######################################################################
# legacy_annotations_gone: true only when the node was READ SUCCESSFULLY and carries
# no legacy-prefixed annotations.
#
# WHY the explicit read check: the obvious `[ -z "$(kubectl ... | jq ...)" ]` is also
# true when kubectl itself fails, and phase 7 polls immediately after a helm upgrade
# that rolls both the operator and its webhook, so a transient API error is likely.
# That would report the prune as complete before it had happened.
legacy_annotations_gone() {
	local node_json
	node_json=$($KUBECTL get node "$TEST_NODE" -o json 2>/dev/null) || return 1
	[ -n "$node_json" ] || return 1
	local remaining
	remaining=$(echo "$node_json" | jq -r --arg p "$LEGACY_PREFIX/" \
		'.metadata.annotations // {} | keys[] | select(startswith($p))') || return 1
	[ -z "$remaining" ]
}

phase_7() {
	log_phase 7 "collapse the rollback window and assert the legacy state is pruned"

	## Confirm there is actually something to prune, so the wait below cannot pass
	## simply because the legacy state was never there.
	[ -s "$FINGERPRINT_DIR/annotations.legacy.after" ] \
		|| fail "no legacy annotations present before the prune; this phase would be vacuous"

	$HELM upgrade "$RELEASE" "$REPO_ROOT/chart" \
		-n "$NAMESPACE" --reuse-values \
		--set "controllerManager.manager.env.legacyCleanupDelay=0" \
		--wait --timeout 5m \
		|| fail "helm upgrade collapsing the rollback window failed"

	wait_for "legacy node annotations pruned" 180 legacy_annotations_gone

	## The prune covers labels and conditions too, not just annotations. This suite's
	## own release notes describe a defect in exactly this area (the prune used to sweep
	## the whole prefix), so gating only on annotations would leave the operator-owned
	## legacy label and conditions unverified.
	capture_state after7pre
	[ -s "$FINGERPRINT_DIR/labels.legacy.before" ] \
		|| fail "no legacy labels were ever captured; the prune label check would be vacuous"
	[ ! -s "$FINGERPRINT_DIR/labels.legacy.after7pre" ] \
		|| fail "operator-owned legacy labels survived the prune: $(cat "$FINGERPRINT_DIR/labels.legacy.after7pre")"
	[ ! -s "$FINGERPRINT_DIR/conditions.legacy.after7pre" ] \
		|| fail "operator-owned legacy conditions survived the prune: $(cat "$FINGERPRINT_DIR/conditions.legacy.after7pre")"
	log "operator-owned legacy labels and conditions pruned"

	## The prune must remove ONLY the legacy copies; the live state is the new prefix.
	node_annotations "$TEST_NODE" "$NEW_PREFIX" "$TEST_NAME" | grep -q 'nodeState_' \
		|| fail "prune removed the live nodewright state, not just the legacy copies"
	log "nodewright state intact after prune"

	## The prune must also leave keys the operator does not own alone, even though they
	## sit under the legacy prefix. make setup-kind-cluster stamps
	## skyhook.nvidia.com/test-node on this node and the operator never writes it, which
	## makes it a genuine stand-in for a user's own label. Before the ownership scoping
	## the prune swept the whole prefix and deleted it.
	$KUBECTL get node "$TEST_NODE" -o json \
		| jq -e --arg k "$LEGACY_PREFIX/test-node" '.metadata.labels | has($k)' >/dev/null \
		|| fail "prune deleted $LEGACY_PREFIX/test-node, a label the operator does not own"
	log "user-owned $LEGACY_PREFIX/test-node label survived the prune"

	## The runtime-required taint key is the one skyhook.nvidia.com key that did NOT
	## move in the rename, and migrateNodePrefixToNodeWright documents that it must be
	## left alone. The seeded package has no interrupt, so no taint is expected here;
	## this reports rather than asserts so it cannot read as coverage it does not give.
	if $KUBECTL get node "$TEST_NODE" -o jsonpath='{.spec.taints}' | grep -q "$LEGACY_PREFIX"; then
		log "runtime-required taint preserved"
	else
		log "NOTE: no runtime-required taint on this node (package has no interrupt), so taint preservation is NOT covered here"
	fi

	capture_state after7
	require_clean_diff "$FINGERPRINT_DIR/logfiles.before" "$FINGERPRINT_DIR/logfiles.after7" \
		"no package re-ran during the prune"

	log "phase 7 complete"
}

#######################################################################
# Phase 8: the migration hold
#######################################################################
phase_8() {
	log_phase 8 "assert an in-flight legacy Skyhook holds the cutover"

	## Anchor before creating the object so a hold line from an earlier run of this
	## phase cannot satisfy the assertion.
	local startedAt
	startedAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	sleep 1

	$KUBECTL apply -f "$SUITE_DIR/skyhook-hold.yaml" >/dev/null

	## A legacy Skyhook's status is frozen once nothing reconciles that kind, which is
	## what makes this deterministic: the status we write here stays put.
	$KUBECTL patch skyhook.skyhook.nvidia.com "$HOLD_NAME" --subresource=status --type=merge \
		-p '{"status":{"status":"in_progress"}}' >/dev/null \
		|| fail "could not patch the legacy Skyhook status to in_progress"

	local got
	got=$($KUBECTL get skyhook.skyhook.nvidia.com "$HOLD_NAME" -o jsonpath='{.status.status}')
	[ "$got" = "in_progress" ] || fail "status patch did not stick (got '$got'); phase 8 cannot test the hold"
	log "legacy Skyhook parked at in_progress"

	## Restart the operator rather than waiting for a reconcile to happen on its own.
	##
	## WHY: the hold is evaluated inside the NodeWright reconcile, and patching a legacy
	## Skyhook's STATUS does not bump its generation, so the mirror does not touch the
	## NodeWright and no reconcile is queued. With every object settled, nothing triggers
	## one for many minutes and a plain wait is a coin flip. A restart is also the
	## faithful scenario: docs/nodewright-migration.md describes the hold as an
	## on-startup protection for an operator upgraded while a rollout was in flight,
	## which is exactly a fresh process finding a non-complete legacy Skyhook.
	restart_operator "with $HOLD_NAME parked in_progress"

	wait_for "migration hold engaged naming $HOLD_NAME" 180 \
		bash -c "$KUBECTL logs -n $NAMESPACE -l control-plane=controller-manager -c manager --since-time='$startedAt' \
			| grep 'holding NodeWright reconcile' | grep -q '$HOLD_NAME'"

	$KUBECTL delete skyhook.skyhook.nvidia.com "$HOLD_NAME" >/dev/null
	log "deleted $HOLD_NAME"

	## Simply asserting "no hold lines since deletion" would be vacuous for the same
	## reason the wait above was flaky: if nothing reconciles, nothing logs either way.
	## Restart again so the operator definitely re-evaluates from scratch, confirm its
	## controllers actually started, and only then require the absence of a hold.
	## Read the post-restart pods by name rather than time-filtering the log; see
	## operator_logs_since_restart for why a timestamp anchor is wrong on both sides.
	restart_operator "with no in-flight legacy Skyhook"

	wait_for "operator controllers started after restart" 180 operator_started_workers

	## The hold requeues every 20s while active, so a quiet window after a confirmed
	## start means it genuinely is not holding.
	sleep 45
	local stillHolding
	stillHolding=$(operator_logs_since_restart | grep -c 'holding NodeWright reconcile' || true)
	[ "$stillHolding" = "0" ] \
		|| fail "the migration hold is still firing ${stillHolding}x after the in-flight legacy Skyhook was deleted"
	log "operator restarted and reconciling, with no hold; the hold cleared"

	## The mirror created a NodeWright for the hold object before the hold engaged, and
	## the mirror never deletes its target. Leaving it behind breaks a later
	## --from-phase 6 run, which counts NodeWrights.
	$KUBECTL delete nodewright "$HOLD_NAME" --ignore-not-found >/dev/null
	log "cleaned up the mirrored NodeWright/$HOLD_NAME"

	log "phase 8 complete"
}

main() {
	mkdir -p "$FINGERPRINT_DIR"
	should_run 1 && phase_1
	should_run 2 && phase_2
	should_run 3 && phase_3
	should_run 4 && phase_4
	should_run 5 && phase_5
	should_run 6 && phase_6
	should_run 7 && phase_7
	should_run 8 && phase_8

	[ "$PHASES_RUN" -gt 0 ] || fail "no phases ran; refusing to report success"
	echo
	echo "phases $FROM_PHASE..$TO_PHASE completed ($PHASES_RUN phase(s) run)"
}

main
