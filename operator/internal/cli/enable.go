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

// NewEnableCmd creates the enable command
func NewEnableCmd(ctx *cliContext.CLIContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable <skyhook-name>",
		Short: "Enable a disabled Skyhook",
		Long: `Enable a disabled Skyhook by removing the disable annotation.

The operator will resume normal processing after this command.`,
		Example: `  # Enable a disabled Skyhook
  kubectl skyhook enable gpu-init`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skyhookName := args[0]

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			if err := utils.RemoveSkyhookAnnotation(cmd.Context(), kubeClient.Dynamic(), skyhookName, utils.DisableAnnotation); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skyhook %q enabled\n", skyhookName)
			return nil
		},
	}

	return cmd
}
