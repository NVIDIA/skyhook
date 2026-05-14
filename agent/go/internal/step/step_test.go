// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package step

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStep(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Step Suite")
}

var (
	_ Step = RegularStep{}
	_ Step = UpgradeStep{}
)

var _ = Describe("Step interface", func() {
	It("is satisfied by RegularStep", func() {
		var s Step = RegularStep{path: "foo.sh"}
		Expect(s.Path()).To(Equal("foo.sh"))
	})

	It("is satisfied by UpgradeStep", func() {
		var s Step = UpgradeStep{RegularStep: RegularStep{path: "foo.sh"}}
		Expect(s.Path()).To(Equal("foo.sh"))
	})

	It("promotes RegularStep getters to UpgradeStep through embedding", func() {
		rs := RegularStep{
			name:        "explicit-name",
			path:        "foo.sh",
			arguments:   []string{},
			returncodes: []int{0},
			env:         map[string]string{"K": "V"},
			onHost:      true,
			idempotence: Auto,
		}
		u := UpgradeStep{RegularStep: rs}

		Expect(u.Name()).To(Equal("explicit-name"))
		Expect(u.Path()).To(Equal("foo.sh"))
		Expect(u.Arguments()).To(Equal([]string{}))
		Expect(u.Returncodes()).To(Equal([]int{0}))
		Expect(u.Env()).To(Equal(map[string]string{"K": "V"}))
		Expect(u.OnHost()).To(BeTrue())
		Expect(u.Idempotence()).To(Equal(Auto))
		Expect(u.RequiresInterrupt()).To(BeFalse())
	})

	It("supports type-switching between RegularStep and UpgradeStep", func() {
		steps := []Step{
			RegularStep{path: "foo.sh"},
			UpgradeStep{RegularStep: RegularStep{path: "upgrade.sh"}},
		}

		var sawRegular, sawUpgrade bool
		for _, s := range steps {
			switch s.(type) {
			case RegularStep:
				sawRegular = true
			case UpgradeStep:
				sawUpgrade = true
			}
		}
		Expect(sawRegular).To(BeTrue())
		Expect(sawUpgrade).To(BeTrue())
	})
})

var _ = Describe("Idempotence", func() {
	It("accepts the recognised values", func() {
		Expect(Auto.Validate()).To(Succeed())
		Expect(Disabled.Validate()).To(Succeed())
	})

	It("rejects unrecognised values with a go-specific message", func() {
		Expect(Idempotence("bad").Validate()).To(MatchError("bad is not a valid idempotence value"))
	})
})

var _ = Describe("Mode", func() {
	It("treats Interrupt as a non-step mode", func() {
		Expect(IsStepMode(Interrupt)).To(BeFalse())
	})

	It("treats every apply/check pair as a step mode", func() {
		for apply, check := range ApplyToCheck {
			Expect(IsStepMode(apply)).To(BeTrue(), "expected %s to be a step mode", apply)
			Expect(IsStepMode(check)).To(BeTrue(), "expected %s to be a step mode", check)
		}
	})

	It("locks Interrupt as the only known non-step mode", func() {
		Expect(nonStepModes).To(HaveLen(1))
		Expect(nonStepModes).To(HaveKey(Interrupt))
	})
})
