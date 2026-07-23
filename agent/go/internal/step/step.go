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
	"fmt"

	"github.com/NVIDIA/nodewright/agent/internal/execution"
)

// Step is the interface satisfied by every concrete step type the
// agent recognises (RegularStep, UpgradeStep). Callers that must
// distinguish the kinds type-assert on UpgradeStep.
type Step interface {
	// Run executes the step using the supplied filesystem and output
	// composition.
	Run(context.Context, execution.Config) (execution.Status, error)

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
