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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// DefaultNamespace is the default namespace for Skyhook resources
const DefaultNamespace = "skyhook"

// GlobalFlags holds persistent CLI flags that every command uses (kubeconfig, namespace, output, etc.).
type GlobalFlags struct {
	ConfigFlags  *genericclioptions.ConfigFlags
	OutputFormat string
	Verbose      bool
	DryRun       bool
}

// NewGlobalFlags creates a new GlobalFlags with default values.
func NewGlobalFlags() *GlobalFlags {
	flags := genericclioptions.NewConfigFlags(true)
	if flags.Namespace == nil {
		flags.Namespace = new(string)
	}
	if *flags.Namespace == "" {
		*flags.Namespace = DefaultNamespace
	}

	return &GlobalFlags{
		ConfigFlags:  flags,
		OutputFormat: "table",
	}
}

// AddFlags adds the global flags to the provided FlagSet.
func (f *GlobalFlags) AddFlags(flagset *pflag.FlagSet) {
	f.ConfigFlags.AddFlags(flagset)
	flagset.StringVarP(&f.OutputFormat, "output", "o", f.OutputFormat, "Output format. One of: table|json|yaml|wide")
	flagset.BoolVarP(&f.Verbose, "verbose", "v", false, "Enable verbose output")
	flagset.BoolVar(&f.DryRun, "dry-run", false, "Preview changes without applying them")
}

// Validate validates the global flags.
func (f *GlobalFlags) Validate() error {
	validFormats := map[string]struct{}{
		"table": {},
		"json":  {},
		"yaml":  {},
		"wide":  {},
	}
	f.OutputFormat = strings.ToLower(f.OutputFormat)
	if _, ok := validFormats[f.OutputFormat]; !ok {
		return fmt.Errorf("invalid output format %q", f.OutputFormat)
	}
	// No validation needed for boolean Verbose flag
	return nil
}

// Namespace returns the namespace selected via kubeconfig or flag (default "skyhook").
func (f *GlobalFlags) Namespace() string {
	if f.ConfigFlags == nil || f.ConfigFlags.Namespace == nil {
		return DefaultNamespace
	}
	ns := strings.TrimSpace(*f.ConfigFlags.Namespace)
	if ns == "" {
		return DefaultNamespace
	}
	return ns
}

// CLIContext holds the context that is passed around to every command.
// It contains global flags and can be extended with additional context in the future.
type CLIContext struct {
	GlobalFlags *GlobalFlags
	config      *CLIConfig
}

// CLIConfig holds the configuration for the CLI execution.
type CLIConfig struct {
	OutputWriter io.Writer
	ErrorWriter  io.Writer
}

// CLIConfigOption is a function that modifies a CLIConfig.
type CLIConfigOption func(*CLIConfig)

// WithOutputWriter sets the output writer for the CLI.
func WithOutputWriter(w io.Writer) CLIConfigOption {
	return func(c *CLIConfig) {
		c.OutputWriter = w
	}
}

// WithErrorWriter sets the error writer for the CLI.
func WithErrorWriter(w io.Writer) CLIConfigOption {
	return func(c *CLIConfig) {
		c.ErrorWriter = w
	}
}

// NewCLIConfig creates a new CLIConfig with the given options.
func NewCLIConfig(opts ...CLIConfigOption) *CLIConfig {
	config := &CLIConfig{
		OutputWriter: os.Stdout,
		ErrorWriter:  os.Stderr,
	}

	for _, opt := range opts {
		opt(config)
	}

	return config
}

// NewCLIContext creates a new CLIContext with default values.
// If config is nil, a default configuration is created.
func NewCLIContext(config *CLIConfig) *CLIContext {
	if config == nil {
		config = NewCLIConfig()
	}

	return &CLIContext{
		GlobalFlags: NewGlobalFlags(),
		config:      config,
	}
}

// Config returns the CLI configuration.
func (c *CLIContext) Config() *CLIConfig {
	return c.config
}
