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

package wrapper

import (
	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("SkyhookCompartment", func() {
	It("should assign node to compartment", func() {
		compartment := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			},
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
				Labels: map[string]string{"test-label": "test-value"},
			},
		}

		skyhookNode, err := NewSkyhookNode(node, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		compartment.AddNode(skyhookNode)

		compartmentName, err := AssignNodeToCompartment(skyhookNode, map[string]*Compartment{"test-compartment": compartment}, []SkyhookNode{skyhookNode})
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment"))
	})

	It("should assign node to default compartment", func() {
		compartment := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			},
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
				Labels: map[string]string{"test-label": "test-value-other"},
			},
		}

		skyhookNode, err := NewSkyhookNode(node, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		compartmentName, err := AssignNodeToCompartment(skyhookNode, map[string]*Compartment{"test-compartment": compartment}, []SkyhookNode{skyhookNode})
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal(v1alpha1.DefaultCompartmentName))
	})

	It("should assign node to compartment with safer strategy", func() {
		compartment1 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-1",
				Strategy: &v1alpha1.DeploymentStrategy{
					Fixed: &v1alpha1.FixedStrategy{
						InitialBatch: ptr.To(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			},
		}

		compartment2 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-2",
				Strategy: &v1alpha1.DeploymentStrategy{
					Linear: &v1alpha1.LinearStrategy{
						InitialBatch: ptr.To(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			},
		}

		compartment3 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-3",
				Strategy: &v1alpha1.DeploymentStrategy{
					Exponential: &v1alpha1.ExponentialStrategy{
						InitialBatch: ptr.To(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-3": "test-value-3"},
				},
			},
		}

		fixedNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-fixed",
				Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2", "test-label-3": "test-value-3"},
			},
		}

		linearNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-linear",
				Labels: map[string]string{"test-label-2": "test-value-2", "test-label-3": "test-value-3"},
			},
		}

		exponentialNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-exponential",
				Labels: map[string]string{"test-label-3": "test-value-3"},
			},
		}

		skyhookNodeFixed, err := NewSkyhookNode(fixedNode, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-fixed",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		skyhookNodeLinear, err := NewSkyhookNode(linearNode, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-linear",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		skyhookNodeExponential, err := NewSkyhookNode(exponentialNode, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-exponential",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		compartments := map[string]*Compartment{
			"test-compartment-1": compartment1,
			"test-compartment-2": compartment2,
			"test-compartment-3": compartment3,
		}

		nodes := []SkyhookNode{skyhookNodeFixed, skyhookNodeLinear, skyhookNodeExponential}

		compartmentName, err := AssignNodeToCompartment(skyhookNodeFixed, compartments, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-1"))
		compartmentName, err = AssignNodeToCompartment(skyhookNodeLinear, compartments, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-2"))
		compartmentName, err = AssignNodeToCompartment(skyhookNodeExponential, compartments, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-3"))
	})

	It("should assign node to compartment with smaller count budget", func() {
		compartment1 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-1",
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr.To(1),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			},
		}

		compartment2 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-2",
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr.To(2),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			},
		}

		node1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-1",
				Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2"},
			},
		}

		skyhookNode1, err := NewSkyhookNode(node1, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-1",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		compartments := map[string]*Compartment{
			"test-compartment-1": compartment1,
			"test-compartment-2": compartment2,
		}

		nodes := []SkyhookNode{skyhookNode1}

		compartmentName, err := AssignNodeToCompartment(skyhookNode1, compartments, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-1"))
	})

	It("should assign node to compartment with smaller percent budget", func() {
		compartment1 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-1",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr.To(10),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			},
		}

		compartment2 := &Compartment{
			Compartment: v1alpha1.Compartment{
				Name: "test-compartment-2",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr.To(20),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			},
		}

		node1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-1",
				Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2"},
			},
		}

		skyhookNode1, err := NewSkyhookNode(node1, &v1alpha1.Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-1",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		compartments := map[string]*Compartment{
			"test-compartment-1": compartment1,
			"test-compartment-2": compartment2,
		}

		nodes := []SkyhookNode{skyhookNode1}

		compartmentName, err := AssignNodeToCompartment(skyhookNode1, compartments, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-1"))
	})

	It("should assign to compartment with higher percent but smaller effective capacity due to fewer matching nodes", func() {
		// Test that a compartment with a higher percent but fewer matching nodes
		// can have a smaller effective capacity and win the assignment

		// Compartment A: 50% budget, matches 10 nodes total
		compartmentA := NewCompartmentWrapper(&v1alpha1.Compartment{
			Name: "test-compartment-1",
			Budget: v1alpha1.DeploymentBudget{
				Percent: ptr.To(50), // 50% of 10 = 5 capacity
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"test-label-1": "test-value-1"},
			},
			Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}},
		})

		// Compartment B: 80% budget, matches only 2 nodes total
		compartmentB := NewCompartmentWrapper(&v1alpha1.Compartment{
			Name: "test-compartment-2",
			Budget: v1alpha1.DeploymentBudget{
				Percent: ptr.To(80), // 80% of 2 = ceil(1.6) = 2 capacity
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"test-label-2": "test-value-2"},
			},
			Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}},
		})

		// Target node matches both compartments
		targetNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node-1",
				Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2"},
			},
		}

		// Create all nodes
		allNodesList := []*corev1.Node{
			targetNode,
			// 9 more nodes that match compartment A (test-label-1=test-value-1) but not B
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a1", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a2", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a3", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a4", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a5", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a6", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a7", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a8", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node-a9", Labels: map[string]string{"test-label-1": "test-value-1"}}},
			// 1 more node that matches compartment B (test-label-2=test-value-2) but not A
			{ObjectMeta: metav1.ObjectMeta{Name: "node-b1", Labels: map[string]string{"test-label-2": "test-value-2"}}},
		}

		skyhook := &v1alpha1.Skyhook{ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"}}
		var allNodes []SkyhookNode
		for _, n := range allNodesList {
			sn, err := NewSkyhookNode(n, skyhook)
			Expect(err).NotTo(HaveOccurred())
			allNodes = append(allNodes, sn)
		}

		targetSkyhookNode, err := NewSkyhookNode(targetNode, skyhook)
		Expect(err).NotTo(HaveOccurred())

		compartments := map[string]*Compartment{
			"test-compartment-1": compartmentA,
			"test-compartment-2": compartmentB,
		}

		// Should assign to "test-compartment-2" because:
		// - test-compartment-1: 50% × 10 nodes = 5 capacity
		// - test-compartment-2: 80% × 2 nodes = 2 capacity (smaller wins)
		compartmentName, err := AssignNodeToCompartment(targetSkyhookNode, compartments, allNodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(compartmentName).To(Equal("test-compartment-2"), "higher percent but fewer matching nodes = smaller capacity")
	})

	It("should use lexicographic order as final tiebreaker when strategy and capacity are identical", func() {
		testCases := []struct {
			name           string
			compartmentA   string
			compartmentB   string
			selectorA      map[string]string
			selectorB      map[string]string
			expectedWinner string
		}{
			{
				name:           "zebra vs apple - apple wins",
				compartmentA:   "zebra-compartment",
				compartmentB:   "apple-compartment",
				selectorA:      map[string]string{"env": "prod"},
				selectorB:      map[string]string{"tier": "frontend"},
				expectedWinner: "apple-compartment",
			},
			{
				name:           "production vs development - development wins",
				compartmentA:   "production",
				compartmentB:   "development",
				selectorA:      map[string]string{"env": "prod"},
				selectorB:      map[string]string{"tier": "frontend"},
				expectedWinner: "development",
			},
			{
				name:           "comp-2 vs comp-1 - comp-1 wins",
				compartmentA:   "comp-2",
				compartmentB:   "comp-1",
				selectorA:      map[string]string{"env": "prod"},
				selectorB:      map[string]string{"tier": "frontend"},
				expectedWinner: "comp-1",
			},
		}

		for _, tc := range testCases {
			By(tc.name)

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{"env": "prod", "tier": "frontend", "zone": "us-west"},
				},
			}

			skyhook := &v1alpha1.Skyhook{ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"}}
			skyhookNode, err := NewSkyhookNode(node, skyhook)
			Expect(err).NotTo(HaveOccurred())

			compartmentA := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: tc.compartmentA,
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr.To(5),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: tc.selectorA,
				},
				Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}},
			})

			compartmentB := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: tc.compartmentB,
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr.To(5), // Same capacity
				},
				Selector: metav1.LabelSelector{
					MatchLabels: tc.selectorB,
				},
				Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}}, // Same strategy
			})

			compartments := map[string]*Compartment{
				tc.compartmentA: compartmentA,
				tc.compartmentB: compartmentB,
			}

			nodes := []SkyhookNode{skyhookNode}

			compartmentName, err := AssignNodeToCompartment(skyhookNode, compartments, nodes)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal(tc.expectedWinner), "lexicographic tie-break")

			// Run multiple times to ensure consistency
			for i := 0; i < 5; i++ {
				result, err := AssignNodeToCompartment(skyhookNode, compartments, nodes)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(compartmentName), "should be deterministic across multiple calls")
			}
		}
	})
})
