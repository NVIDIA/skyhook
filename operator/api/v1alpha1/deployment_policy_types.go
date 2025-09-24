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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Strategy parameters
type FixedStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit int `json:"safetyLimit,omitempty"`
}

type LinearStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	Delta int `json:"delta,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit int `json:"safetyLimit,omitempty"`
}

type ExponentialStrategy struct {
	// +kubebuilder:validation:Minimum=1
	// +optional
	InitialBatch int `json:"initialBatch,omitempty"`
	// +kubebuilder:validation:Minimum=2
	// +optional
	GrowthFactor int `json:"growthFactor,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	BatchThreshold int `json:"batchThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +optional
	FailureThreshold int `json:"failureThreshold,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	SafetyLimit int `json:"safetyLimit,omitempty"`
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

// PolicyDefault defines default budget and strategy for unmatched nodes
type PolicyDefault struct {
	// Exactly one of percent or count
	Budget DeploymentBudget `json:"budget,omitempty"`
	// Strategy to use
	// +optional
	Strategy *DeploymentStrategy `json:"strategy,omitempty"`
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

func init() {
	SchemeBuilder.Register(&DeploymentPolicy{})
}
