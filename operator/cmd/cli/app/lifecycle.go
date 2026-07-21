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
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/nodewright/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/nodewright/operator/internal/cli/context"
	"github.com/NVIDIA/nodewright/operator/internal/cli/preflight"
	"github.com/NVIDIA/nodewright/operator/internal/cli/utils"
)

type lifecycleAction string

const (
	lifecycleActionSet    lifecycleAction = "set"
	lifecycleActionRemove lifecycleAction = "remove"
)

// lifecycleConfig defines the configuration for a lifecycle command
type lifecycleConfig struct {
	use          string
	short        string
	long         string
	example      string
	annotation   string
	action       lifecycleAction
	verb         string // past tense for output message (e.g., "paused", "resumed")
	confirmVerb  string // verb for confirmation prompt (e.g., "pause", "disable")
	needsConfirm bool
}

// lifecycleOptions holds the options for lifecycle commands that need confirmation
type lifecycleOptions struct {
	confirm bool
}

// newLifecycleCmd creates a lifecycle command based on the provided configuration
func newLifecycleCmd(ctx *cliContext.CLIContext, cfg lifecycleConfig) *cobra.Command {
	opts := &lifecycleOptions{}

	cmd := &cobra.Command{
		Use:     cfg.use,
		Short:   cfg.short,
		Long:    cfg.long,
		Example: cfg.example,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skyhookName := args[0]

			if cfg.needsConfirm && !opts.confirm {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "This will %s NodeWright %q. Continue? [y/N]: ",
					cfg.confirmVerb, skyhookName)
				reader := bufio.NewReader(cmd.InOrStdin())
				response, err := reader.ReadString('\n')
				if err != nil {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
				response = strings.TrimSpace(response)
				if response != "y" && response != "Y" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			// Preflight before the dry-run short-circuit (matching node/package/reset): a
			// dry-run against a legacy-only operator would otherwise print "Would ..." for a
			// NodeWright that the cluster cannot serve.
			if err := preflight.EnsureNodeWrightServed(kubeClient.Kubernetes().Discovery()); err != nil {
				return err
			}

			// Check dry-run before making changes
			if ctx.GlobalFlags.DryRun {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] Would %s NodeWright %q\n", cfg.confirmVerb, skyhookName)
				return nil
			}

			// Fetch the NodeWright to check operator version from its annotation
			skyhook, err := utils.GetSkyhook(cmd.Context(), kubeClient.Dynamic(), skyhookName)
			if err != nil {
				return err
			}

			// Check operator version compatibility using NodeWright's version annotation
			// If annotation is missing or invalid (e.g., "dev"), fall back to deployment version
			opVersion := utils.GetSkyhookVersion(skyhook)
			if opVersion == "" || !utils.IsValidVersion(opVersion) {
				// Try to get version from deployment instead
				deployVersion, err := utils.DiscoverOperatorVersion(cmd.Context(), kubeClient.Kubernetes(), ctx.GlobalFlags.Namespace())
				if err == nil && utils.IsValidVersion(deployVersion) {
					opVersion = deployVersion
				} else {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: unable to determine operator version; "+
						"cannot verify compatibility (requires %s+)\n", utils.MinAnnotationSupportVersion)
				}
			}

			if utils.IsValidVersion(opVersion) && utils.CompareVersions(opVersion, utils.MinAnnotationSupportVersion) < 0 {
				// Operator is older than v0.8.0 - annotations won't work
				if cfg.annotation == utils.PauseAnnotation {
					// pause/resume - feature exists but uses spec field instead of annotation
					specValue := "true"
					if cfg.action == lifecycleActionRemove {
						specValue = "false"
					}
					return fmt.Errorf("operator version %s uses spec.pause instead of annotations; "+
						"use 'kubectl edit skyhook %s' and set spec.pause: %s", opVersion, skyhookName, specValue)
				}
				// disable/enable - feature doesn't exist at all in older versions
				return fmt.Errorf("operator version %s does not support %s; this feature was added in %s",
					opVersion, cfg.confirmVerb, utils.MinAnnotationSupportVersion)
			}

			if cfg.action == lifecycleActionSet {
				err = utils.SetSkyhookAnnotation(cmd.Context(), kubeClient.Dynamic(), skyhookName, cfg.annotation, "true")
			} else {
				err = utils.RemoveSkyhookAnnotation(cmd.Context(), kubeClient.Dynamic(), skyhookName, cfg.annotation)
			}
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "NodeWright %q %s\n", skyhookName, cfg.verb)
			return nil
		},
	}

	if cfg.needsConfirm {
		cmd.Flags().BoolVarP(&opts.confirm, "confirm", "y", false, "Skip confirmation prompt")
	}

	return cmd
}

// NewPauseCmd creates the pause command
func NewPauseCmd(ctx *cliContext.CLIContext) *cobra.Command {
	return newLifecycleCmd(ctx, lifecycleConfig{
		use:   "pause <nodewright-name>",
		short: "Pause a NodeWright from processing",
		long: `Pause a NodeWright by setting the pause annotation.

When a NodeWright is paused, the operator will stop processing new nodes
but will not interrupt any currently running operations.`,
		example: `  # Pause a NodeWright
  kubectl nodewright pause gpu-init

  # Pause without confirmation
  kubectl nodewright pause gpu-init --confirm`,
		annotation:   utils.PauseAnnotation,
		action:       lifecycleActionSet,
		verb:         "paused",
		confirmVerb:  "pause",
		needsConfirm: true,
	})
}

// NewResumeCmd creates the resume command
func NewResumeCmd(ctx *cliContext.CLIContext) *cobra.Command {
	return newLifecycleCmd(ctx, lifecycleConfig{
		use:   "resume <nodewright-name>",
		short: "Resume a paused NodeWright",
		long: `Resume a paused NodeWright by removing the pause annotation.

The operator will resume processing nodes after this command.`,
		example: `  # Resume a paused NodeWright
  kubectl nodewright resume gpu-init

  # Resume without confirmation
  kubectl nodewright resume gpu-init --confirm`,
		annotation:   utils.PauseAnnotation,
		action:       lifecycleActionRemove,
		verb:         "resumed",
		confirmVerb:  "resume",
		needsConfirm: true,
	})
}

// NewDisableCmd creates the disable command
func NewDisableCmd(ctx *cliContext.CLIContext) *cobra.Command {
	return newLifecycleCmd(ctx, lifecycleConfig{
		use:   "disable <nodewright-name>",
		short: "Disable a NodeWright completely",
		long: `Disable a NodeWright by setting the disable annotation.

When a NodeWright is disabled, the operator will completely stop processing
and the NodeWright will be effectively inactive.`,
		example: `  # Disable a NodeWright
  kubectl nodewright disable gpu-init

  # Disable without confirmation
  kubectl nodewright disable gpu-init --confirm`,
		annotation:   utils.DisableAnnotation,
		action:       lifecycleActionSet,
		verb:         "disabled",
		confirmVerb:  "disable",
		needsConfirm: true,
	})
}

// NewEnableCmd creates the enable command
func NewEnableCmd(ctx *cliContext.CLIContext) *cobra.Command {
	return newLifecycleCmd(ctx, lifecycleConfig{
		use:   "enable <nodewright-name>",
		short: "Enable a disabled NodeWright",
		long: `Enable a disabled NodeWright by removing the disable annotation.

The operator will resume normal processing after this command.`,
		example: `  # Enable a disabled NodeWright
  kubectl nodewright enable gpu-init

  # Enable without confirmation
  kubectl nodewright enable gpu-init --confirm`,
		annotation:   utils.DisableAnnotation,
		action:       lifecycleActionRemove,
		verb:         "enabled",
		confirmVerb:  "enable",
		needsConfirm: true,
	})
}
