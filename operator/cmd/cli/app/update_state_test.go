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

package app

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/NVIDIA/nodewright/operator/internal/cli/context"
)

var _ = Describe("UpdateState Command", func() {
	Describe("NewUpdateStateCmd", func() {
		It("creates the command with the expected use string and arg count", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewUpdateStateCmd(ctx)
			Expect(cmd.Use).To(Equal("update-state <skyhook-name> <package> <version> <stage> <state>"))
			Expect(cmd.Args).NotTo(BeNil())
			Expect(cmd.Args(cmd, []string{"a", "b", "c", "d"})).To(HaveOccurred())
			Expect(cmd.Args(cmd, []string{"a", "b", "c", "d", "e"})).NotTo(HaveOccurred())
		})

		It("registers --node, --selector, --confirm, --add flags", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewUpdateStateCmd(ctx)
			Expect(cmd.Flags().Lookup("node")).NotTo(BeNil())
			Expect(cmd.Flags().Lookup("selector")).NotTo(BeNil())
			Expect(cmd.Flags().Lookup("confirm")).NotTo(BeNil())
			Expect(cmd.Flags().Lookup("add")).NotTo(BeNil())
		})

		It("marks --node and --selector mutually exclusive", func() {
			ctx := context.NewCLIContext(nil)
			cmd := NewUpdateStateCmd(ctx)
			cmd.SetArgs([]string{"sh", "p", "1.0", "config", "complete", "--node", "n1", "--selector", "role=gpu"})
			err := cmd.Execute()
			Expect(err).To(HaveOccurred())
			// cobra's MarkFlagsMutuallyExclusive emits this exact phrasing
			Expect(err.Error()).To(MatchRegexp(`(?i)none of the others can be`))
		})
	})
})
