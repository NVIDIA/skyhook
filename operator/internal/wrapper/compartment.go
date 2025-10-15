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
)

func NewCompartmentWrapper(c *v1alpha1.Compartment, batchState *v1alpha1.BatchProcessingState) *Compartment {
	comp := &Compartment{
		Compartment: *c,
	}

	if batchState != nil {
		comp.BatchState = *batchState
	} else {
		comp.BatchState = v1alpha1.BatchProcessingState{
			CurrentBatch: 1,
		}
	}

	return comp
}

type Compartment struct {
	v1alpha1.Compartment
	Nodes []SkyhookNode
	// BatchState tracks the persistent batch processing state
	BatchState v1alpha1.BatchProcessingState
}

func (c *Compartment) GetName() string {
	return c.Name
}

func (c *Compartment) GetNodes() []SkyhookNode {
	return c.Nodes
}

func (c *Compartment) GetNode(name string) SkyhookNode {
	for _, node := range c.Nodes {
		if node.GetNode().Name == name {
			return node
		}
	}
	return nil
}

func (c *Compartment) AddNode(node SkyhookNode) {
	c.Nodes = append(c.Nodes, node)
}

func (c *Compartment) calculateCeiling() int {
	if c.Budget.Count != nil {
		return *c.Budget.Count
	}
	if c.Budget.Percent != nil {
		matched := len(c.Nodes)
		if matched == 0 {
			return 0
		}
		limit := float64(*c.Budget.Percent) / 100
		return max(1, int(float64(matched)*limit))
	}
	return 0
}

func (c *Compartment) getInProgressCount() int {
	inProgress := 0
	for _, node := range c.Nodes {
		if node.Status() == v1alpha1.StatusInProgress {
			inProgress++
		}
	}
	return inProgress
}

func (c *Compartment) GetNodesForNextBatch() []SkyhookNode {
	if c.Strategy != nil && c.BatchState.ShouldStop {
		return nil
	}

	// If there's a batch in progress (nodes are InProgress), don't start a new one
	if c.getInProgressCount() > 0 {
		return c.getInProgressNodes()
	}

	// No batch in progress, create a new one
	return c.createNewBatch()
}

func (c *Compartment) getInProgressNodes() []SkyhookNode {
	inProgressNodes := make([]SkyhookNode, 0)
	for _, node := range c.Nodes {
		if node.Status() == v1alpha1.StatusInProgress {
			inProgressNodes = append(inProgressNodes, node)
		}
	}
	return inProgressNodes
}

func (c *Compartment) createNewBatch() []SkyhookNode {
	var batchSize int
	if c.Strategy != nil {
		batchSize = c.Strategy.CalculateBatchSize(len(c.Nodes), &c.BatchState)
	} else {
		ceiling := c.calculateCeiling()
		availableCapacity := ceiling - c.getInProgressCount()
		batchSize = max(0, availableCapacity)
	}

	if batchSize <= 0 {
		return nil
	}

	selectedNodes := make([]SkyhookNode, 0)
	priority := []v1alpha1.Status{v1alpha1.StatusInProgress, v1alpha1.StatusUnknown, v1alpha1.StatusBlocked, v1alpha1.StatusErroring}

	for _, status := range priority {
		for _, node := range c.Nodes {
			if len(selectedNodes) >= batchSize {
				break
			}
			if node.Status() != status {
				continue
			}
			if !node.IsComplete() {
				selectedNodes = append(selectedNodes, node)
			}
		}
		if len(selectedNodes) >= batchSize {
			break
		}
	}

	return selectedNodes
}

// IsBatchComplete checks if the current batch has reached terminal states
// A batch is complete when there are no nodes in InProgress status
func (c *Compartment) IsBatchComplete() bool {
	return c.getInProgressCount() == 0
}

// EvaluateCurrentBatch evaluates the current batch result if it's complete
// Uses delta-based tracking: compares current state to last checkpoint
func (c *Compartment) EvaluateCurrentBatch() (bool, int, int) {
	if !c.IsBatchComplete() {
		return false, 0, 0 // Batch not complete yet
	}

	// If this is the first batch (nothing has been processed yet), skip evaluation
	// The batch will be started in the next reconcile
	if c.BatchState.CurrentBatch == 0 {
		c.BatchState.CurrentBatch = 1
		return false, 0, 0
	}

	// Count current state in the compartment
	currentCompleted := 0
	currentFailed := 0
	for _, node := range c.Nodes {
		if node.IsComplete() {
			currentCompleted++
		} else if node.Status() == v1alpha1.StatusErroring {
			currentFailed++
		}
	}

	// Calculate delta from last checkpoint
	deltaCompleted := currentCompleted - c.BatchState.CompletedNodes
	deltaFailed := currentFailed - c.BatchState.FailedNodes

	// Only evaluate if there's actually a change (batch was processed)
	if deltaCompleted == 0 && deltaFailed == 0 {
		return false, 0, 0
	}

	// Update checkpoints
	c.BatchState.CompletedNodes = currentCompleted
	c.BatchState.FailedNodes = currentFailed

	return true, deltaCompleted, deltaFailed
}

// EvaluateAndUpdateBatchState evaluates a completed batch and updates the persistent state
func (c *Compartment) EvaluateAndUpdateBatchState(batchSize int, successCount int, failureCount int) {
	if c.Strategy != nil {
		// Use strategy-specific evaluation
		c.Strategy.EvaluateBatchResult(&c.BatchState, batchSize, successCount, failureCount, len(c.Nodes))
	} else {
		// No strategy: just update basic counters
		c.BatchState.CurrentBatch++
		c.BatchState.LastBatchSize = batchSize
	}
}

// GetBatchState returns the current batch processing state
func (c *Compartment) GetBatchState() v1alpha1.BatchProcessingState {
	return c.BatchState
}

// strategySafetyOrder defines the safety ordering of strategies
// Lower values indicate safer strategies (less aggressive rollout)
// Strategy safety order: Fixed (0) > Linear (1) > Exponential (2)
var strategySafetyOrder = map[v1alpha1.StrategyType]int{
	v1alpha1.StrategyTypeFixed:       0,
	v1alpha1.StrategyTypeLinear:      1,
	v1alpha1.StrategyTypeExponential: 2,
	v1alpha1.StrategyTypeUnknown:     999, // Unknown is least safe
}

// GetStrategyType returns the strategy type for a compartment
func GetStrategyType(strategy *v1alpha1.DeploymentStrategy) v1alpha1.StrategyType {
	if strategy == nil {
		return v1alpha1.StrategyTypeUnknown
	}
	if strategy.Fixed != nil {
		return v1alpha1.StrategyTypeFixed
	}
	if strategy.Linear != nil {
		return v1alpha1.StrategyTypeLinear
	}
	if strategy.Exponential != nil {
		return v1alpha1.StrategyTypeExponential
	}
	return v1alpha1.StrategyTypeUnknown
}

// StrategyIsSafer returns true if strategy a is safer than strategy b
// Strategy safety order: Fixed > Linear > Exponential
func StrategyIsSafer(a, b v1alpha1.StrategyType) bool {
	return strategySafetyOrder[a] < strategySafetyOrder[b]
}

// ComputeEffectiveCapacity calculates the effective ceiling for a compartment's budget
// given the number of matched nodes
func ComputeEffectiveCapacity(budget v1alpha1.DeploymentBudget, matchedNodes int) int {
	if budget.Count != nil {
		return *budget.Count
	}
	if budget.Percent != nil {
		// capacity = max(1, floor(percent/100 × matched))
		// Use floor for safer rollouts - never exceed the intended percentage
		capacity := float64(*budget.Percent) / 100.0 * float64(matchedNodes)
		return max(1, int(capacity))
	}
	// Should not happen due to validation
	return 0
}
