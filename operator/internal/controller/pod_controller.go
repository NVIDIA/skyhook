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
	"errors"
	"fmt"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// this is a sudo controller, it used to be one, but now are just functions of the skyhook controller
// the reason for this was to less issues around race conditions since they would be handled by one controller
// not sure that actually helped to be honest, but was the reason its acts like one.

// moved here for easier testing, vs being anonymous inline
func podHandlerFunc(ctx context.Context, o client.Object) []reconcile.Request {
	// logger := log.FromContext(ctx)

	pod := o.(*corev1.Pod)

	// if is skyhook package pod, then we care

	if labels.Set(pod.Labels).Has(fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX)) {
		// logger.Info("pod event", "name", o.GetName(), "labels", o.GetLabels(), "phase", pod.Status.Phase)

		return []reconcile.Request{{NamespacedName: types.NamespacedName{
			Name:      fmt.Sprintf("pod---%s", pod.Name), // adding prefix to help track pod events from other events
			Namespace: pod.Namespace,
		}}}
	}
	return nil
}

//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile

func (r *SkyhookReconciler) PodReconcile(ctx context.Context, pod *corev1.Pod) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("pod-reconcile")

	// Every package pod is a Job's child now, so completion and cleanup belong to JobReconcile
	// and the Job controller. This watch exists only to surface in-flight erroring: a step that
	// fails mid-Job shows up here before the Job itself goes terminal. It never deletes a pod or
	// records completion, which would race the Job path for the same node-state key.
	//
	// Erroring is guarded: a terminating pod (pause suspension, a sweep, a manual delete) or a
	// disruption casualty (eviction/preemption/PodGC) has no failure verdict and stays silent;
	// only a genuine terminal step failure marks erroring.
	if pod.DeletionTimestamp != nil || hasDisruptionTarget(pod) {
		return ctrl.Result{}, nil
	}

	containerName, state, restarts := containerExitedSuccessfully(pod)
	if state == containerStateFailed && podFailureIsGenuine(pod) {
		if _, err := r.UpdateNodeState(ctx, pod, v1alpha1.StateErroring, containerName, restarts); err != nil {
			logger.Error(err, "error updating node state for job pod failure", "pod", pod.Name)
		}
	}
	return ctrl.Result{}, nil
}

// podFailureIsGenuine reports whether the pod's first failing init container is a real terminal
// step failure — a nonzero exit (including OOMKilled), or an interrupt Job's CrashLoopBackOff —
// rather than a kubelet-couldn't-tell node-crash artifact (ContainerStatusUnknown) or an
// admission rejection (no container statuses).
func podFailureIsGenuine(pod *corev1.Pod) bool {
	for _, s := range pod.Status.InitContainerStatuses {
		switch {
		case s.State.Terminated != nil && s.State.Terminated.ExitCode == 0:
			continue // succeeded step, keep looking down the chain
		case s.State.Terminated != nil:
			return s.State.Terminated.Reason != "ContainerStatusUnknown"
		case s.State.Waiting != nil && s.State.Waiting.Reason == "CrashLoopBackOff":
			return true
		default:
			return false // an init container still running/pending: no terminal failure yet
		}
	}
	return false
}

// UpdateNodeState returns error and if to requeue, not all errors should be requeued, and some times there is no error but should be requeued
func (r *SkyhookReconciler) UpdateNodeState(ctx context.Context, pod *corev1.Pod, state v1alpha1.State, containerName string, restarts int32) (bool, error) {
	packagePtr, err := GetPackage(pod)
	if err != nil {
		return false, fmt.Errorf("error getting package from pod: %w", err)
	}

	if packagePtr == nil {
		return false, errors.New("there is no package, this pod is missing important info, or is labeled bad")
	}

	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, &node); err != nil {
		return true, err
	}

	patch := client.StrategicMergeFrom(node.DeepCopy())

	skyhookNode, err := wrapper.NewSkyhookNodeOnly(&node, packagePtr.Skyhook)
	if err != nil {
		return false, fmt.Errorf("error creating node wrapper: %w", err)
	}

	updated := false
	if state == v1alpha1.StateComplete {
		updated, err = r.HandleCompletePod(ctx, skyhookNode, packagePtr, containerName)
		if err != nil {
			return false, fmt.Errorf("error updating state for complete pod: %w", err)
		}
	}

	if !updated {
		err = skyhookNode.Upsert(packagePtr.PackageRef, packagePtr.Image, state, packagePtr.Stage, restarts, packagePtr.ContainerSHA)
		if err != nil {
			return false, err
		}
	}

	if v1alpha1.StateToStatus(state) == v1alpha1.StatusErroring {
		skyhookNode.SetStatus(v1alpha1.StateToStatus(state))
	}

	if skyhookNode.Changed() {
		if err := r.Patch(ctx, &node, patch); err != nil {
			return true, fmt.Errorf("error updating node with state from pod: %w", err)
		}

		r.recorder.Eventf(&node, nil, EventTypeNormal, EventsReasonSkyhookApply, "UpdateNodeState", "Package [%s:%s] state %s on [skyhook:%s]", packagePtr.Name, packagePtr.Version, state, packagePtr.Skyhook)
	}

	return false, nil
}

// HandleCompletePod handles the complete pod, this is called when the pod has exited successfully
// and we need to update the node state to complete and handles special cases like interrupts, upgrades, and uninstalls
func (r *SkyhookReconciler) HandleCompletePod(ctx context.Context, skyhookNode wrapper.SkyhookNodeOnly, packagePtr *PackageSkyhook, containerName string) (bool, error) {
	updated := false

	if containerName == InterruptContainerName {
		// in this one case do we need a skyhook instance to get packages
		// kind of sucks, but does not update, just reads so that is better
		// seems safer to leave it this way unfortunately.
		// by passing in packages we can not update load packages
		skyhook, err := r.dal.GetSkyhook(ctx, packagePtr.Skyhook)
		if err != nil {
			return false, err
		}

		upgraded, err := wrapper.Convert(skyhookNode, skyhook)
		if err != nil {
			return false, fmt.Errorf("error converting node wrapper: %w", err)
		}

		// progress forward any skipped packages that this interrupt completed
		if err := upgraded.ProgressSkipped(); err != nil {
			return false, fmt.Errorf("error progressing skipped packages: %w", err)
		}
	} else if packagePtr.Stage == v1alpha1.StageUpgrade {
		nodeState, err := skyhookNode.State()
		if err != nil {
			return false, fmt.Errorf("error getting node state: %w", err)
		}

		// go through and remove all the old node states for the package
		// after upgrade has finished
		for _, packageStatus := range nodeState {
			if packageStatus.Name == packagePtr.Name && packageStatus.Version != packagePtr.Version {
				packageStatusRef := v1alpha1.PackageRef{
					Name:    packageStatus.Name,
					Version: packageStatus.Version,
				}

				err = skyhookNode.RemoveState(packageStatusRef)
				if err != nil {
					return false, fmt.Errorf("error removing node state: %w", err)
				}
			}
		}
	} else if packagePtr.Stage == v1alpha1.StageUninstall {
		skyhook, err := r.dal.GetSkyhook(ctx, packagePtr.Skyhook)
		if err != nil {
			return false, err
		}

		if skyhook != nil {
			_package, exists := skyhook.Spec.Packages[packagePtr.Name]

			if !exists {
				// Package removed from spec — clean up node state.
				if err = skyhookNode.RemoveState(packagePtr.PackageRef); err != nil {
					return false, fmt.Errorf("error removing state for removed package: %w", err)
				}
				updated = true
				return updated, nil
			}

			if _package.Version != packagePtr.Version {
				// Defensive: webhook rejects version changes unless the package is
				// already fully uninstalled, so the uninstall pod shouldn't complete
				// for a version that's not in spec. Clean up defensively.
				if err = skyhookNode.RemoveState(packagePtr.PackageRef); err != nil {
					return false, fmt.Errorf("error cleaning up stale uninstall: %w", err)
				}
				updated = true
				return updated, nil
			}

			// Same version in spec: explicit or finalizer-driven uninstall.
			if _package.HasInterrupt() {
				// Package has an interrupt — advance to the uninstall-interrupt stage
				// so ProcessInterrupt fires the interrupt pod on the next reconcile.
				if err = skyhookNode.Upsert(packagePtr.PackageRef, packagePtr.Image,
					v1alpha1.StateInProgress, v1alpha1.StageUninstallInterrupt, 0, packagePtr.ContainerSHA); err != nil {
					return false, fmt.Errorf("error transitioning to uninstall-interrupt: %w", err)
				}
			} else {
				// No interrupt — remove state immediately (absent = uninstalled per D2).
				if err = skyhookNode.RemoveState(packagePtr.PackageRef); err != nil {
					return false, fmt.Errorf("error removing uninstalled package state: %w", err)
				}
				zeroOutSkyhookPackageMetrics(packagePtr.Skyhook, packagePtr.Name, packagePtr.Version)
			}
			updated = true
		}
	}

	return updated, nil
}

const (
	containerStateSuccess string = "Success"
	containerStateWaiting string = "Waiting"
	containerStateRunning string = "Running"
	containerStateFailed  string = "Failed"
)

func containerExitedSuccessfully(pod *corev1.Pod) (string, string, int32) {

	// can be either
	// apply and check
	// or just interrupt
	// need to check all passed or all failed

	checkStatus := func(status corev1.ContainerStatus) (string, int32) {
		if status.State.Terminated != nil {
			if status.State.Terminated.ExitCode == 0 {
				return containerStateSuccess, status.RestartCount
			}
			return containerStateFailed, status.RestartCount // TODO: is this always true? or should it be configuration?
		}
		if status.State.Running != nil {
			return containerStateRunning, status.RestartCount
		}
		if status.State.Waiting != nil {
			if status.State.Waiting.Reason == "CrashLoopBackOff" {
				return containerStateFailed, status.RestartCount
			}
			return containerStateWaiting, status.RestartCount
		}
		return "", int32(0)
	}

	state := ""
	restarts := int32(0)
	name := ""
	for _, status := range pod.Status.InitContainerStatuses {

		state, restarts = checkStatus(status)
		name = status.Name

		if state == containerStateFailed {
			return name, state, restarts
		}
	}

	return name, state, restarts
}
