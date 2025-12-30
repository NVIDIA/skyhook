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

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/skyhook/operator/internal/cli/context"
	"github.com/NVIDIA/skyhook/operator/internal/cli/utils"
)

const (
	nodeStateAnnotationPrefix = v1alpha1.METADATA_PREFIX + "/nodeState_"
	statusAnnotationPrefix    = v1alpha1.METADATA_PREFIX + "/status_"
	cordonAnnotationPrefix    = v1alpha1.METADATA_PREFIX + "/cordon_"
	versionAnnotationPrefix   = v1alpha1.METADATA_PREFIX + "/version_"
	statusLabelPrefix         = v1alpha1.METADATA_PREFIX + "/status_"
)

// resetOptions holds the options for the reset command
type resetOptions struct {
	confirm bool
}

// NewResetCmd creates the reset command
func NewResetCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &resetOptions{}

	cmd := &cobra.Command{
		Use:   "reset <skyhook-name>",
		Short: "Reset all nodes for a Skyhook",
		Long: `Reset all package state on all nodes for a specific Skyhook, forcing a complete re-run.

This command removes all Skyhook state from all nodes that have state for the
specified Skyhook, causing the operator to re-execute all packages from the beginning.

Unlike 'node reset' which resets specific nodes, 'skyhook reset' resets ALL nodes
that have state for the specified Skyhook.`,
		Example: `  # Reset all nodes for gpu-init Skyhook
  kubectl skyhook reset gpu-init --confirm

  # Preview changes without applying (dry-run)
  kubectl skyhook reset gpu-init --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skyhookName := args[0]

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			return runReset(cmd.Context(), cmd, kubeClient, skyhookName, opts, ctx)
		},
	}

	cmd.Flags().BoolVarP(&opts.confirm, "confirm", "y", false, "Skip confirmation prompt")

	return cmd
}

func runReset(ctx context.Context, cmd *cobra.Command, kubeClient *client.Client, skyhookName string, opts *resetOptions, cliCtx *cliContext.CLIContext) error {
	// Get all nodes
	nodeList, err := kubeClient.Kubernetes().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	// Find nodes that have the specified Skyhook annotation
	annotationKey := nodeStateAnnotationPrefix + skyhookName
	nodesToReset := make([]string, 0)
	nodeStates := make(map[string]v1alpha1.NodeState)

	for _, node := range nodeList.Items {
		annotation, ok := node.Annotations[annotationKey]
		if !ok {
			continue
		}

		var nodeState v1alpha1.NodeState
		if err := json.Unmarshal([]byte(annotation), &nodeState); err != nil {
			if cliCtx.GlobalFlags.Verbose {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: skipping node %q - invalid annotation: %v\n", node.Name, err)
			}
			continue
		}

		nodesToReset = append(nodesToReset, node.Name)
		nodeStates[node.Name] = nodeState
	}

	if len(nodesToReset) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No nodes have state for Skyhook %q\n", skyhookName)
		return nil
	}

	// Print summary
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skyhook: %s\n", skyhookName)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Nodes to reset (%d):\n", len(nodesToReset))
	for _, nodeName := range nodesToReset {
		nodeState := nodeStates[nodeName]
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%d packages)\n", nodeName, len(nodeState))
	}

	// Dry run check
	if cliCtx.GlobalFlags.DryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n[dry-run] No changes applied\n")
		return nil
	}

	// Confirmation
	if !opts.confirm {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThis will remove ALL package state for Skyhook %q on %d node(s).\n", skyhookName, len(nodesToReset))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "All packages will re-run from the beginning.\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Continue? [y/N]: ")

		reader := bufio.NewReader(cmd.InOrStdin())
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Aborted\n")
			return nil
		}
	}

	// Apply changes - clear all skyhook-related annotations and labels
	var updateErrors []string
	successCount := 0

	for _, nodeName := range nodesToReset {
		// Clear all annotations and labels for this skyhook
		annotationsToRemove := []string{
			nodeStateAnnotationPrefix + skyhookName,
			statusAnnotationPrefix + skyhookName,
			cordonAnnotationPrefix + skyhookName,
			versionAnnotationPrefix + skyhookName,
		}
		labelsToRemove := []string{
			statusLabelPrefix + skyhookName,
		}

		// Try to remove the main nodeState annotation first - this is the critical one
		mainAnnotationKey := nodeStateAnnotationPrefix + skyhookName
		if err := utils.RemoveNodeAnnotation(ctx, kubeClient.Kubernetes(), nodeName, mainAnnotationKey); err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("%s: failed to remove nodeState annotation: %v", nodeName, err))
			continue
		}

		// Remove other annotations (non-critical, so we don't fail if they don't exist)
		for _, annKey := range annotationsToRemove {
			if annKey == mainAnnotationKey {
				continue // Already removed
			}
			if err := utils.RemoveNodeAnnotation(ctx, kubeClient.Kubernetes(), nodeName, annKey); err != nil {
				// Don't fail if annotation doesn't exist - just log it
				if cliCtx.GlobalFlags.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to remove annotation %q from node %q: %v\n", annKey, nodeName, err)
				}
			}
		}

		// Remove labels (non-critical, so we don't fail if they don't exist)
		for _, labelKey := range labelsToRemove {
			if err := utils.RemoveNodeLabel(ctx, kubeClient.Kubernetes(), nodeName, labelKey); err != nil {
				// Don't fail if label doesn't exist - just log it
				if cliCtx.GlobalFlags.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to remove label %q from node %q: %v\n", labelKey, nodeName, err)
				}
			}
		}

		successCount++
	}

	// Print results
	if len(updateErrors) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nErrors resetting some nodes:\n")
		for _, e := range updateErrors {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", e)
		}
	}

	if successCount > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully reset %d node(s) for Skyhook %q\n", successCount, skyhookName)
	}

	return nil
}
