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

package interrupts

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// invalidSerializedInterruptError mirrors the Python ValueError message so
// callers comparing error text across language boundaries stay compatible.
const invalidSerializedInterruptError = "serialized interrupt must be base64 encoded {'type': str, **kwargs: dict[str, any]}"

// Interrupt is the pure data contract for an interrupt type.
type Interrupt interface {
	Type() string
	InterruptCmd() [][]string
	Serialize() ([]byte, error)
}

// Encode serializes an Interrupt to the base64+JSON wire form expected
// by the operator's interrupt controller input. A nil receiver is
// rejected explicitly so a misuse from a caller returns a typed error
// rather than panicking on a nil interface method call.
func Encode(i Interrupt) (string, error) {
	if i == nil {
		return "", errors.New("nil interrupt")
	}
	data, err := i.Serialize()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// Decode parses a base64+JSON serialized interrupt produced by Encode
// (or its Python counterpart skyhook_agent.interrupts.inflate).
// Wire-shape errors return invalidSerializedInterruptError; unknown
// type names return a descriptive error listing the supported types.
func Decode(serializedValue string) (Interrupt, error) {
	data, err := base64.StdEncoding.DecodeString(serializedValue)
	if err != nil {
		return nil, errors.New(invalidSerializedInterruptError)
	}

	// *string distinguishes "type field absent" (Python KeyError →
	// wire-shape error) from "type field present but empty" (Python
	// dispatch miss → unknown-type error).
	var head struct {
		Type *string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, errors.New(invalidSerializedInterruptError)
	}
	if head.Type == nil {
		return nil, errors.New(invalidSerializedInterruptError)
	}

	switch *head.Type {
	case NodeRestart{}.Type():
		return NodeRestart{}, nil
	case ServiceRestart{}.Type():
		var wire struct {
			Services []string `json:"services"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, errors.New(invalidSerializedInterruptError)
		}
		return ServiceRestart{Services: wire.Services}, nil
	case RestartAllServices{}.Type():
		return RestartAllServices{}, nil
	case NoOp{}.Type():
		return NoOp{}, nil
	case ScriptInterrupt{}.Type():
		return ScriptInterrupt{}, nil
	default:
		return nil, fmt.Errorf(
			"unknown interrupt %q must be one of: %s",
			*head.Type,
			supportedTypes(),
		)
	}
}

// supportedTypes returns the comma-separated list of known interrupt
// type names in the order Decode dispatches on them.
func supportedTypes() string {
	return "node_restart, service_restart, restart_all_services, no_op, script_interrupt"
}

// marshalTypeOnly serializes an interrupt whose wire form is just
// {"type": "..."}.
func marshalTypeOnly(typeName string) ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: typeName})
}
