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

package step

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RegularStep.applyDefaults", func() {
	It("fills name from path when name is empty", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Name).To(Equal("foo.sh"))
	})

	It("preserves name when it is set", func() {
		s := RegularStep{Name: "explicit", ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Name).To(Equal("explicit"))
	})

	It("defaults arguments to an empty slice", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Arguments).To(Equal([]string{}))
	})

	It("defaults returncodes to [0]", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Returncodes).To(Equal([]int{0}))
	})

	It("defaults env to an empty map", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Env).To(Equal(map[string]string{}))
	})

	It("defaults idempotence to Auto", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		s.applyDefaults()
		Expect(s.Idempotence).To(Equal(Auto))
	})

	It("does not change OnHost (the constructor seeds it; the schema requires it)", func() {
		s := RegularStep{ScriptPath: "foo.sh", OnHost: true}
		s.applyDefaults()
		Expect(s.OnHost).To(BeTrue())
	})
})

var _ = Describe("RegularStep.Encode", func() {
	It("emits upgrade_step:false", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		dumped, err := s.Encode()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(dumped)).To(ContainSubstring(`"upgrade_step":false`))
	})

	It("does not mutate the caller's value when applying defaults", func() {
		s := RegularStep{ScriptPath: "foo.sh"}
		_, err := s.Encode()
		Expect(err).NotTo(HaveOccurred())

		Expect(s.Name).To(Equal(""), "Encode received a value copy; the caller's RegularStep must be untouched")
		Expect(s.Idempotence).To(Equal(Idempotence("")))
	})

	It("rejects an explicitly invalid idempotence value", func() {
		s := RegularStep{ScriptPath: "foo.sh", Idempotence: "bogus"}
		_, err := s.Encode()
		Expect(err).To(MatchError(ContainSubstring("bogus is not a valid idempotence value")))
	})
})

var _ = Describe("NewRegularStep", func() {
	It("applies all defaults when only path is provided", func() {
		s := NewRegularStep("foo.sh")
		Expect(s.ScriptPath).To(Equal("foo.sh"))
		Expect(s.Name).To(Equal("foo.sh"))
		Expect(s.Arguments).To(Equal([]string{}))
		Expect(s.Returncodes).To(Equal([]int{0}))
		Expect(s.Env).To(Equal(map[string]string{}))
		Expect(s.OnHost).To(BeTrue())
		Expect(s.Idempotence).To(Equal(Auto))
		Expect(s.RequiresInterrupt).To(BeFalse())
	})

	It("honors WithName instead of the path fallback", func() {
		s := NewRegularStep("foo.sh", WithName("explicit"))
		Expect(s.Name).To(Equal("explicit"))
	})

	It("honors WithOnHost(false) over the on-by-default value", func() {
		s := NewRegularStep("foo.sh", WithOnHost(false))
		Expect(s.OnHost).To(BeFalse())
	})

	It("applies every option when combined", func() {
		s := NewRegularStep(
			"foo.sh",
			WithName("custom"),
			WithArguments([]string{"-x", "y"}),
			WithReturncodes([]int{0, 2}),
			WithEnv(map[string]string{"K": "V"}),
			WithOnHost(false),
			WithIdempotence(Disabled),
			WithRequiresInterrupt(true),
		)
		Expect(s.Name).To(Equal("custom"))
		Expect(s.Arguments).To(Equal([]string{"-x", "y"}))
		Expect(s.Returncodes).To(Equal([]int{0, 2}))
		Expect(s.Env).To(Equal(map[string]string{"K": "V"}))
		Expect(s.OnHost).To(BeFalse())
		Expect(s.Idempotence).To(Equal(Disabled))
		Expect(s.RequiresInterrupt).To(BeTrue())
	})

	It("round-trips through Encode/Decode", func() {
		start := NewRegularStep("foo.sh", WithIdempotence(Disabled))
		dumped, err := start.Encode()
		Expect(err).NotTo(HaveOccurred())

		round, err := Decode(dumped)
		Expect(err).NotTo(HaveOccurred())
		Expect(round.(RegularStep).ScriptPath).To(Equal("foo.sh"))
		Expect(round.(RegularStep).Idempotence).To(Equal(Disabled))
	})
})
