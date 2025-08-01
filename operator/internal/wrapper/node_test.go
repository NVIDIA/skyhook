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

package wrapper

import (
	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SkyhookNode", func() {
	Context("RunNext", func() {
		It("should return packages in deterministic order and respect dependencies", func() {
			node := corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
			}
			skyhook := v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"a-package": {
							PackageRef: v1alpha1.PackageRef{Name: "a-package", Version: "1.0"},
							Image:      "a-image",
						},
						"b-package": {
							PackageRef: v1alpha1.PackageRef{Name: "b-package", Version: "1.0"},
							Image:      "b-image",
						},
						"c-package": {
							PackageRef: v1alpha1.PackageRef{Name: "c-package", Version: "1.0"},
							Image:      "c-image",
							DependsOn:  map[string]string{"a-package": "1.0", "b-package": "1.0"},
						},
						"d-package": {
							PackageRef: v1alpha1.PackageRef{Name: "d-package", Version: "1.0"},
							Image:      "d-image",
							DependsOn:  map[string]string{"c-package": "1.0"},
						},
						"e-package": {
							PackageRef: v1alpha1.PackageRef{Name: "e-package", Version: "1.0"},
							Image:      "e-image",
							DependsOn:  map[string]string{"c-package": "1.0"},
						},
						"f-package": {
							PackageRef: v1alpha1.PackageRef{Name: "f-package", Version: "1.0"},
							Image:      "f-image",
							DependsOn:  map[string]string{"d-package": "1.0", "e-package": "1.0"},
						},
					},
				},
			}

			// Create node
			skyhookNode, err := NewSkyhookNode(&node, &skyhook)
			Expect(err).NotTo(HaveOccurred())

			// First run should return a and b in alphabetical order
			pkgs, err := skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(2))
			Expect(pkgs[0].Name).To(Equal("a-package"))
			Expect(pkgs[1].Name).To(Equal("b-package"))

			// Complete b-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "b-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Should still get a-package since c-package depends on both a and b
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(1))
			Expect(pkgs[0].Name).To(Equal("a-package"))

			// Complete a-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "a-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Now should get c-package since both dependencies are complete
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(1))
			Expect(pkgs[0].Name).To(Equal("c-package"))

			// Complete c-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "c-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Now should get d-package since c-package is complete
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(2))
			Expect(pkgs[0].Name).To(Equal("d-package"))
			Expect(pkgs[1].Name).To(Equal("e-package"))

			// Complete e-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "e-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Now should get d-package since c-package and e-package are complete
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(1))
			Expect(pkgs[0].Name).To(Equal("d-package"))

			// Complete d-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "d-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Now should get f-package since both d-package and e-package are complete
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(HaveLen(1))
			Expect(pkgs[0].Name).To(Equal("f-package"))

			// Complete f-package
			err = skyhookNode.Upsert(v1alpha1.PackageRef{Name: "f-package", Version: "1.0"}, "image", v1alpha1.StateComplete, v1alpha1.StageConfig, 0)
			Expect(err).NotTo(HaveOccurred())
			// Now should get nothing since all packages are complete
			pkgs, err = skyhookNode.RunNext()
			Expect(err).NotTo(HaveOccurred())
			Expect(pkgs).To(BeEmpty())
		})
	})
})
