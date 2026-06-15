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

package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	kubebuilderAssetsVar = "KUBEBUILDER_ASSETS"
	envtestK8SVersionVar = "ENVTEST_K8S_VERSION"
)

func BinaryAssetsDirectory() (string, error) {
	if assets := os.Getenv(kubebuilderAssetsVar); assets != "" {
		return assets, nil
	}

	operatorRoot, err := findOperatorRoot()
	if err != nil {
		return "", err
	}

	version := strings.TrimPrefix(os.Getenv(envtestK8SVersionVar), "v")
	if version == "" {
		version, err = makeDefault(operatorRoot, envtestK8SVersionVar)
		if err != nil {
			return "", err
		}
	}

	return filepath.Join(operatorRoot, "bin", "k8s",
		fmt.Sprintf("%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)), nil
}

func findOperatorRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "k8s-test-versions.mk")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("checking k8s-test-versions.mk: %w", err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("finding operator root from %s", dir)
		}
		dir = parent
	}
}

func makeDefault(operatorRoot, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(operatorRoot, "k8s-test-versions.mk"))
	if err != nil {
		return "", fmt.Errorf("reading k8s-test-versions.mk: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, name) {
			continue
		}

		remainder := strings.TrimSpace(strings.TrimPrefix(line, name))
		if !strings.HasPrefix(remainder, "?=") {
			continue
		}

		value := strings.TrimSpace(strings.TrimPrefix(remainder, "?="))
		if beforeComment, _, ok := strings.Cut(value, "#"); ok {
			value = strings.TrimSpace(beforeComment)
		}
		value = strings.TrimPrefix(strings.Trim(value, `"'`), "v")
		if value == "" {
			return "", fmt.Errorf("%s is empty in k8s-test-versions.mk", name)
		}
		return value, nil
	}

	return "", fmt.Errorf("%s not found in k8s-test-versions.mk", name)
}
