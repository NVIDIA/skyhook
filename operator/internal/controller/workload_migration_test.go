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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
)

var _ = Describe("reconcileLegacyLabeledWorkloads", func() {
	const (
		shName = "legacy-sh"
		ns     = "skyhook"
	)
	ctx := context.Background()

	opts := SkyhookOperatorOptions{
		Namespace:            ns,
		MaxInterval:          time.Second * 61,
		CopyDirRoot:          "/tmp",
		RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
		AgentImage:           "foo:bar",
		PauseImage:           "foo:bar",
		AgentLogRoot:         "/log",
		JobOperatorOptions: JobOperatorOptions{
			JobTTLSucceeded: time.Hour,
			JobTTLFailed:    24 * time.Hour,
			JobStageTimeout: time.Hour,
			JobBackoffLimit: 3,
		},
	}

	buildClient := func(objs ...client.Object) client.Client {
		s := runtime.NewScheme()
		Expect(corev1.AddToScheme(s)).To(Succeed())
		Expect(v1alpha1.AddToScheme(s)).To(Succeed())
		return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	}

	legacyPod := func(name, node string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels: map[string]string{
					"skyhook.nvidia.com/name":      shName,
					"skyhook.nvidia.com/Node.name": node,
				},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}
	}

	legacyCM := func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "legacy-cm",
				Namespace: ns,
				Labels: map[string]string{
					"skyhook.nvidia.com/skyhook-node-meta": shName,
					"skyhook.nvidia.com/name":              shName,
				},
			},
			Data: map[string]string{"k": "v"},
		}
	}

	It("converge: adds the nodewright ConfigMap label but keeps the legacy label and the legacy pods", func() {
		c := buildClient(legacyPod("legacy-pod", "node-a"), legacyCM())
		r, err := NewSkyhookReconciler(c.Scheme(), c, k8sfake.NewClientset(), events.NewFakeRecorder(10), opts)
		Expect(err).ToNot(HaveOccurred())

		hadLegacy, changed, err := r.reconcileLegacyLabeledWorkloads(ctx, shName, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(hadLegacy).To(BeTrue())
		Expect(changed).To(BeTrue())

		By("leaving the legacy pod in place for a possible rollback")
		Expect(c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "legacy-pod"}, &corev1.Pod{})).To(Succeed())

		By("adding the nodewright labels while keeping the legacy ones")
		gotCM := &corev1.ConfigMap{}
		Expect(c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "legacy-cm"}, gotCM)).To(Succeed())
		Expect(gotCM.Labels).To(HaveKeyWithValue("nodewright.nvidia.com/skyhook-node-meta", shName))
		Expect(gotCM.Labels).To(HaveKeyWithValue("skyhook.nvidia.com/skyhook-node-meta", shName))
		Expect(gotCM.Data).To(HaveKeyWithValue("k", "v"))

		By("being idempotent (the nodewright label is already present)")
		hadLegacy2, changed2, err := r.reconcileLegacyLabeledWorkloads(ctx, shName, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(hadLegacy2).To(BeTrue())
		Expect(changed2).To(BeFalse())
	})

	It("prune: graceful-deletes legacy pods, drops the legacy ConfigMap label, and leaves other objects untouched", func() {
		newPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-pod",
				Namespace: ns,
				Labels:    map[string]string{"nodewright.nvidia.com/name": shName},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		}
		otherPod := legacyPod("other-pod", "node-a")
		otherPod.Labels["skyhook.nvidia.com/name"] = "some-other-skyhook"

		// A converged ConfigMap carries both prefixes.
		cm := legacyCM()
		cm.Labels["nodewright.nvidia.com/skyhook-node-meta"] = shName
		cm.Labels["nodewright.nvidia.com/name"] = shName

		c := buildClient(legacyPod("legacy-pod", "node-a"), newPod, otherPod, cm)
		r, err := NewSkyhookReconciler(c.Scheme(), c, k8sfake.NewClientset(), events.NewFakeRecorder(10), opts)
		Expect(err).ToNot(HaveOccurred())

		hadLegacy, changed, err := r.reconcileLegacyLabeledWorkloads(ctx, shName, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(hadLegacy).To(BeTrue())
		Expect(changed).To(BeTrue())

		By("deleting the legacy-labeled pod for this skyhook")
		Expect(apierrors.IsNotFound(
			c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "legacy-pod"}, &corev1.Pod{}),
		)).To(BeTrue())

		By("leaving the new-labeled pod untouched")
		Expect(c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "new-pod"}, &corev1.Pod{})).To(Succeed())

		By("leaving another skyhook's legacy pod untouched (scoped by name)")
		Expect(c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "other-pod"}, &corev1.Pod{})).To(Succeed())

		By("dropping the legacy ConfigMap label while preserving the nodewright label and data")
		gotCM := &corev1.ConfigMap{}
		Expect(c.Get(ctx, types.NamespacedName{Namespace: ns, Name: "legacy-cm"}, gotCM)).To(Succeed())
		Expect(gotCM.Labels).To(HaveKeyWithValue("nodewright.nvidia.com/skyhook-node-meta", shName))
		for k := range gotCM.Labels {
			Expect(k).ToNot(HavePrefix("skyhook.nvidia.com/"))
		}
		Expect(gotCM.Data).To(HaveKeyWithValue("k", "v"))

		By("reporting nothing legacy remains on a second prune pass")
		hadLegacy2, changed2, err := r.reconcileLegacyLabeledWorkloads(ctx, shName, true)
		Expect(err).ToNot(HaveOccurred())
		Expect(hadLegacy2).To(BeFalse())
		Expect(changed2).To(BeFalse())
	})

	It("is a cheap no-op when there is nothing legacy-labeled", func() {
		c := buildClient()
		r, err := NewSkyhookReconciler(c.Scheme(), c, k8sfake.NewClientset(), events.NewFakeRecorder(10), opts)
		Expect(err).ToNot(HaveOccurred())

		hadLegacy, changed, err := r.reconcileLegacyLabeledWorkloads(ctx, shName, false)
		Expect(err).ToNot(HaveOccurred())
		Expect(hadLegacy).To(BeFalse())
		Expect(changed).To(BeFalse())
	})
})
