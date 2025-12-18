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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/tabwriter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
)

// Output format constants
const (
	OutputFormatTable = "table"
	OutputFormatJSON  = "json"
	OutputFormatYAML  = "yaml"
	OutputFormatWide  = "wide"
)

// MatchNodes matches node patterns against a list of available nodes.
// Patterns can be exact node names or regex patterns.
func MatchNodes(patterns []string, availableNodes []string) ([]string, error) {
	matched := make(map[string]bool)

	for _, pattern := range patterns {
		// Check if it's a regex pattern (contains regex metacharacters)
		isRegex := strings.ContainsAny(pattern, ".*+?^${}[]|()\\")

		if isRegex {
			re, err := regexp.Compile("^" + pattern + "$")
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern %q: %w", pattern, err)
			}

			for _, node := range availableNodes {
				if re.MatchString(node) {
					matched[node] = true
				}
			}
		} else {
			// Exact match
			for _, node := range availableNodes {
				if node == pattern {
					matched[node] = true
				}
			}
		}
	}

	result := make([]string, 0, len(matched))
	for node := range matched {
		result = append(result, node)
	}

	return result, nil
}

// UnstructuredToSkyhook converts an unstructured object to a Skyhook.
func UnstructuredToSkyhook(u *unstructured.Unstructured) (*v1alpha1.Skyhook, error) {
	data, err := u.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshaling unstructured: %w", err)
	}

	var skyhook v1alpha1.Skyhook
	if err := json.Unmarshal(data, &skyhook); err != nil {
		return nil, fmt.Errorf("unmarshaling to skyhook: %w", err)
	}

	return &skyhook, nil
}

// Skyhook annotation and label keys
const (
	PauseAnnotation   = v1alpha1.METADATA_PREFIX + "/pause"
	DisableAnnotation = v1alpha1.METADATA_PREFIX + "/disable"
	NodeIgnoreLabel   = v1alpha1.METADATA_PREFIX + "/ignore"
)

// SetSkyhookAnnotation sets an annotation on a Skyhook CR using dynamic client
// Note: Skyhook is a cluster-scoped resource (not namespaced)
func SetSkyhookAnnotation(ctx context.Context, dynamicClient dynamic.Interface, skyhookName, annotation, value string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, annotation, value)

	gvr := v1alpha1.GroupVersion.WithResource("skyhooks")
	_, err := dynamicClient.Resource(gvr).Patch(
		ctx,
		skyhookName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching skyhook %q: %w", skyhookName, err)
	}

	return nil
}

// RemoveSkyhookAnnotation removes an annotation from a Skyhook CR using dynamic client
// Note: Skyhook is a cluster-scoped resource (not namespaced)
func RemoveSkyhookAnnotation(ctx context.Context, dynamicClient dynamic.Interface, skyhookName, annotation string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:null}}}`, annotation)

	gvr := v1alpha1.GroupVersion.WithResource("skyhooks")
	_, err := dynamicClient.Resource(gvr).Patch(
		ctx,
		skyhookName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching skyhook %q: %w", skyhookName, err)
	}

	return nil
}

// SetNodeAnnotation sets an annotation on a Node using merge patch
func SetNodeAnnotation(ctx context.Context, kubeClient kubernetes.Interface, nodeName, key, value string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, key, value)

	_, err := kubeClient.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching node %q: %w", nodeName, err)
	}

	return nil
}

// RemoveNodeAnnotation removes an annotation from a Node using merge patch
func RemoveNodeAnnotation(ctx context.Context, kubeClient kubernetes.Interface, nodeName, key string) error {
	patch := fmt.Sprintf(`{"metadata":{"annotations":{%q:null}}}`, key)

	_, err := kubeClient.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching node %q: %w", nodeName, err)
	}

	return nil
}

// SetNodeLabel sets a label on a Node using merge patch
func SetNodeLabel(ctx context.Context, kubeClient kubernetes.Interface, nodeName, key, value string) error {
	patch := fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, key, value)

	_, err := kubeClient.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching node %q: %w", nodeName, err)
	}

	return nil
}

// RemoveNodeLabel removes a label from a Node using merge patch
func RemoveNodeLabel(ctx context.Context, kubeClient kubernetes.Interface, nodeName, key string) error {
	patch := fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, key)

	_, err := kubeClient.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		[]byte(patch),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patching node %q: %w", nodeName, err)
	}

	return nil
}

// OutputJSON writes data as indented JSON to the writer
func OutputJSON(out io.Writer, data any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling json: %w", err)
	}
	_, _ = fmt.Fprintln(out, string(jsonData))
	return nil
}

// OutputYAML writes data as YAML to the writer
func OutputYAML(out io.Writer, data any) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling yaml: %w", err)
	}
	_, _ = fmt.Fprint(out, string(yamlData))
	return nil
}

// TableConfig defines the column configuration for table/wide output
// T is the type of items being displayed
type TableConfig[T any] struct {
	// Headers for table mode (always shown)
	Headers []string
	// Extract returns column values for table mode
	Extract func(T) []string
	// WideHeaders are additional headers appended in wide mode (optional)
	WideHeaders []string
	// WideExtract returns additional column values for wide mode (optional)
	WideExtract func(T) []string
}

// OutputTable writes items as a table using the provided config
func OutputTable[T any](out io.Writer, cfg TableConfig[T], items []T) error {
	return outputTableInternal(out, cfg, items, false)
}

// OutputWide writes items as a wide table (table columns + wide columns)
func OutputWide[T any](out io.Writer, cfg TableConfig[T], items []T) error {
	return outputTableInternal(out, cfg, items, true)
}

// OutputTableWithHeader writes items as a table with a header line above
func OutputTableWithHeader[T any](out io.Writer, headerLine string, cfg TableConfig[T], items []T) error {
	_, _ = fmt.Fprintln(out, headerLine)
	_, _ = fmt.Fprintln(out)
	return outputTableInternal(out, cfg, items, false)
}

// OutputWideWithHeader writes items as a wide table with a header line above
func OutputWideWithHeader[T any](out io.Writer, headerLine string, cfg TableConfig[T], items []T) error {
	_, _ = fmt.Fprintln(out, headerLine)
	_, _ = fmt.Fprintln(out)
	return outputTableInternal(out, cfg, items, true)
}

func outputTableInternal[T any](out io.Writer, cfg TableConfig[T], items []T, wide bool) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	// Build headers
	headers := cfg.Headers
	if wide && len(cfg.WideHeaders) > 0 {
		headers = append(headers, cfg.WideHeaders...)
	}

	// Write headers
	_, _ = fmt.Fprintln(tw, strings.Join(headers, "\t"))

	// Write separator
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	_, _ = fmt.Fprintln(tw, strings.Join(seps, "\t"))

	// Write rows
	for _, item := range items {
		row := cfg.Extract(item)
		if wide && cfg.WideExtract != nil {
			row = append(row, cfg.WideExtract(item)...)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	return tw.Flush()
}
