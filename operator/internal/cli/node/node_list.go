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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

// nodeListOptions holds the options for the node list command
type nodeListOptions struct {
	skyhookName string
	output      string
}

// BindToCmd binds the options to the command flags
func (o *nodeListOptions) BindToCmd(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.skyhookName, "skyhook", "", "Name of the Skyhook CR (required)")
	cmd.Flags().StringVarP(&o.output, "output", "o", "table", "Output format: table, json, yaml")

	_ = cmd.MarkFlagRequired("skyhook")
}

// NewListCmd creates the node list command
func NewListCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &nodeListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all nodes targeted by a Skyhook",
		Long: `List all nodes that have activity for a specific Skyhook.

This command shows all nodes that have Skyhook state annotations for the
specified Skyhook CR, along with a summary of package completion status.`,
		Example: `  # List all nodes targeted by gpu-init Skyhook
  kubectl skyhook node list --skyhook gpu-init

  # Output as JSON
  kubectl skyhook node list --skyhook gpu-init -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.skyhookName == "" {
				return fmt.Errorf("--skyhook flag is required")
			}

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			return runNodeList(cmd.Context(), cmd.OutOrStdout(), kubeClient, opts)
		},
	}

	opts.BindToCmd(cmd)

	return cmd
}

// nodeListEntry represents a node in the list output
type nodeListEntry struct {
	NodeName         string `json:"nodeName"`
	Status           string `json:"status"`
	PackagesComplete int    `json:"packagesComplete"`
	PackagesTotal    int    `json:"packagesTotal"`
}

func runNodeList(ctx context.Context, out io.Writer, kubeClient *client.Client, opts *nodeListOptions) error {
	// Get all nodes
	nodeList, err := kubeClient.Kubernetes().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	annotationKey := nodeStateAnnotationPrefix + opts.skyhookName
	entries := make([]nodeListEntry, 0, len(nodeList.Items))

	for _, node := range nodeList.Items {
		annotation, ok := node.Annotations[annotationKey]
		if !ok {
			continue
		}

		var nodeState v1alpha1.NodeState
		if err := json.Unmarshal([]byte(annotation), &nodeState); err != nil {
			continue
		}

		completeCount := 0
		hasError := false
		hasInProgress := false

		for _, pkgStatus := range nodeState {
			switch pkgStatus.State {
			case v1alpha1.StateComplete:
				completeCount++
			case v1alpha1.StateErroring:
				hasError = true
			case v1alpha1.StateInProgress:
				hasInProgress = true
			}
		}

		// Determine overall status
		status := string(v1alpha1.StateUnknown)
		if hasError {
			status = string(v1alpha1.StateErroring)
		} else if completeCount == len(nodeState) && len(nodeState) > 0 {
			status = string(v1alpha1.StateComplete)
		} else if hasInProgress || completeCount > 0 {
			status = string(v1alpha1.StateInProgress)
		}

		entries = append(entries, nodeListEntry{
			NodeName:         node.Name,
			Status:           status,
			PackagesComplete: completeCount,
			PackagesTotal:    len(nodeState),
		})
	}

	// Sort by node name
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].NodeName < entries[j].NodeName
	})

	if len(entries) == 0 {
		_, _ = fmt.Fprintf(out, "No nodes found for Skyhook %q\n", opts.skyhookName)
		return nil
	}

	// Output based on format
	switch opts.output {
	case "json":
		return outputNodeListJSON(out, opts.skyhookName, entries)
	case "yaml":
		return outputNodeListYAML(out, opts.skyhookName, entries)
	default:
		return outputNodeListTable(out, opts.skyhookName, entries)
	}
}

func outputNodeListJSON(out io.Writer, skyhookName string, entries []nodeListEntry) error {
	output := struct {
		SkyhookName string          `json:"skyhookName"`
		Nodes       []nodeListEntry `json:"nodes"`
	}{
		SkyhookName: skyhookName,
		Nodes:       entries,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling json: %w", err)
	}
	_, _ = fmt.Fprintln(out, string(data))
	return nil
}

func outputNodeListYAML(out io.Writer, skyhookName string, entries []nodeListEntry) error {
	output := struct {
		SkyhookName string          `yaml:"skyhookName"`
		Nodes       []nodeListEntry `yaml:"nodes"`
	}{
		SkyhookName: skyhookName,
		Nodes:       entries,
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshaling yaml: %w", err)
	}
	_, _ = fmt.Fprint(out, string(data))
	return nil
}

func outputNodeListTable(out io.Writer, skyhookName string, entries []nodeListEntry) error {
	_, _ = fmt.Fprintf(out, "Skyhook: %s\n\n", skyhookName)

	// Calculate summary
	totalNodes := len(entries)
	completeNodes := 0
	errorNodes := 0
	for _, e := range entries {
		switch e.Status {
		case string(v1alpha1.StateComplete):
			completeNodes++
		case string(v1alpha1.StateErroring):
			errorNodes++
		}
	}

	_, _ = fmt.Fprintf(out, "Summary: %d nodes (%d complete, %d erroring, %d in progress)\n\n",
		totalNodes, completeNodes, errorNodes, totalNodes-completeNodes-errorNodes)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NODE\tSTATUS\tPACKAGES")
	_, _ = fmt.Fprintln(w, "----\t------\t--------")

	for _, e := range entries {
		status := e.Status
		if e.Status == string(v1alpha1.StateErroring) {
			status = strings.ToUpper(status)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d/%d\n",
			e.NodeName, status, e.PackagesComplete, e.PackagesTotal)
	}

	return w.Flush()
}
