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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

var _ = Describe("Enable Command", func() {
	Describe("NewEnableCmd", func() {
		It("should create command with correct properties", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewEnableCmd(ctx)

			Expect(cmd.Use).To(Equal("enable <skyhook-name>"))
			Expect(cmd.Short).To(ContainSubstring("Enable"))
		})

		It("should not have confirm flag (enable is safe)", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewEnableCmd(ctx)

			confirmFlag := cmd.Flags().Lookup("confirm")
			Expect(confirmFlag).To(BeNil())
		})

		It("should require exactly one argument", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewEnableCmd(ctx)

			err := cmd.Args(cmd, []string{})
			Expect(err).To(HaveOccurred())

			err = cmd.Args(cmd, []string{"skyhook1"})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should have examples in help", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewEnableCmd(ctx)

			Expect(cmd.Example).To(ContainSubstring("kubectl skyhook enable"))
		})
	})
})
