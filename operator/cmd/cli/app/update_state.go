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
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/nodewright/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/nodewright/operator/internal/cli/context"
)

type updateStateOptions struct {
	nodes    []string
	selector string
	confirm  bool
	add      bool
}

// NewUpdateStateCmd creates the `update-state` command.
func NewUpdateStateCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &updateStateOptions{}

	cmd := &cobra.Command{
		Use:   "update-state <skyhook-name> <package> <version> <stage> <state>",
		Short: "Update the recorded state of a single package on Skyhook-managed nodes",
		Long: `Edit the per-node nodeState annotation for one package.

By default the command applies to every node that already has state for the
named Skyhook. Use --node or --selector to narrow the affected nodes.

This is an administrator escape hatch. It does not validate that the new
(stage, state) combination is reachable by the operator from the current
state, and it does not gate destructive stages (uninstall,
uninstall-interrupt) behind extra prompts. Pause the Skyhook CR before
running this command — otherwise the operator may overwrite the edit
immediately.

Use --add to create a fresh nodeState entry for nodes that do not yet have
one for this package. --add requires --node or --selector so the scope is
explicit.`,
		Example: `  # Mark pkg1@1.0 as complete on every tracked node
  kubectl skyhook update-state gpu-init pkg1 1.0 config complete --confirm

  # Same, but only on one node
  kubectl skyhook update-state gpu-init pkg1 1.0 config complete --node n1 --confirm

  # Create a fresh entry on selected nodes
  kubectl skyhook update-state gpu-init pkg1 1.0 apply in_progress --selector role=gpu --add --confirm`,
		Args: cobra.ExactArgs(5),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}
			return runUpdateState(cmd.Context(), cmd, kubeClient, args, opts, ctx)
		},
	}

	cmd.Flags().StringArrayVar(&opts.nodes, "node", nil, "Limit the update to specific node(s); repeat for multiple")
	cmd.Flags().StringVarP(&opts.selector, "selector", "l", "", "Limit the update to nodes matching a label selector")
	cmd.Flags().BoolVarP(&opts.confirm, "confirm", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&opts.add, "add", false, "Create a fresh nodeState entry on nodes that do not yet have one (requires --node or --selector)")
	cmd.MarkFlagsMutuallyExclusive("node", "selector")

	return cmd
}

func runUpdateState(ctx context.Context, cmd *cobra.Command, kubeClient *client.Client, args []string, opts *updateStateOptions, cliCtx *cliContext.CLIContext) error {
	return fmt.Errorf("not implemented")
}
