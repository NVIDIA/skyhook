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
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("node state delta merge", func() {
	const skyhookName = "gpu-init"
	key := nodeStateAnnotationKey(skyhookName)

	status := func(name string, state v1alpha1.State, stage v1alpha1.Stage) v1alpha1.PackageStatus {
		return v1alpha1.PackageStatus{Name: name, Version: "1.0.0", Image: "img", State: state, Stage: stage}
	}

	nodeWith := func(state v1alpha1.NodeState) *corev1.Node {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}
		if state != nil {
			raw, err := json.Marshal(state)
			Expect(err).ToNot(HaveOccurred())
			node.Annotations = map[string]string{key: string(raw)}
		}
		return node
	}

	Describe("computeNodeStateDelta", func() {
		It("reports only the entries the pass actually changed", func() {
			before := v1alpha1.NodeState{
				"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply),
				"b|1.0.0": status("b", v1alpha1.StateInProgress, v1alpha1.StageApply),
			}
			after := v1alpha1.NodeState{
				"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply), // untouched
				"b|1.0.0": status("b", v1alpha1.StateComplete, v1alpha1.StageApply),   // changed
			}

			delta := computeNodeStateDelta(before, after)
			Expect(delta).To(HaveLen(1))
			Expect(delta).To(HaveKey("b|1.0.0"))
			Expect(delta["b|1.0.0"].State).To(Equal(v1alpha1.StateComplete))
		})

		It("records an added entry and a removed one", func() {
			before := v1alpha1.NodeState{"gone|1.0.0": status("gone", v1alpha1.StateComplete, v1alpha1.StageConfig)}
			after := v1alpha1.NodeState{"new|1.0.0": status("new", v1alpha1.StateInProgress, v1alpha1.StageApply)}

			delta := computeNodeStateDelta(before, after)
			Expect(delta).To(HaveLen(2))
			Expect(delta["new|1.0.0"]).ToNot(BeNil())
			Expect(delta["gone|1.0.0"]).To(BeNil(), "a nil value marks a removal")
		})
	})

	Describe("apply", func() {
		// The whole point: an entry the pass never touched keeps whatever another writer put
		// there, rather than being restamped from the pass's snapshot.
		It("preserves a concurrent write to an entry the pass did not touch", func() {
			before := v1alpha1.NodeState{
				"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply),
				"b|1.0.0": status("b", v1alpha1.StateInProgress, v1alpha1.StageApply),
			}
			after := before.DeepCopy()
			after["b|1.0.0"] = status("b", v1alpha1.StateComplete, v1alpha1.StageApply)

			// meanwhile JobReconciler recorded package a complete
			current := before.DeepCopy()
			current["a|1.0.0"] = status("a", v1alpha1.StateComplete, v1alpha1.StageApply)

			merged := computeNodeStateDelta(before, after).apply(current)

			Expect(merged["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete), "concurrent completion must survive")
			Expect(merged["b|1.0.0"].State).To(Equal(v1alpha1.StateComplete), "the pass's own change must land")
		})

		It("keeps an entry another NodeWright added after the snapshot", func() {
			before := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply)}
			after := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateComplete, v1alpha1.StageApply)}
			current := v1alpha1.NodeState{
				"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply),
				"c|2.0.0": status("c", v1alpha1.StateInProgress, v1alpha1.StageConfig),
			}

			merged := computeNodeStateDelta(before, after).apply(current)
			Expect(merged).To(HaveKey("c|2.0.0"))
			Expect(merged["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete))
		})

		It("applies a removal even against a changed current value", func() {
			before := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageUninstall)}
			after := v1alpha1.NodeState{}
			current := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateErroring, v1alpha1.StageUninstall)}

			merged := computeNodeStateDelta(before, after).apply(current)
			Expect(merged).ToNot(HaveKey("a|1.0.0"))
		})
	})

	Describe("applyPassChanges", func() {
		It("merges node state while replaying the pass's other metadata edits", func() {
			original := nodeWith(v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply)})
			original.Labels = map[string]string{"keep": "yes", "drop": "soon"}

			modified := original.DeepCopy()
			raw, err := json.Marshal(v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateComplete, v1alpha1.StageApply)})
			Expect(err).ToNot(HaveOccurred())
			modified.Annotations[key] = string(raw)
			modified.Labels["added"] = "1"
			delete(modified.Labels, "drop")
			modified.Spec.Unschedulable = true

			// fresh carries a second NodeWright's key that the pass never saw
			fresh := original.DeepCopy()
			fresh.Annotations["nodewright.nvidia.com/nodeState_other"] = "{}"
			fresh.Labels["set-by-someone-else"] = "1"

			before, err := parseNodeState(original, key)
			Expect(err).ToNot(HaveOccurred())
			afterState, err := parseNodeState(modified, key)
			Expect(err).ToNot(HaveOccurred())

			target, err := applyPassChanges(fresh, original, modified, key, computeNodeStateDelta(before, afterState))
			Expect(err).ToNot(HaveOccurred())

			state, err := parseNodeState(target, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(state["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete))

			Expect(target.Annotations).To(HaveKey("nodewright.nvidia.com/nodeState_other"), "another NodeWright's key must survive")
			Expect(target.Labels).To(HaveKeyWithValue("set-by-someone-else", "1"), "a concurrent label must survive")
			Expect(target.Labels).To(HaveKeyWithValue("added", "1"), "the pass's addition must land")
			Expect(target.Labels).ToNot(HaveKey("drop"), "the pass's deletion must land")
			Expect(target.Spec.Unschedulable).To(BeTrue(), "cordon is this controller's alone")
		})

		It("keeps a stale in-flight entry when the pass is the one that is behind", func() {
			// The pass thinks a is still in_progress and says nothing about it; another writer
			// completed it. The pass must not drag it back.
			before := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply)}
			original := nodeWith(before)
			modified := original.DeepCopy() // pass touched other things, not node state
			modified.Labels = map[string]string{"touched": "1"}

			fresh := nodeWith(v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateComplete, v1alpha1.StageApply)})

			afterState, err := parseNodeState(modified, key)
			Expect(err).ToNot(HaveOccurred())
			target, err := applyPassChanges(fresh, original, modified, key, computeNodeStateDelta(before, afterState))
			Expect(err).ToNot(HaveOccurred())

			got, err := parseNodeState(target, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(got["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete))
			Expect(target.Labels).To(HaveKeyWithValue("touched", "1"))
		})

		It("is a no-op on node state when the pass changed none of it", func() {
			state := v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateInProgress, v1alpha1.StageApply)}
			original := nodeWith(state)
			modified := original.DeepCopy()

			// another writer completed the package while this pass touched nothing
			fresh := nodeWith(v1alpha1.NodeState{"a|1.0.0": status("a", v1alpha1.StateComplete, v1alpha1.StageApply)})

			before, err := parseNodeState(original, key)
			Expect(err).ToNot(HaveOccurred())
			afterState, err := parseNodeState(modified, key)
			Expect(err).ToNot(HaveOccurred())

			target, err := applyPassChanges(fresh, original, modified, key, computeNodeStateDelta(before, afterState))
			Expect(err).ToNot(HaveOccurred())

			got, err := parseNodeState(target, key)
			Expect(err).ToNot(HaveOccurred())
			Expect(got["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete))
		})
	})
})

var _ = Describe("saveNodeChanges", func() {
	const skyhookName = "gpu-init"
	const nodeName = "worker-1"
	key := nodeStateAnnotationKey(skyhookName)

	status := func(name string, state v1alpha1.State) v1alpha1.PackageStatus {
		return v1alpha1.PackageStatus{Name: name, Version: "1.0.0", Image: "img", State: state, Stage: v1alpha1.StageApply}
	}
	stateJSON := func(state v1alpha1.NodeState) string {
		raw, err := json.Marshal(state)
		Expect(err).ToNot(HaveOccurred())
		return string(raw)
	}

	// This is the regression the change exists for: the heavy pass computes its result from a
	// snapshot, JobReconciler records a completion in between, and then the pass writes. Restamping
	// the whole snapshot value reverted that completion, and nothing re-recorded it because the Job
	// was already marked state-recorded.
	It("does not revert a completion recorded after the pass took its snapshot", func() {
		snapshot := v1alpha1.NodeState{
			"a|1.0.0": status("a", v1alpha1.StateInProgress),
			"b|1.0.0": status("b", v1alpha1.StateInProgress),
		}
		original := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name: nodeName, Annotations: map[string]string{key: stateJSON(snapshot)},
		}}

		// what is actually stored by the time the pass writes: package a completed by JobReconcile
		storedState := snapshot.DeepCopy()
		storedState["a|1.0.0"] = status("a", v1alpha1.StateComplete)
		stored := original.DeepCopy()
		stored.Annotations[key] = stateJSON(storedState)

		scr := &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName, Namespace: "skyhook"}}

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stored, scr).Build()

		r, err := NewSkyhookReconciler(scheme, c, c, k8sfake.NewClientset(), events.NewFakeRecorder(10),
			SkyhookOperatorOptions{
				Namespace:            "skyhook",
				CopyDirRoot:          "/var/lib/skyhook",
				AgentLogRoot:         "/var/log/skyhook",
				RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
				AgentImage:           "ghcr.io/nvidia/skyhook/agent:1.2.3",
				PauseImage:           "registry.k8s.io/pause:3.10",
				MaxInterval:          10 * time.Minute,
				JobOperatorOptions: JobOperatorOptions{
					JobTTLSucceeded: time.Hour,
					JobTTLFailed:    24 * time.Hour,
					JobStageTimeout: time.Hour,
				},
			})
		Expect(err).ToNot(HaveOccurred())

		// the pass advanced package b, still believing a is in_progress
		passNode := original.DeepCopy()
		passState := snapshot.DeepCopy()
		passState["b|1.0.0"] = status("b", v1alpha1.StateComplete)
		passNode.Annotations[key] = stateJSON(passState)
		sn, err := wrapper.NewSkyhookNode(passNode, scr)
		Expect(err).ToNot(HaveOccurred())

		Expect(r.saveNodeChanges(ctx, original, sn, skyhookName)).To(Succeed())

		var got corev1.Node
		Expect(c.Get(ctx, types.NamespacedName{Name: nodeName}, &got)).To(Succeed())
		final, err := parseNodeState(&got, key)
		Expect(err).ToNot(HaveOccurred())

		Expect(final["a|1.0.0"].State).To(Equal(v1alpha1.StateComplete), "concurrent completion must survive the pass write")
		Expect(final["b|1.0.0"].State).To(Equal(v1alpha1.StateComplete), "the pass's own transition must land")
	})
})
