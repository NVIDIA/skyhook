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
	var skyhookWebhook *SkyhookWebhook

	BeforeEach(func() {
		skyhookWebhook = &SkyhookWebhook{
			Client: k8sClient,
		}
	})

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

	It("should validate the deployment policy", func() {
		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: SkyhookSpec{
				DeploymentPolicy: "foobar",
				InterruptionBudget: InterruptionBudget{
					Percent: ptr.To(25),
				},
			},
		}

		_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("deploymentPolicy and interruptionBudget are mutually exclusive"))
	})

	It("should reject skyhook with non-existent deployment policy", func() {
		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook",
			},
			Spec: SkyhookSpec{
				DeploymentPolicy: "non-existent-policy",
				Packages: Packages{
					"test-pkg": {
						PackageRef: PackageRef{
							Name:    "test-pkg",
							Version: "1.0.0",
						},
					},
				},
			},
		}

		_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("deploymentPolicy \"non-existent-policy\" not found"))
	})

	It("should accept skyhook when deployment policy exists", func() {
		// Create a cluster-scoped deployment policy first
		policy := &DeploymentPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-policy-webhook",
			},
			Spec: DeploymentPolicySpec{
				Default: PolicyDefault{
					Budget: DeploymentBudget{
						Percent: ptr.To(100),
					},
					Strategy: &DeploymentStrategy{
						Fixed: &FixedStrategy{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())

		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-valid",
			},
			Spec: SkyhookSpec{
				DeploymentPolicy: "test-policy-webhook",
				Packages: Packages{
					"test-pkg": {
						PackageRef: PackageRef{
							Name:    "test-pkg",
							Version: "1.0.0",
						},
					},
				},
			},
		}

		_, err := skyhookWebhook.ValidateCreate(ctx, skyhook)
		Expect(err).NotTo(HaveOccurred())

		// Cleanup
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
	})

	It("should reject skyhook update to reference non-existent deployment policy", func() {
		// Create a Skyhook without deployment policy
		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-update",
			},
			Spec: SkyhookSpec{
				Packages: Packages{
					"test-pkg": {
						PackageRef: PackageRef{
							Name:    "test-pkg",
							Version: "1.0.0",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, skyhook)).To(Succeed())

		// Try to update it to reference a non-existent policy
		updatedSkyhook := skyhook.DeepCopy()
		updatedSkyhook.Spec.DeploymentPolicy = "does-not-exist"

		_, err := skyhookWebhook.ValidateUpdate(ctx, skyhook, updatedSkyhook)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("deploymentPolicy \"does-not-exist\" not found"))

		// Cleanup
		Expect(k8sClient.Delete(ctx, skyhook)).To(Succeed())
	})

	It("should allow skyhook update to reference valid deployment policy", func() {
		// Create a deployment policy
		policy := &DeploymentPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "valid-policy-for-update",
			},
			Spec: DeploymentPolicySpec{
				Default: PolicyDefault{
					Budget: DeploymentBudget{
						Percent: ptr.To(100),
					},
					Strategy: &DeploymentStrategy{
						Fixed: &FixedStrategy{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, policy)).To(Succeed())

		// Create a Skyhook without deployment policy
		skyhook := &Skyhook{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-skyhook-update-valid",
			},
			Spec: SkyhookSpec{
				Packages: Packages{
					"test-pkg": {
						PackageRef: PackageRef{
							Name:    "test-pkg",
							Version: "1.0.0",
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, skyhook)).To(Succeed())

		// Update it to reference the valid policy - should succeed
		updatedSkyhook := skyhook.DeepCopy()
		updatedSkyhook.Spec.DeploymentPolicy = "valid-policy-for-update"

		_, err := skyhookWebhook.ValidateUpdate(ctx, skyhook, updatedSkyhook)
		Expect(err).NotTo(HaveOccurred())

		// Cleanup
		Expect(k8sClient.Delete(ctx, skyhook)).To(Succeed())
		Expect(k8sClient.Delete(ctx, policy)).To(Succeed())
	})
})
