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
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("job controller", func() {

	It("maps only Jobs we own to job---<name> requests", func() {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-init-tuning-1-0-0-apply-worker-7",
				Namespace: "skyhook",
				Labels: map[string]string{
					fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): "gpu-init",
				},
			},
		}

		ret := jobHandlerFunc(ctx, job)
		Expect(ret).To(HaveLen(1))
		Expect(ret[0].Name).To(BeEquivalentTo("job---gpu-init-tuning-1-0-0-apply-worker-7"))
		Expect(ret[0].Namespace).To(Equal("skyhook"))

		// a Job without our name label (e.g. a CronJob's Job) is ignored
		job.Labels = map[string]string{"foo": "bar"}
		Expect(jobHandlerFunc(ctx, job)).To(BeNil())
	})

	Describe("jobMatchesPackage", func() {

		const node = "worker-7"

		var (
			opts    SkyhookOperatorOptions
			skyhook *wrapper.Skyhook
			pkg     *v1alpha1.Package
		)

		BeforeEach(func() {
			pkg = &v1alpha1.Package{
				PackageRef: v1alpha1.PackageRef{Name: "tuning", Version: "1.0.0"},
				Image:      "ghcr.io/nvidia/skyhook-packages/tuning",
			}
			opts = SkyhookOperatorOptions{
				Namespace:            "skyhook",
				CopyDirRoot:          "/var/lib/skyhook",
				AgentLogRoot:         "/var/log/skyhook",
				RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
				AgentImage:           "ghcr.io/nvidia/skyhook/agent:1.2.3",
				PauseImage:           "registry.k8s.io/pause:3.10",
				JobStageTimeout:      time.Hour,
			}
			skyhook = wrapper.NewSkyhookWrapper(&v1alpha1.NodeWright{
				ObjectMeta: metav1.ObjectMeta{Name: "gpu-init", UID: "abc", Generation: 4},
				Spec:       v1alpha1.NodeWrightSpec{Packages: v1alpha1.Packages{"tuning": *pkg}},
			})
		})

		It("matches a package Job the operator just built for the stage", func() {
			job := createJobFromPackage(opts, pkg, skyhook, node, v1alpha1.StageConfig)
			Expect(jobMatchesPackage(opts, pkg, *job, skyhook, v1alpha1.StageConfig)).To(BeTrue())
		})

		It("does not match once the package version changes", func() {
			job := createJobFromPackage(opts, pkg, skyhook, node, v1alpha1.StageApply)
			bumped := *pkg
			bumped.Version = "2.0.0"
			Expect(jobMatchesPackage(opts, &bumped, *job, skyhook, v1alpha1.StageApply)).To(BeFalse())
		})

		It("matches an interrupt Job for the same package", func() {
			// argEncode is deliberately not part of the match: podMatchesPackage does not
			// compare init-container Args, so its value here is arbitrary.
			job := createInterruptJobFromPackage(opts, &v1alpha1.Interrupt{Type: v1alpha1.REBOOT}, "encoded-interrupt-args", pkg, skyhook, node, v1alpha1.StageInterrupt)
			Expect(jobMatchesPackage(opts, pkg, *job, skyhook, v1alpha1.StageInterrupt)).To(BeTrue())
		})

		It("does not match an interrupt Job once the package version drifts", func() {
			// interrupt Jobs run at the uninstall-interrupt stage too, not only interrupt
			job := createInterruptJobFromPackage(opts, &v1alpha1.Interrupt{Type: v1alpha1.REBOOT}, "", pkg, skyhook, node, v1alpha1.StageUninstallInterrupt)
			bumped := *pkg
			bumped.Version = "2.0.0"
			Expect(jobMatchesPackage(opts, &bumped, *job, skyhook, v1alpha1.StageUninstallInterrupt)).To(BeFalse())
		})

		It("does not match once the package image changes (same name/version)", func() {
			job := createJobFromPackage(opts, pkg, skyhook, node, v1alpha1.StageApply)
			moved := *pkg
			moved.Image = "ghcr.io/nvidia/skyhook-packages/tuning-relocated"
			Expect(jobMatchesPackage(opts, &moved, *job, skyhook, v1alpha1.StageApply)).To(BeFalse())
		})

		It("does not match (and does not panic) when the init-container count differs", func() {
			job := createJobFromPackage(opts, pkg, skyhook, node, v1alpha1.StageApply)

			// an extra actual init container would index past the expected slice
			extra := job.DeepCopy()
			extra.Spec.Template.Spec.InitContainers = append(extra.Spec.Template.Spec.InitContainers, corev1.Container{Name: "rogue"})
			Expect(jobMatchesPackage(opts, pkg, *extra, skyhook, v1alpha1.StageApply)).To(BeFalse())

			// a missing actual init container must not silently match
			missing := job.DeepCopy()
			inits := missing.Spec.Template.Spec.InitContainers
			missing.Spec.Template.Spec.InitContainers = inits[:len(inits)-1]
			Expect(jobMatchesPackage(opts, pkg, *missing, skyhook, v1alpha1.StageApply)).To(BeFalse())
		})
	})
})
