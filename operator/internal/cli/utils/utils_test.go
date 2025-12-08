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

package utils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
					"apiVersion": "skyhook.nvidia.com/v1alpha1",
					"kind":       "Skyhook",
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
					"apiVersion": "skyhook.nvidia.com/v1alpha1",
					"kind":       "Skyhook",
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

	Describe("EscapeJSONPointer", func() {
		It("should escape tilde", func() {
			result := EscapeJSONPointer("key~value")
			Expect(result).To(Equal("key~0value"))
		})

		It("should escape forward slash", func() {
			result := EscapeJSONPointer("key/value")
			Expect(result).To(Equal("key~1value"))
		})

		It("should escape both tilde and forward slash", func() {
			result := EscapeJSONPointer("key~with/special")
			Expect(result).To(Equal("key~0with~1special"))
		})

		It("should handle string without special characters", func() {
			result := EscapeJSONPointer("normalkey")
			Expect(result).To(Equal("normalkey"))
		})

		It("should handle empty string", func() {
			result := EscapeJSONPointer("")
			Expect(result).To(Equal(""))
		})

		It("should escape tilde before forward slash per RFC 6901", func() {
			// RFC 6901 requires ~ to be escaped first, then /
			// This ensures ~/value becomes ~0~1value, not ~01value
			result := EscapeJSONPointer("~/value")
			Expect(result).To(Equal("~0~1value"))
		})
	})
})
