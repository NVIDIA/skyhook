/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package pkg

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

var _ = Describe("Package Logs Command", func() {
	Describe("getContainerStatus", func() {
		It("should find init container status", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "init-container", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}

			status := getContainerStatus(pod, "init-container")
			Expect(status).NotTo(BeNil())
			Expect(status.Name).To(Equal("init-container"))
		})

		It("should find regular container status", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "main-container", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}

			status := getContainerStatus(pod, "main-container")
			Expect(status).NotTo(BeNil())
			Expect(status.Name).To(Equal("main-container"))
		})

		It("should prefer init container when both have same name", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "container", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
					},
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "container", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}

			status := getContainerStatus(pod, "container")
			Expect(status).NotTo(BeNil())
			// Should return init container (checked first)
			Expect(status.State.Terminated).NotTo(BeNil())
		})

		It("should return nil for non-existent container", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "other-container"},
					},
				},
			}

			status := getContainerStatus(pod, "non-existent")
			Expect(status).To(BeNil())
		})
	})

	Describe("getContainersToLog", func() {
		It("should return specific container when stage is specified", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "mypackage-apply"},
						{Name: "mypackage-config"},
					},
				},
			}
			opts := &logsOptions{packageName: "mypackage", stage: "apply"}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("mypackage-apply"))
		})

		It("should check regular containers for stage match", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "mypackage-apply"},
					},
				},
			}
			opts := &logsOptions{packageName: "mypackage", stage: "apply"}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("mypackage-apply"))
		})

		It("should return nil when stage container not found", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "other-container"},
					},
				},
			}
			opts := &logsOptions{packageName: "mypackage", stage: "apply"}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(BeNil())
		})

		It("should find running init container when no stage specified", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "pkg-init", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
						{Name: "pkg-apply", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("pkg-apply"))
		})

		It("should skip init containers ending with -init when other containers exist", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "pkg-init", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
						{Name: "pkg-apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			// Should return pkg-apply, not pkg-init
			Expect(containers).To(ConsistOf("pkg-apply"))
		})

		It("should find terminated init container", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "pkg-apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("pkg-apply"))
		})

		It("should find running regular container when no init containers have run", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "main", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("main"))
		})

		It("should skip pause container if other containers exist", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{Name: "pause", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
						{Name: "main", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("main"))
		})

		It("should fallback to first init container if nothing has run", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "first-init"},
						{Name: "second-init"},
					},
				},
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{
						{Name: "first-init", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("first-init"))
		})

		It("should fallback to first regular container if no init containers", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "first-container"},
					},
				},
			}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(ConsistOf("first-container"))
		})

		It("should return nil for empty pod", func() {
			pod := &corev1.Pod{}
			opts := &logsOptions{}

			containers := getContainersToLog(pod, opts)
			Expect(containers).To(BeNil())
		})
	})

	Describe("NewLogsCmd", func() {
		var logsCmd *cobra.Command

		BeforeEach(func() {
			testCtx := context.NewCLIContext(nil)
			logsCmd = NewLogsCmd(testCtx)
		})

		It("should require --skyhook flag", func() {
			logsCmd.SetArgs([]string{"pkg1"})
			err := logsCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("skyhook"))
		})

		It("should validate stage flag values", func() {
			logsCmd.SetArgs([]string{"pkg1", "--skyhook", "test", "--stage", "invalid"})
			err := logsCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid stage"))
		})

		It("should accept valid stage values", func() {
			validStages := []string{"apply", "config", "interrupt", "post-interrupt"}
			for _, stage := range validStages {
				freshCtx := context.NewCLIContext(nil)
				freshCmd := NewLogsCmd(freshCtx)
				freshCmd.SetArgs([]string{"pkg1", "--skyhook", "test", "--stage", stage})
				err := freshCmd.Execute()
				// Should not be a stage validation error (will fail later at client creation)
				if err != nil {
					Expect(err.Error()).NotTo(ContainSubstring("invalid stage"))
				}
			}
		})

		It("should have follow flag with shorthand", func() {
			followFlag := logsCmd.Flags().Lookup("follow")
			Expect(followFlag).NotTo(BeNil())
			Expect(followFlag.Shorthand).To(Equal("f"))
		})

		It("should have tail flag with default -1", func() {
			tailFlag := logsCmd.Flags().Lookup("tail")
			Expect(tailFlag).NotTo(BeNil())
			Expect(tailFlag.DefValue).To(Equal("-1"))
		})

		It("should have node flag", func() {
			nodeFlag := logsCmd.Flags().Lookup("node")
			Expect(nodeFlag).NotTo(BeNil())
		})

		It("should have stage flag", func() {
			stageFlag := logsCmd.Flags().Lookup("stage")
			Expect(stageFlag).NotTo(BeNil())
		})
	})
})
