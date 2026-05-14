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

// UpgradeStep is a Step that runs only during the Upgrade and
// UpgradeCheck modes. It mirrors Python's UpgradeStep subclass.
//
// Invariant: UpgradeSteps may not declare arguments. Construct via
// NewUpgradeStep or call Validate() before encoding.
type UpgradeStep struct {
	RegularStep
}

var _ Step = UpgradeStep{}

// NewUpgradeStep constructs an UpgradeStep using the same option set
// as NewRegularStep, then checks the no-arguments invariant. The
// shared option set keeps construction symmetric; if a caller passes
// WithArguments, the Python-parity error is returned at runtime
// (matching Python's UpgradeStep.__init__ which raises StepError).
func NewUpgradeStep(path string, opts ...RegularStepOption) (UpgradeStep, error) {
	u := UpgradeStep{RegularStep: NewRegularStep(path, opts...)}
	if err := u.Validate(); err != nil {
		return UpgradeStep{}, err
	}
	return u, nil
}

// Validate reports whether u satisfies the UpgradeStep invariant:
// arguments must be empty. Mirrors Python's StepError raised in
// UpgradeStep.__init__.
func (u UpgradeStep) Validate() error {
	if len(u.arguments) != 0 {
		return fmt.Errorf(
			"UpgradeStep %s can not have any arguments, but found: %v",
			u.name, u.arguments,
		)
	}
	return nil
}

// Encode overrides RegularStep.Encode to validate the no-arguments
// invariant and write upgrade_step:true. Without this override, Go's
// method promotion would silently call RegularStep.Encode and emit
// upgrade_step:false.
func (u UpgradeStep) Encode() ([]byte, error) {
	if err := u.Validate(); err != nil {
		return nil, err
	}
	u.RegularStep.applyDefaults()
	if err := u.idempotence.Validate(); err != nil {
		return nil, err
	}
	return marshalRegularStep(u.RegularStep, true)
}
