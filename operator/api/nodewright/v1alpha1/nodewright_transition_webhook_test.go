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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// These version-transition (downgrade/upgrade/uninstall) validation specs were moved
// here from the legacy api/v1alpha1 package. The legacy Skyhook is read-only once
// migrated (see legacy_readonly_webhook.go there), so its ValidateUpdate rejects the
// spec change before this validation runs; NodeWright is the writable object where the
// logic is live, so its coverage belongs here. The validation logic itself is unchanged.
var _ = Describe("NodeWright version-transition validation", func() {

	It("rejects downgrade of an enabled package without apply=true", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "2.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "1.0.0"}, // downgrade
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false}, // apply not set
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("uninstall.apply=true"))
		Expect(err.Error()).To(ContainSubstring("downgrad"))
	})

	It("rejects downgrade when old apply=false", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v2.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v1.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("set uninstall.apply=true first"))
	})

	It("rejects downgrade when node state still contains the package", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v2.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: true},
					},
				},
			},
			Status: NodeWrightStatus{
				NodeState: map[string]NodeState{
					"node-1": {
						"my-pkg|v2.0.0": PackageStatus{
							Name: "my-pkg", Version: "v2.0.0",
							Stage: StageUninstall, State: StateInProgress,
						},
					},
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v1.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: true},
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("uninstall has not yet completed"))
	})

	It("allows downgrade when old apply=true AND package absent from all nodes", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v2.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: true},
					},
				},
			},
			Status: NodeWrightStatus{
				NodeState: map[string]NodeState{
					"node-1": {}, // package absent = fully uninstalled per D2
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v1.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		Expect(err).ToNot(HaveOccurred())
	})

	It("allows upgrade regardless of apply setting", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v1.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "v2.0.0"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		Expect(err).ToNot(HaveOccurred())
	})

	It("skips the downgrade check for invalid semver (defers to Validate)", func() {
		oldNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "not-a-semver"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		newNodeWright := &NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
			Spec: NodeWrightSpec{
				Packages: Packages{
					"my-pkg": Package{
						PackageRef: PackageRef{Name: "my-pkg", Version: "also-invalid"},
						Image:      "my-image",
						Uninstall:  &Uninstall{Enabled: true, Apply: false},
					},
				},
			},
		}
		webhook := &NodeWrightWebhook{}
		_, err := webhook.ValidateUpdate(ctx, oldNodeWright, newNodeWright)
		// Either pass (skipped) or fail on the separate Validate() check, but NOT the
		// "set uninstall.apply=true first" downgrade message.
		if err != nil {
			Expect(err.Error()).ToNot(ContainSubstring("set uninstall.apply=true first"))
			Expect(err.Error()).ToNot(ContainSubstring("uninstall has not yet completed"))
		}
	})
})
