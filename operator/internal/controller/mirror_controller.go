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

// MIGRATION-SHIM: transition-only for the skyhook.nvidia.com -> nodewright.nvidia.com
// rename. Delete this whole file (and its wiring in cmd/manager/main.go) together
// with the legacy skyhook.nvidia.com group in the removal release.
//
// The mirror controller is the migration bridge from the legacy
// skyhook.nvidia.com API group to the new nodewright.nvidia.com group. It
// watches the legacy Skyhook and DeploymentPolicy objects and, one-way only
// (legacy -> new), ensures an equivalent object exists in the new group built
// via the api/v1alpha1 converters.
//
// It is level-triggered and idempotent and holds no reconciler-owned state:
// every decision is re-derived from the legacy object, the target object, and
// the bookkeeping annotations stamped on the target. The informer delivers a
// Create event for every pre-existing legacy object on startup, so this watch
// performs the bulk import automatically with no separate one-shot.
//
// Two small reconcilers (one per kind) share a generic core rather than one
// type-erased reconciler. The shared logic is identical, but Skyhook->NodeWright
// is a Kind change while DeploymentPolicy->DeploymentPolicy keeps the Kind, and
// the two source/target type pairs are distinct Go types with no common
// interface beyond client.Object. A generic core parameterized by the converter
// and an equality func captures the shared bookkeeping without forcing a runtime
// type switch on every reconcile.
package controller

import (
	"context"
	"fmt"
	"strconv"

	nwv1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	skyhookv1 "github.com/NVIDIA/nodewright/operator/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// MirroredFromAnnotation marks a NodeWright-group object as created and owned
	// by the mirror controller. Its value is the name of the legacy object it was
	// mirrored from. Absence (or a mismatched value) means the target is
	// user/Argo-managed and the mirror must not touch it.
	MirroredFromAnnotation = nwv1.METADATA_PREFIX + "/mirrored-from"
	// MirroredGenerationAnnotation records the legacy object's metadata.generation
	// (as a string) at the time it was last mirrored. The mirror compares this to
	// the current legacy generation to decide whether a spec re-sync is needed, so
	// steady-state informer resyncs do no writes.
	MirroredGenerationAnnotation = nwv1.METADATA_PREFIX + "/mirrored-generation"
)

//+kubebuilder:rbac:groups=nodewright.nvidia.com,resources=nodewrights,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nodewright.nvidia.com,resources=nodewrights/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nodewright.nvidia.com,resources=nodewrights/finalizers,verbs=update
//+kubebuilder:rbac:groups=nodewright.nvidia.com,resources=deploymentpolicies,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=nodewright.nvidia.com,resources=deploymentpolicies/status,verbs=get;update;patch

// SkyhookMirrorReconciler mirrors legacy Skyhook objects into NodeWright objects.
type SkyhookMirrorReconciler struct {
	Client client.Client
}

func (r *SkyhookMirrorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	legacy := &skyhookv1.Skyhook{}
	target := &nwv1.NodeWright{}

	return reconcileMirror(ctx, r.Client, req.NamespacedName, "Skyhook", "NodeWright", legacy, target,
		func() error { return skyhookv1.Convert_Skyhook_To_NodeWright(legacy, target) },
		func() { target.Spec = nwv1.NodeWrightSpec{} },
	)
}

func (r *SkyhookMirrorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("skyhook-mirror").
		For(&skyhookv1.Skyhook{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// DeploymentPolicyMirrorReconciler mirrors legacy DeploymentPolicy objects into
// new-group DeploymentPolicy objects (same Kind, new group).
type DeploymentPolicyMirrorReconciler struct {
	Client client.Client
}

func (r *DeploymentPolicyMirrorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	legacy := &skyhookv1.DeploymentPolicy{}
	target := &nwv1.DeploymentPolicy{}

	return reconcileMirror(ctx, r.Client, req.NamespacedName, "DeploymentPolicy", "DeploymentPolicy", legacy, target,
		func() error { return skyhookv1.Convert_DeploymentPolicy_To_NodeWright(legacy, target) },
		func() { target.Spec = nwv1.DeploymentPolicySpec{} },
	)
}

func (r *DeploymentPolicyMirrorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Client == nil {
		r.Client = mgr.GetClient()
	}
	return ctrl.NewControllerManagedBy(mgr).
		Named("deploymentpolicy-mirror").
		For(&skyhookv1.DeploymentPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// reconcileMirror is the shared, type-agnostic mirror core. legacy and target
// are caller-owned destination objects (so Convert can write directly into
// target). convert builds the desired target from the freshly-fetched legacy
// object; resetTargetSpec zeroes the target spec before a re-sync so the
// converter's overwrite does not leave stale sub-fields behind.
func reconcileMirror(
	ctx context.Context,
	c client.Client,
	key types.NamespacedName,
	legacyKind, targetKind string,
	legacy, target client.Object,
	convert func() error,
	resetTargetSpec func(),
) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("legacyKind", legacyKind, "name", key.Name)

	// 1. Get the legacy object. Not found -> nothing to mirror.
	if err := c.Get(ctx, key, legacy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("getting legacy %s %s: %w", legacyKind, key.Name, err)
	}

	// 2. Legacy object being deleted: do nothing. The mirror must NEVER delete the
	// target. Once mirrored, the NodeWright object is the real, reconciled source
	// of truth carrying live workload state (node annotations, in-flight
	// interrupts); deleting it because the legacy object went away would destroy
	// that state. Deleting a legacy object simply stops further mirroring.
	if !legacy.GetDeletionTimestamp().IsZero() {
		logger.Info("legacy object is being deleted; leaving mirrored target untouched")
		return ctrl.Result{}, nil
	}

	// 3. Build the desired target from the legacy object via the converter.
	if err := convert(); err != nil {
		return ctrl.Result{}, fmt.Errorf("converting legacy %s %s: %w", legacyKind, key.Name, err)
	}

	// 4. Inspect the existing target.
	existing := target.DeepCopyObject().(client.Object)
	getErr := c.Get(ctx, key, existing)
	switch {
	case apierrors.IsNotFound(getErr):
		stampMirror(target, key.Name, legacy.GetGeneration())
		if err := c.Create(ctx, target); err != nil {
			// A racing create (informer hasn't observed our own write yet) is benign;
			// the next reconcile converges.
			if apierrors.IsAlreadyExists(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, fmt.Errorf("creating mirrored %s %s: %w", targetKind, key.Name, err)
		}
		logger.Info("created mirrored target", "targetKind", targetKind)
		return ctrl.Result{}, nil

	case getErr != nil:
		return ctrl.Result{}, fmt.Errorf("getting mirrored %s %s: %w", targetKind, key.Name, getErr)
	}

	// Found. Back off unless the target is mirror-owned for THIS legacy object.
	// A user/Argo-managed target (no stamp, or a stamp naming a different source)
	// is authoritative; the mirror never modifies it.
	ann := existing.GetAnnotations()
	if ann[MirroredFromAnnotation] != key.Name {
		logger.Info("target exists but is not mirror-owned; backing off",
			"targetKind", targetKind, "mirroredFrom", ann[MirroredFromAnnotation])
		return ctrl.Result{}, nil
	}

	// Mirror-owned and the generation stamp matches the current legacy generation:
	// steady state, nothing to write.
	if ann[MirroredGenerationAnnotation] == strconv.FormatInt(legacy.GetGeneration(), 10) {
		return ctrl.Result{}, nil
	}

	// Mirror-owned but the legacy spec advanced: re-sync the spec, labels, and
	// annotations from the converter onto the live target (preserving its
	// resourceVersion and status), then refresh the generation stamp.
	resetTargetSpec()
	if err := convert(); err != nil {
		return ctrl.Result{}, fmt.Errorf("converting legacy %s %s: %w", legacyKind, key.Name, err)
	}

	syncMirrorOntoExisting(existing, target, key.Name, legacy.GetGeneration())
	if err := c.Update(ctx, existing); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating mirrored %s %s: %w", targetKind, key.Name, err)
	}
	logger.Info("updated mirrored target", "targetKind", targetKind, "generation", legacy.GetGeneration())
	return ctrl.Result{}, nil
}

// stampMirror writes the mirror bookkeeping annotations onto a fresh target.
func stampMirror(target client.Object, legacyName string, legacyGeneration int64) {
	ann := target.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[MirroredFromAnnotation] = legacyName
	ann[MirroredGenerationAnnotation] = strconv.FormatInt(legacyGeneration, 10)
	target.SetAnnotations(ann)
}

// syncMirrorOntoExisting copies the converted desired spec/labels/annotations
// onto the live target, leaving server-managed fields (resourceVersion, UID,
// status) intact so the Update is a minimal spec/metadata patch.
func syncMirrorOntoExisting(existing, desired client.Object, legacyName string, legacyGeneration int64) {
	existing.SetLabels(desired.GetLabels())
	existing.SetAnnotations(desired.GetAnnotations())
	stampMirror(existing, legacyName, legacyGeneration)

	switch e := existing.(type) {
	case *nwv1.NodeWright:
		e.Spec = desired.(*nwv1.NodeWright).Spec
	case *nwv1.DeploymentPolicy:
		e.Spec = desired.(*nwv1.DeploymentPolicy).Spec
	}
}

var (
	_ reconcile.Reconciler = &SkyhookMirrorReconciler{}
	_ reconcile.Reconciler = &DeploymentPolicyMirrorReconciler{}
)
