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

package utils

import (
	"bytes"
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
)

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils CLI Tests Suite")
}

var _ = Describe("CLI Utility Functions", func() {
	Describe("MatchNodes", func() {
		It("should match nodes exactly", func() {
			nodes := []string{"node1", "node2", "node3"}
			patterns := []string{"node1", "node2"}
			matched, err := MatchNodes(patterns, nodes)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(ConsistOf("node1", "node2"))
		})

		It("should match nodes with regex patterns", func() {
			nodes := []string{"node1", "node2", "node3"}
			patterns := []string{"node.*"}
			matched, err := MatchNodes(patterns, nodes)
			Expect(err).NotTo(HaveOccurred())
			Expect(matched).To(ConsistOf("node1", "node2", "node3"))
		})

		It("should return an error for invalid regex patterns", func() {
			nodes := []string{"node1", "node2", "node3"}
			patterns := []string{"[invalid"}
			_, err := MatchNodes(patterns, nodes)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UnstructuredToSkyhook", func() {
		It("should convert an unstructured object to a Skyhook", func() {
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "nodewright.nvidia.com/v1alpha1",
					"kind":       "NodeWright",
					"metadata": map[string]interface{}{
						"name":      "test-skyhook",
						"namespace": "default",
					},
					"spec": map[string]interface{}{},
				},
			}
			skyhook, err := UnstructuredToSkyhook(u)
			Expect(err).NotTo(HaveOccurred())
			Expect(skyhook.Name).To(Equal("test-skyhook"))
		})

		It("should handle unstructured with packages", func() {
			u := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "nodewright.nvidia.com/v1alpha1",
					"kind":       "NodeWright",
					"metadata": map[string]interface{}{
						"name": "test-skyhook",
					},
					"spec": map[string]interface{}{
						"packages": map[string]interface{}{
							"pkg1": map[string]interface{}{
								"version": "1.0.0",
							},
						},
					},
				},
			}
			skyhook, err := UnstructuredToSkyhook(u)
			Expect(err).NotTo(HaveOccurred())
			Expect(skyhook.Spec.Packages).To(HaveKey("pkg1"))
		})

	})

	Describe("CompareVersions", func() {
		It("should return -1 when v1 < v2", func() {
			Expect(CompareVersions("v0.7.6", "v0.8.0")).To(Equal(-1))
			Expect(CompareVersions("v0.7.0", "v0.7.6")).To(Equal(-1))
			Expect(CompareVersions("v1.0.0", "v2.0.0")).To(Equal(-1))
		})

		It("should return 0 when v1 == v2", func() {
			Expect(CompareVersions("v0.8.0", "v0.8.0")).To(Equal(0))
			Expect(CompareVersions("v1.2.3", "v1.2.3")).To(Equal(0))
		})

		It("should return 1 when v1 > v2", func() {
			Expect(CompareVersions("v0.8.0", "v0.7.6")).To(Equal(1))
			Expect(CompareVersions("v1.0.0", "v0.9.9")).To(Equal(1))
			Expect(CompareVersions("v2.0.0", "v1.0.0")).To(Equal(1))
		})

		It("should handle versions without v prefix", func() {
			Expect(CompareVersions("0.7.6", "0.8.0")).To(Equal(-1))
			Expect(CompareVersions("0.8.0", "v0.8.0")).To(Equal(0))
			Expect(CompareVersions("v0.8.0", "0.7.6")).To(Equal(1))
		})

		It("should handle empty versions", func() {
			Expect(CompareVersions("", "v0.8.0")).To(Equal(-1))
			Expect(CompareVersions("v0.8.0", "")).To(Equal(1))
			Expect(CompareVersions("", "")).To(Equal(0))
		})

		It("should return 0 for invalid semver versions like dev", func() {
			// Invalid versions should return 0 (unknown/equal) not -1
			Expect(CompareVersions("dev", "v0.8.0")).To(Equal(0))
			Expect(CompareVersions("vdev", "v0.8.0")).To(Equal(0))
			Expect(CompareVersions("latest", "v0.8.0")).To(Equal(0))
			Expect(CompareVersions("v0.8.0", "dev")).To(Equal(0))
		})
	})

	Describe("IsValidVersion", func() {
		It("should return true for valid semver versions", func() {
			Expect(IsValidVersion("v0.8.0")).To(BeTrue())
			Expect(IsValidVersion("v1.2.3")).To(BeTrue())
			Expect(IsValidVersion("0.8.0")).To(BeTrue()) // without v prefix
			Expect(IsValidVersion("v1.0.0-alpha")).To(BeTrue())
		})

		It("should return false for invalid versions", func() {
			Expect(IsValidVersion("")).To(BeFalse())
			Expect(IsValidVersion("dev")).To(BeFalse())
			Expect(IsValidVersion("latest")).To(BeFalse())
			Expect(IsValidVersion("vdev")).To(BeFalse())
		})
	})

	Describe("ExtractImageTag", func() {
		It("should extract tag from image with tag", func() {
			Expect(ExtractImageTag("ghcr.io/nvidia/skyhook/operator:v1.2.3")).To(Equal("v1.2.3"))
			Expect(ExtractImageTag("nginx:1.19")).To(Equal("1.19"))
		})

		It("should extract tag from image with tag and digest", func() {
			Expect(ExtractImageTag("ghcr.io/nvidia/skyhook/operator:v1.2.3@sha256:abc123")).To(Equal("v1.2.3"))
		})

		It("should return empty string for image without tag", func() {
			Expect(ExtractImageTag("ghcr.io/nvidia/skyhook/operator")).To(Equal(""))
			Expect(ExtractImageTag("nginx")).To(Equal(""))
		})

		It("should handle image with only digest", func() {
			Expect(ExtractImageTag("ghcr.io/nvidia/skyhook/operator@sha256:abc123")).To(Equal(""))
		})
	})

	Describe("ResetCompartmentBatchStates (API method)", func() {
		It("should handle skyhook with nil CompartmentStatuses", func() {
			skyhook := &v1alpha1.NodeWright{
				Status: v1alpha1.NodeWrightStatus{
					CompartmentStatuses: nil,
				},
			}
			result := skyhook.ResetCompartmentBatchStates()
			Expect(result).To(BeFalse())
			Expect(skyhook.Status.CompartmentStatuses).To(BeNil())
		})

		It("should handle skyhook with empty CompartmentStatuses", func() {
			skyhook := &v1alpha1.NodeWright{
				Status: v1alpha1.NodeWrightStatus{
					CompartmentStatuses: map[string]v1alpha1.CompartmentStatus{},
				},
			}
			result := skyhook.ResetCompartmentBatchStates()
			Expect(result).To(BeFalse())
			Expect(skyhook.Status.CompartmentStatuses).To(BeEmpty())
		})

		It("should reset batch state for a single compartment", func() {
			skyhook := &v1alpha1.NodeWright{
				Status: v1alpha1.NodeWrightStatus{
					CompartmentStatuses: map[string]v1alpha1.CompartmentStatus{
						"default": {
							Matched:         10,
							Ceiling:         3,
							InProgress:      2,
							Completed:       5,
							ProgressPercent: 50,
							BatchState: &v1alpha1.BatchProcessingState{
								CurrentBatch:        3,
								ConsecutiveFailures: 2,
								CompletedNodes:      5,
								FailedNodes:         1,
								ShouldStop:          true,
								LastBatchSize:       4,
								LastBatchFailed:     true,
							},
						},
					},
				},
			}

			result := skyhook.ResetCompartmentBatchStates()
			Expect(result).To(BeTrue())

			Expect(skyhook.Status.CompartmentStatuses).To(HaveKey("default"))
			compartment := skyhook.Status.CompartmentStatuses["default"]

			// Verify non-batch fields are preserved
			Expect(compartment.Matched).To(Equal(10))
			Expect(compartment.Ceiling).To(Equal(3))
			Expect(compartment.InProgress).To(Equal(2))
			Expect(compartment.Completed).To(Equal(5))
			Expect(compartment.ProgressPercent).To(Equal(50))

			// Verify batch state is reset to fresh state
			Expect(compartment.BatchState).NotTo(BeNil())
			Expect(compartment.BatchState.CurrentBatch).To(Equal(1))
			Expect(compartment.BatchState.ConsecutiveFailures).To(Equal(0))
			Expect(compartment.BatchState.CompletedNodes).To(Equal(0))
			Expect(compartment.BatchState.FailedNodes).To(Equal(0))
			Expect(compartment.BatchState.ShouldStop).To(BeFalse())
			Expect(compartment.BatchState.LastBatchSize).To(Equal(0))
			Expect(compartment.BatchState.LastBatchFailed).To(BeFalse())
		})

		It("should reset batch state for multiple compartments", func() {
			skyhook := &v1alpha1.NodeWright{
				Status: v1alpha1.NodeWrightStatus{
					CompartmentStatuses: map[string]v1alpha1.CompartmentStatus{
						"compartment-a": {
							BatchState: &v1alpha1.BatchProcessingState{
								CurrentBatch:        5,
								ConsecutiveFailures: 3,
								CompletedNodes:      20,
								FailedNodes:         5,
								ShouldStop:          true,
								LastBatchSize:       10,
								LastBatchFailed:     true,
							},
						},
						"compartment-b": {
							BatchState: &v1alpha1.BatchProcessingState{
								CurrentBatch:        2,
								ConsecutiveFailures: 0,
								CompletedNodes:      10,
								FailedNodes:         0,
								ShouldStop:          false,
								LastBatchSize:       5,
								LastBatchFailed:     false,
							},
						},
					},
				},
			}

			result := skyhook.ResetCompartmentBatchStates()
			Expect(result).To(BeTrue())

			// Verify both compartments are reset
			for _, compartmentName := range []string{"compartment-a", "compartment-b"} {
				Expect(skyhook.Status.CompartmentStatuses).To(HaveKey(compartmentName))
				compartment := skyhook.Status.CompartmentStatuses[compartmentName]

				Expect(compartment.BatchState).NotTo(BeNil())
				Expect(compartment.BatchState.CurrentBatch).To(Equal(1))
				Expect(compartment.BatchState.ConsecutiveFailures).To(Equal(0))
				Expect(compartment.BatchState.CompletedNodes).To(Equal(0))
				Expect(compartment.BatchState.FailedNodes).To(Equal(0))
				Expect(compartment.BatchState.ShouldStop).To(BeFalse())
				Expect(compartment.BatchState.LastBatchSize).To(Equal(0))
				Expect(compartment.BatchState.LastBatchFailed).To(BeFalse())
			}
		})

		It("should handle compartment without existing batch state", func() {
			skyhook := &v1alpha1.NodeWright{
				Status: v1alpha1.NodeWrightStatus{
					CompartmentStatuses: map[string]v1alpha1.CompartmentStatus{
						"default": {
							Matched:    10,
							BatchState: nil,
						},
					},
				},
			}

			result := skyhook.ResetCompartmentBatchStates()
			Expect(result).To(BeTrue())

			compartment := skyhook.Status.CompartmentStatuses["default"]
			Expect(compartment.BatchState).NotTo(BeNil())
			Expect(compartment.BatchState.CurrentBatch).To(Equal(1))
		})
	})

	Describe("SetNodeAnnotation", func() {
		It("preserves arbitrary JSON characters in the value", func() {
			kube := fake.NewClientset()
			_, err := kube.CoreV1().Nodes().Create(context.Background(),
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			value := `{"a":"b\"c","x":"yz","unicode":"café"}`
			Expect(SetNodeAnnotation(context.Background(), kube, "n1", "nodewright.nvidia.com/nodeState_demo", value)).To(Succeed())

			got, err := kube.CoreV1().Nodes().Get(context.Background(), "n1", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(got.Annotations["nodewright.nvidia.com/nodeState_demo"]).To(Equal(value))
		})
	})

	Describe("ConfirmYN", func() {
		DescribeTable("recognises y/n responses",
			func(input string, expected bool) {
				cmd := &cobra.Command{}
				out := &bytes.Buffer{}
				cmd.SetOut(out)
				cmd.SetIn(bytes.NewBufferString(input))
				ok, err := ConfirmYN(cmd, "Continue?")
				Expect(err).NotTo(HaveOccurred())
				Expect(ok).To(Equal(expected))
				Expect(out.String()).To(ContainSubstring("Continue?"))
			},
			Entry("yes lowercase", "y\n", true),
			Entry("yes word", "yes\n", true),
			Entry("YES uppercase", "YES\n", true),
			Entry("no lowercase", "n\n", false),
			Entry("empty defaults to no", "\n", false),
			Entry("garbage defaults to no", "potato\n", false),
		)
	})

	Describe("ListNodesWithSkyhookState", func() {
		var kube *fake.Clientset
		BeforeEach(func() {
			kube = fake.NewClientset()
		})

		addNode := func(name, skyhook, annotationJSON string) {
			ann := map[string]string{}
			if skyhook != "" {
				ann["nodewright.nvidia.com/nodeState_"+skyhook] = annotationJSON
			}
			_, err := kube.CoreV1().Nodes().Create(context.Background(),
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: ann}},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		It("returns a map keyed by node name with parsed NodeState", func() {
			addNode("n1", "demo", `{"pkg1|1.0":{"name":"pkg1","version":"1.0","stage":"apply","state":"complete"}}`)
			addNode("n2", "demo", `{}`)
			addNode("n3", "", "")
			addNode("n4", "other", `{"x|1":{"name":"x","version":"1"}}`)

			got, err := ListNodesWithSkyhookState(context.Background(), kube, "demo", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(2))
			Expect(got).To(HaveKey("n1"))
			Expect(got).To(HaveKey("n2"))
			Expect(got["n1"]).To(HaveKey("pkg1|1.0"))
			Expect(got["n2"]).To(BeEmpty())
		})

		It("skips nodes with malformed annotations and surfaces an error", func() {
			addNode("n1", "demo", `not json`)
			addNode("n2", "demo", `{"pkg1|1.0":{"name":"pkg1","version":"1.0"}}`)

			got, err := ListNodesWithSkyhookState(context.Background(), kube, "demo", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("n1"))
			Expect(got).To(HaveKey("n2"))
			Expect(got).NotTo(HaveKey("n1"))
		})

		It("honours a label selector", func() {
			_, err := kube.CoreV1().Nodes().Create(context.Background(),
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{
					Name:        "n1",
					Labels:      map[string]string{"role": "gpu"},
					Annotations: map[string]string{"nodewright.nvidia.com/nodeState_demo": `{"p|1":{"name":"p","version":"1"}}`},
				}}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = kube.CoreV1().Nodes().Create(context.Background(),
				&corev1.Node{ObjectMeta: metav1.ObjectMeta{
					Name:        "n2",
					Annotations: map[string]string{"nodewright.nvidia.com/nodeState_demo": `{"p|1":{"name":"p","version":"1"}}`},
				}}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			got, err := ListNodesWithSkyhookState(context.Background(), kube, "demo", "role=gpu")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(1))
			Expect(got).To(HaveKey("n1"))
		})
	})
})
