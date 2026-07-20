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

package controller

import (
	"context"
	"fmt"
	"time"

	skyhookv1 "github.com/NVIDIA/nodewright/operator/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// legacyMigrationHoldRequeue is how long the reconciler waits before re-checking
// whether the legacy Skyhooks have drained during a migration hold.
const legacyMigrationHoldRequeue = 20 * time.Second

// inFlightLegacySkyhooks lists legacy skyhook.nvidia.com Skyhooks whose rollout is
// still actively in flight, so taking over their nodes from the new operator could
// double-drive a package pod the pre-rename operator left mutating the host.
//
// A Skyhook is NOT in-flight (safe to migrate in place, no hold) when its status is:
//   - empty: never reconciled by the old operator (brand new, or created post-upgrade
//     and mirrored), so there is no host work to wait on;
//   - complete: nothing is running;
//   - paused or disabled: an explicit, annotation-driven state that the mirror carries
//     onto the NodeWright, which then does not roll out. We deliberately do NOT force a
//     user to unpause/enable a Skyhook to migrate it: it migrates in that state and the
//     NodeWright stays paused/disabled.
//
// Every other status (in_progress, erroring, blocked, waiting, unknown) is treated as
// in-flight and holds the migration until it is finished, rolled back, or deleted.
func inFlightLegacySkyhooks(ctx context.Context, reader client.Reader) ([]string, error) {
	list := &skyhookv1.SkyhookList{}
	if err := reader.List(ctx, list); err != nil {
		// The legacy skyhook.nvidia.com CRD not being installed (a nodewright-only
		// topology, or the migration's own end state once the legacy CRD is removed)
		// definitively means there are no legacy objects to wait on. Treat it as "no
		// hold" rather than wedging the operator forever; only genuine transient list
		// errors should hold.
		if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listing legacy skyhooks for migration hold: %w", err)
	}

	var inFlight []string
	for i := range list.Items {
		switch list.Items[i].Status.Status {
		case "", skyhookv1.StatusComplete, skyhookv1.StatusPaused, skyhookv1.StatusDisabled:
			// safe to migrate in place; no hold
		default:
			inFlight = append(inFlight, list.Items[i].Name)
		}
	}
	return inFlight, nil
}

// legacyMigrationHold returns a non-nil requeue result while the NodeWright reconcile
// must hold its cutover, else nil.
//
// WHY: on an upgrade from the pre-rename operator, a legacy Skyhook that was still
// rolling out may have left an in-flight package pod doing host mutation. Taking over
// that node from the new (NodeWright) reconcile could double-drive it. So while any
// legacy Skyhook is still actively in flight we do nothing and requeue, and surface a
// loud log telling the operator to finish or roll back those rollouts on the pre-rename
// version before upgrading. Paused/disabled Skyhooks are NOT in-flight and migrate as
// they are (see inFlightLegacySkyhooks). This is a fail-safe stop, not an auto-resume:
// a legacy Skyhook's status is frozen once we stop reconciling that kind, so an
// in-flight hold clears only when those Skyhooks are finished, rolled back, or deleted.
// A genuine (transient) list error is also treated as a hold (requeue) so we never take
// over on unknown state; a missing legacy CRD is not unknown state (no legacy objects
// exist) and does not hold.
func (r *SkyhookReconciler) legacyMigrationHold(ctx context.Context) *ctrl.Result {
	inFlight, err := inFlightLegacySkyhooks(ctx, r.Client)
	if err != nil {
		log.FromContext(ctx).Error(err, "could not check legacy Skyhooks for migration hold; requeueing before taking over")
		return &ctrl.Result{RequeueAfter: legacyMigrationHoldRequeue}
	}
	if len(inFlight) == 0 {
		return nil
	}

	log.FromContext(ctx).Info("holding NodeWright reconcile: legacy Skyhooks are still rolling out; finish, roll back, or delete them on the pre-rename operator before upgrading",
		"inFlightSkyhooks", inFlight)
	return &ctrl.Result{RequeueAfter: legacyMigrationHoldRequeue}
}
