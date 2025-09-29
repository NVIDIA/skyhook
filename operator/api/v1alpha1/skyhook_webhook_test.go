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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Skyhook Webhook", func() {
	skyhookWebhook := &SkyhookWebhook{}

	Context("When creating Skyhook under Defaulting Webhook", func() {
		It("Should fill in the default value if a required field is empty", func() {

			// TODO(user): Add your logic here

		})
	})

	Context("When creating Skyhook under Validating Webhook", func() {
		It("Should deny if missing a depends on", func() {

			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar",
								Version: "1.0.0",
							},
							DependsOn: map[string]string{"CATS": "2.3"}, // missing
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateUpdate(ctx, nil, skyhook)
			Expect(err).ToNot(BeNil())

		})
		It("Should deny if duplicate packages", func() {

			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar",
								Version: "1.0.0",
							},
						},
						"foobar2": {
							PackageRef: PackageRef{
								Name:    "foobar", // dup
								Version: "1.0.0",
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateUpdate(ctx, nil, skyhook)
			Expect(err).ToNot(BeNil())
		})

		It("Should deny if a package's name is explicitly set and changed", func() {

			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "foo",
								Version: "1.0.0",
							},
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "bar",
								Version: "1.0.0",
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).To(BeNil())

			skyhook = &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "changed",
								Version: "1.0.0",
							},
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "changed",
								Version: "1.0.0",
							},
						},
					},
				},
			}

			_, err = skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).ToNot(BeNil())
		})

		It("Should deny if an image tag for a package is explicitly set and changed", func() {

			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "foo",
								Version: "1.0.0",
							},
							Image: "testing",
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "bar",
								Version: "1.0.0",
							},
							Image: "testing:1.0.0",
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).To(BeNil())

			skyhook = &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "foo",
								Version: "1.0.0",
							},
							Image: "testing:1.2.1",
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "bar",
								Version: "1.0.0",
							},
							Image: "testing:1.2.1",
						},
					},
				},
			}

			_, err = skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).ToNot(BeNil())
		})

		It("should validate that the configInterrupts are for valid configMaps", func() {
			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "foo",
								Version: "1.0.0",
							},
							ConfigMap: map[string]string{
								"key": "value",
								"dog": "value",
							},
							ConfigInterrupts: map[string]Interrupt{
								"dog": {
									Type: REBOOT,
								},
							},
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "bar",
								Version: "1.0.0",
							},
							ConfigMap: map[string]string{
								"key": "value",
								"dog": "value",
							},
							ConfigInterrupts: map[string]Interrupt{
								"cat": {
									Type: REBOOT,
								},
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateUpdate(ctx, nil, skyhook)
			Expect(err).ToNot(BeNil())

			skyhook = &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{
								Name:    "foo",
								Version: "1.0.0",
							},
							ConfigMap: map[string]string{
								"key": "value",
								"dog": "value",
							},
							ConfigInterrupts: map[string]Interrupt{
								"key": {
									Type:     SERVICE,
									Services: []string{"cron"},
								},
							},
						},
						"bar": {
							PackageRef: PackageRef{
								Name:    "bar",
								Version: "1.0.0",
							},
							ConfigMap: map[string]string{
								"key": "value",
								"dog": "value",
							},
							ConfigInterrupts: map[string]Interrupt{
								"key": {
									Type: REBOOT,
								},
								"dog": {
									Type:     SERVICE,
									Services: []string{"cat"},
								},
							},
						},
					},
				},
			}

			_, err = skyhookWebhook.ValidateUpdate(ctx, nil, skyhook)
			Expect(err).To(BeNil())
		})

		It("should allow glob patterns in configInterrupts that match at least one key", func() {
			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{Name: "foo", Version: "1.0.0"},
							ConfigMap: map[string]string{
								"config.sh":  "abc",
								"upgrade.sh": "def",
							},
							ConfigInterrupts: map[string]Interrupt{
								"*.sh": {Type: SERVICE, Services: []string{"kubelet"}},
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).To(BeNil())
		})

		It("should reject glob patterns in configInterrupts that match no keys", func() {
			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foo": {
							PackageRef: PackageRef{Name: "foo", Version: "1.0.0"},
							ConfigMap: map[string]string{
								"config.txt": "abc",
							},
							ConfigInterrupts: map[string]Interrupt{
								"*.sh": {Type: SERVICE, Services: []string{"kubelet"}},
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).ToNot(BeNil())
		})

		It("Should deny if ambiguous version match", func() {
			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"cats": {
							PackageRef: PackageRef{
								Name:    "cats",
								Version: "2.0.0",
							},
						},
						"cats2": {
							PackageRef: PackageRef{
								Name:    "cats", // dup
								Version: "1.0.0",
							},
						},
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar", // dup
								Version: "1.0.0",
							},
							DependsOn: map[string]string{"cats": "1.0.0"},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateUpdate(ctx, nil, skyhook)
			Expect(err).ToNot(BeNil())
		})

		It("Should deny if invalid version string is provided", func() {
			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"cats": {
							PackageRef: PackageRef{
								Name:    "cats",
								Version: "2.0.0",
							},
						},
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar", // dup
								Version: "2024/07/06",
							},
							DependsOn: map[string]string{"cats": "2.0.0"},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).ToNot(BeNil())

			skyhook = &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"cats": {
							PackageRef: PackageRef{
								Name:    "cats",
								Version: "2.1.1",
							},
						},
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar", // dup
								Version: "2024.7.6",
							},
							DependsOn: map[string]string{"cats": "2.1.1"},
						},
					},
				},
			}

			_, err = skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).To(BeNil())
		})

		It("Should admit graph is valid", func() {

			skyhook := &Skyhook{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: SkyhookSpec{
					Packages: Packages{
						"foobar": {
							PackageRef: PackageRef{
								Name:    "foobar",
								Version: "1.0.0",
							},
						},
					},
				},
			}

			_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
			Expect(err).To(BeNil())
		})

		It("Should validate node selectors", func() {

			// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
			tests := []struct {
				labels map[string]string
				valid  bool
			}{
				{labels: map[string]string{"foo": ""}, valid: true},
				{labels: map[string]string{"foo": "bar"}, valid: true},
				{labels: map[string]string{"-foo": "bar"}, valid: false},
				{labels: map[string]string{"_foo": "bar"}, valid: false},
				{labels: map[string]string{"foo": "-bar"}, valid: false},
				{labels: map[string]string{"foo": "123123123112312312311231231231123123123112312312311231231231123123"}, valid: false},
			}

			for _, t := range tests {
				skyhook := &Skyhook{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: SkyhookSpec{
						NodeSelector: metav1.LabelSelector{MatchLabels: t.labels},
					},
				}
				err := skyhook.Validate()

				if t.valid {
					Expect(err).To(BeNil())
				} else {
					Expect(err).ToNot(BeNil())
				}
			}

		})

		It("should validate resource override requirements", func() {
			basePkg := Package{
				PackageRef: PackageRef{Name: "foo", Version: "1.0.0"},
				Image:      "alpine",
			}
			mkSkyhook := func(res *ResourceRequirements) *Skyhook {
				return &Skyhook{
					ObjectMeta: metav1.ObjectMeta{Name: "test"},
					Spec: SkyhookSpec{
						Packages: Packages{
							"foo": func() Package { p := basePkg; p.Resources = res; return p }(),
						},
					},
				}
			}

			// 1. All unset (valid)
			Expect(mkSkyhook(&ResourceRequirements{}).Validate()).To(Succeed())

			// 2. All set and valid
			res := &ResourceRequirements{
				CPURequest:    resource.MustParse("100m"),
				CPULimit:      resource.MustParse("200m"),
				MemoryRequest: resource.MustParse("128Mi"),
				MemoryLimit:   resource.MustParse("256Mi"),
			}
			Expect(mkSkyhook(res).Validate()).To(Succeed())

			// 3. Only some set (invalid)
			res3 := res
			res3.CPULimit = resource.Quantity{} // unset
			Expect(mkSkyhook(res3).Validate()).NotTo(Succeed())

			// 4. Limit < request (invalid)
			res4 := res
			res4.CPULimit = resource.MustParse("50m")
			Expect(mkSkyhook(res4).Validate()).NotTo(Succeed())
			res4 = res
			res4.MemoryLimit = resource.MustParse("64Mi")
			Expect(mkSkyhook(res4).Validate()).NotTo(Succeed())

			// 5. Negative or zero values (invalid)
			res5 := res
			res5.CPURequest = resource.MustParse("0")
			Expect(mkSkyhook(res5).Validate()).NotTo(Succeed())
			res5 = res
			res5.MemoryLimit = resource.MustParse("-1Mi")
			Expect(mkSkyhook(res5).Validate()).NotTo(Succeed())
		})
	})

	It("packages should UnmarshalJSON correctly", func() {
		js := `{"foo": {"version":"1.2.2", "image":"bar:1.2.2"}}`

		var ret Packages
		Expect(json.Unmarshal([]byte(js), &ret)).Should(Succeed())

		Expect(ret["foo"].Name).To(Equal("foo"))
		Expect(ret["foo"].Image).To(Equal("bar"))
	})

	It("should validate the package name is a valid RFC 1123 name", func() {

		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: SkyhookSpec{
				Packages: Packages{
					"11": {
						PackageRef: PackageRef{Version: "1"},
					},
				},
			},
		}

		_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
		Expect(err).ToNot(BeNil())

	})
})

var _ = Describe("DeploymentPolicy", func() {
	Context("Strategy sum-type shape validation", func() {
		It("should require exactly one strategy type", func() {
			// No strategy set (invalid)
			strategy := &DeploymentStrategy{}
			err := strategy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of fixed, linear, or exponential must be set"))

			// Multiple strategies set (invalid)
			strategy = &DeploymentStrategy{
				Fixed:  &FixedStrategy{},
				Linear: &LinearStrategy{},
			}
			err = strategy.Validate()
			Expect(err).To(HaveOccurred())

			strategy = &DeploymentStrategy{
				Fixed:       &FixedStrategy{},
				Linear:      &LinearStrategy{},
				Exponential: &ExponentialStrategy{},
			}
			err = strategy.Validate()
			Expect(err).To(HaveOccurred())

			// Exactly one strategy set (valid)
			strategy = &DeploymentStrategy{Fixed: &FixedStrategy{}}
			strategy.Default()
			err = strategy.Validate()
			Expect(err).ToNot(HaveOccurred())

			strategy = &DeploymentStrategy{Linear: &LinearStrategy{}}
			strategy.Default()
			err = strategy.Validate()
			Expect(err).ToNot(HaveOccurred())

			strategy = &DeploymentStrategy{Exponential: &ExponentialStrategy{}}
			strategy.Default()
			err = strategy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Per-type bounds validation", func() {
		It("should validate FixedStrategy bounds", func() {
			// Valid defaults
			strategy := FixedStrategy{}
			strategy.Default()
			Expect(strategy.Validate()).ToNot(HaveOccurred())

			// Valid custom values
			Expect((&FixedStrategy{
				InitialBatch:     ptr.To(5),
				BatchThreshold:   ptr.To(75),
				FailureThreshold: ptr.To(2),
				SafetyLimit:      ptr.To(30),
			}).Validate()).ToNot(HaveOccurred())

			// Test invalid bounds for common fields
			Expect((&FixedStrategy{InitialBatch: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{BatchThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{BatchThreshold: ptr.To(101)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{FailureThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{SafetyLimit: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{SafetyLimit: ptr.To(101)}).Validate()).To(HaveOccurred())
		})

		It("should validate LinearStrategy bounds", func() {
			// Valid defaults
			strategy := LinearStrategy{}
			strategy.Default()
			Expect(strategy.Validate()).ToNot(HaveOccurred())

			// Valid custom values
			Expect((&LinearStrategy{
				InitialBatch:     ptr.To(3),
				Delta:            ptr.To(2),
				BatchThreshold:   ptr.To(90),
				FailureThreshold: ptr.To(5),
				SafetyLimit:      ptr.To(25),
			}).Validate()).ToNot(HaveOccurred())

			// Test LinearStrategy-specific field
			Expect((&LinearStrategy{Delta: ptr.To(0)}).Validate()).To(HaveOccurred())
		})

		It("should validate ExponentialStrategy bounds", func() {
			// Valid defaults
			strategy := ExponentialStrategy{}
			strategy.Default()
			Expect(strategy.Validate()).ToNot(HaveOccurred())

			// Valid custom values
			Expect((&ExponentialStrategy{
				InitialBatch:     ptr.To(2),
				GrowthFactor:     ptr.To(3),
				BatchThreshold:   ptr.To(85),
				FailureThreshold: ptr.To(4),
				SafetyLimit:      ptr.To(40),
			}).Validate()).ToNot(HaveOccurred())

			// Test ExponentialStrategy-specific field
			Expect((&ExponentialStrategy{GrowthFactor: ptr.To(1)}).Validate()).To(HaveOccurred())
		})

		It("should validate common strategy field bounds", func() {
			// InitialBatch bounds
			Expect((&FixedStrategy{InitialBatch: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&LinearStrategy{InitialBatch: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&ExponentialStrategy{InitialBatch: ptr.To(0)}).Validate()).To(HaveOccurred())

			// BatchThreshold bounds
			Expect((&FixedStrategy{BatchThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{BatchThreshold: ptr.To(101)}).Validate()).To(HaveOccurred())
			Expect((&LinearStrategy{BatchThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&LinearStrategy{BatchThreshold: ptr.To(101)}).Validate()).To(HaveOccurred())

			// FailureThreshold bounds
			Expect((&FixedStrategy{FailureThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&LinearStrategy{FailureThreshold: ptr.To(0)}).Validate()).To(HaveOccurred())

			// SafetyLimit bounds
			Expect((&FixedStrategy{SafetyLimit: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&FixedStrategy{SafetyLimit: ptr.To(101)}).Validate()).To(HaveOccurred())
			Expect((&ExponentialStrategy{SafetyLimit: ptr.To(0)}).Validate()).To(HaveOccurred())
			Expect((&ExponentialStrategy{SafetyLimit: ptr.To(101)}).Validate()).To(HaveOccurred())
		})
	})

	Context("Budget percent/count XOR validation", func() {
		It("should require exactly one of percent or count", func() {
			// Neither set (invalid)
			budget := &DeploymentBudget{}
			err := budget.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one of percent or count must be set"))

			// Both set (invalid)
			budget = &DeploymentBudget{
				Percent: ptr.To(50),
				Count:   ptr.To(10),
			}
			err = budget.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("percent and count are mutually exclusive"))

			// Only percent set (valid)
			budget = &DeploymentBudget{Percent: ptr.To(25)}
			err = budget.Validate()
			Expect(err).ToNot(HaveOccurred())

			// Only count set (valid)
			budget = &DeploymentBudget{Count: ptr.To(5)}
			err = budget.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should validate percent bounds", func() {
			tests := []struct {
				percent   int
				shouldErr bool
			}{
				{0, true},    // too low
				{1, false},   // minimum valid
				{50, false},  // middle valid
				{100, false}, // maximum valid
				{101, true},  // too high
			}

			for _, test := range tests {
				budget := &DeploymentBudget{Percent: ptr.To(test.percent)}
				err := budget.Validate()
				if test.shouldErr {
					Expect(err).To(HaveOccurred(), "Percent: %d", test.percent)
				} else {
					Expect(err).ToNot(HaveOccurred(), "Percent: %d", test.percent)
				}
			}
		})

		It("should validate count bounds", func() {
			tests := []struct {
				count     int
				shouldErr bool
			}{
				{0, true},   // too low
				{1, false},  // minimum valid
				{10, false}, // valid
			}

			for _, test := range tests {
				budget := &DeploymentBudget{Count: ptr.To(test.count)}
				err := budget.Validate()
				if test.shouldErr {
					Expect(err).To(HaveOccurred(), "Count: %d", test.count)
				} else {
					Expect(err).ToNot(HaveOccurred(), "Count: %d", test.count)
				}
			}
		})
	})

	Context("Compartment unique names validation", func() {
		It("should require unique compartment names", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
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

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment name "system" is not unique`))
		})

		It("should allow different compartment names", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
						{
							Name:     "gpu",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api"}},
							Budget:   DeploymentBudget{Count: ptr.To(3)},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Compartment selector equality validation", func() {
		It("should reject identical selectors", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name:     "comp1",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
						{
							Name:     "comp2",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}}, // identical selector
							Budget:   DeploymentBudget{Count: ptr.To(3)},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment "comp2" has identical selector to compartment "comp1"`))
		})

		It("should allow different selectors", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
						},
						{
							Name:     "gpu",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api"}},
							Budget:   DeploymentBudget{Count: ptr.To(3)},
						},
						{
							Name:     "storage",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "db", "env": "prod"}},
							Budget:   DeploymentBudget{Percent: ptr.To(10)},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Default inheritance validation", func() {
		It("should inherit strategy from default when compartment strategy is nil", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
						Strategy: &DeploymentStrategy{
							Linear: &LinearStrategy{
								InitialBatch: ptr.To(2),
								Delta:        ptr.To(3),
							},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
							// Strategy not specified, should inherit from default
						},
					},
				},
			}

			// Apply defaults
			policy.Default()

			// Check that compartment inherited the strategy
			Expect(policy.Spec.Compartments[0].Strategy).ToNot(BeNil())
			Expect(policy.Spec.Compartments[0].Strategy.Linear).ToNot(BeNil())
			Expect(*policy.Spec.Compartments[0].Strategy.Linear.InitialBatch).To(Equal(2))
			Expect(*policy.Spec.Compartments[0].Strategy.Linear.Delta).To(Equal(3))

			// Validate after applying defaults
			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not override explicit compartment strategy", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
						Strategy: &DeploymentStrategy{
							Linear: &LinearStrategy{
								InitialBatch: ptr.To(2),
								Delta:        ptr.To(3),
							},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "web-servers",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
							Strategy: &DeploymentStrategy{
								Fixed: &FixedStrategy{
									InitialBatch: ptr.To(5),
								},
							},
						},
					},
				},
			}

			// Apply defaults
			policy.Default()

			// Check that compartment kept its explicit strategy
			Expect(policy.Spec.Compartments[0].Strategy).ToNot(BeNil())
			Expect(policy.Spec.Compartments[0].Strategy.Fixed).ToNot(BeNil())
			Expect(policy.Spec.Compartments[0].Strategy.Linear).To(BeNil())
			Expect(*policy.Spec.Compartments[0].Strategy.Fixed.InitialBatch).To(Equal(5))

			// Validate after applying defaults
			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Policy defaulting behavior", func() {
		It("should apply defaults to strategies", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget:   DeploymentBudget{Percent: ptr.To(50)},
						Strategy: &DeploymentStrategy{Fixed: &FixedStrategy{}},
					},
					Compartments: []Compartment{
						{
							Name:     "web-servers",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
							Strategy: &DeploymentStrategy{Linear: &LinearStrategy{}},
						},
					},
				},
			}

			// Apply defaults
			policy.Default()

			// Check default strategy got defaults applied
			fixed := policy.Spec.Default.Strategy.Fixed
			Expect(fixed).ToNot(BeNil())
			Expect(*fixed.InitialBatch).To(Equal(1))
			Expect(*fixed.BatchThreshold).To(Equal(100))
			Expect(*fixed.FailureThreshold).To(Equal(3))
			Expect(*fixed.SafetyLimit).To(Equal(50))

			// Check compartment strategy got defaults applied
			linear := policy.Spec.Compartments[0].Strategy.Linear
			Expect(linear).ToNot(BeNil())
			Expect(*linear.InitialBatch).To(Equal(1))
			Expect(*linear.Delta).To(Equal(1))
			Expect(*linear.BatchThreshold).To(Equal(100))
			Expect(*linear.FailureThreshold).To(Equal(3))
			Expect(*linear.SafetyLimit).To(Equal(50))

			// Validate after applying defaults
			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Complete policy validation", func() {
		It("should validate a complete valid policy", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
						Strategy: &DeploymentStrategy{
							Exponential: &ExponentialStrategy{
								InitialBatch:     ptr.To(1),
								GrowthFactor:     ptr.To(2),
								BatchThreshold:   ptr.To(75),
								FailureThreshold: ptr.To(2),
								SafetyLimit:      ptr.To(40),
							},
						},
					},
					Compartments: []Compartment{
						{
							Name:     "system",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"criticality": "high"}},
							Budget:   DeploymentBudget{Count: ptr.To(1)},
							Strategy: &DeploymentStrategy{
								Fixed: &FixedStrategy{
									InitialBatch:     ptr.To(1),
									BatchThreshold:   ptr.To(100),
									FailureThreshold: ptr.To(1),
									SafetyLimit:      ptr.To(25),
								},
							},
						},
						{
							Name:     "gpu",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}},
							Budget:   DeploymentBudget{Percent: ptr.To(75)},
							// Strategy inherited from default
						},
					},
				},
			}

			// Apply defaults
			policy.Default()

			// Validate
			err := policy.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject invalid default budget", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{}, // invalid - neither percent nor count
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("default budget"))
		})

		It("should reject invalid default strategy", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
						Strategy: &DeploymentStrategy{
							Fixed: &FixedStrategy{
								InitialBatch: ptr.To(0), // invalid
							},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("default strategy"))
		})

		It("should reject invalid compartment budget", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name:     "invalid-comp",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{}, // invalid - neither percent nor count
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment "invalid-comp" budget`))
		})

		It("should reject invalid compartment strategy", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name:     "invalid-comp",
							Selector: metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}},
							Budget:   DeploymentBudget{Percent: ptr.To(25)},
							Strategy: &DeploymentStrategy{
								Linear: &LinearStrategy{
									Delta: ptr.To(0), // invalid
								},
							},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment "invalid-comp" strategy`))
		})

		It("should reject invalid label selector", func() {
			policy := &DeploymentPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: DeploymentPolicySpec{
					Default: PolicyDefault{
						Budget: DeploymentBudget{Percent: ptr.To(50)},
					},
					Compartments: []Compartment{
						{
							Name: "invalid-selector",
							Selector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"invalid-key!": "value", // invalid label key
								},
							},
							Budget: DeploymentBudget{Percent: ptr.To(25)},
						},
					},
				},
			}

			err := policy.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`compartment "invalid-selector" has invalid selector`))
		})
	})
})
