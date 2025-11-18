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

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/NVIDIA/skyhook/plugin/pkg/version"
)

const defaultNamespace = "skyhook"

// GlobalOptions holds persistent CLI options (kubeconfig, namespace, output, etc.).
type GlobalOptions struct {
	ConfigFlags  *genericclioptions.ConfigFlags
	OutputFormat string
	Verbose      bool
}

// NewGlobalOptions creates a new GlobalOptions with default values.
func NewGlobalOptions() *GlobalOptions {
	flags := genericclioptions.NewConfigFlags(true)
	if flags.Namespace == nil {
		flags.Namespace = new(string)
	}
	if *flags.Namespace == "" {
		*flags.Namespace = defaultNamespace
	}

	return &GlobalOptions{
		ConfigFlags:  flags,
		OutputFormat: "table",
	}
}

func (o *GlobalOptions) addFlags(flagset *pflag.FlagSet) {
	o.ConfigFlags.AddFlags(flagset)
	flagset.StringVarP(&o.OutputFormat, "output", "o", o.OutputFormat, "Output format. One of: table|json|yaml|wide")
	flagset.BoolVarP(&o.Verbose, "verbose", "v", false, "Enable verbose output")
}

func (o *GlobalOptions) validate() error {
	validFormats := map[string]struct{}{
		"table": {},
		"json":  {},
		"yaml":  {},
		"wide":  {},
	}
	o.OutputFormat = strings.ToLower(o.OutputFormat)
	if _, ok := validFormats[o.OutputFormat]; !ok {
		return fmt.Errorf("invalid output format %q", o.OutputFormat)
	}
	// No validation needed for boolean Verbose flag
	return nil
}

// Namespace returns the namespace selected via kubeconfig or flag (default "skyhook").
func (o *GlobalOptions) Namespace() string {
	if o.ConfigFlags == nil || o.ConfigFlags.Namespace == nil {
		return defaultNamespace
	}
	ns := strings.TrimSpace(*o.ConfigFlags.Namespace)
	if ns == "" {
		return defaultNamespace
	}
	return ns
}

// NewSkyhookCommand creates the root skyhook command with all subcommands.
func NewSkyhookCommand(opts *GlobalOptions) *cobra.Command {
	// skyhookCmd represents the root command
	skyhookCmd := &cobra.Command{
		Use:           "skyhook",
		Short:         "Skyhook SRE plugin",
		Long:          "kubectl-compatible helper for managing Skyhook deployments.",
		Version:       version.Summary(),
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return opts.validate()
		},
	}

	// Add global flags
	opts.addFlags(skyhookCmd.PersistentFlags())

	// Customize the version output template
	skyhookCmd.SetVersionTemplate("Skyhook plugin: {{.Version}}\n")

	// Add subcommands
	skyhookCmd.AddCommand(
		NewVersionCmd(opts),
	)

	return skyhookCmd
}

// Execute runs the Skyhook CLI and returns the exit code.
func Execute() int {
	opts := NewGlobalOptions()
	if err := NewSkyhookCommand(opts).Execute(); err != nil {
		return 1
	}
	return 0
}
