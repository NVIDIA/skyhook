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
	"math"
	"strings"
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("job builders", func() {

	var (
		opts     SkyhookOperatorOptions
		skyhook  *wrapper.Skyhook
		pkg      *v1alpha1.Package
		nodeName string
	)

	prefix := func(suffix string) string { return v1alpha1.METADATA_PREFIX + "/" + suffix }

	BeforeEach(func() {
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
		})
		pkg = &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{Name: "tuning", Version: "1.0.0"},
			Image:      "ghcr.io/nvidia/skyhook-packages/tuning",
		}
		nodeName = "worker-7"
	})

	Describe("createJobFromPackage (package Job)", func() {

		var job *batchv1.Job

		JustBeforeEach(func() {
			job = createJobFromPackage(opts, pkg, skyhook, nodeName, v1alpha1.StageApply)
		})

		It("uses the package pod name and the operator namespace", func() {
			pod := createPodFromPackage(opts, pkg, skyhook, nodeName, v1alpha1.StageApply)
			Expect(job.Name).To(Equal(pod.Name))
			Expect(job.Namespace).To(Equal("skyhook"))
		})

		It("carries the full label set on the Job and the pod template, no interrupt label", func() {
			for _, labels := range []map[string]string{job.Labels, job.Spec.Template.Labels} {
				Expect(labels).To(HaveKeyWithValue(prefix("name"), "gpu-init"))
				Expect(labels).To(HaveKeyWithValue(prefix("package"), "tuning-1.0.0"))
				Expect(labels).To(HaveKeyWithValue(prefix("stage"), "apply"))
				Expect(labels).To(HaveKeyWithValue(prefix("node"), nodeName))
				Expect(labels).To(HaveKeyWithValue(prefix("generation"), "4"))
				Expect(labels).ToNot(HaveKey(prefix("interrupt")))
			}
		})

		It("records the full resource-id as an annotation", func() {
			Expect(job.Annotations).To(HaveKeyWithValue(prefix("resource-id"), skyhook.ResourceID()))
		})

		It("sets a run-to-completion spec with TTL unset at creation", func() {
			Expect(*job.Spec.Parallelism).To(Equal(int32(1)))
			Expect(*job.Spec.Completions).To(Equal(int32(1)))
			Expect(*job.Spec.BackoffLimit).To(Equal(int32(math.MaxInt32)))
			Expect(*job.Spec.PodReplacementPolicy).To(Equal(batchv1.Failed))
			Expect(job.Spec.TTLSecondsAfterFinished).To(BeNil())
		})

		It("uses restartPolicy Never and ignores DisruptionTarget disruptions", func() {
			Expect(job.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
			Expect(job.Spec.PodFailurePolicy).ToNot(BeNil())
			Expect(job.Spec.PodFailurePolicy.Rules).To(HaveLen(1))
			rule := job.Spec.PodFailurePolicy.Rules[0]
			Expect(rule.Action).To(Equal(batchv1.PodFailurePolicyActionIgnore))
			Expect(rule.OnPodConditions).To(HaveLen(1))
			Expect(rule.OnPodConditions[0].Type).To(Equal(corev1.DisruptionTarget))
		})

		It("replaces the pause container with an exit-0 container on the package image", func() {
			// The package image (not the agent image): the init-copy container already runs
			// /bin/sh from it, so the package image is guaranteed to have a shell; a minimal
			// agent image may not, which would StartError the exit-0 container and hang the Job.
			containers := job.Spec.Template.Spec.Containers
			Expect(containers).To(HaveLen(1))
			Expect(containers[0].Name).To(Equal("done"))
			Expect(containers[0].Image).To(Equal("ghcr.io/nvidia/skyhook-packages/tuning:1.0.0"))
			Expect(containers[0].Command).To(Equal([]string{"/bin/sh", "-c", "exit 0"}))
		})

		It("preserves the init-container chain and node pinning from the pod builder", func() {
			pod := createPodFromPackage(opts, pkg, skyhook, nodeName, v1alpha1.StageApply)
			Expect(job.Spec.Template.Spec.InitContainers).To(Equal(pod.Spec.InitContainers))
			Expect(job.Spec.Template.Spec.NodeName).To(Equal(nodeName))
		})

		It("adds unbounded not-ready and unreachable NoExecute tolerations", func() {
			byKey := map[string]corev1.Toleration{}
			for _, t := range job.Spec.Template.Spec.Tolerations {
				byKey[t.Key] = t
			}
			for _, key := range []string{"node.kubernetes.io/not-ready", "node.kubernetes.io/unreachable"} {
				t, ok := byKey[key]
				Expect(ok).To(BeTrue(), "expected toleration for %s", key)
				Expect(t.Operator).To(Equal(corev1.TolerationOpExists))
				Expect(t.Effect).To(Equal(corev1.TaintEffectNoExecute))
				Expect(t.TolerationSeconds).To(BeNil())
			}
		})

		Describe("activeDeadlineSeconds", func() {
			It("defaults to the operator JobStageTimeout", func() {
				Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
				Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(3600)))
			})

			When("the package sets its own stageTimeout", func() {
				BeforeEach(func() { pkg.StageTimeout = &metav1.Duration{Duration: 2 * time.Hour} })
				It("uses the package value", func() {
					Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(7200)))
				})
			})

			When("the package disables the deadline with 0", func() {
				BeforeEach(func() { pkg.StageTimeout = &metav1.Duration{Duration: 0} })
				It("omits activeDeadlineSeconds", func() {
					Expect(job.Spec.ActiveDeadlineSeconds).To(BeNil())
				})
			})

			When("the operator default is also 0", func() {
				BeforeEach(func() { opts.JobStageTimeout = 0 })
				It("omits activeDeadlineSeconds", func() {
					Expect(job.Spec.ActiveDeadlineSeconds).To(BeNil())
				})
			})

			When("the package sets a positive sub-second stageTimeout", func() {
				BeforeEach(func() { pkg.StageTimeout = &metav1.Duration{Duration: 500 * time.Millisecond} })
				It("rounds up to at least one second, never 0 (which would insta-fail)", func() {
					Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
					Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(1)))
				})
			})
		})

		When("the package sets gracefulShutdown and the operator sets an image pull secret", func() {
			BeforeEach(func() {
				opts.ImagePullSecret = "regcred"
				pkg.GracefulShutdown = &metav1.Duration{Duration: 45 * time.Second}
			})
			It("carries both through to the pod template", func() {
				Expect(job.Spec.Template.Spec.TerminationGracePeriodSeconds).ToNot(BeNil())
				Expect(*job.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(45)))
				Expect(job.Spec.Template.Spec.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "regcred"}))
			})
		})

		When("the node name exceeds the label-value limit", func() {
			BeforeEach(func() { nodeName = "worker-" + strings.Repeat("x", 70) })
			It("hashes the node label but keeps the full nodeName on the template", func() {
				Expect(job.Labels[prefix("node")]).To(Equal(generateSafeName(63, nodeName)))
				Expect(len(job.Labels[prefix("node")])).To(BeNumerically("<=", 63))
				Expect(job.Spec.Template.Spec.NodeName).To(Equal(nodeName))
			})
		})
	})

	Describe("createInterruptJobFromPackage (interrupt Job)", func() {

		var job *batchv1.Job

		JustBeforeEach(func() {
			job = createInterruptJobFromPackage(opts, &v1alpha1.Interrupt{Type: v1alpha1.REBOOT}, "args", pkg, skyhook, nodeName, v1alpha1.StageInterrupt)
		})

		It("uses the interrupt name formula and carries the interrupt label", func() {
			Expect(job.Name).To(Equal(generateSafeName(63, skyhook.Name, string(v1alpha1.StageInterrupt), string(v1alpha1.REBOOT), nodeName)))
			Expect(job.Labels).To(HaveKeyWithValue(prefix("interrupt"), "True"))
			Expect(job.Spec.Template.Labels).To(HaveKeyWithValue(prefix("interrupt"), "True"))
		})

		It("keeps restartPolicy OnFailure with no podFailurePolicy", func() {
			Expect(job.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyOnFailure))
			Expect(job.Spec.PodFailurePolicy).To(BeNil())
		})

		It("applies the stage deadline to the interrupt Job too (reboot time included)", func() {
			Expect(job.Spec.ActiveDeadlineSeconds).ToNot(BeNil())
			Expect(*job.Spec.ActiveDeadlineSeconds).To(Equal(int64(3600)))
		})

		It("still replaces the pause container with an exit-0 container", func() {
			// the exit-0 command itself is asserted in the package-Job spec above; the
			// transform is shared, so here just confirm the swap happened.
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(job.Spec.Template.Spec.Containers[0].Name).To(Equal(doneContainerName))
		})
	})

	Describe("API admission (envtest)", func() {
		// The struct-level specs above cannot see apiserver validation of the Job field
		// combinations (podFailurePolicy x restartPolicy x podReplacementPolicy x
		// activeDeadlineSeconds). Create both Job kinds against the envtest apiserver so an
		// admission regression fails here instead of only at e2e / wiring time.
		It("creates both a package and an interrupt Job against the apiserver", func() {
			built := []*batchv1.Job{
				createJobFromPackage(opts, pkg, skyhook, "worker-admission", v1alpha1.StageApply),
				createInterruptJobFromPackage(opts, &v1alpha1.Interrupt{Type: v1alpha1.REBOOT}, "args", pkg, skyhook, "worker-admission", v1alpha1.StageInterrupt),
			}
			for _, job := range built {
				Expect(k8sClient.Create(ctx, job)).To(Succeed(), "apiserver must admit job %s", job.Name)
				DeferCleanup(func(j *batchv1.Job) {
					_ = k8sClient.Delete(ctx, j)
				}, job)
			}
		})
	})
})
