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

package pkg

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/skyhook/operator/internal/cli/context"
	"github.com/NVIDIA/skyhook/operator/internal/cli/utils"
)

var skyhookGVR = schema.GroupVersionResource{
	Group:    v1alpha1.GroupVersion.Group,
	Version:  v1alpha1.GroupVersion.Version,
	Resource: "skyhooks",
}

// nodeStateAnnotationKey returns the annotation key for node state
func nodeStateAnnotationKey(skyhookName string) string {
	return fmt.Sprintf("%s/nodeState_%s", v1alpha1.METADATA_PREFIX, skyhookName)
}

// rerunOptions holds the options for the rerun command
type rerunOptions struct {
	skyhookName string
	nodes       []string
	stage       string
	confirm     bool
}

// NewRerunCmd creates the package rerun command
func NewRerunCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &rerunOptions{}

	cmd := &cobra.Command{
		Use:   "rerun <package-name>",
		Short: "Force a package to re-run on specific node(s)",
		Long: `Force a package to re-run on specific node(s) by removing its state
from the Skyhook status, causing the operator to re-execute the package.

Nodes can be specified using exact names or regex patterns. Multiple
--node flags can be combined to target different nodes.`,
		Example: `  # Re-run the shellscript package on worker-1 (from beginning)
  kubectl skyhook package rerun shellscript --skyhook gpu-init --node worker-1

  # Re-run only the config stage
  kubectl skyhook package rerun shellscript --skyhook gpu-init --node worker-1 --stage config

  # Re-run on all nodes matching a regex pattern
  kubectl skyhook package rerun shellscript --skyhook gpu-init --node "worker-.*"

  # Skip confirmation prompt
  kubectl skyhook package rerun shellscript --skyhook gpu-init --node worker-1 --confirm

  # Preview changes without applying
  kubectl skyhook package rerun shellscript --skyhook gpu-init --node worker-1 --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.skyhookName = cmd.Flag("skyhook").Value.String()
			packageName := args[0]

			// Validate required flags
			if opts.skyhookName == "" {
				return fmt.Errorf("--skyhook flag is required")
			}
			if len(opts.nodes) == 0 {
				return fmt.Errorf("at least one --node flag is required")
			}

			// Validate stage if provided
			if opts.stage != "" {
				validStages := map[string]bool{
					"apply":          true,
					"config":         true,
					"interrupt":      true,
					"post-interrupt": true,
				}
				if !validStages[opts.stage] {
					return fmt.Errorf("invalid stage %q: must be one of apply, config, interrupt, post-interrupt", opts.stage)
				}
			}

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			return rerunPackage(cmd.Context(), cmd, kubeClient, packageName, opts, ctx)
		},
	}

	cmd.Flags().StringVar(&opts.skyhookName, "skyhook", "", "Name of the Skyhook CR (required)")
	cmd.Flags().StringArrayVar(&opts.nodes, "node", nil, "Node name or regex pattern (can be specified multiple times, required)")
	cmd.Flags().StringVar(&opts.stage, "stage", "", "Re-run from specific stage (apply, config, interrupt, post-interrupt)")
	cmd.Flags().BoolVarP(&opts.confirm, "confirm", "y", false, "Skip confirmation prompt")

	_ = cmd.MarkFlagRequired("skyhook")
	_ = cmd.MarkFlagRequired("node")

	return cmd
}

func rerunPackage(ctx context.Context, cmd *cobra.Command, kubeClient *client.Client, packageName string, opts *rerunOptions, cliCtx *cliContext.CLIContext) error {
	// Get the Skyhook CR to find the package spec
	skyhookUnstructured, err := kubeClient.Dynamic().Resource(skyhookGVR).Get(ctx, opts.skyhookName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting skyhook %q: %w", opts.skyhookName, err)
	}

	// Convert to Skyhook type to access spec
	skyhook, err := utils.UnstructuredToSkyhook(skyhookUnstructured)
	if err != nil {
		return fmt.Errorf("parsing skyhook: %w", err)
	}

	// Find the package in the spec
	pkg, found := skyhook.Spec.Packages[packageName]
	if !found {
		return fmt.Errorf("package %q not found in skyhook %q", packageName, opts.skyhookName)
	}

	// Get the unique key for the package (name|version)
	packageKey := pkg.GetUniqueName()

	// Get all nodes that have the skyhook annotation
	annotationKey := nodeStateAnnotationKey(opts.skyhookName)
	nodeList, err := kubeClient.Kubernetes().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	// Build list of nodes that have the annotation
	allNodes := make([]string, 0)
	nodeStates := make(map[string]v1alpha1.NodeState)
	for _, node := range nodeList.Items {
		if annotation, ok := node.Annotations[annotationKey]; ok {
			var nodeState v1alpha1.NodeState
			if err := json.Unmarshal([]byte(annotation), &nodeState); err != nil {
				continue // skip nodes with invalid annotation
			}
			allNodes = append(allNodes, node.Name)
			nodeStates[node.Name] = nodeState
		}
	}

	// Match nodes based on patterns
	matchedNodes, err := utils.MatchNodes(opts.nodes, allNodes)
	if err != nil {
		return fmt.Errorf("matching nodes: %w", err)
	}

	if len(matchedNodes) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No nodes matched the specified patterns\n")
		return nil
	}

	// Filter to only nodes that have this package in their state
	nodesToUpdate := make([]string, 0)
	for _, nodeName := range matchedNodes {
		if nodeState, ok := nodeStates[nodeName]; ok {
			if _, hasPackage := nodeState[packageKey]; hasPackage {
				nodesToUpdate = append(nodesToUpdate, nodeName)
			}
		}
	}

	if len(nodesToUpdate) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Package %q (version %s) has no state on matched nodes\n", packageName, pkg.Version)
		return nil
	}

	// Display what will be changed
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Package: %s (version %s)\n", packageName, pkg.Version)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skyhook: %s\n", opts.skyhookName)
	if opts.stage != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Re-run from stage: %s\n", opts.stage)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Nodes to reset (%d):\n", len(nodesToUpdate))
	for _, nodeName := range nodesToUpdate {
		nodeState := nodeStates[nodeName]
		pkgStatus := nodeState[packageKey]
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s (current state: %s, stage: %s)\n", nodeName, pkgStatus.State, pkgStatus.Stage)
	}

	if cliCtx.GlobalFlags.DryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n[dry-run] No changes applied\n")
		return nil
	}

	// Confirmation prompt
	if !opts.confirm {
		if opts.stage != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThis will reset package state to re-run from the '%s' stage.\n", opts.stage)
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThis will remove package state from node annotations and cause the package to re-run from the beginning.\n")
		}
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

	// Update each node's annotation to trigger re-run
	var updateErrors []string
	successCount := 0
	for _, nodeName := range nodesToUpdate {
		nodeState := nodeStates[nodeName]

		if opts.stage != "" {
			// If a specific stage is requested, update the package state to re-run from that stage
			pkgStatus := nodeState[packageKey]
			pkgStatus.Stage = v1alpha1.Stage(opts.stage)
			pkgStatus.State = v1alpha1.StateInProgress
			nodeState[packageKey] = pkgStatus
		} else {
			// Otherwise, remove the package from the node state entirely to re-run from the beginning
			delete(nodeState, packageKey)
		}

		// Get the current node to ensure we have the latest resourceVersion
		node, err := kubeClient.Kubernetes().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", nodeName, err))
			continue
		}

		// Update the annotation
		if len(nodeState) == 0 {
			// If no packages left, remove the annotation entirely
			delete(node.Annotations, annotationKey)
		} else {
			// Otherwise, update with the modified state
			newAnnotation, err := json.Marshal(nodeState)
			if err != nil {
				updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", nodeName, err))
				continue
			}
			node.Annotations[annotationKey] = string(newAnnotation)
		}

		// Update the node
		_, err = kubeClient.Kubernetes().CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", nodeName, err))
			continue
		}
		successCount++
	}

	if len(updateErrors) > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nErrors updating some nodes:\n")
		for _, e := range updateErrors {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", e)
		}
	}

	if successCount > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully reset package %q on %d node(s)\n", packageName, successCount)
	}

	return nil
}
