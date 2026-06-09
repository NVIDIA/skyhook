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
// agent recognises. Concrete types (RegularStep, UpgradeStep) expose
// their data as exported fields; callers that need to distinguish the
// two kinds use a type switch (or check `_, ok := s.(UpgradeStep)`).
type Step interface {
	// Encode serializes the step to its JSON wire form. Each concrete
	// type owns its own discriminator bit and any invariants it
	// enforces before marshaling.
	Encode() ([]byte, error)
}

// Stage identifies an agent lifecycle stage.
type Stage string

const (
	Uninstall          Stage = "uninstall"
	UninstallCheck     Stage = "uninstall-check"
	Upgrade            Stage = "upgrade"
	UpgradeCheck       Stage = "upgrade-check"
	Apply              Stage = "apply"
	ApplyCheck         Stage = "apply-check"
	Config             Stage = "config"
	ConfigCheck        Stage = "config-check"
	Interrupt          Stage = "interrupt"
	PostInterrupt      Stage = "post-interrupt"
	PostInterruptCheck Stage = "post-interrupt-check"
)

// ApplyToCheck maps each non-check stage to its check counterpart.
var ApplyToCheck = map[Stage]Stage{
	Uninstall:     UninstallCheck,
	Upgrade:       UpgradeCheck,
	Apply:         ApplyCheck,
	Config:        ConfigCheck,
	PostInterrupt: PostInterruptCheck,
}

// CheckToApply maps each check stage to its non-check counterpart.
var CheckToApply = map[Stage]Stage{
	UninstallCheck:     Uninstall,
	UpgradeCheck:       Upgrade,
	ApplyCheck:         Apply,
	ConfigCheck:        Config,
	PostInterruptCheck: PostInterrupt,
}

// nonStepStages is the Go counterpart to Python's NON_STEP_MODES list.
// A "non-step" stage is one the agent dispatches without an apply/check
// companion: Interrupt today, possibly more in the future. Adding a
// stage here is enough to exclude it from IsStepStage.
var nonStepStages = map[Stage]struct{}{
	Interrupt: {},
}

// IsStepStage reports whether s carries normal step/check pairs.
// Non-step stages (see nonStepStages) are dispatched on their own.
func IsStepStage(s Stage) bool {
	_, isNonStep := nonStepStages[s]
	return !isNonStep
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
//
// The error message intentionally differs from Python's
// skyhook_agent.step.Idempotence.validate, which says "is not a valid
// mode". The Go form is more specific so log readers don't confuse it
// with Stage validation. Callers comparing error strings across the
// language boundary must account for this.
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

// Decode parses a JSON step payload, returning either a RegularStep or
// an UpgradeStep based on the upgrade_step discriminator. Stdlib drives
// field parsing (the exported tags + the Idempotence codec); Decode
// routes on the discriminator and applies defaults.
//
// The probe's on_host is *bool so Decode can distinguish "field absent"
// (default to true, matching Python's Step(on_host=True)) from "present
// and false" - the plain bool field on RegularStep can't carry that
// distinction, so the lenient default lives here at the gate.
//
// The Go contract is otherwise intentionally looser than Python's:
// Python's load requires "idempotence" to be present and KeyErrors
// otherwise; Go treats it as optional and defaults to Auto. UpgradeStep
// invariant violations are propagated from Validate.
func Decode(data []byte) (Step, error) {
	var probe struct {
		OnHost      *bool `json:"on_host"`
		UpgradeStep bool  `json:"upgrade_step"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("decode step: %w", err)
	}

	if probe.UpgradeStep {
		var u UpgradeStep
		if err := json.Unmarshal(data, &u); err != nil {
			return nil, fmt.Errorf("decode upgrade step: %w", err)
		}
		if probe.OnHost == nil {
			u.OnHost = true
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
	if probe.OnHost == nil {
		rs.OnHost = true
	}
	rs.applyDefaults()
	return rs, nil
}
