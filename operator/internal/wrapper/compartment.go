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

func NewCompartmentWrapper(c *v1alpha1.Compartment) *Compartment {
	return &Compartment{
		Compartment: *c,
	}
}

type Compartment struct {
	v1alpha1.Compartment
	Nodes []SkyhookNode
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
