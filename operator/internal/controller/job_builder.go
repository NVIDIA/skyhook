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
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// doneContainerName replaces the forever-running pause main container. A Job records
// completion only when its pod reaches Succeeded, which needs every container to
// terminate, so the main container exits 0 immediately.
const doneContainerName = "done"

// jobLabels is the label set a package/interrupt Job carries and its child pods inherit
// (via the pod template), so existing CLI label queries keep working. The node label
// falls back to a bounded safe name when the node name exceeds the 63-char label-value
// limit; the pod template's nodeName stays authoritative for the full name.
func jobLabels(skyhook *wrapper.Skyhook, _package *v1alpha1.Package, stage v1alpha1.Stage, nodeName string, interrupt bool) map[string]string {
	nodeLabel := nodeName
	if len(nodeLabel) > 63 {
		nodeLabel = generateSafeName(63, nodeName)
	}
	labels := map[string]string{
		fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):       skyhook.Name,
		fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX):    fmt.Sprintf("%s-%s", _package.Name, _package.Version),
		fmt.Sprintf("%s/stage", v1alpha1.METADATA_PREFIX):      string(stage),
		fmt.Sprintf("%s/node", v1alpha1.METADATA_PREFIX):       nodeLabel,
		fmt.Sprintf("%s/generation", v1alpha1.METADATA_PREFIX): strconv.FormatInt(skyhook.Generation, 10),
	}
	if interrupt {
		labels[fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX)] = interruptLabelValue
	}
	return labels
}

// createJobFromPackage builds the batch/v1 Job that runs a package stage on a node. It
// wraps the pod this operator builds today (createPodFromPackage) so nothing about the
// executor's shape drifts, then applies the Job differences: the pause container becomes
// exit-0, restartPolicy becomes Never (each attempt a fresh, archivable pod), and
// disruptions are declaratively ignored.
func createJobFromPackage(opts SkyhookOperatorOptions, _package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string, stage v1alpha1.Stage) *batchv1.Job {
	pod := createPodFromPackage(opts, _package, skyhook, nodeName, stage)
	return jobFromPod(opts, pod, skyhook, _package, stage, nodeName, false)
}

// createInterruptJobFromPackage builds the interrupt Job. Interrupt Jobs keep
// restartPolicy OnFailure (a reboot interrupt kills its own pod by design; in-place
// restart after the node returns is the proven recovery) and therefore no
// podFailurePolicy — the API forbids combining them.
func createInterruptJobFromPackage(opts SkyhookOperatorOptions, _interrupt *v1alpha1.Interrupt, argEncode string, _package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string, stage v1alpha1.Stage) *batchv1.Job {
	pod := createInterruptPodForPackage(opts, _interrupt, argEncode, _package, skyhook, nodeName, stage)
	return jobFromPod(opts, pod, skyhook, _package, stage, nodeName, true)
}

// jobFromPod turns a package/interrupt pod into its Job. The old raw-pod builders remain
// the source of the pod spec (shared until they are removed with the legacy path), so this
// only expresses the Job-specific differences.
func jobFromPod(opts SkyhookOperatorOptions, pod *corev1.Pod, skyhook *wrapper.Skyhook, _package *v1alpha1.Package, stage v1alpha1.Stage, nodeName string, interrupt bool) *batchv1.Job {
	// The main container exits 0 so the pod can reach Succeeded; the agent image is
	// already pulled for the init containers, so this adds no new image.
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == pauseContainerName {
			pod.Spec.Containers[i].Name = doneContainerName
			pod.Spec.Containers[i].Image = getAgentImage(opts, _package)
			pod.Spec.Containers[i].Command = []string{shellBinary, "-c", "exit 0"}
		}
	}

	// Without these, DefaultTolerationSeconds admission adds 300s variants and the taint
	// manager evicts a node-pinned pod once a reboot-class interrupt keeps the node
	// NotReady past the default timeout — a pointless replacement + hostPath re-copy for a
	// node that is coming back. Deliberately unbounded (no tolerationSeconds): these pods
	// are node-bound host agents, so eviction elsewhere is never useful.
	pod.Spec.Tolerations = append(pod.Spec.Tolerations,
		corev1.Toleration{Key: "node.kubernetes.io/not-ready", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
		corev1.Toleration{Key: "node.kubernetes.io/unreachable", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
	)

	labels := jobLabels(skyhook, _package, stage, nodeName, interrupt)

	spec := batchv1.JobSpec{
		Parallelism: ptr(int32(1)),
		Completions: ptr(int32(1)),
		// Effectively unlimited, deliberately: the Job controller must never give up
		// before the operator does. Under restartPolicy Never this counts Failed pods,
		// and the podFailurePolicy below keeps disruptions from counting.
		BackoffLimit: ptr(int32(math.MaxInt32)),
		// Replace only after the previous pod is fully terminated so two executors never
		// overlap on the shared hostPath mounts.
		PodReplacementPolicy: ptr(batchv1.Failed),
		// TTLSecondsAfterFinished is deliberately unset at creation; JobReconcile sets it
		// by outcome at completion time so failure logs outlive success logs.
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec:       pod.Spec,
		},
	}

	if interrupt {
		spec.Template.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	} else {
		spec.Template.Spec.RestartPolicy = corev1.RestartPolicyNever
		spec.PodFailurePolicy = &batchv1.PodFailurePolicy{
			Rules: []batchv1.PodFailurePolicyRule{{
				Action: batchv1.PodFailurePolicyActionIgnore,
				OnPodConditions: []batchv1.PodFailurePolicyOnPodConditionsPattern{{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
				}},
			}},
		}
	}

	// From the package's stageTimeout, else the operator default. A value of 0 disables
	// the deadline: the field is omitted, because a literal 0 would insta-fail every Job.
	if timeout := effectiveStageTimeout(opts, _package); timeout > 0 {
		spec.ActiveDeadlineSeconds = ptr(int64(timeout.Seconds()))
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: opts.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				// Full provenance; ResourceID is routinely too long for a label value.
				fmt.Sprintf("%s/resource-id", v1alpha1.METADATA_PREFIX): skyhook.ResourceID(),
			},
		},
		Spec: spec,
	}
}

func effectiveStageTimeout(opts SkyhookOperatorOptions, _package *v1alpha1.Package) time.Duration {
	if _package.StageTimeout != nil {
		return _package.StageTimeout.Duration
	}
	return opts.JobStageTimeout
}
