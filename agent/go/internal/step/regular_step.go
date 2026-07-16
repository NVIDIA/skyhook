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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/NVIDIA/nodewright/agent/internal/command"
)

// RegularStep is the default Step implementation. Its fields are
// exported with JSON tags so stdlib json.Marshal / json.Unmarshal
// operate on it directly; Idempotence carries its own enum<->bool
// codec, so no custom marshaler is needed here.
//
// applyDefaults makes Name fall back to ScriptPath, Arguments to []string{},
// Returncodes to command.SuccessExitCode, Env to map[string]string{}, and
// Idempotence to Auto.
// NewRegularStep additionally defaults OnHost to true.
//
// ScriptPath carries the json:"path" wire name and IdempotenceMode the
// json:"idempotence" wire name; both fields are named distinctly so
// RegularStep can expose Path() and Idempotence() to satisfy Step.
type RegularStep struct {
	Name            string             `json:"name"`
	ScriptPath      string             `json:"path"`
	Arguments       []string           `json:"arguments"`
	Returncodes     []command.ExitCode `json:"returncodes"`
	OnHost          bool               `json:"on_host"`
	IdempotenceMode Idempotence        `json:"idempotence"`
	UpgradeStep     bool               `json:"upgrade_step"`
	Env             map[string]string  `json:"env,omitempty"`

	// RequiresInterrupt round-trips via the wire as "requires_interrupt",
	// emitted only when true; it stays schema-valid because the step schema
	// permits extra properties.
	RequiresInterrupt bool `json:"requires_interrupt,omitempty"`
}

var _ Step = RegularStep{}

// Path returns the step's script path, relative to the package root.
func (s RegularStep) Path() string { return s.ScriptPath }

// Idempotence reports how the agent treats re-runs of the step.
func (s RegularStep) Idempotence() Idempotence { return s.IdempotenceMode }

// Run resolves the step into a command and executes it in its configured host context.
func (s RegularStep) Run(ctx context.Context, config RunConfig) (Status, error) {
	if ctx == nil {
		return StatusFailed, errors.New("running step: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return StatusFailed, fmt.Errorf("running step %q: %w", s.ScriptPath, err)
	}
	if err := config.validate(); err != nil {
		return StatusFailed, fmt.Errorf("running step %q: invalid run config: %w", s.ScriptPath, err)
	}
	if !filepath.IsLocal(s.ScriptPath) {
		return StatusFailed, fmt.Errorf("running step: path %q must be relative to the step root", s.ScriptPath)
	}

	s.applyDefaults()
	arguments, missing := s.resolveArguments()
	if len(missing) > 0 {
		return StatusFailed, fmt.Errorf(
			"running step %q: expected environment variables do not exist: %s",
			s.ScriptPath,
			strings.Join(missing, ", "),
		)
	}

	runner := command.NewRunner(command.WithChroot(config.rootMount))
	if !s.OnHost {
		runner = command.NewRunner()
	}

	environment := maps.Clone(s.Env)
	environment["STEP_ROOT"] = config.stepRoot
	environment["SKYHOOK_DIR"] = config.skyhookDir
	cmd := command.NewCommand(
		filepath.Join(config.stepRoot, s.ScriptPath),
		command.WithArguments(arguments...),
		command.WithWorkingDirectory(config.skyhookDir),
		command.WithEnvironment(environment),
		command.WithStdout(config.stdout),
		command.WithStderr(config.stderr),
	)

	commandResult, runErr := runner.Run(ctx, cmd)
	if runErr != nil {
		return StatusFailed, fmt.Errorf("running step %q command: %w", s.ScriptPath, runErr)
	}
	if commandResult.Signal != nil || !slices.Contains(s.Returncodes, commandResult.ExitCode) {
		return StatusFailed, nil
	}

	return StatusSuccess, nil
}

// Fingerprint returns a stable SHA-256 hex digest of the step's
// execution-relevant inputs. Nil and empty arguments, return codes, and
// env hash identically so hand-constructed and decoded steps agree.
func (s RegularStep) Fingerprint() (string, error) {
	arguments := s.Arguments
	if arguments == nil {
		arguments = []string{}
	}
	returnCodes := s.Returncodes
	if returnCodes == nil {
		returnCodes = []command.ExitCode{}
	}
	env := s.Env
	if env == nil {
		env = map[string]string{}
	}
	payload, err := json.Marshal(struct {
		Path        string             `json:"path"`
		Arguments   []string           `json:"arguments"`
		ReturnCodes []command.ExitCode `json:"returnCodes"`
		Env         map[string]string  `json:"env"`
		OnHost      bool               `json:"onHost"`
	}{
		Path:        s.ScriptPath,
		Arguments:   arguments,
		ReturnCodes: returnCodes,
		Env:         env,
		OnHost:      s.OnHost,
	})
	if err != nil {
		return "", fmt.Errorf("encoding step fingerprint: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

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
	if err := s.IdempotenceMode.Validate(); err != nil {
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
func WithReturncodes(codes []command.ExitCode) RegularStepOption {
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
	return func(s *RegularStep) { s.IdempotenceMode = i }
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
		s.Returncodes = []command.ExitCode{command.SuccessExitCode}
	}
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	if s.IdempotenceMode == "" {
		s.IdempotenceMode = Auto
	}
}

func (s RegularStep) resolveArguments() ([]string, []string) {
	arguments := slices.Clone(s.Arguments)
	missing := make([]string, 0)
	seenMissing := make(map[string]struct{})
	for index, argument := range arguments {
		name, found := strings.CutPrefix(argument, "env:")
		if !found {
			continue
		}
		value, ok := os.LookupEnv(name)
		if !ok {
			if _, seen := seenMissing[name]; !seen {
				missing = append(missing, name)
				seenMissing[name] = struct{}{}
			}
			continue
		}
		arguments[index] = value
	}
	return arguments, missing
}
