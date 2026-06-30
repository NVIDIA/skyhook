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
	"encoding/json"
	"fmt"
)

// RegularStep is the default Step implementation. Its fields are
// exported with JSON tags so stdlib json.Marshal / json.Unmarshal
// operate on it directly; Idempotence carries its own enum<->bool
// codec, so no custom marshaler is needed here.
//
// applyDefaults makes Name fall back to ScriptPath, Arguments to []string{},
// Returncodes to []int{0}, Env to map[string]string{}, and Idempotence to Auto.
// NewRegularStep additionally defaults OnHost to true.
//
// ScriptPath carries the json:"path" wire name; the field is named
// distinctly so RegularStep can expose Path() to satisfy Step.
type RegularStep struct {
	Name        string            `json:"name"`
	ScriptPath  string            `json:"path"`
	Arguments   []string          `json:"arguments"`
	Returncodes []int             `json:"returncodes"`
	OnHost      bool              `json:"on_host"`
	Idempotence Idempotence       `json:"idempotence"`
	UpgradeStep bool              `json:"upgrade_step"`
	Env         map[string]string `json:"env,omitempty"`

	// RequiresInterrupt round-trips via the wire as "requires_interrupt",
	// emitted only when true; it stays schema-valid because the step schema
	// permits extra properties.
	RequiresInterrupt bool `json:"requires_interrupt,omitempty"`
}

var _ Step = RegularStep{}

// Path returns the step's script path, relative to the package root.
func (s RegularStep) Path() string { return s.ScriptPath }

// Encode writes the JSON wire form with upgrade_step:false. Defaults
// are applied to a local copy so the caller's value is not mutated;
// Idempotence is validated after defaulting so a zero-value RegularStep
// (which defaults to Auto) still encodes successfully. The discriminator
// is pinned to false here so a caller that set the exported UpgradeStep
// field cannot coerce a RegularStep into the upgrade-step wire form.
func (s RegularStep) Encode() ([]byte, error) {
	s.UpgradeStep = false
	return s.encode()
}

// encode is the shared marshal path for RegularStep and UpgradeStep:
// each public Encode pins its own discriminator on the value copy, then
// delegates here for defaulting, validation, and marshaling.
func (s RegularStep) encode() ([]byte, error) {
	s.applyDefaults()
	if err := s.Idempotence.Validate(); err != nil {
		return nil, fmt.Errorf("validate idempotence: %w", err)
	}
	out, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("marshal RegularStep: %w", err)
	}
	return out, nil
}

// RegularStepOption configures a RegularStep during construction. Use
// the With* helpers; the type is exported so callers can build option
// slices and pass them through.
type RegularStepOption func(*RegularStep)

// WithName overrides the default Name (which otherwise falls back to ScriptPath).
func WithName(name string) RegularStepOption { return func(s *RegularStep) { s.Name = name } }

// WithArguments sets the step's arguments.
func WithArguments(args []string) RegularStepOption {
	return func(s *RegularStep) { s.Arguments = args }
}

// WithReturncodes sets the accepted return codes (default: [0]).
func WithReturncodes(codes []int) RegularStepOption {
	return func(s *RegularStep) { s.Returncodes = codes }
}

// WithEnv sets the environment map applied to the step process.
func WithEnv(env map[string]string) RegularStepOption {
	return func(s *RegularStep) { s.Env = env }
}

// WithOnHost overrides the default OnHost=true.
func WithOnHost(onHost bool) RegularStepOption {
	return func(s *RegularStep) { s.OnHost = onHost }
}

// WithIdempotence overrides the default Idempotence=Auto.
func WithIdempotence(i Idempotence) RegularStepOption {
	return func(s *RegularStep) { s.Idempotence = i }
}

// WithRequiresInterrupt sets the requires_interrupt flag.
func WithRequiresInterrupt(r bool) RegularStepOption {
	return func(s *RegularStep) { s.RequiresInterrupt = r }
}

// NewRegularStep constructs a RegularStep, applying defaults (Name
// from Path, Arguments=[], Returncodes=[0], Env={}, Idempotence=Auto,
// OnHost=true) for any fields the caller did not set via With* options.
func NewRegularStep(path string, opts ...RegularStepOption) RegularStep {
	// OnHost is seeded true here because Go's bool has no "absent"
	// state, so applyDefaults can't tell zero apart from "explicit
	// false"; WithOnHost(false) overrides this initial value.
	rs := RegularStep{ScriptPath: path, OnHost: true}
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
// OnHost is intentionally not defaulted here: Go's bool has no "absent"
// state, and a schema-valid config always carries on_host, so the
// constructor seeds it and stdlib decode reads whatever the payload set.
func (s *RegularStep) applyDefaults() {
	if s.Name == "" {
		s.Name = s.ScriptPath
	}
	if len(s.Arguments) == 0 {
		s.Arguments = []string{}
	}
	if len(s.Returncodes) == 0 {
		s.Returncodes = []int{0}
	}
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	if s.Idempotence == "" {
		s.Idempotence = Auto
	}
}
