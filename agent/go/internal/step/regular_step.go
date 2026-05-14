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

// RegularStep is the default Step implementation. It satisfies the
// Step interface via its getters.
//
// Defaults: Name falls back to Path, Arguments to []string{},
// Returncodes to []int{0}, Env to map[string]string{}, OnHost to true,
// and Idempotence to Auto. applyDefaults centralizes these.
type RegularStep struct {
	name        string
	path        string
	arguments   []string
	onHost      bool
	returncodes []int
	env         map[string]string
	idempotence Idempotence

	// requiresInterrupt is populated by Decode but intentionally not
	// emitted by Encode, mirroring Python's Step.dump which also omits
	// it. Round-tripping a Step with RequiresInterrupt=true returns
	// false; by design for wire parity.
	requiresInterrupt bool
}

var _ Step = RegularStep{}

func (s RegularStep) Path() string             { return s.path }
func (s RegularStep) Name() string             { return s.name }
func (s RegularStep) Arguments() []string      { return s.arguments }
func (s RegularStep) Returncodes() []int       { return s.returncodes }
func (s RegularStep) Env() map[string]string   { return s.env }
func (s RegularStep) OnHost() bool             { return s.onHost }
func (s RegularStep) Idempotence() Idempotence { return s.idempotence }
func (s RegularStep) RequiresInterrupt() bool  { return s.requiresInterrupt }

// Encode writes the JSON wire form with upgrade_step:false. Defaults
// are applied to a local copy so the caller's value is not mutated;
// Idempotence is validated after defaulting so a zero-value RegularStep
// (which defaults to Auto) still encodes successfully.
func (s RegularStep) Encode() ([]byte, error) {
	s.applyDefaults()
	if err := s.idempotence.Validate(); err != nil {
		return nil, err
	}
	return marshalRegularStep(s, false)
}

// RegularStepOption configures a RegularStep during construction. Use
// the With* helpers; the type is exported so callers can build option
// slices and pass them through.
type RegularStepOption func(*RegularStep)

// WithName overrides the default Name (which otherwise falls back to Path).
func WithName(name string) RegularStepOption { return func(s *RegularStep) { s.name = name } }

// WithArguments sets the step's arguments.
func WithArguments(args []string) RegularStepOption {
	return func(s *RegularStep) { s.arguments = args }
}

// WithReturncodes sets the accepted return codes (default: [0]).
func WithReturncodes(codes []int) RegularStepOption {
	return func(s *RegularStep) { s.returncodes = codes }
}

// WithEnv sets the environment map applied to the step process.
func WithEnv(env map[string]string) RegularStepOption {
	return func(s *RegularStep) { s.env = env }
}

// WithOnHost overrides the default OnHost=true.
func WithOnHost(onHost bool) RegularStepOption {
	return func(s *RegularStep) { s.onHost = onHost }
}

// WithIdempotence overrides the default Idempotence=Auto.
func WithIdempotence(i Idempotence) RegularStepOption {
	return func(s *RegularStep) { s.idempotence = i }
}

// WithRequiresInterrupt sets the requires_interrupt flag.
func WithRequiresInterrupt(r bool) RegularStepOption {
	return func(s *RegularStep) { s.requiresInterrupt = r }
}

// NewRegularStep constructs a RegularStep, applying defaults (Name
// from Path, Arguments=[], Returncodes=[0], Env={}, Idempotence=Auto,
// OnHost=true) for any fields the caller did not set via With* options.
func NewRegularStep(path string, opts ...RegularStepOption) RegularStep {
	// onHost defaults true here because Go's bool has no "absent"
	// state, so applyDefaults can't tell zero apart from "explicit
	// false"; WithOnHost(false) overrides this initial value.
	rs := RegularStep{path: path, onHost: true}
	for _, opt := range opts {
		opt(&rs)
	}
	rs.applyDefaults()
	return rs
}

// applyDefaults fills zero-valued fields with their canonical defaults.
// Encode and Decode both call this so callers can construct a
// RegularStep{} literally and still round-trip cleanly.
//
// OnHost is intentionally not defaulted here: Go's bool has no
// "absent" state, so Decode applies OnHost=true via its *bool overlay
// and Encode emits whatever the caller set.
func (s *RegularStep) applyDefaults() {
	if s.name == "" {
		s.name = s.path
	}
	if len(s.arguments) == 0 {
		s.arguments = []string{}
	}
	if len(s.returncodes) == 0 {
		s.returncodes = []int{0}
	}
	if s.env == nil {
		s.env = map[string]string{}
	}
	if s.idempotence == "" {
		s.idempotence = Auto
	}
}
