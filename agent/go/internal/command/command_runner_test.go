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

package command

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Command runner executable permissions", func() {
	It("requires an absolute executable path", func() {
		_, err := NewRunner().Run(context.Background(), Command{
			Executable:  "script",
			Permissions: 0o755,
		})

		Expect(err).To(MatchError(ContainSubstring("validating command execution")))
		Expect(err).To(MatchError(ContainSubstring("permissions require an absolute executable path")))
	})

	It("wraps executable permission errors", func() {
		path := filepath.Join(GinkgoT().TempDir(), "missing")

		_, err := NewRunner().Run(context.Background(), Command{
			Executable:  path,
			Permissions: 0o755,
		})

		Expect(err).To(MatchError(ContainSubstring("applying command executable permissions")))
		Expect(errors.Is(err, fs.ErrNotExist)).To(BeTrue())
	})

	It("preserves executable permissions by default", func() {
		path := filepath.Join(GinkgoT().TempDir(), "script")
		Expect(os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0o555)).To(Succeed())
		Expect(os.Chmod(path, 0o555)).To(Succeed())

		result, err := NewRunner().Run(context.Background(), NewCommand(path))

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(SuccessExitCode))
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(fs.FileMode(0o555)))
	})

	It("sets the requested executable permissions", func() {
		path := filepath.Join(GinkgoT().TempDir(), "script")
		Expect(os.WriteFile(path, []byte("#!/bin/sh\nprintf executable"), 0o640)).To(Succeed())
		var stdout bytes.Buffer

		result, err := NewRunner().Run(context.Background(), Command{
			Executable:  path,
			Stdout:      &stdout,
			Permissions: 0o755,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(SuccessExitCode))
		Expect(stdout.String()).To(Equal("executable"))
		info, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(fs.FileMode(0o755)))
	})

	It("returns a permission error when execute bits are required but absent", func() {
		path := filepath.Join(GinkgoT().TempDir(), "script")
		Expect(os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0o600)).To(Succeed())

		_, err := NewRunner().Run(context.Background(), Command{Executable: path})

		Expect(err).To(MatchError(ContainSubstring("executing command")))
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, fs.ErrPermission)).To(BeTrue())
	})
})
