/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package wrapper

import (
	"strings"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// legacySkyhookMetadataPrefix is the metadata prefix the pre-rename operator used
// for every node annotation, node label, and node condition type. The operator now
// keys all of that under v1alpha1.METADATA_PREFIX ("nodewright.nvidia.com"). This
// literal is intentionally hardcoded rather than imported from the legacy api
// package: it is a one-shot migration constant (the value can never change), and
// wrapper otherwise depends only on the new api group.
const legacySkyhookMetadataPrefix = "skyhook.nvidia.com"

// MIGRATION-SHIM: transition-only for the skyhook.nvidia.com -> nodewright.nvidia.com
// rename. Delete this whole file (and its call in wrapper/node.go's Migrate) with the
// legacy skyhook.nvidia.com group in the removal release.
//
// migrateNodePrefixToNodeWright re-keys, in place, any node metadata still under the
// legacy skyhook.nvidia.com prefix to the current nodewright.nvidia.com prefix.
//
// WHY: after the Skyhook -> NodeWright rename the operator reads node state
// (nodeState_/status_/version_/cordon_/drainStart_/autoTaint_ annotations, status
// labels, and skyhook.nvidia.com/<name>/<type> conditions) under the NEW prefix. A
// node last written by the pre-rename operator carries only OLD-prefix keys, so
// without this rewrite the operator sees no state and re-runs every package on
// every node. It must run before GetVersion()/State() are trusted (both read the
// new prefix), so a renamed node is adopted instead of treated as fresh.
//
// Idempotent: a key already present under the new prefix wins and the legacy
// duplicate is dropped; with no legacy keys present it is a no-op and does not mark
// the node updated. The bare runtime-required taint key ("skyhook.nvidia.com", no
// trailing slash, and a taint not an annotation/label) intentionally does not match
// and is left untouched: that key did not move in the rename.
func migrateNodePrefixToNodeWright(node *skyhookNode, logger logr.Logger) error {
	oldPrefix := legacySkyhookMetadataPrefix + "/"
	newPrefix := v1alpha1.METADATA_PREFIX + "/"

	changed := false
	rekeyPrefixedMap(node.Annotations, oldPrefix, newPrefix, &changed)
	rekeyPrefixedMap(node.Labels, oldPrefix, newPrefix, &changed)

	conditions := node.GetNode().Status.Conditions
	for i := range conditions {
		t := string(conditions[i].Type)
		if strings.HasPrefix(t, oldPrefix) {
			conditions[i].Type = corev1.NodeConditionType(newPrefix + strings.TrimPrefix(t, oldPrefix))
			changed = true
		}
	}

	if changed {
		node.updated = true
		logger.Info("migrated legacy node metadata to the nodewright prefix", "node", node.Name, "skyhook", node.skyhookName)
	}
	return nil
}

// rekeyPrefixedMap moves every key in m from oldPrefix to newPrefix in place. A key
// already present under newPrefix is kept and the legacy duplicate is dropped. Sets
// *changed when anything moved. A nil map is a no-op.
func rekeyPrefixedMap(m map[string]string, oldPrefix, newPrefix string, changed *bool) {
	// Collect legacy keys first so we do not mutate the map while ranging it.
	var legacy []string
	for k := range m {
		if strings.HasPrefix(k, oldPrefix) {
			legacy = append(legacy, k)
		}
	}
	for _, k := range legacy {
		newKey := newPrefix + strings.TrimPrefix(k, oldPrefix)
		if _, exists := m[newKey]; !exists {
			m[newKey] = m[k]
		}
		delete(m, k)
		*changed = true
	}
}
