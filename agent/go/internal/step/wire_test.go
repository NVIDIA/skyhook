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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Decode", func() {
	It("loads defaults like python step.load", func() {
		data := []byte(`{"path":"foo.sh","idempotence":false,"upgrade_step":false}`)
		s, err := Decode(data)
		Expect(err).NotTo(HaveOccurred())

		Expect(s.Name()).To(Equal("foo.sh"))
		Expect(s.Path()).To(Equal("foo.sh"))
		Expect(s.Arguments()).To(Equal([]string{}))
		Expect(s.Returncodes()).To(Equal([]int{0}))
		Expect(s.OnHost()).To(BeTrue())
		Expect(s.Env()).To(Equal(map[string]string{}))
		Expect(s.Idempotence()).To(Equal(Auto))
	})

	It("returns a RegularStep when upgrade_step is false", func() {
		data := []byte(`{"path":"foo.sh","idempotence":false,"upgrade_step":false}`)
		s, err := Decode(data)
		Expect(err).NotTo(HaveOccurred())
		_, ok := s.(RegularStep)
		Expect(ok).To(BeTrue(), "expected RegularStep, got %T", s)
	})

	It("returns an UpgradeStep when upgrade_step is true", func() {
		data := []byte(`{"path":"upgrade.sh","idempotence":false,"upgrade_step":true}`)
		s, err := Decode(data)
		Expect(err).NotTo(HaveOccurred())
		_, ok := s.(UpgradeStep)
		Expect(ok).To(BeTrue(), "expected UpgradeStep, got %T", s)
	})

	It("rejects an upgrade_step payload with non-empty arguments", func() {
		data := []byte(`{"name":"u","path":"u.sh","arguments":["x"],"idempotence":false,"upgrade_step":true}`)
		_, err := Decode(data)
		Expect(err).To(MatchError(
			"UpgradeStep u can not have any arguments, but found: [x]",
		))
	})
})

var _ = Describe("Encode/Decode round-trip", func() {
	It("round-trips idempotence and preserves RegularStep type", func() {
		start := RegularStep{
			name:        "foo",
			path:        "foo",
			arguments:   []string{},
			returncodes: []int{0},
			onHost:      true,
			idempotence: Disabled,
		}

		dumped, err := start.Encode()
		Expect(err).NotTo(HaveOccurred())

		end, err := Decode(dumped)
		Expect(err).NotTo(HaveOccurred())

		rs, ok := end.(RegularStep)
		Expect(ok).To(BeTrue(), "expected RegularStep, got %T", end)
		Expect(rs.Idempotence()).To(Equal(Disabled))
	})

	It("preserves UpgradeStep type through the round-trip", func() {
		start, err := NewUpgradeStep(
			"upgrade",
			WithName("upgrade"),
			WithReturncodes([]int{0}),
			WithOnHost(true),
			WithIdempotence(Auto),
		)
		Expect(err).NotTo(HaveOccurred())

		dumped, err := start.Encode()
		Expect(err).NotTo(HaveOccurred())

		end, err := Decode(dumped)
		Expect(err).NotTo(HaveOccurred())

		_, ok := end.(UpgradeStep)
		Expect(ok).To(BeTrue(), "expected UpgradeStep, got %T", end)
	})

	It("encodes a zero-value RegularStep by applying defaults", func() {
		bare := RegularStep{path: "foo.sh"}
		dumped, err := bare.Encode()
		Expect(err).NotTo(HaveOccurred())

		round, err := Decode(dumped)
		Expect(err).NotTo(HaveOccurred())
		Expect(round.Name()).To(Equal("foo.sh"))
		Expect(round.Arguments()).To(Equal([]string{}))
		Expect(round.Returncodes()).To(Equal([]int{0}))
		Expect(round.Idempotence()).To(Equal(Auto))
	})
})

var _ = Describe("Encode env handling", func() {
	It("omits env when empty and includes it when populated", func() {
		noEnv := RegularStep{
			name:        "a",
			path:        "a",
			arguments:   []string{},
			returncodes: []int{0},
			onHost:      true,
			idempotence: Auto,
		}
		noEnvJSON, err := noEnv.Encode()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(noEnvJSON)).NotTo(ContainSubstring(`"env"`))

		withEnv := noEnv
		withEnv.env = map[string]string{"hello": "world"}
		withEnvJSON, err := withEnv.Encode()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(withEnvJSON)).To(ContainSubstring(`"env":{"hello":"world"}`))
	})
})

var _ = Describe("Golden bytes", func() {
	const stepFixtureJSON = `{"name":"foo","path":"foo","arguments":[],"returncodes":[0],"on_host":true,"idempotence":false,"upgrade_step":false}`

	It("matches python-shape serialization", func() {
		s, err := Decode([]byte(stepFixtureJSON))
		Expect(err).NotTo(HaveOccurred())
		roundTrip, err := s.Encode()
		Expect(err).NotTo(HaveOccurred())

		Expect(string(roundTrip)).To(Equal(strings.TrimSpace(stepFixtureJSON)))
	})
})

var _ = Describe("RequiresInterrupt wire asymmetry", func() {
	It("is populated by Decode but dropped by Encode for python parity", func() {
		data := []byte(`{"path":"foo.sh","idempotence":false,"upgrade_step":false,"requires_interrupt":true}`)
		s, err := Decode(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(s.RequiresInterrupt()).To(BeTrue())

		dumped, err := s.Encode()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(dumped)).NotTo(ContainSubstring("requires_interrupt"))

		round, err := Decode(dumped)
		Expect(err).NotTo(HaveOccurred())
		Expect(round.RequiresInterrupt()).To(BeFalse())
	})
})
