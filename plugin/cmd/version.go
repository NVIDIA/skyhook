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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/NVIDIA/skyhook/plugin/pkg/client"
	"github.com/NVIDIA/skyhook/plugin/pkg/version"
)

// NewVersionCmd creates the version command.
func NewVersionCmd(globals *GlobalOptions) *cobra.Command {
	var timeout time.Duration
	var clientOnly bool

	// versionCmd represents the version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show plugin and Skyhook operator versions",
		Long: `Display version information for the Skyhook plugin and the Skyhook operator running in the cluster.

The plugin version is always shown. By default, the command also queries the cluster
to discover the Skyhook operator version. Use --client-only to skip the cluster query.`,
		Example: `  # Show both plugin and operator versions
  skyhook version
  kubectl skyhook version

  # Show only the plugin version (no cluster query)
  skyhook version --client-only

  # Query operator in a specific namespace
  skyhook version -n skyhook-system`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Skyhook plugin:\t%s\n", version.Summary())

			if clientOnly {
				return nil
			}

			clientFactory := client.NewFactory(globals.ConfigFlags)
			cli, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			opVersion, err := discoverOperatorVersion(ctx, cli.Kubernetes(), globals.Namespace())
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Skyhook operator:\tunknown (%v)\n", err)
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Skyhook operator:\t%s\n", opVersion)
			return nil
		},
	}

	versionCmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "Time limit for contacting the Kubernetes API")
	versionCmd.Flags().BoolVar(&clientOnly, "client-only", false, "Only print the plugin version without querying the cluster")

	return versionCmd
}

func discoverOperatorVersion(ctx context.Context, kube kubernetes.Interface, namespace string) (string, error) {
	if kube == nil {
		return "", fmt.Errorf("nil kubernetes client")
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	deploymentName := "skyhook-operator-controller-manager"
	deployment, err := kube.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return "", fmt.Errorf("skyhook operator deployment %q not found in namespace %q", deploymentName, namespace)
		}
		return "", fmt.Errorf("querying operator deployment: %w", err)
	}

	// Try to get version from Helm label (preferred for Helm deployments)
	if v := deployment.Labels["app.kubernetes.io/version"]; strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v), nil
	}

	// Fallback: parse version from container image tag (works for kustomize deployments)
	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		image := deployment.Spec.Template.Spec.Containers[0].Image
		if tag := extractImageTag(image); tag != "" && tag != "latest" {
			return tag, nil
		}
	}

	return "", fmt.Errorf("unable to determine operator version from deployment labels or image tag")
}

// extractImageTag extracts the tag from a container image reference.
// Examples:
//   - "ghcr.io/nvidia/skyhook/operator:v1.2.3" -> "v1.2.3"
//   - "ghcr.io/nvidia/skyhook/operator:v1.2.3@sha256:..." -> "v1.2.3"
//   - "ghcr.io/nvidia/skyhook/operator" -> ""
func extractImageTag(image string) string {
	// Remove digest if present (e.g., @sha256:...)
	if idx := strings.Index(image, "@"); idx > 0 {
		image = image[:idx]
	}

	// Split on ":" to get tag
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return ""
	}

	return strings.TrimSpace(parts[len(parts)-1])
}
