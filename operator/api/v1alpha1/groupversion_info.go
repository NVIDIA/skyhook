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

// MIGRATION-SHIM: this entire package is the legacy skyhook.nvidia.com API group,
// kept only for the transition (mirror source + deprecation webhooks + converters).
// Delete the whole api/v1alpha1 package with the legacy group in the removal release.
//
// Package v1alpha1 contains API Schema definitions for the skyhook v1alpha1 API group
// +kubebuilder:object:generate=true
// +groupName=skyhook.nvidia.com
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "skyhook.nvidia.com", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &schemeBuilder{}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

// schemeBuilder mirrors the (now deprecated) controller-runtime scheme.Builder API
// without taking on its controller-runtime import — keeping this api package light
// per the upstream guidance in sigs.k8s.io/controller-runtime/pkg/scheme.
type schemeBuilder struct {
	runtime.SchemeBuilder
}

// Register queues types for AddKnownTypes against GroupVersion when AddToScheme is called.
func (b *schemeBuilder) Register(objects ...runtime.Object) {
	b.SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, objects...)
		metav1.AddToGroupVersion(s, GroupVersion)
		return nil
	})
}
