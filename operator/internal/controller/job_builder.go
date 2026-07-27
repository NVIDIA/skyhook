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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
// podFailurePolicy: the API forbids combining them.
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
	// NotReady past the default timeout: a pointless replacement + hostPath re-copy for a
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
	// Round up so a positive sub-second timeout can't truncate to 0 (which would also
	// insta-fail); any positive value yields at least a 1s deadline.
	if timeout := effectiveStageTimeout(opts, _package); timeout > 0 {
		spec.ActiveDeadlineSeconds = ptr(int64(math.Ceil(timeout.Seconds())))
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

// createPodFromPackage creates a pod spec for a skyhook pod for a given package
func createPodFromPackage(opts SkyhookOperatorOptions, _package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string, stage v1alpha1.Stage) *corev1.Pod {
	// Generate consistent names that won't exceed k8s limits
	volumeName := generateSafeName(63, "metadata", nodeName)
	configMapName := generateSafeName(253, skyhook.Name, nodeName, "metadata")

	volumes := []corev1.Volume{
		{
			Name: volumeNameRootMount,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
		{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:             volumeNameRootMount,
			MountPath:        mountPathRoot,
			MountPropagation: ptr(corev1.MountPropagationHostToContainer),
		},
		{
			Name:      volumeName,
			MountPath: "/skyhook-package/node-metadata",
		},
	}

	if len(_package.ConfigMap) > 0 {
		volumes = append(volumes, corev1.Volume{
			Name: _package.Name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: strings.ToLower(fmt.Sprintf("%s-%s-%s", skyhook.Name, _package.Name, _package.Version)),
					},
				},
			},
		})

		// Mount each configMap key as its own subPath rather than mounting the
		// whole configMap as a directory. A directory mount replaces the entire
		// path, hiding any files the package image baked in under
		// mountPathConfigMaps; per-key subPath mounts overlay individual files
		// on top of the image content instead. Trade-off: subPath mounts do not
		// receive live configMap updates, which is fine here because package
		// pods are recreated per stage / version bump. Keys are iterated in
		// sorted order so the generated pod spec is deterministic.
		configMapKeys := make([]string, 0, len(_package.ConfigMap))
		for key := range _package.ConfigMap {
			configMapKeys = append(configMapKeys, key)
		}
		sort.Strings(configMapKeys)

		for _, key := range configMapKeys {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      _package.Name,
				MountPath: fmt.Sprintf("%s/%s", mountPathConfigMaps, key),
				SubPath:   key,
				ReadOnly:  true,
			})
		}
	}

	copyDir := fmt.Sprintf("%s/%s/%s-%s-%s-%d",
		opts.CopyDirRoot,
		skyhook.Name,
		_package.Name,
		_package.Version,
		skyhook.UID,
		skyhook.Generation,
	)
	applyargs := []string{strings.ToLower(string(stage)), mountPathRoot, copyDir}
	checkargs := []string{strings.ToLower(string(stage) + "-check"), mountPathRoot, copyDir}

	agentEnvs := append(
		_package.Env,
		getAgentConfigEnvVars(opts, _package.Name, _package.Version, skyhook.ResourceID(), skyhook.Name, skyhook.NodeOrder(nodeName))...,
	)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateSafeName(63, skyhook.Name, _package.Name, _package.Version, string(stage), nodeName),
			Namespace: opts.Namespace,
			Labels: map[string]string{
				fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):    skyhook.Name,
				fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX): fmt.Sprintf("%s-%s", _package.Name, _package.Version),
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyOnFailure,
			InitContainers: []corev1.Container{
				{
					Name:            fmt.Sprintf("%s-init", trunstr(_package.Name, 43)),
					Image:           getPackageImage(_package),
					ImagePullPolicy: corev1.PullAlways,
					Command:         []string{shellBinary},
					Args: []string{
						"-c",
						"mkdir -p /root/${SKYHOOK_DIR} && cp -r /skyhook-package/* /root/${SKYHOOK_DIR}",
					},
					Env: []corev1.EnvVar{
						{
							Name:  "SKYHOOK_DIR",
							Value: copyDir,
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
				},
				{
					Name:            fmt.Sprintf("%s-%s", trunstr(_package.Name, 43), stage),
					Image:           getAgentImage(opts, _package),
					ImagePullPolicy: corev1.PullAlways,
					Args:            applyargs,
					Env:             agentEnvs,
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
				},
				{
					Name:            fmt.Sprintf("%s-%scheck", trunstr(_package.Name, 43), stage),
					Image:           getAgentImage(opts, _package),
					ImagePullPolicy: corev1.PullAlways,
					Args:            checkargs,
					Env:             agentEnvs,
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  pauseContainerName,
					Image: opts.PauseImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
			},
			Volumes:     volumes,
			HostPID:     true,
			HostNetwork: true,
			// If you change these go change the SelectNode toleration in cluster_state.go
			Tolerations: append([]corev1.Toleration{ // tolerate all cordon
				{
					Key:      TaintUnschedulable,
					Operator: corev1.TolerationOpExists,
				},
				opts.GetRuntimeRequiredToleration(),
			}, skyhook.Spec.AdditionalTolerations...),
		},
	}
	if opts.ImagePullSecret != "" {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: opts.ImagePullSecret,
			},
		}
	}
	if _package.GracefulShutdown != nil {
		pod.Spec.TerminationGracePeriodSeconds = ptr(int64(_package.GracefulShutdown.Duration.Seconds()))
	}
	setPodResources(pod, _package.Resources)
	return pod
}

// createInterruptPodForPackage returns the pod spec for an interrupt pod given an package
func createInterruptPodForPackage(opts SkyhookOperatorOptions, _interrupt *v1alpha1.Interrupt, argEncode string, _package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string, stage v1alpha1.Stage) *corev1.Pod {
	copyDir := fmt.Sprintf("%s/%s/%s-%s-%s-%d",
		opts.CopyDirRoot,
		skyhook.Name,
		_package.Name,
		_package.Version,
		skyhook.UID,
		skyhook.Generation,
	)

	volumes := []corev1.Volume{
		{
			Name: volumeNameRootMount,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
		{
			// node names in different CSPs might include dots which isn't allowed in volume names
			// so we have to replace all dots with dashes
			Name: generateSafeName(63, skyhook.Name, nodeName, "metadata"),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhook.Name, nodeName), ".", "-"),
					},
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:             volumeNameRootMount,
			MountPath:        mountPathRoot,
			MountPropagation: ptr(corev1.MountPropagationHostToContainer),
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generateSafeName(63, skyhook.Name, string(stage), string(_interrupt.Type), nodeName),
			Namespace: opts.Namespace,
			Labels: map[string]string{
				fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):      skyhook.Name,
				fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX):   fmt.Sprintf("%s-%s", _package.Name, _package.Version),
				fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX): interruptLabelValue,
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyOnFailure,
			InitContainers: []corev1.Container{
				{
					Name:  InterruptContainerName,
					Image: getAgentImage(opts, _package),
					Args:  []string{"interrupt", mountPathRoot, copyDir, argEncode},
					Env:   getAgentConfigEnvVars(opts, _package.Name, _package.Version, skyhook.ResourceID(), skyhook.Name, skyhook.NodeOrder(nodeName)),
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  pauseContainerName,
					Image: opts.PauseImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
			},
			HostPID:     true,
			HostNetwork: true,
			// If you change these go change the SelectNode toleration in cluster_state.go
			Tolerations: append([]corev1.Toleration{ // tolerate all cordon
				{
					Key:      TaintUnschedulable,
					Operator: corev1.TolerationOpExists,
				},
				opts.GetRuntimeRequiredToleration(),
			}, skyhook.Spec.AdditionalTolerations...),
			Volumes: volumes,
		},
	}
	if opts.ImagePullSecret != "" {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: opts.ImagePullSecret,
			},
		}
	}
	return pod
}

// PodMatchesPackage asserts that a given pod matches the given pod spec
func podMatchesPackage(opts SkyhookOperatorOptions, _package *v1alpha1.Package, pod corev1.Pod, skyhook *wrapper.Skyhook, stage v1alpha1.Stage) bool {
	var expectedPod *corev1.Pod

	// need to differentiate whether the pod is for an interrupt or not so we know
	// what to expect and how to compare them
	isInterrupt := false
	_, limitRange := pod.Annotations["kubernetes.io/limit-ranger"]

	if pod.Labels[fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX)] == interruptLabelValue {
		expectedPod = createInterruptPodForPackage(opts, &v1alpha1.Interrupt{}, "", _package, skyhook, "", stage)
		isInterrupt = true
	} else {
		expectedPod = createPodFromPackage(opts, _package, skyhook, "", stage)
	}

	actualPod := pod.DeepCopy()

	// check to see whether the name or the version of the package changed
	packageLabel := fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)
	if actualPod.Labels[packageLabel] != expectedPod.Labels[packageLabel] {
		return false
	}

	// compare initContainers since this is where a lot of the important info lives
	for i := range actualPod.Spec.InitContainers {
		expectedContainer := expectedPod.Spec.InitContainers[i]
		actualContainer := actualPod.Spec.InitContainers[i]

		if expectedContainer.Name != actualContainer.Name {
			return false
		}

		if expectedContainer.Image != actualContainer.Image {
			return false
		}

		// compare the containers env vars except for the ones that are inserted
		// by the operator by default as the SKYHOOK_RESOURCE_ID will change every
		// time the skyhook is updated and would cause every pod to be removed
		// TODO: This is ignoring all the static env vars that are set by operator config.
		// It probably should be just SKYHOOK_RESOURCE_ID that is ignored. Otherwise,
		// a user will have to manually delete the pod to update the package when operator is updated.
		dummyAgentEnv := getAgentConfigEnvVars(opts, "", "", "", "", 0)
		excludedEnvs := make([]string, len(dummyAgentEnv))
		for i, env := range dummyAgentEnv {
			excludedEnvs[i] = env.Name
		}
		expectedFilteredEnv := FilterEnv(expectedContainer.Env, excludedEnvs...)
		actualFilteredEnv := FilterEnv(actualContainer.Env, excludedEnvs...)
		if !reflect.DeepEqual(expectedFilteredEnv, actualFilteredEnv) {
			return false
		}

		if !isInterrupt { // dont compare these since they are not configured on interrupt
			// compare resource requests and limits (CPU, memory, etc.)
			expectedResources := expectedContainer.Resources
			actualResources := actualContainer.Resources
			if skyhook.Spec.Packages[_package.Name].Resources != nil {
				// If CR has resources specified, they should match exactly
				if !reflect.DeepEqual(expectedResources, actualResources) {
					return false
				}
			} else {
				// If CR has no resources specified, ensure pod has no resource overrides
				if !limitRange {
					if actualResources.Requests != nil || actualResources.Limits != nil {
						return false
					}
				}
			}
		}
	}

	return true
}
