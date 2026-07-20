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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("legacy read-only webhook", func() {
	pauseKey := fmt.Sprintf("%s/pause", METADATA_PREFIX)
	disableKey := fmt.Sprintf("%s/disable", METADATA_PREFIX)

	base := func() *Skyhook {
		return &Skyhook{
			ObjectMeta: metav1.ObjectMeta{Name: "demo"},
			Spec: SkyhookSpec{
				InterruptionBudget: InterruptionBudget{Percent: ptr.To(25)},
				Packages: Packages{
					"p": Package{PackageRef: PackageRef{Name: "p", Version: "1.0.0"}, Image: "alpine:3.21.0"},
				},
			},
		}
	}

	It("allows a create (no old object)", func() {
		Expect(legacyReadOnlyError(nil, base())).To(Succeed())
	})

	It("allows a no-op update (identical spec and control annotations)", func() {
		Expect(legacyReadOnlyError(base(), base())).To(Succeed())
	})

	It("rejects a spec change", func() {
		oldSH, newSH := base(), base()
		newSH.Spec.InterruptionBudget.Percent = ptr.To(50)
		Expect(legacyReadOnlyError(oldSH, newSH)).To(MatchError(ContainSubstring("read-only")))
	})

	It("rejects toggling the pause annotation", func() {
		oldSH, newSH := base(), base()
		newSH.Annotations = map[string]string{pauseKey: "true"}
		Expect(legacyReadOnlyError(oldSH, newSH)).To(MatchError(ContainSubstring("NodeWright")))
	})

	It("rejects toggling the disable annotation", func() {
		oldSH, newSH := base(), base()
		newSH.Annotations = map[string]string{disableKey: "true"}
		Expect(legacyReadOnlyError(oldSH, newSH)).To(HaveOccurred())
	})

	It("allows an incidental (non pause/disable) annotation change so GitOps metadata churn is not blocked", func() {
		oldSH, newSH := base(), base()
		newSH.Annotations = map[string]string{"example.com/note": "x"}
		Expect(legacyReadOnlyError(oldSH, newSH)).To(Succeed())
	})

	It("allows any update once the object is being deleted (finalizer strip / delete)", func() {
		oldSH, newSH := base(), base()
		now := metav1.Now()
		newSH.DeletionTimestamp = &now
		newSH.Spec.InterruptionBudget.Percent = ptr.To(50) // even a spec diff is allowed during deletion
		Expect(legacyReadOnlyError(oldSH, newSH)).To(Succeed())
	})

	It("rejects a DeploymentPolicy spec change but allows no-op and deletion", func() {
		oldDP := &DeploymentPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "dp"},
			Spec:       DeploymentPolicySpec{Default: PolicyDefault{Budget: DeploymentBudget{Count: ptr.To(3)}}},
		}

		Expect(legacyDeploymentPolicyReadOnlyError(oldDP, oldDP.DeepCopy())).To(Succeed())

		changed := oldDP.DeepCopy()
		changed.Spec.Default.Budget.Count = ptr.To(5)
		Expect(legacyDeploymentPolicyReadOnlyError(oldDP, changed)).To(MatchError(ContainSubstring("read-only")))

		deleting := changed.DeepCopy()
		now := metav1.Now()
		deleting.DeletionTimestamp = &now
		Expect(legacyDeploymentPolicyReadOnlyError(oldDP, deleting)).To(Succeed())
	})
})
