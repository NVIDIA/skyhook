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
	"fmt"
	"math"
	"sort"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// getStrategyType returns the strategy type for a compartment
func getStrategyType(strategy *v1alpha1.DeploymentStrategy) v1alpha1.StrategyType {
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

// strategyIsSafer returns true if strategy a is safer than strategy b
// Strategy safety order: Fixed > Linear > Exponential
func strategyIsSafer(a, b v1alpha1.StrategyType) bool {
	return strategySafetyOrder[a] < strategySafetyOrder[b]
}

// computeEffectiveCapacity calculates the effective ceiling for a compartment's budget
// given the number of matched nodes
func computeEffectiveCapacity(budget v1alpha1.DeploymentBudget, matchedNodes int) int {
	if budget.Count != nil {
		return *budget.Count
	}
	if budget.Percent != nil {
		// capacity = max(1, ceil(percent/100 × matched))
		capacity := float64(*budget.Percent) / 100.0 * float64(matchedNodes)
		return max(1, int(math.Ceil(capacity)))
	}
	// Should not happen due to validation
	return 0
}

// compartmentMatch represents a compartment that matches a node
type compartmentMatch struct {
	name         string
	strategyType v1alpha1.StrategyType
	capacity     int
}

// countMatchingNodes counts how many nodes from allNodes match the given selector
func countMatchingNodes(allNodes []SkyhookNode, selector metav1.LabelSelector) (int, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, node := range allNodes {
		if labelSelector.Matches(labels.Set(node.GetNode().Labels)) {
			count++
		}
	}
	return count, nil
}

// AssignNodeToCompartment assigns a single node to the appropriate compartment using overlap resolution.
// When a node matches multiple compartments, it resolves using:
// 1. Strategy safety order: Fixed is safer than Linear, which is safer than Exponential
// 2. Tie-break on same strategy: Choose compartment with smaller effective ceiling (window)
// 3. Final tie-break: Lexicographically by compartment name for determinism
// The allNodes parameter is used to compute effective capacity for percent-based budgets.
// Assignments are recalculated fresh on every reconcile based on current cluster state.
func AssignNodeToCompartment(node SkyhookNode, compartments map[string]*Compartment, allNodes []SkyhookNode) (string, error) {
	nodeLabels := labels.Set(node.GetNode().Labels)

	matches := []compartmentMatch{}

	// Collect all matching compartments (excluding default)
	for _, compartment := range compartments {
		// Skip the default compartment - it's a fallback
		if compartment.Name == v1alpha1.DefaultCompartmentName {
			continue
		}

		selector, err := metav1.LabelSelectorAsSelector(&compartment.Selector)
		if err != nil {
			return "", fmt.Errorf("invalid selector for compartment %s: %w", compartment.Name, err)
		}

		if selector.Matches(nodeLabels) {
			// Count how many nodes in total match this compartment's selector
			matchedCount, err := countMatchingNodes(allNodes, compartment.Selector)
			if err != nil {
				return "", fmt.Errorf("error counting matching nodes for compartment %s: %w", compartment.Name, err)
			}

			// Ensure at least 1 node for capacity calculation
			if matchedCount == 0 {
				matchedCount = 1
			}

			stratType := getStrategyType(compartment.Strategy)
			capacity := computeEffectiveCapacity(compartment.Budget, matchedCount)

			matches = append(matches, compartmentMatch{
				name:         compartment.Name,
				strategyType: stratType,
				capacity:     capacity,
			})
		}
	}

	// No matches - assign to default
	if len(matches) == 0 {
		return v1alpha1.DefaultCompartmentName, nil
	}

	// Single match - return it
	if len(matches) == 1 {
		return matches[0].name, nil
	}

	// Multiple matches - apply overlap resolution
	// Sort matches using the safety heuristic
	sort.Slice(matches, func(i, j int) bool {
		// 1. Strategy safety order: Fixed > Linear > Exponential
		if matches[i].strategyType != matches[j].strategyType {
			return strategyIsSafer(matches[i].strategyType, matches[j].strategyType)
		}

		// 2. Tie-break on same strategy: smaller window (capacity)
		if matches[i].capacity != matches[j].capacity {
			return matches[i].capacity < matches[j].capacity
		}

		// 3. Final tie-break: lexicographically by name for determinism
		return matches[i].name < matches[j].name
	})

	// Return the safest compartment
	return matches[0].name, nil
}
