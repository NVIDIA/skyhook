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
	"k8s.io/utils/ptr"
)

var _ = Describe("Compartment", func() {
	Context("calculateCeiling", func() {
		It("should calculate ceiling for count budget", func() {
			compartment := &Compartment{
				Compartment: v1alpha1.Compartment{
					Budget: v1alpha1.DeploymentBudget{Count: ptr.To(3)},
				},
			}

			// Add 10 mock nodes (just need count for ceiling calculation)
			for i := 0; i < 10; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			ceiling := compartment.calculateCeiling()
			Expect(ceiling).To(Equal(3))
		})

		It("should calculate ceiling for percent budget", func() {
			compartment := &Compartment{
				Compartment: v1alpha1.Compartment{
					Budget: v1alpha1.DeploymentBudget{Percent: ptr.To(30)},
				},
			}

			// Add 10 mock nodes - 30% should be 3
			for i := 0; i < 10; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			ceiling := compartment.calculateCeiling()
			Expect(ceiling).To(Equal(3)) // max(1, int(10 * 0.3)) = 3
		})

		It("should handle small percent budgets with minimum 1", func() {
			compartment := &Compartment{
				Compartment: v1alpha1.Compartment{
					Budget: v1alpha1.DeploymentBudget{Percent: ptr.To(30)},
				},
			}

			// Add 2 mock nodes - 30% of 2 = 0.6, should round to 1
			for i := 0; i < 2; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			ceiling := compartment.calculateCeiling()
			Expect(ceiling).To(Equal(1)) // max(1, int(2 * 0.3)) = max(1, 0) = 1
		})

		It("should return 0 for no nodes", func() {
			compartment := &Compartment{
				Compartment: v1alpha1.Compartment{
					Budget: v1alpha1.DeploymentBudget{Percent: ptr.To(50)},
				},
			}

			ceiling := compartment.calculateCeiling()
			Expect(ceiling).To(Equal(0))
		})
	})

	Context("NewCompartmentWrapperWithState", func() {
		It("should create compartment with provided batch state", func() {
			batchState := &v1alpha1.BatchProcessingState{
				CurrentBatch:        3,
				ConsecutiveFailures: 1,
				ProcessedNodes:      5,
			}

			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:   "test",
				Budget: v1alpha1.DeploymentBudget{Count: ptr.To(5)},
			}, batchState)

			state := compartment.GetBatchState()
			Expect(state.CurrentBatch).To(Equal(3))
			Expect(state.ConsecutiveFailures).To(Equal(1))
			Expect(state.ProcessedNodes).To(Equal(5))
		})

		It("should create compartment with default batch state when nil", func() {
			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:   "test",
				Budget: v1alpha1.DeploymentBudget{Count: ptr.To(5)},
			}, nil)

			state := compartment.GetBatchState()
			Expect(state.CurrentBatch).To(Equal(1))
			Expect(state.ConsecutiveFailures).To(Equal(0))
			Expect(state.ProcessedNodes).To(Equal(0))
		})
	})

	Context("EvaluateAndUpdateBatchState", func() {
		It("should update basic state without strategy", func() {
			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:   "test-compartment",
				Budget: v1alpha1.DeploymentBudget{Count: ptr.To(10)},
			}, &v1alpha1.BatchProcessingState{
				CurrentBatch:   1,
				ProcessedNodes: 0,
			})

			compartment.EvaluateAndUpdateBatchState(3, 2, 1)

			state := compartment.GetBatchState()
			Expect(state.ProcessedNodes).To(Equal(3))
			Expect(state.CurrentBatch).To(Equal(2))
			Expect(state.SuccessfulInBatch).To(Equal(2))
			Expect(state.FailedInBatch).To(Equal(1))
		})

		It("should reset consecutive failures on successful batch", func() {
			strategy := &v1alpha1.DeploymentStrategy{
				Fixed: &v1alpha1.FixedStrategy{
					InitialBatch:     ptr.To(3),
					BatchThreshold:   ptr.To(80),
					FailureThreshold: ptr.To(2),
					SafetyLimit:      ptr.To(50),
				},
			}

			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:     "test-compartment",
				Budget:   v1alpha1.DeploymentBudget{Count: ptr.To(10)},
				Strategy: strategy,
			}, &v1alpha1.BatchProcessingState{
				CurrentBatch:        1,
				ProcessedNodes:      0,
				ConsecutiveFailures: 1, // Should reset on success
			})

			// Add 10 mock nodes for totalNodes calculation
			for i := 0; i < 10; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			// 80% success (4 out of 5)
			compartment.EvaluateAndUpdateBatchState(5, 4, 1)

			state := compartment.GetBatchState()
			Expect(state.ConsecutiveFailures).To(Equal(0)) // Should reset
			Expect(state.ShouldStop).To(BeFalse())
		})

		It("should increment consecutive failures and trigger stop when below safety limit", func() {
			strategy := &v1alpha1.DeploymentStrategy{
				Fixed: &v1alpha1.FixedStrategy{
					InitialBatch:     ptr.To(3),
					BatchThreshold:   ptr.To(80),
					FailureThreshold: ptr.To(2),
					SafetyLimit:      ptr.To(50),
				},
			}

			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:     "test-compartment",
				Budget:   v1alpha1.DeploymentBudget{Count: ptr.To(10)},
				Strategy: strategy,
			}, &v1alpha1.BatchProcessingState{
				CurrentBatch:        2,
				ProcessedNodes:      1, // After adding 3 more: (1+3)/10 = 40% (below 50% safety limit)
				ConsecutiveFailures: 1, // Will increment to 2 (threshold)
			})

			// Add 10 mock nodes for totalNodes calculation
			for i := 0; i < 10; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			// 33% success (1 out of 3) - below 80% threshold, progress will be (1+3)/10 = 40% (below safety limit)
			compartment.EvaluateAndUpdateBatchState(3, 1, 2)

			state := compartment.GetBatchState()
			Expect(state.ConsecutiveFailures).To(Equal(2)) // Should increment
			Expect(state.ShouldStop).To(BeTrue())          // Should trigger stop (below safety limit)
		})

		It("should not trigger stop when above safety limit", func() {
			strategy := &v1alpha1.DeploymentStrategy{
				Fixed: &v1alpha1.FixedStrategy{
					InitialBatch:     ptr.To(3),
					BatchThreshold:   ptr.To(80),
					FailureThreshold: ptr.To(2),
					SafetyLimit:      ptr.To(50),
				},
			}

			compartment := NewCompartmentWrapper(&v1alpha1.Compartment{
				Name:     "test-compartment",
				Budget:   v1alpha1.DeploymentBudget{Count: ptr.To(10)},
				Strategy: strategy,
			}, &v1alpha1.BatchProcessingState{
				CurrentBatch:        3,
				ProcessedNodes:      6, // 60% progress (above 50% safety limit)
				ConsecutiveFailures: 1,
			})

			// Add 10 mock nodes for totalNodes calculation
			for i := 0; i < 10; i++ {
				compartment.Nodes = append(compartment.Nodes, nil)
			}

			// 40% success (2 out of 5) - below 80% threshold, but above safety limit
			compartment.EvaluateAndUpdateBatchState(5, 2, 3)

			state := compartment.GetBatchState()
			Expect(state.ConsecutiveFailures).To(Equal(2)) // Should increment
			Expect(state.ShouldStop).To(BeFalse())         // Should NOT stop (above safety limit)
		})
	})
})
