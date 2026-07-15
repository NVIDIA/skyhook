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
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Chroot command runner", func() {
	It("wraps validation errors", func() {
		_, err := NewRunner(WithChroot(GinkgoT().TempDir())).Run(context.Background(), Command{})

		Expect(err).To(MatchError(ContainSubstring("validating chroot command execution")))
	})

	It("wraps executable permission errors", func() {
		_, err := NewRunner(WithChroot(GinkgoT().TempDir())).Run(context.Background(), Command{
			Executable:  "/missing",
			Permissions: 0o755,
		})

		Expect(err).To(MatchError(ContainSubstring("applying chroot command executable permissions")))
		Expect(errors.Is(err, fs.ErrNotExist)).To(BeTrue())
	})

	It("changes permissions relative to the chroot", func() {
		root := GinkgoT().TempDir()
		path := filepath.Join(root, "script")
		Expect(os.WriteFile(path, []byte("not runnable in the test chroot"), 0o600)).To(Succeed())

		_, err := NewRunner(WithChroot(root)).Run(context.Background(), Command{
			Executable:  "/script",
			Permissions: 0o755,
		})

		Expect(err).To(MatchError(ContainSubstring("executing chroot command")))
		info, statErr := os.Stat(path)
		Expect(statErr).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(fs.FileMode(0o755)))
	})
})

var _ = Describe("Chroot PATH lookup", func() {
	It("searches every PATH entry inside the target root", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "first"), 0o755)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(rootPath, "second"), 0o755)).To(Succeed())
		writeExecutable(filepath.Join(rootPath, "second", "path-tool"))
		root := openTestRoot(rootPath)

		executable, err := lookPathInRoot(root, "/", "/first:/second", "path-tool")

		Expect(err).NotTo(HaveOccurred())
		Expect(executable).To(Equal("/second/path-tool"))
	})

	It("does not fall back to the host filesystem", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "bin"), 0o755)).To(Succeed())

		_, err := lookPathInRoot(openTestRoot(rootPath), "/", "/bin", "sh")

		Expect(err).To(MatchError(ContainSubstring("executable \"sh\" not found in PATH")))
	})

	It("returns symlinks for the kernel to resolve inside the chroot", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "bin"), 0o755)).To(Succeed())
		Expect(os.Symlink("/real/tool", filepath.Join(rootPath, "bin", "tool"))).To(Succeed())

		executable, err := lookPathInRoot(openTestRoot(rootPath), "/", "/bin", "tool")

		Expect(err).NotTo(HaveOccurred())
		Expect(executable).To(Equal("/bin/tool"))
	})

	It("resolves relative PATH entries against the working directory", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(rootPath, "work", "bin"), 0o755)).To(Succeed())
		writeExecutable(filepath.Join(rootPath, "work", "bin", "tool"))

		executable, err := lookPathInRoot(openTestRoot(rootPath), "/work", "bin", "tool")

		Expect(err).NotTo(HaveOccurred())
		Expect(executable).To(Equal("/work/bin/tool"))
	})

	It("does not search the working directory for an empty PATH", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "work"), 0o755)).To(Succeed())
		writeExecutable(filepath.Join(rootPath, "work", "tool"))

		_, err := lookPathInRoot(openTestRoot(rootPath), "/work", "", "tool")

		Expect(err).To(MatchError(ContainSubstring("executable \"tool\" not found in PATH")))
	})

	It("uses the working directory for an empty PATH component", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "work"), 0o755)).To(Succeed())
		writeExecutable(filepath.Join(rootPath, "work", "tool"))

		executable, err := lookPathInRoot(openTestRoot(rootPath), "/work", ":", "tool")

		Expect(err).NotTo(HaveOccurred())
		Expect(executable).To(Equal("/work/tool"))
	})

	It("continues past non-executable candidates", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "first"), 0o755)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(rootPath, "second"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(rootPath, "first", "tool"), nil, 0o600)).To(Succeed())
		writeExecutable(filepath.Join(rootPath, "second", "tool"))

		executable, err := lookPathInRoot(openTestRoot(rootPath), "/", "/first:/second", "tool")

		Expect(err).NotTo(HaveOccurred())
		Expect(executable).To(Equal("/second/tool"))
	})

	It("returns a permission error when no candidate is executable", func() {
		rootPath := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(rootPath, "bin"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(rootPath, "bin", "tool"), nil, 0o600)).To(Succeed())

		_, err := lookPathInRoot(openTestRoot(rootPath), "/", "/bin", "tool")

		Expect(errors.Is(err, fs.ErrPermission)).To(BeTrue())
	})
})

func openTestRoot(path string) *os.Root {
	root, err := os.OpenRoot(path)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(root.Close()).To(Succeed())
	})
	return root
}

func writeExecutable(path string) {
	Expect(os.WriteFile(path, []byte("executable"), 0o755)).To(Succeed())
}
