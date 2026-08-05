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

package wrapper

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("migrateNodePrefixToNodeWright", func() {

	newKey := func(suffix string) string {
		return fmt.Sprintf("%s/%s", v1alpha1.METADATA_PREFIX, suffix)
	}
	oldKey := func(suffix string) string {
		return fmt.Sprintf("%s/%s", legacySkyhookMetadataPrefix, suffix)
	}

	conditionTypes := func(node *skyhookNode) []string {
		var out []string
		for _, c := range node.GetNode().Status.Conditions {
			out = append(out, string(c.Type))
		}
		return out
	}

	It("copies legacy annotations, labels, and conditions to the nodewright prefix, keeping the legacy keys", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker",
					Annotations: map[string]string{
						oldKey("nodeState_myskyhook"): `{"pkg":"state"}`,
						oldKey("version_myskyhook"):   "v0.17.0",
						"kubernetes.io/arch":          "amd64",
					},
					Labels: map[string]string{
						oldKey("status_myskyhook"): "complete",
						"topology/zone":            "a",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeConditionType(oldKey("myskyhook/NotReady")), Status: corev1.ConditionFalse},
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
		}

		Expect(migrateNodePrefixToNodeWright(node, logr.Discard())).To(Succeed())

		// New-prefix copies exist so the new operator adopts the node...
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("nodeState_myskyhook"), `{"pkg":"state"}`))
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("version_myskyhook"), "v0.17.0"))
		Expect(node.Labels).To(HaveKeyWithValue(newKey("status_myskyhook"), "complete"))
		// ...and the legacy keys are KEPT so a rolled-back pre-rename operator still reads them.
		Expect(node.Annotations).To(HaveKeyWithValue(oldKey("nodeState_myskyhook"), `{"pkg":"state"}`))
		Expect(node.Annotations).To(HaveKeyWithValue(oldKey("version_myskyhook"), "v0.17.0"))
		Expect(node.Labels).To(HaveKeyWithValue(oldKey("status_myskyhook"), "complete"))
		Expect(node.Annotations).To(HaveKeyWithValue("kubernetes.io/arch", "amd64"))
		Expect(node.Labels).To(HaveKeyWithValue("topology/zone", "a"))

		Expect(conditionTypes(node)).To(ContainElements(
			newKey("myskyhook/NotReady"), oldKey("myskyhook/NotReady"), string(corev1.NodeReady)))
		Expect(node.Changed()).To(BeTrue())
	})

	It("is a no-op with no legacy keys and does not mark the node changed", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "worker",
					Annotations: map[string]string{newKey("nodeState_myskyhook"): `{"pkg":"state"}`},
				},
			},
		}

		Expect(migrateNodePrefixToNodeWright(node, logr.Discard())).To(Succeed())

		Expect(node.Annotations).To(HaveKeyWithValue(newKey("nodeState_myskyhook"), `{"pkg":"state"}`))
		Expect(node.Changed()).To(BeFalse())
	})

	It("keeps the explicit new-prefix value and retains the legacy duplicate on collision", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker",
					Annotations: map[string]string{
						newKey("version_myskyhook"): "new",
						oldKey("version_myskyhook"): "legacy",
					},
				},
			},
		}

		Expect(migrateNodePrefixToNodeWright(node, logr.Discard())).To(Succeed())

		// Explicit nodewright value wins deterministically; the legacy key is retained.
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("version_myskyhook"), "new"))
		Expect(node.Annotations).To(HaveKeyWithValue(oldKey("version_myskyhook"), "legacy"))
	})

	It("adopts a pre-rename node during Migrate so it is not treated as fresh", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "worker",
					Annotations: map[string]string{oldKey("version_myskyhook"): "v0.17.0"},
				},
			},
		}

		Expect(node.Migrate(logr.Discard())).To(Succeed())

		// The legacy version annotation is adopted under the new prefix, so the node
		// reports the stored version rather than "" (which would read as fresh). The
		// legacy key is kept until the rollback window elapses.
		Expect(node.GetVersion()).To(Equal("v0.17.0"))
		Expect(node.Annotations).To(HaveKey(oldKey("version_myskyhook")))
	})

	It("PruneLegacyMetadata drops the legacy keys/labels/conditions and keeps the nodewright copies", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker",
					Annotations: map[string]string{
						oldKey("version_myskyhook"): "v0.17.0",
						newKey("version_myskyhook"): "v0.17.0",
						"kubernetes.io/arch":        "amd64",
					},
					Labels: map[string]string{
						oldKey("status_myskyhook"): "complete",
						newKey("status_myskyhook"): "complete",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeConditionType(oldKey("myskyhook/NotReady"))},
						{Type: corev1.NodeConditionType(newKey("myskyhook/NotReady"))},
						{Type: corev1.NodeReady},
					},
				},
			},
		}

		Expect(node.PruneLegacyMetadata()).To(BeTrue())

		Expect(node.Annotations).ToNot(HaveKey(oldKey("version_myskyhook")))
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("version_myskyhook"), "v0.17.0"))
		Expect(node.Annotations).To(HaveKeyWithValue("kubernetes.io/arch", "amd64"))
		Expect(node.Labels).ToNot(HaveKey(oldKey("status_myskyhook")))
		Expect(node.Labels).To(HaveKeyWithValue(newKey("status_myskyhook"), "complete"))

		Expect(conditionTypes(node)).To(ContainElements(newKey("myskyhook/NotReady"), string(corev1.NodeReady)))
		Expect(conditionTypes(node)).ToNot(ContainElement(oldKey("myskyhook/NotReady")))
	})

	// The metadata prefix is the product's domain name, so users legitimately put
	// their own keys under it. Those are not the operator's to migrate or delete.
	It("leaves user-owned keys under the legacy prefix completely alone", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker",
					Annotations: map[string]string{
						oldKey("nodeState_myskyhook"): `{"pkg":"state"}`,
						oldKey("owner"):               "platform-team",
					},
					Labels: map[string]string{
						oldKey("status_myskyhook"): "complete",
						oldKey("pool"):             "gpu",
						oldKey("ignore"):           "true",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeConditionType(oldKey("myskyhook/NotReady")), Status: corev1.ConditionFalse},
						{Type: corev1.NodeConditionType(oldKey("SomeUserCondition")), Status: corev1.ConditionTrue},
					},
				},
			},
		}

		Expect(migrateNodePrefixToNodeWright(node, logr.Discard())).To(Succeed())

		By("not copying a user annotation or label into the operator's namespace")
		Expect(node.Annotations).ToNot(HaveKey(newKey("owner")))
		Expect(node.Labels).ToNot(HaveKey(newKey("pool")))
		Expect(node.Annotations).To(HaveKeyWithValue(oldKey("owner"), "platform-team"))
		Expect(node.Labels).To(HaveKeyWithValue(oldKey("pool"), "gpu"))

		By("still migrating the operator's own keys, including the operator-defined ignore label")
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("nodeState_myskyhook"), `{"pkg":"state"}`))
		Expect(node.Labels).To(HaveKeyWithValue(newKey("status_myskyhook"), "complete"))
		Expect(node.Labels).To(HaveKeyWithValue(newKey("ignore"), "true"))

		By("not copying a condition type the operator does not write")
		Expect(conditionTypes(node)).ToNot(ContainElement(newKey("SomeUserCondition")))
		Expect(conditionTypes(node)).To(ContainElement(newKey("myskyhook/NotReady")))

		By("pruning only the operator's own keys, leaving the user's behind")
		Expect(node.PruneLegacyMetadata()).To(BeTrue())
		Expect(node.Annotations).To(HaveKeyWithValue(oldKey("owner"), "platform-team"))
		Expect(node.Labels).To(HaveKeyWithValue(oldKey("pool"), "gpu"))
		Expect(node.Annotations).ToNot(HaveKey(oldKey("nodeState_myskyhook")))
		Expect(node.Labels).ToNot(HaveKey(oldKey("status_myskyhook")))
		Expect(node.Labels).ToNot(HaveKey(oldKey("ignore")))
		Expect(conditionTypes(node)).To(ContainElement(oldKey("SomeUserCondition")))
		Expect(conditionTypes(node)).ToNot(ContainElement(oldKey("myskyhook/NotReady")))
	})

	It("PruneLegacyMetadata is a no-op when no legacy keys remain", func() {
		node := &skyhookNode{
			skyhookName: "myskyhook",
			Node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "worker",
					Annotations: map[string]string{newKey("version_myskyhook"): "v0.17.0"},
				},
			},
		}

		Expect(node.PruneLegacyMetadata()).To(BeFalse())
		Expect(node.Annotations).To(HaveKeyWithValue(newKey("version_myskyhook"), "v0.17.0"))
	})
})
