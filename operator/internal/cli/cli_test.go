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

package cli

import (
	"testing"

	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

// TestNewSkyhookCommand verifies the root command is properly configured
func TestNewSkyhookCommand(t *testing.T) {
	ctx := context.NewCLIContext(nil)
	cmd := NewSkyhookCommand(ctx)

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
