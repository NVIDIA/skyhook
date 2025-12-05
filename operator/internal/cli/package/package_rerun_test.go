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
	"bytes"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

func TestPackage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Package CLI Tests Suite")
}

var _ = Describe("Package Rerun Command", func() {
	Describe("nodeStateAnnotationKey", func() {
		It("should return correct annotation key format", func() {
			key := nodeStateAnnotationKey("my-skyhook")
			Expect(key).To(Equal("skyhook.nvidia.com/nodeState_my-skyhook"))
		})

		It("should handle different skyhook names", func() {
			Expect(nodeStateAnnotationKey("gpu-init")).To(Equal("skyhook.nvidia.com/nodeState_gpu-init"))
			Expect(nodeStateAnnotationKey("test")).To(Equal("skyhook.nvidia.com/nodeState_test"))
		})
	})

	Describe("filterNodesWithPackage", func() {
		var nodeStates map[string]v1alpha1.NodeState

		BeforeEach(func() {
			nodeStates = map[string]v1alpha1.NodeState{
				"node1": {
					"pkg1|v1": {Name: "pkg1", Version: "v1", State: v1alpha1.StateComplete},
					"pkg2|v1": {Name: "pkg2", Version: "v1", State: v1alpha1.StateComplete},
				},
				"node2": {
					"pkg1|v1": {Name: "pkg1", Version: "v1", State: v1alpha1.StateComplete},
				},
				"node3": {
					"pkg3|v1": {Name: "pkg3", Version: "v1", State: v1alpha1.StateComplete},
				},
			}
		})

		It("should filter nodes that have the package", func() {
			matched := []string{"node1", "node2", "node3"}
			result := filterNodesWithPackage(matched, nodeStates, "pkg1|v1")
			Expect(result).To(ConsistOf("node1", "node2"))
		})

		It("should return empty when no nodes have the package", func() {
			matched := []string{"node1", "node2", "node3"}
			result := filterNodesWithPackage(matched, nodeStates, "nonexistent|v1")
			Expect(result).To(BeEmpty())
		})

		It("should only check matched nodes", func() {
			matched := []string{"node1"}
			result := filterNodesWithPackage(matched, nodeStates, "pkg1|v1")
			Expect(result).To(ConsistOf("node1"))
		})

		It("should handle nodes not in nodeStates map", func() {
			matched := []string{"node1", "node-unknown"}
			result := filterNodesWithPackage(matched, nodeStates, "pkg1|v1")
			Expect(result).To(ConsistOf("node1"))
		})
	})

	Describe("printRerunSummary", func() {
		var (
			cmd        *cobra.Command
			output     *bytes.Buffer
			nodeStates map[string]v1alpha1.NodeState
		)

		BeforeEach(func() {
			output = &bytes.Buffer{}
			cmd = &cobra.Command{}
			cmd.SetOut(output)

			nodeStates = map[string]v1alpha1.NodeState{
				"node1": {
					"pkg1|v1": {Name: "pkg1", Version: "v1", State: v1alpha1.StateComplete, Stage: v1alpha1.StageApply},
				},
				"node2": {
					"pkg1|v1": {Name: "pkg1", Version: "v1", State: v1alpha1.StateInProgress, Stage: v1alpha1.StageConfig},
				},
			}
		})

		It("should print package and skyhook info", func() {
			opts := &rerunOptions{skyhookName: "my-skyhook"}
			printRerunSummary(cmd, "pkg1", "v1", opts, []string{"node1"}, nodeStates, "pkg1|v1")

			Expect(output.String()).To(ContainSubstring("Package: pkg1 (version v1)"))
			Expect(output.String()).To(ContainSubstring("Skyhook: my-skyhook"))
		})

		It("should print stage when specified", func() {
			opts := &rerunOptions{skyhookName: "my-skyhook", stage: "config"}
			printRerunSummary(cmd, "pkg1", "v1", opts, []string{"node1"}, nodeStates, "pkg1|v1")

			Expect(output.String()).To(ContainSubstring("Re-run from stage: config"))
		})

		It("should list all nodes to reset", func() {
			opts := &rerunOptions{skyhookName: "my-skyhook"}
			printRerunSummary(cmd, "pkg1", "v1", opts, []string{"node1", "node2"}, nodeStates, "pkg1|v1")

			Expect(output.String()).To(ContainSubstring("Nodes to reset (2):"))
			Expect(output.String()).To(ContainSubstring("node1"))
			Expect(output.String()).To(ContainSubstring("node2"))
		})
	})

	Describe("promptConfirmation", func() {
		var (
			cmd    *cobra.Command
			output *bytes.Buffer
		)

		BeforeEach(func() {
			output = &bytes.Buffer{}
			cmd = &cobra.Command{}
			cmd.SetOut(output)
		})

		It("should return true when confirm flag is set", func() {
			opts := &rerunOptions{confirm: true}
			result, err := promptConfirmation(cmd, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should return true for 'y' input", func() {
			opts := &rerunOptions{confirm: false}
			cmd.SetIn(strings.NewReader("y\n"))

			result, err := promptConfirmation(cmd, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should return true for 'yes' input", func() {
			opts := &rerunOptions{confirm: false}
			cmd.SetIn(strings.NewReader("yes\n"))

			result, err := promptConfirmation(cmd, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should return false for 'n' input", func() {
			opts := &rerunOptions{confirm: false}
			cmd.SetIn(strings.NewReader("n\n"))

			result, err := promptConfirmation(cmd, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false for empty input", func() {
			opts := &rerunOptions{confirm: false}
			cmd.SetIn(strings.NewReader("\n"))

			result, err := promptConfirmation(cmd, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should print stage message when stage is set", func() {
			opts := &rerunOptions{confirm: false, stage: "config"}
			cmd.SetIn(strings.NewReader("n\n"))

			_, _ = promptConfirmation(cmd, opts)
			Expect(output.String()).To(ContainSubstring("re-run from the 'config' stage"))
		})

		It("should print default message when no stage", func() {
			opts := &rerunOptions{confirm: false}
			cmd.SetIn(strings.NewReader("n\n"))

			_, _ = promptConfirmation(cmd, opts)
			Expect(output.String()).To(ContainSubstring("re-run from the beginning"))
		})
	})

	Describe("printRerunResults", func() {
		var (
			cmd    *cobra.Command
			output *bytes.Buffer
		)

		BeforeEach(func() {
			output = &bytes.Buffer{}
			cmd = &cobra.Command{}
			cmd.SetOut(output)
		})

		It("should print success message", func() {
			printRerunResults(cmd, "pkg1", 3, nil)
			Expect(output.String()).To(ContainSubstring("Successfully reset package \"pkg1\" on 3 node(s)"))
		})

		It("should print errors when present", func() {
			errors := []string{"node1: connection refused", "node2: timeout"}
			printRerunResults(cmd, "pkg1", 1, errors)

			Expect(output.String()).To(ContainSubstring("Errors updating some nodes:"))
			Expect(output.String()).To(ContainSubstring("node1: connection refused"))
			Expect(output.String()).To(ContainSubstring("node2: timeout"))
		})

		It("should not print success when count is zero", func() {
			printRerunResults(cmd, "pkg1", 0, nil)
			Expect(output.String()).NotTo(ContainSubstring("Successfully"))
		})
	})

	Describe("NewRerunCmd", func() {
		var rerunCmd *cobra.Command

		BeforeEach(func() {
			testCtx := context.NewCLIContext(nil)
			rerunCmd = NewRerunCmd(testCtx)
		})

		It("should validate stage flag values", func() {
			rerunCmd.SetArgs([]string{"pkg1", "--skyhook", "test", "--node", "node1", "--stage", "invalid"})
			err := rerunCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid stage"))
		})

		It("should accept valid stage values", func() {
			validStages := []string{"apply", "config", "interrupt", "post-interrupt"}
			for _, stage := range validStages {
				freshCtx := context.NewCLIContext(nil)
				freshCmd := NewRerunCmd(freshCtx)
				// This will fail at client creation, but stage validation should pass
				freshCmd.SetArgs([]string{"pkg1", "--skyhook", "test", "--node", "node1", "--stage", stage})
				err := freshCmd.Execute()
				// Should not be a stage validation error
				if err != nil {
					Expect(err.Error()).NotTo(ContainSubstring("invalid stage"))
				}
			}
		})

		It("should require --skyhook flag", func() {
			rerunCmd.SetArgs([]string{"pkg1", "--node", "node1"})
			err := rerunCmd.Execute()
			Expect(err).To(HaveOccurred())
		})

		It("should require --node flag", func() {
			rerunCmd.SetArgs([]string{"pkg1", "--skyhook", "test"})
			err := rerunCmd.Execute()
			Expect(err).To(HaveOccurred())
		})
	})
})
