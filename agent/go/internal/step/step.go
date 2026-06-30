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

// Step is the interface satisfied by every concrete step type the
// agent recognises (RegularStep, UpgradeStep). Callers that must
// distinguish the kinds type-assert on UpgradeStep.
type Step interface {
	// Encode serializes the step to its JSON wire form. Each concrete
	// type owns its own discriminator bit and any invariants it
	// enforces before marshaling.
	Encode() ([]byte, error)

	// Path is the step's script path, relative to the package root.
	Path() string
}

// Idempotence controls how the agent treats step re-runs.
type Idempotence string

const (
	// Auto means the agent runs the step once or until success.
	Auto Idempotence = "auto"
	// Disabled means the step itself decides; the agent may invoke it
	// many times.
	Disabled Idempotence = "disabled"
)

// Validate reports whether the receiver is a recognised value.
func (i Idempotence) Validate() error {
	switch i {
	case Auto, Disabled:
		return nil
	default:
		return fmt.Errorf("%s is not a valid idempotence value", i)
	}
}

// MarshalJSON encodes idempotence as the wire bool Python uses:
// true == Disabled (the step manages its own idempotence), false ==
// Auto. Owning the codec here lets the step structs serialize via
// stdlib without a custom marshaler.
func (i Idempotence) MarshalJSON() ([]byte, error) {
	return json.Marshal(i == Disabled)
}

// UnmarshalJSON decodes the wire bool back into the enum: true ->
// Disabled, false (or absent) -> Auto.
func (i *Idempotence) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err != nil {
		return fmt.Errorf("unmarshal idempotence: %w", err)
	}
	if b {
		*i = Disabled
	} else {
		*i = Auto
	}
	return nil
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
