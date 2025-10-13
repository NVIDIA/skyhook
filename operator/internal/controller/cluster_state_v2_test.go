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
	kptr "k8s.io/utils/ptr"
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

	Describe("PersistCompartmentBatchStates", func() {
		var skyhook *wrapper.Skyhook
		var sn *skyhookNodes

		BeforeEach(func() {
			skyhook = &wrapper.Skyhook{
				Skyhook: &v1alpha1.Skyhook{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-skyhook",
					},
					Status: v1alpha1.SkyhookStatus{},
				},
			}

			sn = &skyhookNodes{
				skyhook:      skyhook,
				nodes:        []wrapper.SkyhookNode{},
				compartments: make(map[string]*wrapper.Compartment),
			}
		})

		It("should return false when there are no compartments", func() {
			result := sn.PersistCompartmentBatchStates()
			Expect(result).To(BeFalse())
			Expect(skyhook.Updated).To(BeFalse())
		})

		It("should persist batch state when compartment has CurrentBatch > 0", func() {
			// Create a compartment with batch state
			batchState := &v1alpha1.BatchProcessingState{
				CurrentBatch:   1,
				CompletedNodes: 4,
				FailedNodes:    1,
			}
			compartment := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "compartment1",
				Budget: v1alpha1.DeploymentBudget{
					Count: kptr.To(10),
				},
				Strategy: &v1alpha1.DeploymentStrategy{
					Fixed: &v1alpha1.FixedStrategy{InitialBatch: kptr.To(5)},
				},
			}, batchState)

			sn.AddCompartment("compartment1", compartment)

			result := sn.PersistCompartmentBatchStates()

			Expect(result).To(BeTrue())
			Expect(skyhook.Updated).To(BeTrue())
			Expect(skyhook.Status.CompartmentBatchStates).ToNot(BeNil())
			Expect(skyhook.Status.CompartmentBatchStates).To(HaveKey("compartment1"))
			Expect(skyhook.Status.CompartmentBatchStates["compartment1"].CurrentBatch).To(Equal(1))
			Expect(skyhook.Status.CompartmentBatchStates["compartment1"].CompletedNodes).To(Equal(4))
		})

		It("should persist batch state when compartment has nodes", func() {
			// Create a compartment with nodes but no batch started yet
			compartment := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "compartment1",
				Budget: v1alpha1.DeploymentBudget{
					Count: kptr.To(10),
				},
				Strategy: &v1alpha1.DeploymentStrategy{
					Fixed: &v1alpha1.FixedStrategy{InitialBatch: kptr.To(5)},
				},
			}, nil)

			// Add a node to the compartment
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
			skyhookNode, err := wrapper.NewSkyhookNode(node, skyhook.Skyhook)
			Expect(err).NotTo(HaveOccurred())
			compartment.AddNode(skyhookNode)

			sn.AddCompartment("compartment1", compartment)

			result := sn.PersistCompartmentBatchStates()

			Expect(result).To(BeTrue())
			Expect(skyhook.Updated).To(BeTrue())
			Expect(skyhook.Status.CompartmentBatchStates).ToNot(BeNil())
			Expect(skyhook.Status.CompartmentBatchStates).To(HaveKey("compartment1"))
		})

		It("should persist multiple compartments with meaningful state", func() {
			// Create multiple compartments
			batchState1 := &v1alpha1.BatchProcessingState{
				CurrentBatch:   1,
				CompletedNodes: 5,
				FailedNodes:    0,
			}
			compartment1 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "compartment1",
				Budget: v1alpha1.DeploymentBudget{
					Count: kptr.To(10),
				},
				Strategy: &v1alpha1.DeploymentStrategy{
					Fixed: &v1alpha1.FixedStrategy{InitialBatch: kptr.To(5)},
				},
			}, batchState1)

			batchState2 := &v1alpha1.BatchProcessingState{
				CurrentBatch:   2,
				CompletedNodes: 8,
				FailedNodes:    2,
			}
			compartment2 := wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
				Name: "compartment2",
				Budget: v1alpha1.DeploymentBudget{
					Count: kptr.To(5),
				},
				Strategy: &v1alpha1.DeploymentStrategy{
					Linear: &v1alpha1.LinearStrategy{},
				},
			}, batchState2)

			sn.AddCompartment("compartment1", compartment1)
			sn.AddCompartment("compartment2", compartment2)

			result := sn.PersistCompartmentBatchStates()

			Expect(result).To(BeTrue())
			Expect(skyhook.Updated).To(BeTrue())
			Expect(skyhook.Status.CompartmentBatchStates).ToNot(BeNil())
			Expect(skyhook.Status.CompartmentBatchStates).To(HaveLen(2))
			Expect(skyhook.Status.CompartmentBatchStates["compartment1"].CurrentBatch).To(Equal(1))
			Expect(skyhook.Status.CompartmentBatchStates["compartment2"].CurrentBatch).To(Equal(2))
		})
	})

	Describe("IntrospectSkyhook", func() {
		var testSkyhook *v1alpha1.Skyhook
		var testNode *corev1.Node

		BeforeEach(func() {
			testSkyhook = &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-skyhook",
					Annotations: map[string]string{},
				},
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"test-package": {
							PackageRef: v1alpha1.PackageRef{Name: "test-package", Version: "1.0.0"},
							Image:      "test-image",
						},
					},
				},
				Status: v1alpha1.SkyhookStatus{
					Status: v1alpha1.StatusInProgress,
				},
			}

			testNode = &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
			}
		})

		It("should set status to disabled when skyhook is disabled", func() {
			// Set up the skyhook as disabled
			testSkyhook.Annotations["skyhook.nvidia.com/disable"] = "true"

			skyhookNode, err := wrapper.NewSkyhookNode(testNode, testSkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(testSkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode},
			}

			// Call the function
			changed := IntrospectSkyhook(skyhookNodes, []SkyhookNodes{skyhookNodes})

			// Verify the result
			Expect(changed).To(BeTrue())
			Expect(skyhookNodes.Status()).To(Equal(v1alpha1.StatusDisabled))
		})

		It("should set status to paused when skyhook is paused", func() {
			// Set up the skyhook as paused
			testSkyhook.Annotations["skyhook.nvidia.com/pause"] = "true"

			skyhookNode, err := wrapper.NewSkyhookNode(testNode, testSkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(testSkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode},
			}

			// Call the function
			changed := IntrospectSkyhook(skyhookNodes, []SkyhookNodes{skyhookNodes})

			// Verify the result
			Expect(changed).To(BeTrue())
			Expect(skyhookNodes.Status()).To(Equal(v1alpha1.StatusPaused))
		})

		It("should set status to waiting when another skyhook has higher priority", func() {
			// Create higher priority skyhook (priority 1)
			higherPrioritySkyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "skyhook-1"},
				Spec: v1alpha1.SkyhookSpec{
					Priority: 1,
					Packages: map[string]v1alpha1.Package{
						"test-package-1": {
							PackageRef: v1alpha1.PackageRef{Name: "test-package-1", Version: "1.0.0"},
							Image:      "test-image-1",
						},
					},
				},
				Status: v1alpha1.SkyhookStatus{Status: v1alpha1.StatusInProgress},
			}

			// Create lower priority skyhook (priority 2)
			lowerPrioritySkyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "skyhook-2"},
				Spec: v1alpha1.SkyhookSpec{
					Priority: 2,
					Packages: map[string]v1alpha1.Package{
						"test-package-2": {
							PackageRef: v1alpha1.PackageRef{Name: "test-package-2", Version: "1.0.0"},
							Image:      "test-image-2",
						},
					},
				},
				Status: v1alpha1.SkyhookStatus{Status: v1alpha1.StatusInProgress},
			}

			node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
			node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}}

			skyhookNode1, err := wrapper.NewSkyhookNode(node1, higherPrioritySkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNode2, err := wrapper.NewSkyhookNode(node2, lowerPrioritySkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes1 := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(higherPrioritySkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode1},
			}

			skyhookNodes2 := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(lowerPrioritySkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode2},
			}

			allSkyhooks := []SkyhookNodes{skyhookNodes1, skyhookNodes2}

			// Call the function - skyhook2 should be waiting because skyhook1 has higher priority
			changed := IntrospectSkyhook(skyhookNodes2, allSkyhooks)

			// Verify the result
			Expect(changed).To(BeTrue())
			Expect(skyhookNodes2.Status()).To(Equal(v1alpha1.StatusWaiting))
		})

		It("should not change status when skyhook is complete", func() {
			// Create a complete skyhook with no packages
			completeSkyhook := &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test-skyhook"},
				Status:     v1alpha1.SkyhookStatus{Status: v1alpha1.StatusComplete},
			}

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			}

			skyhookNode, err := wrapper.NewSkyhookNode(node, completeSkyhook)
			Expect(err).NotTo(HaveOccurred())
			skyhookNode.SetStatus(v1alpha1.StatusComplete)

			skyhookNodes := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(completeSkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode},
			}

			// Call the function
			_ = IntrospectSkyhook(skyhookNodes, []SkyhookNodes{skyhookNodes})

			// Verify the result - status should stay complete
			Expect(skyhookNodes.Status()).To(Equal(v1alpha1.StatusComplete))
		})

		It("should return true when node status changes", func() {
			skyhookNode, err := wrapper.NewSkyhookNode(testNode, testSkyhook)
			Expect(err).NotTo(HaveOccurred())
			skyhookNode.SetStatus(v1alpha1.StatusUnknown)

			skyhookNodes := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(testSkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode},
			}

			// Call the function
			changed := IntrospectSkyhook(skyhookNodes, []SkyhookNodes{skyhookNodes})

			// Verify the result
			Expect(changed).To(BeTrue())
		})

		It("should handle multiple nodes correctly when disabled", func() {
			// Set up the skyhook as disabled
			testSkyhook.Annotations["skyhook.nvidia.com/disable"] = "true"

			node1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
			node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}}

			skyhookNode1, err := wrapper.NewSkyhookNode(node1, testSkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNode2, err := wrapper.NewSkyhookNode(node2, testSkyhook)
			Expect(err).NotTo(HaveOccurred())

			skyhookNodes := &skyhookNodes{
				skyhook: wrapper.NewSkyhookWrapper(testSkyhook),
				nodes:   []wrapper.SkyhookNode{skyhookNode1, skyhookNode2},
			}

			// Call the function
			changed := IntrospectSkyhook(skyhookNodes, []SkyhookNodes{skyhookNodes})

			// Verify the result
			Expect(changed).To(BeTrue())
			Expect(skyhookNodes.Status()).To(Equal(v1alpha1.StatusDisabled))
			Expect(skyhookNode1.Status()).To(Equal(v1alpha1.StatusDisabled))
			Expect(skyhookNode2.Status()).To(Equal(v1alpha1.StatusDisabled))
		})
	})
})
