/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package cmd

import (
	"testing"
)

// TestNewSkyhookCommand verifies the root command is properly configured
func TestNewSkyhookCommand(t *testing.T) {
	opts := NewGlobalOptions()
	cmd := NewSkyhookCommand(opts)

	if cmd == nil {
		t.Fatal("NewSkyhookCommand returned nil")
	}

	// Verify version command is registered
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("version command not registered")
	}
}

// TestGlobalOptions_Validate tests the validation logic
func TestGlobalOptions_Validate(t *testing.T) {
	tests := []struct {
		name         string
		outputFormat string
		wantErr      bool
	}{
		{"valid json", "json", false},
		{"valid yaml", "yaml", false},
		{"valid table", "table", false},
		{"valid wide", "wide", false},
		{"case insensitive", "JSON", false},
		{"invalid format", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewGlobalOptions()
			opts.OutputFormat = tt.outputFormat

			err := opts.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
