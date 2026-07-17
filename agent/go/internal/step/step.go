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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Step is the interface satisfied by every concrete step type the
// agent recognises (RegularStep, UpgradeStep). Callers that must
// distinguish the kinds type-assert on UpgradeStep.
type Step interface {
	// Run executes the step using the supplied filesystem and output
	// composition.
	Run(context.Context, RunConfig) (Status, error)

	// Encode serializes the step to its JSON wire form. Each concrete
	// type owns its own discriminator bit and any invariants it
	// enforces before marshaling.
	Encode() ([]byte, error)

	// Path is the step's script path, relative to the package root.
	Path() string

	// Fingerprint is a stable SHA-256 hex digest of the step's
	// execution-relevant inputs (path, arguments, return codes, env,
	// host execution); changing any of them changes the fingerprint.
	Fingerprint() (string, error)

	// Idempotence reports how the agent treats re-runs of the step.
	Idempotence() Idempotence
}

// Status reports whether a step run satisfied its execution policy.
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

// RunConfig contains the execution policy shared by steps in one agent run.
// Construct it with NewRunConfig.
type RunConfig struct {
	rootMount  string
	stepRoot   string
	skyhookDir string
	stdout     io.Writer
	stderr     io.Writer
}

func (config RunConfig) validate() error {
	if !filepath.IsAbs(config.rootMount) {
		return fmt.Errorf("root mount %q is not absolute", config.rootMount)
	}
	if !filepath.IsAbs(config.stepRoot) {
		return fmt.Errorf("step root %q is not absolute", config.stepRoot)
	}
	if !filepath.IsAbs(config.skyhookDir) {
		return fmt.Errorf("skyhook directory %q is not absolute", config.skyhookDir)
	}
	if config.stdout == nil {
		return errors.New("stdout writer is nil")
	}
	if config.stderr == nil {
		return errors.New("stderr writer is nil")
	}
	return nil
}

// RunOption configures a RunConfig.
type RunOption func(*RunConfig)

// NewRunConfig constructs step execution policy.
func NewRunConfig(options ...RunOption) (RunConfig, error) {
	config := RunConfig{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, option := range options {
		option(&config)
	}
	if err := config.validate(); err != nil {
		return RunConfig{}, fmt.Errorf("constructing step run config: %w", err)
	}
	return config, nil
}

// WithRootMount sets the absolute host root used by host-executed steps.
func WithRootMount(rootMount string) RunOption {
	return func(config *RunConfig) {
		config.rootMount = rootMount
	}
}

// WithStepRoot sets the child-visible absolute path containing step scripts.
func WithStepRoot(stepRoot string) RunOption {
	return func(config *RunConfig) {
		config.stepRoot = stepRoot
	}
}

// WithSkyhookDir sets the child-visible absolute package working directory.
func WithSkyhookDir(skyhookDir string) RunOption {
	return func(config *RunConfig) {
		config.skyhookDir = skyhookDir
	}
}

// WithRunOutput sets the parent streams that receive raw command output. A nil
// writer discards its corresponding stream.
func WithRunOutput(stdout, stderr io.Writer) RunOption {
	return func(config *RunConfig) {
		if stdout == nil {
			stdout = io.Discard
		}
		if stderr == nil {
			stderr = io.Discard
		}
		config.stdout = stdout
		config.stderr = stderr
	}
}

// Decode parses a JSON step payload, returning either a RegularStep or an
// UpgradeStep based on the upgrade_step discriminator. Stdlib drives field
// parsing (the exported tags + the Idempotence codec); Decode routes on the
// discriminator and applies defaults.
//
// Missing JSON fields retain their Go zero values unless applyDefaults handles
// them. Full package configs are schema-validated before they reach Decode, so
// required fields such as on_host are present on that path.
func Decode(data []byte) (Step, error) {
	var probe struct {
		UpgradeStep bool `json:"upgrade_step"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode step: %w", err)
	}

	if probe.UpgradeStep {
		var u UpgradeStep
		if err := json.Unmarshal(data, &u); err != nil {
			return nil, fmt.Errorf("decode upgrade step: %w", err)
		}
		u.RegularStep.applyDefaults()
		if err := u.Validate(); err != nil {
			return nil, fmt.Errorf("upgrade step validation failed: %w", err)
		}
		return u, nil
	}

	var rs RegularStep
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("decode step: %w", err)
	}
	rs.applyDefaults()
	return rs, nil
}
