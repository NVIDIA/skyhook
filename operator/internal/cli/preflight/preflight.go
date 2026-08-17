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

// Package preflight holds cluster-capability checks the CLI runs before it
// operates on NodeWright resources.
package preflight

import (
	"errors"
	"fmt"

	"k8s.io/client-go/discovery"

	nwv1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	skyhookv1 "github.com/NVIDIA/nodewright/operator/api/v1alpha1"
)

// EnsureNodeWrightServed verifies the cluster serves the nodewright.nvidia.com
// API group. When it does not but the legacy skyhook.nvidia.com group is served,
// it returns an actionable error telling the operator to upgrade: every
// NodeWright-operating CLI command would otherwise fail with a confusing
// NotFound against a legacy-only operator. When neither group is served the
// check passes and the command surfaces its own NotFound: absence of the legacy
// group means this is not the legacy-only case this preflight exists to catch.
func EnsureNodeWrightServed(disco discovery.DiscoveryInterface) error {
	groups, err := disco.ServerGroups()
	// ServerGroups can return usable partial results alongside a group-discovery error
	// (e.g. one broken aggregated APIService). Only fail on a non-partial error so a
	// single unhealthy group does not block the CLI against an otherwise-valid cluster.
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return fmt.Errorf("discovering served API groups: %w", err)
	}
	if groups == nil {
		return nil
	}

	var nodewrightServed, skyhookServed bool
	for _, group := range groups.Groups {
		switch group.Name {
		case nwv1.GroupVersion.Group:
			nodewrightServed = true
		case skyhookv1.GroupVersion.Group:
			skyhookServed = true
		}
	}

	if nodewrightServed {
		return nil
	}
	// If the nodewright group itself was the casualty of a partial discovery failure, it
	// is absent from groups.Groups even though the operator may well be serving it. Do not
	// mistake that for a legacy-only cluster and tell the user to "upgrade"; surface the
	// real discovery error instead.
	if groupDiscoveryFailedFor(err, nwv1.GroupVersion.Group) {
		return fmt.Errorf("discovering served API groups: %w", err)
	}
	if skyhookServed {
		return fmt.Errorf(
			"this CLI targets the %s API group, but the cluster's operator only serves the legacy %s group; "+
				"upgrade to a NodeWright-capable operator (see docs/user-guide/cli.md)",
			nwv1.GroupVersion.Group, skyhookv1.GroupVersion.Group)
	}
	return nil
}

// groupDiscoveryFailedFor reports whether err is a partial group-discovery failure that
// includes the given API group, meaning that group's served state is unknown rather than
// confirmed absent.
func groupDiscoveryFailedFor(err error, group string) bool {
	var failed *discovery.ErrGroupDiscoveryFailed
	if !errors.As(err, &failed) {
		return false
	}
	for gv := range failed.Groups {
		if gv.Group == group {
			return true
		}
	}
	return false
}
