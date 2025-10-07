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

/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package v1alpha1

import (
	"fmt"
	"math"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// Strategy parameters
type FixedStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch *int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold *int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold *int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit *int `json:"safetyLimit,omitempty"`
}

type LinearStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch *int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	Delta *int `json:"delta,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold *int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold *int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit *int `json:"safetyLimit,omitempty"`
}

type ExponentialStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch *int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=2
	// +optional
	GrowthFactor *int `json:"growthFactor,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold *int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold *int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit *int `json:"safetyLimit,omitempty"`
}

// DeploymentStrategy is a single-key sum-type: exactly one of fixed|linear|exponential must be set
type DeploymentStrategy struct {
	// +optional
	Fixed *FixedStrategy `json:"fixed,omitempty"`
	// +optional
	Linear *LinearStrategy `json:"linear,omitempty"`
	// +optional
	Exponential *ExponentialStrategy `json:"exponential,omitempty"`
}

// Budget ceiling either in percent or count
type DeploymentBudget struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	Percent *int `json:"percent,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	Count *int `json:"count,omitempty"`
}

const (
	DefaultCompartmentName = "__default__"
)

// PolicyDefault defines default budget and strategy for unmatched nodes
type PolicyDefault struct {
	// Exactly one of percent or count
	Budget DeploymentBudget `json:"budget,omitempty"`
	// Strategy to use
	Strategy *DeploymentStrategy `json:"strategy"`
}

// Compartment defines a named selector with its own ceiling and optional strategy
type Compartment struct {
	// Unique name within the policy
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Selector defining the nodes in this compartment
	Selector metav1.LabelSelector `json:"selector"`
	// Exactly one of percent or count
	Budget DeploymentBudget `json:"budget"`
	// Optional per-compartment strategy override
	// +optional
	Strategy *DeploymentStrategy `json:"strategy,omitempty"`
}

// DeploymentPolicySpec defines rollout ceilings/strategy by default and per-compartment
type DeploymentPolicySpec struct {
	// Default budget/strategy for unmatched nodes
	Default PolicyDefault `json:"default"`
	// Compartments, each with selector and budget; optional strategy
	// +optional
	Compartments []Compartment `json:"compartments,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced

// DeploymentPolicy configures safe rollout defaults and compartment overrides
type DeploymentPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DeploymentPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

type DeploymentPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeploymentPolicy `json:"items"`
}

// Default applies default values to DeploymentStrategy
func (s *DeploymentStrategy) Default() {
	switch {
	case s.Fixed != nil:
		s.Fixed.Default()
	case s.Linear != nil:
		s.Linear.Default()
	case s.Exponential != nil:
		s.Exponential.Default()
	}
}

// defaultCommonStrategyFields applies default values to common strategy fields
func defaultCommonStrategyFields(initialBatch, batchThreshold, failureThreshold, safetyLimit **int) {
	if *initialBatch == nil {
		*initialBatch = ptr.To(1)
	}
	if *batchThreshold == nil {
		*batchThreshold = ptr.To(100)
	}
	if *failureThreshold == nil {
		*failureThreshold = ptr.To(3)
	}
	if *safetyLimit == nil {
		*safetyLimit = ptr.To(50)
	}
}

// Default applies default values to FixedStrategy
func (s *FixedStrategy) Default() {
	defaultCommonStrategyFields(&s.InitialBatch, &s.BatchThreshold, &s.FailureThreshold, &s.SafetyLimit)
}

// Default applies default values to LinearStrategy
func (s *LinearStrategy) Default() {
	defaultCommonStrategyFields(&s.InitialBatch, &s.BatchThreshold, &s.FailureThreshold, &s.SafetyLimit)
	if s.Delta == nil {
		s.Delta = ptr.To(1)
	}
}

// Default applies default values to ExponentialStrategy
func (s *ExponentialStrategy) Default() {
	defaultCommonStrategyFields(&s.InitialBatch, &s.BatchThreshold, &s.FailureThreshold, &s.SafetyLimit)
	if s.GrowthFactor == nil {
		s.GrowthFactor = ptr.To(2)
	}
}

// Validate validates the DeploymentStrategy
func (s *DeploymentStrategy) Validate() error {
	count := 0
	if s.Fixed != nil {
		count++
	}
	if s.Linear != nil {
		count++
	}
	if s.Exponential != nil {
		count++
	}

	if count != 1 {
		return fmt.Errorf("exactly one of fixed, linear, or exponential must be set")
	}

	return nil
}

// BatchProcessingState tracks the current state of batch processing for a compartment
type BatchProcessingState struct {
	// Current batch number (starts at 1)
	CurrentBatch int `json:"currentBatch,omitempty"`
	// Number of consecutive failures
	ConsecutiveFailures int `json:"consecutiveFailures,omitempty"`
	// Total number of nodes processed so far
	ProcessedNodes int `json:"processedNodes,omitempty"`
	// Number of successful nodes in current batch
	SuccessfulInBatch int `json:"successfulInBatch,omitempty"`
	// Number of failed nodes in current batch
	FailedInBatch int `json:"failedInBatch,omitempty"`
	// Whether the strategy should stop processing due to failures
	ShouldStop bool `json:"shouldStop,omitempty"`
	// Names of nodes in the current batch (persisted across reconciles)
	CurrentBatchNodes []string `json:"currentBatchNodes,omitempty"`
	// Last successful batch size (for slowdown calculations)
	LastBatchSize int `json:"lastBatchSize,omitempty"`
	// Whether the last batch failed (for slowdown logic)
	LastBatchFailed bool `json:"lastBatchFailed,omitempty"`
}

// CalculateBatchSize calculates the next batch size based on the strategy
func (s *DeploymentStrategy) CalculateBatchSize(totalNodes int, state *BatchProcessingState) int {
	switch {
	case s.Fixed != nil:
		return s.Fixed.CalculateBatchSize(totalNodes, state)
	case s.Linear != nil:
		return s.Linear.CalculateBatchSize(totalNodes, state)
	case s.Exponential != nil:
		return s.Exponential.CalculateBatchSize(totalNodes, state)
	default:
		return 1 // fallback
	}
}

// EvaluateBatchResult evaluates the result of a batch and updates state
func (s *DeploymentStrategy) EvaluateBatchResult(state *BatchProcessingState, batchSize int, successCount int, failureCount int, totalNodes int) {
	state.SuccessfulInBatch = successCount
	state.FailedInBatch = failureCount
	state.ProcessedNodes += batchSize

	// Avoid divide by zero
	if batchSize == 0 {
		return
	}

	successPercentage := (successCount * 100) / batchSize
	var progressPercent int
	if totalNodes > 0 {
		progressPercent = (state.ProcessedNodes * 100) / totalNodes
	}

	if successPercentage >= s.getBatchThreshold() {
		state.ConsecutiveFailures = 0
		state.LastBatchFailed = false
	} else {
		state.ConsecutiveFailures++
		state.LastBatchFailed = true
		if progressPercent < s.getSafetyLimit() && state.ConsecutiveFailures >= s.getFailureThreshold() {
			state.ShouldStop = true
		}
	}

	state.LastBatchSize = batchSize
	state.CurrentBatch++
}

// getBatchThreshold returns the batch threshold from the active strategy
func (s *DeploymentStrategy) getBatchThreshold() int {
	switch {
	case s.Fixed != nil:
		return *s.Fixed.BatchThreshold
	case s.Linear != nil:
		return *s.Linear.BatchThreshold
	case s.Exponential != nil:
		return *s.Exponential.BatchThreshold
	default:
		return 100
	}
}

// getSafetyLimit returns the safety limit from the active strategy
func (s *DeploymentStrategy) getSafetyLimit() int {
	switch {
	case s.Fixed != nil:
		return *s.Fixed.SafetyLimit
	case s.Linear != nil:
		return *s.Linear.SafetyLimit
	case s.Exponential != nil:
		return *s.Exponential.SafetyLimit
	default:
		return 50
	}
}

// getFailureThreshold returns the failure threshold from the active strategy
func (s *DeploymentStrategy) getFailureThreshold() int {
	switch {
	case s.Fixed != nil:
		return *s.Fixed.FailureThreshold
	case s.Linear != nil:
		return *s.Linear.FailureThreshold
	case s.Exponential != nil:
		return *s.Exponential.FailureThreshold
	default:
		return 3
	}
}

func (s *FixedStrategy) CalculateBatchSize(totalNodes int, state *BatchProcessingState) int {
	// Fixed strategy doesn't change batch size, but respects remaining nodes
	batchSize := *s.InitialBatch
	remaining := totalNodes - state.ProcessedNodes
	if batchSize > remaining {
		batchSize = remaining
	}
	return max(1, batchSize)
}

func (s *LinearStrategy) CalculateBatchSize(totalNodes int, state *BatchProcessingState) int {
	var batchSize int

	// Avoid divide by zero
	if totalNodes == 0 {
		return 0
	}

	// Check if we should slow down due to last batch failure
	progressPercent := (state.ProcessedNodes * 100) / totalNodes
	if state.LastBatchFailed && progressPercent < *s.SafetyLimit && state.LastBatchSize > 0 {
		// Slow down: reduce by delta from last batch size
		batchSize = max(1, state.LastBatchSize-*s.Delta)
	} else {
		// Normal growth: initialBatch + (currentBatch - 1) * delta
		batchSize = *s.InitialBatch + (state.CurrentBatch-1)*(*s.Delta)
	}

	remaining := totalNodes - state.ProcessedNodes
	if batchSize > remaining {
		batchSize = remaining
	}
	return max(1, batchSize)
}

func (s *ExponentialStrategy) CalculateBatchSize(totalNodes int, state *BatchProcessingState) int {
	var batchSize int

	// Avoid divide by zero
	if totalNodes == 0 {
		return 0
	}

	// Check if we should slow down due to last batch failure
	progressPercent := (state.ProcessedNodes * 100) / totalNodes
	if state.LastBatchFailed && progressPercent < *s.SafetyLimit && state.LastBatchSize > 0 && *s.GrowthFactor > 0 {
		// Slow down: divide last batch size by growth factor
		batchSize = max(1, state.LastBatchSize / *s.GrowthFactor)
	} else {
		// Normal growth: initialBatch * (growthFactor ^ (currentBatch - 1))
		// Use math.Pow for efficiency and to avoid overflow issues with large batch numbers
		exponent := state.CurrentBatch - 1
		growthMultiplier := math.Pow(float64(*s.GrowthFactor), float64(exponent))
		batchSize = int(float64(*s.InitialBatch) * growthMultiplier)

		// Cap at remaining nodes to prevent unreasonably large batch sizes
		// and potential integer overflow
		if batchSize > totalNodes {
			batchSize = totalNodes
		}
	}

	remaining := totalNodes - state.ProcessedNodes
	if batchSize > remaining {
		batchSize = remaining
	}
	return max(1, batchSize)
}

// Validate validates the Compartment
func (c *Compartment) Validate() error {
	// Validate compartment budget
	if err := c.Budget.Validate(); err != nil {
		return fmt.Errorf("compartment %q budget: %w", c.Name, err)
	}

	// Validate compartment strategy if present
	if c.Strategy != nil {
		if err := c.Strategy.Validate(); err != nil {
			return fmt.Errorf("compartment %q strategy: %w", c.Name, err)
		}
	}

	// Validate label selector syntax
	if _, err := metav1.LabelSelectorAsSelector(&c.Selector); err != nil {
		return fmt.Errorf("compartment %q has invalid selector: %w", c.Name, err)
	}

	return nil
}

// Validate validates the DeploymentBudget
func (b *DeploymentBudget) Validate() error {
	hasPercent := b.Percent != nil
	hasCount := b.Count != nil

	if !hasPercent && !hasCount {
		return fmt.Errorf("exactly one of percent or count must be set")
	}

	if hasPercent && hasCount {
		return fmt.Errorf("percent and count are mutually exclusive")
	}

	return nil
}

func init() {
	SchemeBuilder.Register(&DeploymentPolicy{}, &DeploymentPolicyList{})
}
