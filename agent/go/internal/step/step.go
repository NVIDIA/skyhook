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

import "fmt"

// Step is the interface satisfied by every concrete step type the
// agent recognises. Callers that need fields use the getters; callers
// that need to distinguish the two concrete kinds use a type switch
// (or check `_, ok := s.(UpgradeStep)`).
type Step interface {
	Path() string
	Name() string
	Arguments() []string
	Returncodes() []int
	Env() map[string]string
	OnHost() bool
	Idempotence() Idempotence
	RequiresInterrupt() bool

	// Encode serializes the step to its JSON wire form. Each concrete
	// type owns its own discriminator bit and any invariants it
	// enforces before marshaling.
	Encode() ([]byte, error)
}

// Mode identifies an agent lifecycle mode.
type Mode string

const (
	Uninstall          Mode = "uninstall"
	UninstallCheck     Mode = "uninstall-check"
	Upgrade            Mode = "upgrade"
	UpgradeCheck       Mode = "upgrade-check"
	Apply              Mode = "apply"
	ApplyCheck         Mode = "apply-check"
	Config             Mode = "config"
	ConfigCheck        Mode = "config-check"
	Interrupt          Mode = "interrupt"
	PostInterrupt      Mode = "post-interrupt"
	PostInterruptCheck Mode = "post-interrupt-check"
)

// ApplyToCheck maps each non-check mode to its check counterpart.
var ApplyToCheck = map[Mode]Mode{
	Uninstall:     UninstallCheck,
	Upgrade:       UpgradeCheck,
	Apply:         ApplyCheck,
	Config:        ConfigCheck,
	PostInterrupt: PostInterruptCheck,
}

// CheckToApply maps each check mode to its non-check counterpart.
var CheckToApply = map[Mode]Mode{
	UninstallCheck:     Uninstall,
	UpgradeCheck:       Upgrade,
	ApplyCheck:         Apply,
	ConfigCheck:        Config,
	PostInterruptCheck: PostInterrupt,
}

// nonStepModes is the Go counterpart to Python's NON_STEP_MODES list.
// A "non-step" mode is one the agent dispatches without an apply/check
// companion: Interrupt today, possibly more in the future. Adding a
// mode here is enough to exclude it from IsStepMode.
var nonStepModes = map[Mode]struct{}{
	Interrupt: {},
}

// IsStepMode reports whether m carries normal step/check pairs.
// Non-step modes (see nonStepModes) are dispatched on their own.
func IsStepMode(m Mode) bool {
	_, isNonStep := nonStepModes[m]
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
// with Mode validation. Callers comparing error strings across the
// language boundary must account for this.
func (i Idempotence) Validate() error {
	switch i {
	case Auto, Disabled:
		return nil
	default:
		return fmt.Errorf("%s is not a valid idempotence value", i)
	}
}
