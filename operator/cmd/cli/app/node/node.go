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

package node

import (
	"github.com/NVIDIA/nodewright/operator/internal/cli/context"
	"github.com/spf13/cobra"
)

func NewNodeCmd(ctx *context.CLIContext) *cobra.Command {
	// NodeCmd represents the node command that subcommands attach to
	nodeCmd := &cobra.Command{
		Use:   "node",
		Short: "Manage NodeWright nodes",
		Long: `The node command group provides functionality to manage, inspect,
and troubleshoot NodeWright nodes across your cluster.

Available operations:
  - List all nodes targeted by a NodeWright
  - Query all NodeWright activity on specific nodes
  - Reset all package state on a node
  - Ignore/unignore nodes from NodeWright processing

Common use cases:
  - Viewing all NodeWright CRs affecting a specific node
  - Listing all nodes in a NodeWright deployment
  - Debugging node failures by examining state
  - Forcing a complete re-run on a node
  - Temporarily excluding nodes for maintenance`,
	}

	nodeCmd.AddCommand(
		NewListCmd(ctx),
		NewStatusCmd(ctx),
		NewResetCmd(ctx),
		NewIgnoreCmd(ctx),
		NewUnignoreCmd(ctx),
	)

	return nodeCmd
}
