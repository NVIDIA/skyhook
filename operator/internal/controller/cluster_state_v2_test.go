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

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	skyhookNodesMock "github.com/NVIDIA/skyhook/operator/internal/controller/mock"
	"github.com/NVIDIA/skyhook/operator/internal/wrapper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("cluster state v2 tests", func() {

	It("should check taint toleration", func() {
		taints := []corev1.Taint{
			{
				Key:    "key1",
				Value:  "value1",
				Effect: corev1.TaintEffectNoSchedule,
			},
			{
				Key:    "key2",
				Value:  "value2",
				Effect: corev1.TaintEffectNoSchedule,
			},
		}

		tolerations := []corev1.Toleration{
			{
				Key:      "key1",
				Value:    "value1",
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpEqual,
			},
			{
				Key:      "key2",
				Value:    "value2",
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpEqual,
			},
		}

		Expect(CheckTaintToleration(tolerations, taints)).To(BeTrue())
	})

	It("Must tolerate all taints", func() {
		taints := []corev1.Taint{
			{
				Key:    "key1",
				Value:  "value1",
				Effect: corev1.TaintEffectNoSchedule,
			},
			{
				Key:    "key2",
				Value:  "value2",
				Effect: corev1.TaintEffectNoExecute,
			},
		}

		tolerations := []corev1.Toleration{
			{
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpExists,
			},
		}

		Expect(CheckTaintToleration(tolerations, taints)).To(BeFalse())
	})

	It("When no taints it is tolerated", func() {
		taints := make([]corev1.Taint, 0)

		tolerations := []corev1.Toleration{
			{
				Effect:   corev1.TaintEffectNoSchedule,
				Operator: corev1.TolerationOpExists,
			},
		}

		Expect(CheckTaintToleration(tolerations, taints)).To(BeTrue())
	})

	It("When no taints and no tolerations it is tolerated", func() {
		taints := make([]corev1.Taint, 0)

		tolerations := make([]corev1.Toleration, 0)

		Expect(CheckTaintToleration(tolerations, taints)).To(BeTrue())
	})
})

// --- Add GetNextSkyhook tests ---

var _ = Describe("GetNextSkyhook", func() {
	It("returns the first not-complete, not-disabled skyhook", func() {
		// Helper to make a skyhookNodes with given complete/disabled
		makeSkyhookNodes := func(complete bool, disabled bool) SkyhookNodes {
			sn_mock := skyhookNodesMock.MockSkyhookNodes{}
			sn_mock.EXPECT().IsComplete().Return(complete)
			sn_mock.EXPECT().IsDisabled().Return(disabled)
			return &sn_mock
		}

		// Not complete, not disabled
		n1 := makeSkyhookNodes(false, false)
		// Complete
		n2 := makeSkyhookNodes(true, false)
		// Disabled
		n3 := makeSkyhookNodes(false, true)

		// Should return n1
		result := GetNextSkyhook([]SkyhookNodes{n1, n2, n3})
		Expect(result).To(Equal(n1))

		// Should return nil as all complete or disabled
		n1 = makeSkyhookNodes(true, false)
		result = GetNextSkyhook([]SkyhookNodes{n1, n2, n3})
		Expect(result).To(BeNil())

		// Should return n3 as all others are complete or disabled
		n2 = makeSkyhookNodes(false, true)
		n3 = makeSkyhookNodes(false, false)
		result = GetNextSkyhook([]SkyhookNodes{n1, n2, n3})
		Expect(result).To(Equal(n3))
	})
})

var _ = Describe("BuildState ordering", func() {
	It("orders skyhooks by priority and name", func() {
		priorityKey := v1alpha1.METADATA_PREFIX + "/priority"
		skyhooks := &v1alpha1.SkyhookList{
			Items: []v1alpha1.Skyhook{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "b", Annotations: map[string]string{priorityKey: "2"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "a", Annotations: map[string]string{priorityKey: "1"}},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "c", Annotations: map[string]string{priorityKey: "2"}},
				},
			},
		}
		deploymentPolicies := &v1alpha1.DeploymentPolicyList{Items: []v1alpha1.DeploymentPolicy{}}
		nodes := &corev1.NodeList{Items: []corev1.Node{}}
		clusterState, err := BuildState(skyhooks, nodes, deploymentPolicies)
		Expect(err).ToNot(HaveOccurred())
		ordered := clusterState.skyhooks
		// Should be: a (priority 1), b (priority 2, name b), c (priority 2, name c)
		Expect(ordered[0].GetSkyhook().Name).To(Equal("a"))
		Expect(ordered[1].GetSkyhook().Name).To(Equal("b"))
		Expect(ordered[2].GetSkyhook().Name).To(Equal("c"))
	})
})

var _ = Describe("CleanupRemovedNodes", func() {
	It("should cleanup removed nodes from all status maps", func() {
		// Create mock skyhook nodes
		mockSkyhookNodes := skyhookNodesMock.MockSkyhookNodes{}
		// Create mock nodes that currently exist
		mockNode1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		}
		mockNode2 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		}

		// Create actual wrapper nodes using NewSkyhookNodeOnly
		node1, err := wrapper.NewSkyhookNode(
			mockNode1,
			&v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"},
			},
		)
		Expect(err).NotTo(HaveOccurred())
		node2, err := wrapper.NewSkyhookNode(
			mockNode2,
			&v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// Create mock skyhook wrapper with status maps containing both existing and removed nodes
		mockSkyhook := &wrapper.Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{
					NodeState: map[string]v1alpha1.NodeState{
						"node1":        {},
						"node2":        {},
						"removed-node": {},
					},
					NodeStatus: map[string]v1alpha1.Status{
						"node1":        v1alpha1.StatusComplete,
						"node2":        v1alpha1.StatusInProgress,
						"removed-node": v1alpha1.StatusErroring,
					},
					NodeBootIds: map[string]string{
						"node1":        "boot-id-1",
						"node2":        "boot-id-2",
						"removed-node": "boot-id-removed",
					},
					NodePriority: map[string]metav1.Time{
						"node1":        metav1.Now(),
						"node2":        metav1.Now(),
						"removed-node": metav1.Now(),
					},
					ConfigUpdates: map[string][]string{
						"package1": {"config1"},
						"package2": {"config2"},
						"package3": {"config3"},
					},
				},
			},
			Updated: false,
		}

		// Set up mock expectations
		mockSkyhookNodes.EXPECT().GetNodes().Return([]wrapper.SkyhookNode{
			node1,
			node2,
		})
		mockSkyhookNodes.EXPECT().GetSkyhook().Return(mockSkyhook)

		// Call the function
		CleanupRemovedNodes(&mockSkyhookNodes)

		// Verify that removed-node was cleaned up from all maps
		Expect(mockSkyhook.Status.NodeState).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodeState).To(HaveKey("node2"))
		Expect(mockSkyhook.Status.NodeState).NotTo(HaveKey("removed-node"))

		Expect(mockSkyhook.Status.NodeStatus).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodeStatus).To(HaveKey("node2"))
		Expect(mockSkyhook.Status.NodeStatus).NotTo(HaveKey("removed-node"))

		Expect(mockSkyhook.Status.NodeBootIds).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodeBootIds).To(HaveKey("node2"))
		Expect(mockSkyhook.Status.NodeBootIds).NotTo(HaveKey("removed-node"))

		Expect(mockSkyhook.Status.NodePriority).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodePriority).To(HaveKey("node2"))
		Expect(mockSkyhook.Status.NodePriority).NotTo(HaveKey("removed-node"))

		// ConfigUpdates should NOT be cleaned up by node removal since it's keyed by package names
		Expect(mockSkyhook.Status.ConfigUpdates).To(HaveKey("package1"))
		Expect(mockSkyhook.Status.ConfigUpdates).To(HaveKey("package2"))
		Expect(mockSkyhook.Status.ConfigUpdates).To(HaveKey("package3"))

		// Verify that Updated flag was set since changes were made
		Expect(mockSkyhook.Updated).To(BeTrue())
	})

	It("should not set Updated flag when no nodes are removed", func() {
		// Create mock skyhook nodes
		mockSkyhookNodes := skyhookNodesMock.MockSkyhookNodes{}
		// Create mock nodes that currently exist
		mockNode1 := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		}

		// Create actual wrapper nodes using NewSkyhookNodeOnly
		node1, err := wrapper.NewSkyhookNode(
			mockNode1,
			&v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"},
			},
		)
		Expect(err).NotTo(HaveOccurred())

		// Create mock skyhook wrapper with status maps containing both existing and removed nodes
		mockSkyhook := &wrapper.Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				Status: v1alpha1.SkyhookStatus{
					NodeState: map[string]v1alpha1.NodeState{
						"node1": {},
					},
					NodeStatus: map[string]v1alpha1.Status{
						"node1": v1alpha1.StatusComplete,
					},
					NodeBootIds: map[string]string{
						"node1": "boot-id-1",
					},
					NodePriority: map[string]metav1.Time{
						"node1": metav1.Now(),
					},
					ConfigUpdates: map[string][]string{
						"package1": {"config1"},
					},
				},
			},
			Updated: false,
		}

		// Set up mock expectations
		mockSkyhookNodes.EXPECT().GetNodes().Return([]wrapper.SkyhookNode{
			node1,
		})
		mockSkyhookNodes.EXPECT().GetSkyhook().Return(mockSkyhook)

		// Call the function
		CleanupRemovedNodes(&mockSkyhookNodes)

		// Verify that removed-node was cleaned up from all maps
		Expect(mockSkyhook.Status.NodeState).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodeStatus).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodeBootIds).To(HaveKey("node1"))
		Expect(mockSkyhook.Status.NodePriority).To(HaveKey("node1"))
		// ConfigUpdates should remain unchanged since it's keyed by package names, not node names
		Expect(mockSkyhook.Status.ConfigUpdates).To(HaveKey("package1"))

		// Verify that Updated flag was set since changes were made
		Expect(mockSkyhook.Updated).To(BeFalse())
	})

	Describe("isSkyhookControlledNodeStatus", func() {
		It("should return true for disabled status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusDisabled)
			Expect(result).To(BeTrue())
		})

		It("should return true for paused status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusPaused)
			Expect(result).To(BeTrue())
		})

		It("should return true for waiting status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusWaiting)
			Expect(result).To(BeTrue())
		})

		It("should return false for complete status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusComplete)
			Expect(result).To(BeFalse())
		})

		It("should return false for in_progress status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusInProgress)
			Expect(result).To(BeFalse())
		})

		It("should return false for erroring status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusErroring)
			Expect(result).To(BeFalse())
		})

		It("should return false for blocked status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusBlocked)
			Expect(result).To(BeFalse())
		})

		It("should return false for unknown status", func() {
			result := isSkyhookControlledNodeStatus(v1alpha1.StatusUnknown)
			Expect(result).To(BeFalse())
		})
	})

	Describe("UpdateSkyhookPauseStatus", func() {
		var mockSkyhookNodes *skyhookNodesMock.MockSkyhookNodes
		var mockSkyhook *wrapper.Skyhook
		var mockNode1 wrapper.SkyhookNode
		var mockNode2 wrapper.SkyhookNode

		BeforeEach(func() {
			mockSkyhookNodes = &skyhookNodesMock.MockSkyhookNodes{}

			// Create a real skyhook for testing
			mockSkyhook = &wrapper.Skyhook{
				Skyhook: &v1alpha1.Skyhook{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "test-skyhook",
						Annotations: map[string]string{},
					},
					Status: v1alpha1.SkyhookStatus{
						Status: v1alpha1.StatusInProgress,
					},
				},
			}

			// Create real nodes for testing
			node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
			node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}

			var err error
			mockNode1, err = wrapper.NewSkyhookNode(node1, mockSkyhook.Skyhook)
			Expect(err).NotTo(HaveOccurred())

			mockNode2, err = wrapper.NewSkyhookNode(node2, mockSkyhook.Skyhook)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update status to paused when skyhook is paused and status is not already paused", func() {
			// Set up the skyhook as paused
			mockSkyhook.Annotations[v1alpha1.METADATA_PREFIX+"/pause"] = "true"

			// Set up mock expectations
			mockSkyhookNodes.EXPECT().IsPaused().Return(true)
			mockSkyhookNodes.EXPECT().Status().Return(v1alpha1.StatusInProgress)
			mockSkyhookNodes.EXPECT().SetStatus(v1alpha1.StatusPaused).Once()
			mockSkyhookNodes.EXPECT().GetNodes().Return([]wrapper.SkyhookNode{mockNode1, mockNode2})

			// Call the function
			result := UpdateSkyhookPauseStatus(mockSkyhookNodes)

			// Verify the result
			Expect(result).To(BeTrue())
		})

		It("should not change status when skyhook is paused but status is already paused", func() {
			// Set up the skyhook as paused with paused status
			mockSkyhook.Annotations[v1alpha1.METADATA_PREFIX+"/pause"] = "true"

			// Set up mock expectations
			mockSkyhookNodes.EXPECT().IsPaused().Return(true)
			mockSkyhookNodes.EXPECT().Status().Return(v1alpha1.StatusPaused)

			// Call the function
			result := UpdateSkyhookPauseStatus(mockSkyhookNodes)

			// Verify the result
			Expect(result).To(BeFalse())
		})

		It("should not change status when skyhook is not paused", func() {
			// Set up the skyhook as not paused
			mockSkyhook.Annotations[v1alpha1.METADATA_PREFIX+"/pause"] = "false"

			// Set up mock expectations
			mockSkyhookNodes.EXPECT().IsPaused().Return(false)

			// Call the function
			result := UpdateSkyhookPauseStatus(mockSkyhookNodes)

			// Verify the result
			Expect(result).To(BeFalse())
		})

		It("should not change status when skyhook pause annotation is missing", func() {
			// Set up mock expectations no pause annotation means not paused
			mockSkyhookNodes.EXPECT().IsPaused().Return(false)

			// Call the function
			result := UpdateSkyhookPauseStatus(mockSkyhookNodes)

			// Verify the result
			Expect(result).To(BeFalse())
		})
	})

	Describe("AssignNodeToCompartment", func() {
		It("should assign node to compartment", func() {
			compartment := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			})

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{"test-label": "test-value"},
				},
			}

			skyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
			}

			skyhookNode, err := wrapper.NewSkyhookNode(node, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        []wrapper.SkyhookNode{skyhookNode},
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment", compartment)

			compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment"))
		})

		It("should assign node to default compartment", func() {
			compartment := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label": "test-value"},
				},
			})

			defaultCompartment := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: v1alpha1.DefaultCompartmentName,
			})

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node",
					Labels: map[string]string{"test-label": "test-value-other"},
				},
			}

			skyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
			}

			skyhookNode, err := wrapper.NewSkyhookNode(node, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        []wrapper.SkyhookNode{skyhookNode},
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment", compartment)
			skyhookNodes.AddCompartment(v1alpha1.DefaultCompartmentName, defaultCompartment)

			compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal(v1alpha1.DefaultCompartmentName))
		})

		It("should assign node to compartment with safer strategy", func() {
			compartment1 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-1",
				Strategy: &v1alpha1.DeploymentStrategy{
					Fixed: &v1alpha1.FixedStrategy{
						InitialBatch: ptr(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			})

			compartment2 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-2",
				Strategy: &v1alpha1.DeploymentStrategy{
					Linear: &v1alpha1.LinearStrategy{
						InitialBatch: ptr(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			})

			compartment3 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-3",
				Strategy: &v1alpha1.DeploymentStrategy{
					Exponential: &v1alpha1.ExponentialStrategy{
						InitialBatch: ptr(1),
					},
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-3": "test-value-3"},
				},
			})

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

			skyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
			}

			skyhookNodeFixed, err := wrapper.NewSkyhookNode(fixedNode, skyhook)
			Expect(err).NotTo(HaveOccurred())
			skyhookNodeLinear, err := wrapper.NewSkyhookNode(linearNode, skyhook)
			Expect(err).NotTo(HaveOccurred())
			skyhookNodeExponential, err := wrapper.NewSkyhookNode(exponentialNode, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        []wrapper.SkyhookNode{skyhookNodeFixed, skyhookNodeLinear, skyhookNodeExponential},
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment-1", compartment1)
			skyhookNodes.AddCompartment("test-compartment-2", compartment2)
			skyhookNodes.AddCompartment("test-compartment-3", compartment3)

			compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNodeFixed)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment-1"))
			compartmentName, err = skyhookNodes.AssignNodeToCompartment(skyhookNodeLinear)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment-2"))
			compartmentName, err = skyhookNodes.AssignNodeToCompartment(skyhookNodeExponential)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment-3"))
		})

		It("should assign node to compartment with smaller count budget", func() {
			compartment1 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-1",
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr(1),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			})

			compartment2 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-2",
				Budget: v1alpha1.DeploymentBudget{
					Count: ptr(2),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			})

			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node-1",
					Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2"},
				},
			}

			skyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook-1",
				},
			}

			skyhookNode1, err := wrapper.NewSkyhookNode(node1, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        []wrapper.SkyhookNode{skyhookNode1},
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment-1", compartment1)
			skyhookNodes.AddCompartment("test-compartment-2", compartment2)

			compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNode1)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment-1"))
		})

		It("should assign node to compartment with smaller percent budget", func() {
			compartment1 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-1",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr(10),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
			})

			compartment2 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-2",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr(20),
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-2": "test-value-2"},
				},
			})

			node1 := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-node-1",
					Labels: map[string]string{"test-label-1": "test-value-1", "test-label-2": "test-value-2"},
				},
			}

			skyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook-1",
				},
			}

			skyhookNode1, err := wrapper.NewSkyhookNode(node1, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        []wrapper.SkyhookNode{skyhookNode1},
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment-1", compartment1)
			skyhookNodes.AddCompartment("test-compartment-2", compartment2)

			compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNode1)
			Expect(err).NotTo(HaveOccurred())
			Expect(compartmentName).To(Equal("test-compartment-1"))
		})

		It("should assign to compartment with higher percent but smaller effective capacity due to fewer matching nodes", func() {
			// Test that a compartment with a higher percent but fewer matching nodes
			// can have a smaller effective capacity and win the assignment

			// Compartment A: 50% budget, matches 10 nodes total
			compartmentA := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-1",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr(50), // 50% of 10 = 5 capacity
				},
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"test-label-1": "test-value-1"},
				},
				Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}},
			})

			// Compartment B: 80% budget, matches only 2 nodes total
			compartmentB := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "test-compartment-2",
				Budget: v1alpha1.DeploymentBudget{
					Percent: ptr(80), // 80% of 2 = floor(1.6) = 1 capacity
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
			var allNodes []wrapper.SkyhookNode
			for _, n := range allNodesList {
				sn, err := wrapper.NewSkyhookNode(n, skyhook)
				Expect(err).NotTo(HaveOccurred())
				allNodes = append(allNodes, sn)
			}

			targetSkyhookNode, err := wrapper.NewSkyhookNode(targetNode, skyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook:      wrapper.NewSkyhookWrapper(skyhook),
				nodes:        allNodes,
				compartments: make(map[string]*wrapper.Compartment),
			}
			skyhookNodes.AddCompartment("test-compartment-1", compartmentA)
			skyhookNodes.AddCompartment("test-compartment-2", compartmentB)

			// Should assign to "test-compartment-2" because:
			// - test-compartment-1: 50% × 10 nodes = 5 capacity
			// - test-compartment-2: 80% × 2 nodes = 1 capacity (smaller wins)
			compartmentName, err := skyhookNodes.AssignNodeToCompartment(targetSkyhookNode)
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
				skyhookNode, err := wrapper.NewSkyhookNode(node, skyhook)
				Expect(err).NotTo(HaveOccurred())

				compartmentA := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
					Name: tc.compartmentA,
					Budget: v1alpha1.DeploymentBudget{
						Count: ptr(5),
					},
					Selector: metav1.LabelSelector{
						MatchLabels: tc.selectorA,
					},
					Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}},
				})

				compartmentB := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
					Name: tc.compartmentB,
					Budget: v1alpha1.DeploymentBudget{
						Count: ptr(5), // Same capacity
					},
					Selector: metav1.LabelSelector{
						MatchLabels: tc.selectorB,
					},
					Strategy: &v1alpha1.DeploymentStrategy{Fixed: &v1alpha1.FixedStrategy{}}, // Same strategy
				})

				skyhookNodes := &skyhookNodes{
					skyhook:      wrapper.NewSkyhookWrapper(skyhook),
					nodes:        []wrapper.SkyhookNode{skyhookNode},
					compartments: make(map[string]*wrapper.Compartment),
				}
				skyhookNodes.AddCompartment(tc.compartmentA, compartmentA)
				skyhookNodes.AddCompartment(tc.compartmentB, compartmentB)

				compartmentName, err := skyhookNodes.AssignNodeToCompartment(skyhookNode)
				Expect(err).NotTo(HaveOccurred())
				Expect(compartmentName).To(Equal(tc.expectedWinner), "lexicographic tie-break")

				// Run multiple times to ensure consistency
				for i := 0; i < 5; i++ {
					result, err := skyhookNodes.AssignNodeToCompartment(skyhookNode)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(compartmentName), "should be deterministic across multiple calls")
				}
			}
		})
	})
})
