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

package preflight

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestPreflight(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Preflight Test Suite")
}

func discoveryWithGroups(groupVersions ...string) *fakediscovery.FakeDiscovery {
	resources := make([]*metav1.APIResourceList, 0, len(groupVersions))
	for _, gv := range groupVersions {
		resources = append(resources, &metav1.APIResourceList{GroupVersion: gv})
	}
	return &fakediscovery.FakeDiscovery{Fake: &clienttesting.Fake{Resources: resources}}
}

var _ = Describe("EnsureNodeWrightServed", func() {
	It("passes when the nodewright group is served", func() {
		disco := discoveryWithGroups("v1", "nodewright.nvidia.com/v1alpha1")
		Expect(EnsureNodeWrightServed(disco)).To(Succeed())
	})

	It("passes when both nodewright and legacy skyhook groups are served", func() {
		disco := discoveryWithGroups("nodewright.nvidia.com/v1alpha1", "skyhook.nvidia.com/v1alpha1")
		Expect(EnsureNodeWrightServed(disco)).To(Succeed())
	})

	It("returns an actionable error when only the legacy skyhook group is served", func() {
		disco := discoveryWithGroups("v1", "skyhook.nvidia.com/v1alpha1")
		err := EnsureNodeWrightServed(disco)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("nodewright.nvidia.com"))
		Expect(err.Error()).To(ContainSubstring("skyhook.nvidia.com"))
		Expect(err.Error()).To(ContainSubstring("upgrade"))
	})

	It("passes when neither group is served, letting the command surface its own NotFound", func() {
		disco := discoveryWithGroups("v1")
		Expect(EnsureNodeWrightServed(disco)).To(Succeed())
	})
})
