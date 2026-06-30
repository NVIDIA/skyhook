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

package interrupts

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInterrupts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Interrupts Suite")
}

var _ = Describe("Encode/Decode", func() {
	It("round-trips Encode and Decode for all interrupt types", func() {
		starts := []Interrupt{
			ServiceRestart{Services: []string{"containerd"}},
			NodeRestart{},
			ScriptInterrupt{},
			RestartAllServices{},
			NoOp{},
		}
		for _, start := range starts {
			startSerialized, err := start.Serialize()
			Expect(err).NotTo(HaveOccurred())

			encodedFromSerialized := base64.StdEncoding.EncodeToString(startSerialized)
			encodedFromHelper, err := Encode(start)
			Expect(err).NotTo(HaveOccurred())

			decodedA, err := Decode(encodedFromSerialized)
			Expect(err).NotTo(HaveOccurred())

			decodedB, err := Decode(encodedFromHelper)
			Expect(err).NotTo(HaveOccurred())

			for _, candidate := range []Interrupt{decodedA, decodedB} {
				candidateSerialized, err := candidate.Serialize()
				Expect(err).NotTo(HaveOccurred())
				Expect(string(candidateSerialized)).To(MatchJSON(string(startSerialized)))
				Expect(candidate.InterruptCmd()).To(Equal(start.InterruptCmd()))
				Expect(candidate.Type()).To(Equal(start.Type()))
			}
		}
	})

	It("returns the wire-shape error when input is not base64", func() {
		_, err := Decode("not-base64")
		Expect(err).To(MatchError(errInvalidSerializedInterrupt))
	})

	It("wraps the underlying decode error while staying branchable on the sentinel", func() {
		_, err := Decode("not-base64")
		Expect(errors.Is(err, errInvalidSerializedInterrupt)).To(BeTrue())
		// The original base64 failure is preserved for diagnostics, not dropped.
		Expect(err.Error()).To(ContainSubstring("illegal base64"))
	})

	It("rejects payload without a type field", func() {
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"services":["kubelet"]}`))

		_, err := Decode(encoded)
		Expect(err).To(MatchError(errInvalidSerializedInterrupt))
	})

	It("treats an empty type as unknown", func() {
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":""}`))

		_, err := Decode(encoded)
		Expect(err).To(MatchError(
			`unknown interrupt "" must be one of: node_restart, service_restart, restart_all_services, no_op, script_interrupt`,
		))
	})

	It("errors on an unknown type, listing the supported types", func() {
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":"made_up"}`))

		_, err := Decode(encoded)
		Expect(err).To(MatchError(
			`unknown interrupt "made_up" must be one of: node_restart, service_restart, restart_all_services, no_op, script_interrupt`,
		))
	})

	It("returns an error rather than panicking on a nil Interrupt", func() {
		_, err := Encode(nil)
		Expect(err).To(MatchError("nil interrupt"))
	})
})

var _ = Describe("NodeRestart", func() {
	It("uses the node_restart wire type", func() {
		Expect(NodeRestart{}.Type()).To(Equal("node_restart"))
	})

	It("produces a reboot command", func() {
		Expect(NodeRestart{}.InterruptCmd()).To(Equal([][]string{{"reboot"}}))
	})
})

var _ = Describe("ServiceRestart", func() {
	It("uses the service_restart wire type", func() {
		Expect(ServiceRestart{}.Type()).To(Equal("service_restart"))
	})

	It("produces a daemon-reload followed by per-service restarts", func() {
		Expect(ServiceRestart{Services: []string{"foo", "bar"}}.InterruptCmd()).To(Equal(
			[][]string{
				{"systemctl", "daemon-reload"},
				{"systemctl", "restart", "foo"},
				{"systemctl", "restart", "bar"},
			},
		))
	})

	It("decodes a service payload into a ServiceRestart value", func() {
		payload := map[string]any{
			"type":     ServiceRestart{}.Type(),
			"services": []string{"containerd", "kubelet"},
		}
		data, err := json.Marshal(payload)
		Expect(err).NotTo(HaveOccurred())

		interrupt, err := Decode(base64.StdEncoding.EncodeToString(data))
		Expect(err).NotTo(HaveOccurred())
		Expect(interrupt).To(Equal(ServiceRestart{Services: []string{"containerd", "kubelet"}}))
	})

	It("rejects a payload whose services field is the wrong type", func() {
		encoded := base64.StdEncoding.EncodeToString([]byte(`{"type":"service_restart","services":"kubelet"}`))

		_, err := Decode(encoded)
		Expect(err).To(MatchError(errInvalidSerializedInterrupt))
	})

	It("emits an empty services array when Services is nil, not null", func() {
		data, err := ServiceRestart{}.Serialize()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(`"services":[]`))
		Expect(string(data)).NotTo(ContainSubstring("null"))
	})
})

var _ = Describe("RestartAllServices", func() {
	It("uses the restart_all_services wire type", func() {
		Expect(RestartAllServices{}.Type()).To(Equal("restart_all_services"))
	})

	It("issues a force-reload of procps", func() {
		Expect(RestartAllServices{}.InterruptCmd()).To(Equal(
			[][]string{{"service", "procps", "force-reload"}},
		))
	})
})

var _ = Describe("NoOp", func() {
	It("uses the no_op wire type", func() {
		Expect(NoOp{}.Type()).To(Equal("no_op"))
	})

	It("produces no commands", func() {
		Expect(NoOp{}.InterruptCmd()).To(BeEmpty())
	})
})

var _ = Describe("ScriptInterrupt", func() {
	It("uses the script_interrupt wire type", func() {
		Expect(ScriptInterrupt{}.Type()).To(Equal("script_interrupt"))
	})

	It("produces no commands because the apply script does the work", func() {
		Expect(ScriptInterrupt{}.InterruptCmd()).To(BeEmpty())
	})
})
