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
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/skyhook/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/skyhook/operator/internal/cli/context"
	"github.com/NVIDIA/skyhook/operator/internal/cli/utils"
)

// pauseOptions holds the options for the pause command
type pauseOptions struct {
	confirm bool
}

// BindToCmd binds the options to the command flags
func (o *pauseOptions) BindToCmd(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&o.confirm, "confirm", "y", false, "Skip confirmation prompt")
}

// NewPauseCmd creates the pause command
func NewPauseCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &pauseOptions{}

	cmd := &cobra.Command{
		Use:   "pause <skyhook-name>",
		Short: "Pause a Skyhook from processing",
		Long: `Pause a Skyhook by setting the pause annotation.

When a Skyhook is paused, the operator will stop processing new nodes
but will not interrupt any currently running operations.`,
		Example: `  # Pause a Skyhook
  kubectl skyhook pause gpu-init

  # Pause without confirmation
  kubectl skyhook pause gpu-init --confirm`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skyhookName := args[0]

			if !opts.confirm {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "This will pause Skyhook %q. Continue? [y/N]: ", skyhookName)
				var response string
				if _, err := fmt.Scanln(&response); err != nil || (response != "y" && response != "Y") {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			if err := utils.SetSkyhookAnnotation(cmd.Context(), kubeClient.Dynamic(), skyhookName, utils.PauseAnnotation, "true"); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skyhook %q paused\n", skyhookName)
			return nil
		},
	}

	opts.BindToCmd(cmd)

	return cmd
}
