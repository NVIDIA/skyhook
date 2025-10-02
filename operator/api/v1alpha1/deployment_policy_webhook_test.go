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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("DeploymentPolicy", func() {
	deploymentPolicyWebhook := &DeploymentPolicyWebhook{}

	Context("When creating DeploymentPolicy under Defaulting Webhook", func() {
		It("Should fill in the default value if a required field is empty", func() {
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "foo",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
							Strategy: &DeploymentStrategy{
								Linear: &LinearStrategy{},
							},
						},
						{
							Name:     "bar",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api"}},
							Budget:   DeploymentBudget{Count: ptr.To(3)},
							Strategy: &DeploymentStrategy{
								Exponential: &ExponentialStrategy{},
							},
						},
					},
				},
			}
			err := deploymentPolicyWebhook.Default(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())
			Expect(deploymentPolicy.Spec.Default.Strategy).ToNot(BeNil())
			Expect(deploymentPolicy.Spec.Default.Strategy.Fixed).ToNot(BeNil())
			Expect(*deploymentPolicy.Spec.Default.Strategy.Fixed.InitialBatch).To(Equal(1))
			Expect(*deploymentPolicy.Spec.Default.Strategy.Fixed.BatchThreshold).To(Equal(100))
			Expect(*deploymentPolicy.Spec.Default.Strategy.Fixed.FailureThreshold).To(Equal(3))
			Expect(*deploymentPolicy.Spec.Default.Strategy.Fixed.SafetyLimit).To(Equal(50))

			Expect(deploymentPolicy.Spec.Compartments[0].Strategy).ToNot(BeNil())
			Expect(deploymentPolicy.Spec.Compartments[0].Strategy.Linear).ToNot(BeNil())
			Expect(*deploymentPolicy.Spec.Compartments[0].Strategy.Linear.InitialBatch).To(Equal(1))
			Expect(*deploymentPolicy.Spec.Compartments[0].Strategy.Linear.Delta).To(Equal(1))
			Expect(*deploymentPolicy.Spec.Compartments[0].Strategy.Linear.BatchThreshold).To(Equal(100))
			Expect(*deploymentPolicy.Spec.Compartments[0].Strategy.Linear.FailureThreshold).To(Equal(3))
			Expect(*deploymentPolicy.Spec.Compartments[0].Strategy.Linear.SafetyLimit).To(Equal(50))

			Expect(deploymentPolicy.Spec.Compartments[1].Strategy).ToNot(BeNil())
			Expect(deploymentPolicy.Spec.Compartments[1].Strategy.Exponential).ToNot(BeNil())
			Expect(*deploymentPolicy.Spec.Compartments[1].Strategy.Exponential.InitialBatch).To(Equal(1))
			Expect(*deploymentPolicy.Spec.Compartments[1].Strategy.Exponential.GrowthFactor).To(Equal(2))
			Expect(*deploymentPolicy.Spec.Compartments[1].Strategy.Exponential.BatchThreshold).To(Equal(100))
			Expect(*deploymentPolicy.Spec.Compartments[1].Strategy.Exponential.FailureThreshold).To(Equal(3))
			Expect(*deploymentPolicy.Spec.Compartments[1].Strategy.Exponential.SafetyLimit).To(Equal(50))
		})
	})

	Context("When creating DeploymentPolicy under Validation Webhook", func() {
		It("should require exactly one of fixed, linear, or exponential", func() {
			// No strategy set: should fail
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget:   DeploymentBudget{Percent: ptr.To(25)},
						Strategy: &DeploymentStrategy{},
					},
				},
			}
			_, err := deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of fixed, linear, or exponential must be set"))

			// Exactly one strategy set: should succeed
			deploymentPolicy.Spec.Default.Strategy = &DeploymentStrategy{
				Fixed: &FixedStrategy{},
			}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())

			// Multiple strategies set: should fail
			deploymentPolicy.Spec.Default.Strategy = &DeploymentStrategy{
				Linear:      &LinearStrategy{},
				Exponential: &ExponentialStrategy{},
			}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of fixed, linear, or exponential must be set"))
		})

		It("should require exactly one of percent or count", func() {
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{},
						},
						Budget: DeploymentBudget{}, // neither percent nor count set
					},
				},
			}

			_, err := deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of percent or count must be set"))

			// Only percent set (valid)
			deploymentPolicy.Spec.Default.Budget = DeploymentBudget{Percent: ptr.To(25)}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())

			// Only count set (valid)
			deploymentPolicy.Spec.Default.Budget = DeploymentBudget{Count: ptr.To(5)}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())

			// Both percent and count set (invalid)
			deploymentPolicy.Spec.Default.Budget = DeploymentBudget{
				Count:   ptr.To(5),
				Percent: ptr.To(25),
			}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("percent and count are mutually exclusive"))
		})

		It("should require unique compartment names", func() {
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(25)},
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
						{
							Name:     "system", // duplicate name
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api"}},
							Budget:   DeploymentBudget{Count: ptr.To(3)},
						},
					},
				},
			}

			_, err := deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment name "system" is not unique`))

			// Duplicate name fixed
			deploymentPolicy.Spec.Compartments[1].Name = "gpu"
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject compartment name 'default' as reserved", func() {
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(25)},
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "default", // reserved name
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
					},
				},
			}

			_, err := deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment name "default" is reserved and cannot be used`))

			// Fixed with different name
			deploymentPolicy.Spec.Compartments[0].Name = "system"
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should allow different selectors", func() {
			deploymentPolicy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "foobar"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(25)},
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "system"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
						{
							Name:     "gpu",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "system"}},
							Budget:   DeploymentBudget{Count: ptr.To(3)},
						},
					},
				},
			}

			_, err := deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment "gpu" has identical selector to compartment "system"`))

			// Different selectors
			deploymentPolicy.Spec.Compartments[1].Selector = metav1.LabelSelector{MatchLabels: map[string]string{"node-type": "gpu"}}
			_, err = deploymentPolicyWebhook.ValidateCreate(ctx, deploymentPolicy)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
