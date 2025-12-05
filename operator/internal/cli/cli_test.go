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
	"bytes"
	"testing"

	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestCLISmoke(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI Test Suite")
}

var _ = Describe("Skyhook CLI Tests", func() {
	var rootCmd *cobra.Command
	var testCtx *context.CLIContext
	var output *bytes.Buffer

	BeforeEach(func() {
		config := context.NewCLIConfig()
		testCtx = context.NewCLIContext(config)
		rootCmd = NewSkyhookCommand(testCtx)

		output = &bytes.Buffer{}
		rootCmd.SetOut(output)
		rootCmd.SetErr(output)
	})

	Describe("Command Structure", func() {
		It("should have all main commands registered", func() {
			commandNames := getCommandNames(rootCmd)
			Expect(commandNames).To(ContainElement("package"))
		})

		It("should have complete subcommand structure", func() {
			// Verify package subcommands
			packageCmd := findCommand(rootCmd, "package")
			Expect(packageCmd).NotTo(BeNil())
			packageSubs := getCommandNames(packageCmd)
			expectedPackageCommands := []string{
				"rerun <package-name>",
				"status <package-name>",
				"logs <package-name>",
			}
			for _, expected := range expectedPackageCommands {
				Expect(packageSubs).To(ContainElement(expected))
			}
		})
	})

	Describe("Help and Version", func() {
		It("should display help correctly", func() {
			rootCmd.SetArgs([]string{"--help"})
			err := rootCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
			helpText := output.String()
			Expect(helpText).To(ContainSubstring("kubectl-compatible helper for managing Skyhook deployments."))
			Expect(helpText).To(ContainSubstring("Available Commands:"))
		})

		It("should display version correctly", func() {
			rootCmd.SetArgs([]string{"--version"})
			err := rootCmd.Execute()
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).To(ContainSubstring("Skyhook plugin: "))
		})
	})

	Describe("Global Flags", func() {
		It("should have all flags bound with correct properties", func() {
			persistentFlags := rootCmd.PersistentFlags()

			// Verify all flags exist with correct shortcuts
			Expect(persistentFlags.Lookup("verbose")).NotTo(BeNil())
			Expect(persistentFlags.Lookup("dry-run")).NotTo(BeNil())
		})

		It("should bind flags to config.Flags struct", func() {
			pflags := rootCmd.PersistentFlags()

			// Set flags and verify they update config.Flags
			Expect(pflags.Set("verbose", "true")).To(Succeed())
			Expect(pflags.Set("dry-run", "true")).To(Succeed())
			Expect(testCtx.GlobalFlags.Verbose).To(BeTrue())
			Expect(testCtx.GlobalFlags.DryRun).To(BeTrue())
		})
	})

	Describe("Error Handling", func() {
		It("should handle invalid commands and arguments", func() {
			rootCmd.SetArgs([]string{"invalid-command"})
			err := rootCmd.Execute()
			Expect(err).To(HaveOccurred())
		})
	})
})

func findCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Use == name || cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func getCommandNames(cmd *cobra.Command) []string {
	commands := cmd.Commands()
	names := make([]string, len(commands))
	for i, cmd := range commands {
		names[i] = cmd.Use
	}
	return names
}
