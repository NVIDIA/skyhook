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

package controller

import (
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PackageSkyhook struct {
	v1alpha1.PackageRef `json:",inline"`
	Skyhook             string         `json:"skyhook"`
	Stage               v1alpha1.Stage `json:"stage"`
	Image               string         `json:"image"`
	ContainerSHA        string         `json:"containerSHA,omitempty"`
	Invalid             bool           `json:"invalid,omitempty"`
}

// GetPackage returns the package from the object's annotations. The object is any
// executor carrying the package annotation — a package Pod or, once package stages
// run as Jobs, a batch/v1 Job (and its pod template). Only metadata is touched.
func GetPackage(obj client.Object) (*PackageSkyhook, error) {
	if obj == nil {
		return nil, nil
	}
	s, ok := obj.GetAnnotations()[fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)]
	if !ok {
		return nil, nil
	}
	ret := &PackageSkyhook{}
	err := json.Unmarshal([]byte(s), &ret)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling package: %w", err)
	}

	return ret, nil
}

// SetPackages sets the package in the object's annotations
func SetPackages(obj client.Object, skyhook *v1alpha1.NodeWright, image string, stage v1alpha1.Stage, _package *v1alpha1.Package) error {
	if obj == nil || _package == nil {
		return nil
	}

	strk := &PackageSkyhook{
		Skyhook:      skyhook.Name,
		Stage:        stage,
		PackageRef:   _package.PackageRef,
		Image:        image,
		ContainerSHA: _package.ContainerSHA,
	}

	data, err := json.Marshal(strk)
	if err != nil {
		return fmt.Errorf("error marshalling package: %w", err)
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)] = string(data)
	obj.SetAnnotations(annotations)

	return nil
}

// InvalidatePackage invalidates a package and updates the object, which will trigger the executor to be deleted
func InvalidatePackage(obj client.Object) error {
	if obj == nil {
		return nil
	}

	pkg, err := GetPackage(obj)
	if err != nil {
		return fmt.Errorf("error getting package: %w", err)
	}

	pkg.Invalid = true

	data, err := json.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("error marshalling package: %w", err)
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)] = string(data)
	obj.SetAnnotations(annotations)

	return nil
}

// IsInvalidPackage returns true if the package is invalid
func IsInvalidPackage(obj client.Object) (bool, error) {
	if obj == nil {
		return false, nil
	}

	pkg, err := GetPackage(obj)
	if err != nil {
		return false, fmt.Errorf("error getting package: %w", err)
	}
	return pkg.Invalid, nil
}
