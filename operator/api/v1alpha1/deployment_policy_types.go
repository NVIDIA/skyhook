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

// StrategyType represents the type of deployment strategy
type StrategyType string

const (
	StrategyTypeFixed       StrategyType = "fixed"
	StrategyTypeLinear      StrategyType = "linear"
	StrategyTypeExponential StrategyType = "exponential"
	StrategyTypeUnknown     StrategyType = "unknown"
)

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
