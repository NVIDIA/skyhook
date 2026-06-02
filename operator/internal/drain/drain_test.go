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

package drain

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DecidePod", func() {
	controller := true
	const daemonSetKind = "DaemonSet"

	basePod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workload",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind:       "ReplicaSet",
						Name:       "workload-rs",
						Controller: &controller,
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		}
	}

	It("should preserve the current default behavior", func() {
		pod := basePod()

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionEvict, Reason: ReasonEviction}))
		Expect(decision.BlocksDrain()).To(BeTrue())
		Expect(decision.RequiresAction()).To(BeTrue())
	})

	It("should delete directly when eviction is disabled", func() {
		pod := basePod()
		options := DefaultOptions()
		options.DisableEviction = true

		decision := DecidePod(pod, options)

		Expect(decision).To(Equal(Decision{Action: ActionDelete, Reason: ReasonDelete}))
		Expect(decision.BlocksDrain()).To(BeTrue())
		Expect(decision.RequiresAction()).To(BeTrue())
	})

	It("should ignore pods that are not running or pending", func() {
		pod := basePod()
		pod.Status.Phase = corev1.PodSucceeded

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonPhase}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	It("should ignore pods that are already terminating", func() {
		pod := basePod()
		deletionTimestamp := metav1.NewTime(time.Date(2026, time.June, 2, 12, 0, 0, 0, time.UTC))
		pod.DeletionTimestamp = &deletionTimestamp

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonTerminating}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	It("should never drain pods that tolerate the unschedulable taint", func() {
		pod := basePod()
		pod.Spec.Tolerations = []corev1.Toleration{
			{Key: corev1.TaintNodeUnschedulable},
		}

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonUnschedulableToleration}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	DescribeTable("should apply Kubernetes toleration matching for the unschedulable taint",
		func(toleration corev1.Toleration, expectedDecision Decision) {
			pod := basePod()
			pod.Spec.Tolerations = []corev1.Toleration{toleration}

			decision := DecidePod(pod, DefaultOptions())

			Expect(decision).To(Equal(expectedDecision))
		},
		Entry("matches Exists on the NoSchedule unschedulable taint",
			corev1.Toleration{
				Key:      corev1.TaintNodeUnschedulable,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
			Decision{Action: ActionIgnore, Reason: ReasonUnschedulableToleration},
		),
		Entry("does not match a different effect",
			corev1.Toleration{
				Key:      corev1.TaintNodeUnschedulable,
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoExecute,
			},
			Decision{Action: ActionEvict, Reason: ReasonEviction},
		),
		Entry("does not match a different value with Equal",
			corev1.Toleration{
				Key:      corev1.TaintNodeUnschedulable,
				Operator: corev1.TolerationOpEqual,
				Value:    "true",
				Effect:   corev1.TaintEffectNoSchedule,
			},
			Decision{Action: ActionEvict, Reason: ReasonEviction},
		),
	)

	It("should ignore daemonset pods by default", func() {
		pod := basePod()
		pod.OwnerReferences[0].Kind = daemonSetKind

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonDaemonSet}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	It("should drain daemonset pods when ignoreDaemonSets is false", func() {
		pod := basePod()
		pod.OwnerReferences[0].Kind = daemonSetKind
		options := DefaultOptions()
		options.IgnoreDaemonSets = false

		decision := DecidePod(pod, options)

		Expect(decision).To(Equal(Decision{Action: ActionEvict, Reason: ReasonEviction}))
	})

	It("should never drain kube-system pods", func() {
		pod := basePod()
		pod.Namespace = "kube-system"

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonKubeSystem}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	It("should ignore mirror pods", func() {
		pod := basePod()
		pod.Annotations = map[string]string{
			MirrorPodAnnotationKey: "mirror-id",
		}

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionIgnore, Reason: ReasonMirrorPod}))
		Expect(decision.BlocksDrain()).To(BeFalse())
	})

	It("should evict unmanaged pods by default", func() {
		pod := basePod()
		pod.OwnerReferences = nil

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionEvict, Reason: ReasonEviction}))
	})

	It("should block on unmanaged pods when force is false", func() {
		pod := basePod()
		pod.OwnerReferences = nil
		options := DefaultOptions()
		options.Force = false

		decision := DecidePod(pod, options)

		Expect(decision).To(Equal(Decision{Action: ActionBlock, Reason: ReasonUnmanaged}))
		Expect(decision.BlocksDrain()).To(BeTrue())
		Expect(decision.RequiresAction()).To(BeFalse())
	})

	It("should evict emptyDir pods by default", func() {
		pod := basePod()
		pod.Spec.Volumes = []corev1.Volume{
			{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		}

		decision := DecidePod(pod, DefaultOptions())

		Expect(decision).To(Equal(Decision{Action: ActionEvict, Reason: ReasonEviction}))
	})

	It("should block on emptyDir pods when deleteEmptyDirData is false", func() {
		pod := basePod()
		pod.Spec.Volumes = []corev1.Volume{
			{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		}
		options := DefaultOptions()
		options.DeleteEmptyDirData = false

		decision := DecidePod(pod, options)

		Expect(decision).To(Equal(Decision{Action: ActionBlock, Reason: ReasonEmptyDir}))
		Expect(decision.BlocksDrain()).To(BeTrue())
		Expect(decision.RequiresAction()).To(BeFalse())
	})
})
