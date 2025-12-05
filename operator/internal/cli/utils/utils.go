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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
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

// EscapeJSONPointer escapes special characters in JSON Pointer tokens
// per RFC 6901: ~ becomes ~0, / becomes ~1
func EscapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
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
