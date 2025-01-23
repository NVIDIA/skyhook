/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package controller

import (
	"encoding/json"
	"fmt"

	"gitlab-master.nvidia.com/dgx/infra/skyhook-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type PackageSkyhook struct {
	v1alpha1.PackageRef `json:",inline"`
	Skyhook             string         `json:"skyhook"`
	Stage               v1alpha1.Stage `json:"stage"`
	Image               string         `json:"image"`
}

func GetPackage(pod *corev1.Pod) (*PackageSkyhook, error) {
	if pod == nil {
		return nil, nil
	}
	s, ok := pod.Annotations[fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)]
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

func SetPackages(pod *corev1.Pod, skyhook *v1alpha1.Skyhook, image string, stage v1alpha1.Stage, _package *v1alpha1.Package) error {
	if pod == nil || _package == nil {
		return nil
	}

	strk := &PackageSkyhook{
		Skyhook:    skyhook.Name,
		Stage:      stage,
		PackageRef: _package.PackageRef,
		Image:      image,
	}

	data, err := json.Marshal(strk)
	if err != nil {
		return fmt.Errorf("error marshalling package: %w", err)
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)] = string(data)

	return nil
}
