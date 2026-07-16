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
	"strings"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("package annotation helpers", func() {

	var (
		nw    *v1alpha1.NodeWright
		pkg   *v1alpha1.Package
		image string
	)

	BeforeEach(func() {
		nw = &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: "gpu-init"}}
		pkg = &v1alpha1.Package{
			PackageRef:   v1alpha1.PackageRef{Name: "tuning", Version: "1.2.3"},
			Image:        "ghcr.io/nvidia/skyhook-packages/tuning",
			ContainerSHA: "sha256:" + strings.Repeat("a", 64),
		}
		image = "ghcr.io/nvidia/skyhook-packages/tuning:1.2.3"
	})

	It("round-trips the package annotation identically on a Pod and a Job", func() {
		pod := &corev1.Pod{}
		job := &batchv1.Job{}

		Expect(SetPackages(pod, nw, image, v1alpha1.StageApply, pkg)).To(Succeed())
		Expect(SetPackages(job, nw, image, v1alpha1.StageApply, pkg)).To(Succeed())

		fromPod, err := GetPackage(pod)
		Expect(err).ToNot(HaveOccurred())
		fromJob, err := GetPackage(job)
		Expect(err).ToNot(HaveOccurred())

		Expect(fromJob).To(Equal(fromPod))
		Expect(fromJob.Skyhook).To(Equal("gpu-init"))
		Expect(fromJob.Stage).To(Equal(v1alpha1.StageApply))
		Expect(fromJob.Name).To(Equal("tuning"))

		key := fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)
		Expect(job.GetAnnotations()).To(HaveKey(key))
		Expect(job.GetAnnotations()[key]).To(Equal(pod.GetAnnotations()[key]))
	})

	It("marks a Job's package invalid via InvalidatePackage/IsInvalidPackage", func() {
		job := &batchv1.Job{}
		Expect(SetPackages(job, nw, image, v1alpha1.StageApply, pkg)).To(Succeed())

		invalid, err := IsInvalidPackage(job)
		Expect(err).ToNot(HaveOccurred())
		Expect(invalid).To(BeFalse())

		Expect(InvalidatePackage(job)).To(Succeed())

		invalid, err = IsInvalidPackage(job)
		Expect(err).ToNot(HaveOccurred())
		Expect(invalid).To(BeTrue())
	})
})
