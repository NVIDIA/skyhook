/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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
	"k8s.io/utils/ptr"
)

var _ = Describe("DeploymentStrategy", func() {

	Describe("getBatchThreshold", func() {
		It("should return default 100 when receiver is nil", func() {
			var s *DeploymentStrategy
			Expect(s.getBatchThreshold()).To(Equal(100))
		})

		It("should return default 100 when strategy is empty", func() {
			s := &DeploymentStrategy{}
			Expect(s.getBatchThreshold()).To(Equal(100))
		})

		It("should return default 100 when Fixed strategy exists but BatchThreshold is nil", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{},
			}
			Expect(s.getBatchThreshold()).To(Equal(100))
		})

		It("should return value when Fixed strategy has BatchThreshold set", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{
					BatchThreshold: ptr.To(80),
				},
			}
			Expect(s.getBatchThreshold()).To(Equal(80))
		})

		It("should return default 100 when Linear strategy exists but BatchThreshold is nil", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{},
			}
			Expect(s.getBatchThreshold()).To(Equal(100))
		})

		It("should return value when Linear strategy has BatchThreshold set", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{
					BatchThreshold: ptr.To(90),
				},
			}
			Expect(s.getBatchThreshold()).To(Equal(90))
		})

		It("should return default 100 when Exponential strategy exists but BatchThreshold is nil", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{},
			}
			Expect(s.getBatchThreshold()).To(Equal(100))
		})

		It("should return value when Exponential strategy has BatchThreshold set", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{
					BatchThreshold: ptr.To(75),
				},
			}
			Expect(s.getBatchThreshold()).To(Equal(75))
		})
	})

	Describe("getSafetyLimit", func() {
		It("should return default 50 when receiver is nil", func() {
			var s *DeploymentStrategy
			Expect(s.getSafetyLimit()).To(Equal(50))
		})

		It("should return default 50 when strategy is empty", func() {
			s := &DeploymentStrategy{}
			Expect(s.getSafetyLimit()).To(Equal(50))
		})

		It("should return default 50 when Fixed strategy exists but SafetyLimit is nil", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{},
			}
			Expect(s.getSafetyLimit()).To(Equal(50))
		})

		It("should return value when Fixed strategy has SafetyLimit set", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{
					SafetyLimit: ptr.To(30),
				},
			}
			Expect(s.getSafetyLimit()).To(Equal(30))
		})

		It("should return default 50 when Linear strategy exists but SafetyLimit is nil", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{},
			}
			Expect(s.getSafetyLimit()).To(Equal(50))
		})

		It("should return value when Linear strategy has SafetyLimit set", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{
					SafetyLimit: ptr.To(40),
				},
			}
			Expect(s.getSafetyLimit()).To(Equal(40))
		})

		It("should return default 50 when Exponential strategy exists but SafetyLimit is nil", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{},
			}
			Expect(s.getSafetyLimit()).To(Equal(50))
		})

		It("should return value when Exponential strategy has SafetyLimit set", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{
					SafetyLimit: ptr.To(60),
				},
			}
			Expect(s.getSafetyLimit()).To(Equal(60))
		})
	})

	Describe("getFailureThreshold", func() {
		It("should return nil when receiver is nil", func() {
			var s *DeploymentStrategy
			Expect(s.getFailureThreshold()).To(BeNil())
		})

		It("should return nil when strategy is empty", func() {
			s := &DeploymentStrategy{}
			Expect(s.getFailureThreshold()).To(BeNil())
		})

		It("should return nil when Fixed strategy exists but FailureThreshold is nil", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{},
			}
			Expect(s.getFailureThreshold()).To(BeNil())
		})

		It("should return value when Fixed strategy has FailureThreshold set", func() {
			s := &DeploymentStrategy{
				Fixed: &FixedStrategy{
					FailureThreshold: ptr.To(3),
				},
			}
			Expect(s.getFailureThreshold()).ToNot(BeNil())
			Expect(*s.getFailureThreshold()).To(Equal(3))
		})

		It("should return nil when Linear strategy exists but FailureThreshold is nil", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{},
			}
			Expect(s.getFailureThreshold()).To(BeNil())
		})

		It("should return value when Linear strategy has FailureThreshold set", func() {
			s := &DeploymentStrategy{
				Linear: &LinearStrategy{
					FailureThreshold: ptr.To(5),
				},
			}
			Expect(s.getFailureThreshold()).ToNot(BeNil())
			Expect(*s.getFailureThreshold()).To(Equal(5))
		})

		It("should return nil when Exponential strategy exists but FailureThreshold is nil", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{},
			}
			Expect(s.getFailureThreshold()).To(BeNil())
		})

		It("should return value when Exponential strategy has FailureThreshold set", func() {
			s := &DeploymentStrategy{
				Exponential: &ExponentialStrategy{
					FailureThreshold: ptr.To(2),
				},
			}
			Expect(s.getFailureThreshold()).ToNot(BeNil())
			Expect(*s.getFailureThreshold()).To(Equal(2))
		})
	})
})
