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

package pkg

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

func TestPackage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Package CLI Tests Suite")
}

var _ = Describe("Package Commands", func() {
	var packageCmd *cobra.Command

	BeforeEach(func() {
		testCtx := context.NewCLIContext(nil)
		packageCmd = NewPackageCmd(testCtx)
	})

	Describe("Parent Command", func() {
		It("should have correct structure and all subcommands", func() {
			Expect(packageCmd.Use).To(Equal("package"))
			Expect(packageCmd.Short).To(Equal("Manage Skyhook packages"))

			subcommands := getSubcommandNames(packageCmd)
			expectedCommands := []string{
				"rerun <package-name>",
				"status <package-name>",
				"logs <package-name>",
			}

			for _, expected := range expectedCommands {
				Expect(subcommands).To(ContainElement(expected))
			}
		})
	})

	Describe("Command Properties", func() {
		It("should have rerun command requiring exactly one argument", func() {
			rerunCmd := findSubcommand(packageCmd, "rerun <package-name>")
			Expect(rerunCmd).NotTo(BeNil())
			err := rerunCmd.Args(rerunCmd, []string{})
			Expect(err).To(HaveOccurred())
			err = rerunCmd.Args(rerunCmd, []string{"package-name"})
			Expect(err).NotTo(HaveOccurred())

			// Test for flags
			skyhookFlag := rerunCmd.Flags().Lookup("skyhook")
			Expect(skyhookFlag).NotTo(BeNil())
			nodeFlag := rerunCmd.Flags().Lookup("node")
			Expect(nodeFlag).NotTo(BeNil())
			stageFlag := rerunCmd.Flags().Lookup("stage")
			Expect(stageFlag).NotTo(BeNil())
			confirmFlag := rerunCmd.Flags().Lookup("confirm")
			Expect(confirmFlag).NotTo(BeNil())
		})

		It("should have status command requiring exactly one argument", func() {
			statusCmd := findSubcommand(packageCmd, "status <package-name>")
			Expect(statusCmd).NotTo(BeNil())
			err := statusCmd.Args(statusCmd, []string{})
			Expect(err).To(HaveOccurred())
			err = statusCmd.Args(statusCmd, []string{"package-name"})
			Expect(err).NotTo(HaveOccurred())

			// Test for flags
			skyhookFlag := statusCmd.Flags().Lookup("skyhook")
			Expect(skyhookFlag).NotTo(BeNil())
			nodeFlag := statusCmd.Flags().Lookup("node")
			Expect(nodeFlag).NotTo(BeNil())
			outputFlag := statusCmd.Flags().Lookup("output")
			Expect(outputFlag).NotTo(BeNil())
		})

		It("should have logs command requiring exactly one argument", func() {
			logsCmd := findSubcommand(packageCmd, "logs <package-name>")
			Expect(logsCmd).NotTo(BeNil())
			err := logsCmd.Args(logsCmd, []string{})
			Expect(err).To(HaveOccurred())
			err = logsCmd.Args(logsCmd, []string{"package-name"})
			Expect(err).NotTo(HaveOccurred())

			// Test for flags
			skyhookFlag := logsCmd.Flags().Lookup("skyhook")
			Expect(skyhookFlag).NotTo(BeNil())
			nodeFlag := logsCmd.Flags().Lookup("node")
			Expect(nodeFlag).NotTo(BeNil())
			stageFlag := logsCmd.Flags().Lookup("stage")
			Expect(stageFlag).NotTo(BeNil())
			followFlag := logsCmd.Flags().Lookup("follow")
			Expect(followFlag).NotTo(BeNil())
			tailFlag := logsCmd.Flags().Lookup("tail")
			Expect(tailFlag).NotTo(BeNil())
		})
	})

	Describe("Arguments Validation", func() {
		It("should validate required arguments for commands that need them", func() {
			commandsRequiringArgs := []string{
				"rerun <package-name>",
				"status <package-name>",
				"logs <package-name>",
			}

			for _, cmdName := range commandsRequiringArgs {
				cmd := findSubcommand(packageCmd, cmdName)
				Expect(cmd).NotTo(BeNil(), "Command should exist: %s", cmdName)
				if cmd.Args != nil {
					err := cmd.Args(cmd, []string{}) // Test with no args
					Expect(err).To(HaveOccurred(), "Command should require arguments: %s", cmdName)
				}
			}
		})
	})
})

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Use == name || cmd.Name() == name {
			return cmd
		}
	}
	return nil
}

func getSubcommandNames(cmd *cobra.Command) []string {
	subcommands := cmd.Commands()
	names := make([]string, len(subcommands))
	for i, cmd := range subcommands {
		names[i] = cmd.Use
	}
	return names
}
