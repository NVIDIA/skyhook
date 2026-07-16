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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// legacyMigrationHoldRequeue is how long the reconciler waits before re-checking
// whether the legacy Skyhooks have drained during a migration hold.
const legacyMigrationHoldRequeue = 20 * time.Second

// incompleteLegacySkyhooks lists legacy skyhook.nvidia.com Skyhooks that are still
// mid-rollout: their status is set (the pre-rename operator reconciled them at least
// once) and is not complete. A Skyhook with an EMPTY status is intentionally
// excluded: it was never reconciled by the old operator (either brand new, or created
// after the upgrade and mirrored into a NodeWright), so it is not in-flight host work
// to wait on and must not wedge the hold open forever.
func incompleteLegacySkyhooks(ctx context.Context, reader client.Reader) ([]string, error) {
	list := &skyhookv1.SkyhookList{}
	if err := reader.List(ctx, list); err != nil {
		return nil, fmt.Errorf("listing legacy skyhooks for migration hold: %w", err)
	}

	var incomplete []string
	for i := range list.Items {
		status := list.Items[i].Status.Status
		if status != "" && status != skyhookv1.StatusComplete {
			incomplete = append(incomplete, list.Items[i].Name)
		}
	}
	return incomplete, nil
}

// legacyMigrationHold returns a non-nil requeue result while the NodeWright reconcile
// must hold its cutover, else nil.
//
// WHY: on an upgrade from the pre-rename operator, a legacy Skyhook that was still
// rolling out may have left an in-flight package pod doing host mutation. Taking over
// that node from the new (NodeWright) reconcile could double-drive it. So while any
// legacy Skyhook reads non-complete we do nothing and requeue, and surface a loud log
// telling the operator to finish or roll back those rollouts on the pre-rename version
// before upgrading. This is a fail-safe stop, not an auto-resume: a legacy Skyhook's
// status is frozen once we stop reconciling that kind, so the hold clears only when
// the legacy Skyhooks genuinely read complete (i.e. the prerequisite was met). A list
// error is also treated as a hold (requeue) so we never take over on unknown state.
func (r *SkyhookReconciler) legacyMigrationHold(ctx context.Context) *ctrl.Result {
	incomplete, err := incompleteLegacySkyhooks(ctx, r.Client)
	if err != nil {
		log.FromContext(ctx).Error(err, "could not check legacy Skyhooks for migration hold; requeueing before taking over")
		return &ctrl.Result{RequeueAfter: legacyMigrationHoldRequeue}
	}
	if len(incomplete) == 0 {
		return nil
	}

	log.FromContext(ctx).Info("holding NodeWright reconcile: legacy Skyhooks are not complete; finish or roll back in-flight rollouts on the pre-rename operator before upgrading",
		"incompleteSkyhooks", incomplete)
	return &ctrl.Result{RequeueAfter: legacyMigrationHoldRequeue}
}
