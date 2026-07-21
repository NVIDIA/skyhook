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
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"

	nwv1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	skyhookv1 "github.com/NVIDIA/nodewright/operator/api/v1alpha1"
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

	It("propagates a non-partial discovery error", func() {
		disco := &stubGroupsDiscovery{err: errors.New("connection refused")}
		err := EnsureNodeWrightServed(disco)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("returns the discovery error when the nodewright group itself failed partial discovery", func() {
		partial := &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{
			nwv1.GroupVersion: errors.New("stale aggregated apiservice"),
		}}
		disco := &stubGroupsDiscovery{groups: groupList("v1", skyhookv1.GroupVersion.Group), err: partial}
		err := EnsureNodeWrightServed(disco)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("stale aggregated apiservice"))
		// Must not mistake the failed target group for a legacy-only cluster.
		Expect(err.Error()).ToNot(ContainSubstring("upgrade"))
	})

	It("still reports legacy-only when a partial failure does not involve the nodewright group", func() {
		partial := &discovery.ErrGroupDiscoveryFailed{Groups: map[schema.GroupVersion]error{
			{Group: "metrics.k8s.io", Version: "v1beta1"}: errors.New("boom"),
		}}
		disco := &stubGroupsDiscovery{groups: groupList("v1", skyhookv1.GroupVersion.Group), err: partial}
		err := EnsureNodeWrightServed(disco)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("upgrade"))
	})
})

// stubGroupsDiscovery overrides only ServerGroups so tests can inject the partial and
// non-partial discovery errors the fake discovery client cannot produce on its own.
type stubGroupsDiscovery struct {
	discovery.DiscoveryInterface
	groups *metav1.APIGroupList
	err    error
}

func (s *stubGroupsDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	return s.groups, s.err
}

func groupList(names ...string) *metav1.APIGroupList {
	gl := &metav1.APIGroupList{}
	for _, n := range names {
		gl.Groups = append(gl.Groups, metav1.APIGroup{Name: n})
	}
	return gl
}
