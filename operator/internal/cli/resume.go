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

// NewResumeCmd creates the resume command
func NewResumeCmd(ctx *cliContext.CLIContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resume <skyhook-name>",
		Short: "Resume a paused Skyhook",
		Long: `Resume a paused Skyhook by removing the pause annotation.

The operator will resume processing nodes after this command.`,
		Example: `  # Resume a paused Skyhook
  kubectl skyhook resume gpu-init`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skyhookName := args[0]

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			if err := utils.RemoveSkyhookAnnotation(cmd.Context(), kubeClient.Dynamic(), skyhookName, utils.PauseAnnotation); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skyhook %q resumed\n", skyhookName)
			return nil
		},
	}

	return cmd
}
