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

package context

import (
	"testing"
)

// TestGlobalFlags_Validate tests the validation logic
func TestGlobalFlags_Validate(t *testing.T) {
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
			flags := NewGlobalFlags()
			flags.OutputFormat = tt.outputFormat

			err := flags.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGlobalFlags_Namespace tests the namespace retrieval logic
func TestGlobalFlags_Namespace(t *testing.T) {
	tests := []struct {
		name          string
		setupFlags    func(*GlobalFlags)
		wantNamespace string
	}{
		{
			name: "default namespace",
			setupFlags: func(f *GlobalFlags) {
				// No changes, use default
			},
			wantNamespace: "skyhook",
		},
		{
			name: "custom namespace",
			setupFlags: func(f *GlobalFlags) {
				ns := "custom-ns"
				f.ConfigFlags.Namespace = &ns
			},
			wantNamespace: "custom-ns",
		},
		{
			name: "empty namespace string",
			setupFlags: func(f *GlobalFlags) {
				ns := ""
				f.ConfigFlags.Namespace = &ns
			},
			wantNamespace: "skyhook",
		},
		{
			name: "whitespace namespace",
			setupFlags: func(f *GlobalFlags) {
				ns := "  "
				f.ConfigFlags.Namespace = &ns
			},
			wantNamespace: "skyhook",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := NewGlobalFlags()
			tt.setupFlags(flags)

			got := flags.Namespace()
			if got != tt.wantNamespace {
				t.Errorf("Namespace() = %q, want %q", got, tt.wantNamespace)
			}
		})
	}
}
