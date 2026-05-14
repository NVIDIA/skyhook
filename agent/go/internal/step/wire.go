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

import (
	"encoding/json"
	"fmt"
)

// decodePayload is the wire shape Decode reads. Booleans are *bool so
// we can distinguish "field absent" (apply default) from "field present
// and false" (keep false), matching Python's data.get(...) semantics.
type decodePayload struct {
	Name              string            `json:"name"`
	Path              string            `json:"path"`
	Arguments         []string          `json:"arguments"`
	OnHost            *bool             `json:"on_host"`
	Returncodes       []int             `json:"returncodes"`
	Env               map[string]string `json:"env,omitempty"`
	Idempotence       *bool             `json:"idempotence"`
	RequiresInterrupt *bool             `json:"requires_interrupt,omitempty"`
	UpgradeStep       *bool             `json:"upgrade_step"`
}

// encodePayload is the wire shape both RegularStep.Encode and
// UpgradeStep.Encode write through marshalRegularStep. Field order is
// significant: pins golden-byte tests against Python output.
type encodePayload struct {
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Arguments   []string          `json:"arguments"`
	Returncodes []int             `json:"returncodes"`
	OnHost      bool              `json:"on_host"`
	Idempotence bool              `json:"idempotence"`
	UpgradeStep bool              `json:"upgrade_step"`
	Env         map[string]string `json:"env,omitempty"`
}

// Decode parses a JSON step payload, returning either a RegularStep or
// an UpgradeStep based on the upgrade_step discriminator. The Go
// contract is intentionally looser than Python's: Python's load
// requires "idempotence" to be present and KeyErrors otherwise; Go
// treats it as optional and defaults to Auto. UpgradeStep invariant
// violations are propagated from NewUpgradeStep.
func Decode(data []byte) (Step, error) {
	var payload decodePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal step: %w", err)
	}

	// Pointer fields are only applied when present so we preserve
	// "absent vs explicit false" matching Python's data.get(...).
	opts := []RegularStepOption{
		WithName(payload.Name),
		WithArguments(payload.Arguments),
		WithReturncodes(payload.Returncodes),
		WithEnv(payload.Env),
	}
	if payload.OnHost != nil {
		opts = append(opts, WithOnHost(*payload.OnHost))
	}
	if payload.Idempotence != nil && *payload.Idempotence {
		opts = append(opts, WithIdempotence(Disabled))
	}
	if payload.RequiresInterrupt != nil {
		opts = append(opts, WithRequiresInterrupt(*payload.RequiresInterrupt))
	}

	if payload.UpgradeStep != nil && *payload.UpgradeStep {
		return NewUpgradeStep(payload.Path, opts...)
	}
	return NewRegularStep(payload.Path, opts...), nil
}

// marshalRegularStep writes the JSON wire form for a RegularStep with
// the given upgrade_step discriminator. Used by RegularStep.Encode and
// UpgradeStep.Encode so the wire shape lives in exactly one place.
func marshalRegularStep(s RegularStep, upgrade bool) ([]byte, error) {
	var env map[string]string
	if len(s.env) != 0 {
		env = s.env
	}

	data, err := json.Marshal(encodePayload{
		Name:        s.name,
		Path:        s.path,
		Arguments:   s.arguments,
		Returncodes: s.returncodes,
		OnHost:      s.onHost,
		Idempotence: s.idempotence == Disabled,
		UpgradeStep: upgrade,
		Env:         env,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal step: %w", err)
	}
	return data, nil
}
