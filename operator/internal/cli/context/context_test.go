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
	"bytes"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context CLI Tests Suite")
}

var _ = Describe("CLI Context", func() {
	Describe("GlobalFlags", func() {
		Describe("NewGlobalFlags", func() {
			It("should initialize with default namespace", func() {
				flags := NewGlobalFlags()
				Expect(flags.ConfigFlags.Namespace).NotTo(BeNil())
				Expect(*flags.ConfigFlags.Namespace).To(Equal("skyhook"))
			})

			It("should initialize with default output format", func() {
				flags := NewGlobalFlags()
				Expect(flags.OutputFormat).To(Equal("table"))
			})
		})

		Describe("AddFlags", func() {
			It("should register output flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				outputFlag := flagset.Lookup("output")
				Expect(outputFlag).NotTo(BeNil())
				Expect(outputFlag.Shorthand).To(Equal("o"))
			})

			It("should register verbose flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				verboseFlag := flagset.Lookup("verbose")
				Expect(verboseFlag).NotTo(BeNil())
				Expect(verboseFlag.Shorthand).To(Equal("v"))
			})

			It("should register dry-run flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				dryRunFlag := flagset.Lookup("dry-run")
				Expect(dryRunFlag).NotTo(BeNil())
			})

			It("should bind flags to struct fields", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				Expect(flagset.Set("output", "json")).To(Succeed())
				Expect(flagset.Set("verbose", "true")).To(Succeed())
				Expect(flagset.Set("dry-run", "true")).To(Succeed())

				Expect(flags.OutputFormat).To(Equal("json"))
				Expect(flags.Verbose).To(BeTrue())
				Expect(flags.DryRun).To(BeTrue())
			})
		})

		Describe("Validate", func() {
			It("should accept valid output formats", func() {
				for _, format := range []string{"json", "yaml", "table", "wide"} {
					flags := NewGlobalFlags()
					flags.OutputFormat = format
					Expect(flags.Validate()).To(Succeed())
				}
			})

			It("should be case insensitive", func() {
				flags := NewGlobalFlags()
				flags.OutputFormat = "JSON"
				Expect(flags.Validate()).To(Succeed())
				Expect(flags.OutputFormat).To(Equal("json"))
			})

			It("should reject invalid output formats", func() {
				flags := NewGlobalFlags()
				flags.OutputFormat = "invalid"
				err := flags.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid output format"))
			})
		})

		Describe("Namespace", func() {
			It("should return default namespace when not set", func() {
				flags := NewGlobalFlags()
				Expect(flags.Namespace()).To(Equal("skyhook"))
			})

			It("should return custom namespace when set", func() {
				flags := NewGlobalFlags()
				ns := "custom-ns"
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("custom-ns"))
			})

			It("should return default namespace for empty string", func() {
				flags := NewGlobalFlags()
				ns := ""
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("skyhook"))
			})

			It("should return default namespace for whitespace", func() {
				flags := NewGlobalFlags()
				ns := "  "
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("skyhook"))
			})

			It("should return default namespace when nil", func() {
				flags := NewGlobalFlags()
				flags.ConfigFlags.Namespace = nil
				Expect(flags.Namespace()).To(Equal("skyhook"))
			})
		})
	})

	Describe("CLIConfig", func() {
		It("should create config with default writers", func() {
			config := NewCLIConfig()
			Expect(config.OutputWriter).NotTo(BeNil())
			Expect(config.ErrorWriter).NotTo(BeNil())
		})

		It("should allow custom output writer", func() {
			buf := &bytes.Buffer{}
			config := NewCLIConfig(WithOutputWriter(buf))
			Expect(config.OutputWriter).To(Equal(buf))
		})

		It("should allow custom error writer", func() {
			buf := &bytes.Buffer{}
			config := NewCLIConfig(WithErrorWriter(buf))
			Expect(config.ErrorWriter).To(Equal(buf))
		})
	})

	Describe("CLIContext", func() {
		It("should create context with default config when nil", func() {
			ctx := NewCLIContext(nil)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.GlobalFlags).NotTo(BeNil())
			Expect(ctx.Config()).NotTo(BeNil())
		})

		It("should create context with provided config", func() {
			config := NewCLIConfig()
			ctx := NewCLIContext(config)
			Expect(ctx.Config()).To(Equal(config))
		})

	})
})
