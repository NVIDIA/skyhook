/*
 * LICENSE START
 *
 *    Copyright (c) NVIDIA CORPORATION.  All rights reserved.
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 *
 * LICENSE END
 */




package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/NVIDIA/skyhook/api/v1alpha1"
	skyhookNodesMock "github.com/NVIDIA/skyhook/internal/controller/mock"
	"github.com/NVIDIA/skyhook/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("skyhook controller tests", func() {

	It("Should pick the correct results", func() {
		left := &ctrl.Result{Requeue: true}
		right := &ctrl.Result{}

		Expect(PickResults(nil, nil)).To(BeNil())
		Expect(PickResults(left, nil)).To(BeEquivalentTo(left))
		Expect(PickResults(nil, right)).To(BeEquivalentTo(right))

		Expect(PickResults(left, right)).To(BeEquivalentTo(left))

		left = &ctrl.Result{}
		right = &ctrl.Result{Requeue: true}
		Expect(PickResults(left, right)).To(BeEquivalentTo(right))

		left = &ctrl.Result{RequeueAfter: time.Second * 10}
		right = &ctrl.Result{RequeueAfter: time.Second * 5}
		Expect(PickResults(left, right)).To(BeEquivalentTo(left))

		right = &ctrl.Result{RequeueAfter: time.Second * 15}
		Expect(PickResults(left, right)).To(BeEquivalentTo(right))
	})

	It("should map only pods we created", func() {

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foobar",
				Labels: map[string]string{
					fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): "foobar",
				},
			},
		}

		ret := podHandlerFunc(ctx, pod)
		Expect(ret).To(HaveLen(1))
		Expect(ret[0].Name).To(BeEquivalentTo("pod---foobar"))

		pod.Labels = map[string]string{"foo": "bar"}
		ret = podHandlerFunc(ctx, pod)
		Expect(ret).To(BeNil())

	})

	It("should not return if there are no skyhooks", func() {

		r, err := operator.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: ""}})
		Expect(err).ToNot(HaveOccurred())
		Expect(r.RequeueAfter).To(BeEquivalentTo(0))
		Expect(r.Requeue).To(Equal(false))

	})

	Context("cluster state", func() {
		It("should pick the correct number of nodes by percent", func() {

			testfunc := func(percent, count, expected int) {
				skyhooks := &v1alpha1.SkyhookList{
					Items: []v1alpha1.Skyhook{
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "skyhook1",
							},
							Spec: v1alpha1.SkyhookSpec{
								InterruptionBudget: v1alpha1.InterruptionBudget{
									Percent: ptr[int](percent),
								},
							},
						},
					},
				}

				nodes := &corev1.NodeList{
					Items: make([]corev1.Node, 0),
				}
				for i := 0; i < count; i++ {
					nodes.Items = append(nodes.Items,
						corev1.Node{
							ObjectMeta: v1.ObjectMeta{
								Name: fmt.Sprintf("node_%d", i),
								// Annotations: map[string]string{
								// 	"skyhook.nvidia.com/state": string(v1alpha1.ENABLED),
								// },
							},
						})
				}
				clusterState, err := BuildState(skyhooks, nodes)
				Expect(err).ToNot(HaveOccurred())

				for _, skyhook := range clusterState.skyhooks {
					picker := NewNodePicker(opts.GetRuntimeRequiredToleration())
					pick := picker.SelectNodes(skyhook)
					Expect(pick).To(HaveLen(expected))
				}
			}

			testfunc(20, 2, 1)
			testfunc(20, 6, 1)
			testfunc(20, 10, 2)
			testfunc(20, 15, 3)
			testfunc(0, 15, 1)

		})

		It("should pick the correct number of nodes by count", func() {

			testfunc := func(count, nodeCode, expected int) {
				skyhooks := &v1alpha1.SkyhookList{
					Items: []v1alpha1.Skyhook{
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "skyhook1",
							},
							Spec: v1alpha1.SkyhookSpec{
								InterruptionBudget: v1alpha1.InterruptionBudget{
									Count: ptr[int](count),
								},
							},
						},
					},
				}

				nodes := &corev1.NodeList{
					Items: make([]corev1.Node, 0),
				}
				for i := 0; i < nodeCode; i++ {
					nodes.Items = append(nodes.Items,
						corev1.Node{
							ObjectMeta: v1.ObjectMeta{
								Name: fmt.Sprintf("node_%d", i),
								// Annotations: map[string]string{
								// 	"skyhook.nvidia.com/state": string(v1alpha1.ENABLED),
								// },
							},
						})
				}

				clusterState, err := BuildState(skyhooks, nodes)
				Expect(err).ToNot(HaveOccurred())

				for _, skyhook := range clusterState.skyhooks {
					picker := NewNodePicker(opts.GetRuntimeRequiredToleration())
					pick := picker.SelectNodes(skyhook)
					Expect(pick).To(HaveLen(expected))
				}
			}

			testfunc(1, 2, 1)
			testfunc(2, 6, 2)
			testfunc(0, 10, 1)
		})
	})

	It("should merge interrupts", func() {
		packages := []*v1alpha1.Package{
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "bar"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "bar",
					Version: "3.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"ducks", "kittens"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "buz",
					Version: "2.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "log"},
				},
			},
		}

		// this faulty interrupt should not even be considered
		// as it's not in the packages
		interrupts := map[string][]*v1alpha1.Interrupt{
			"bogus": {
				{
					Type: v1alpha1.REBOOT,
				},
			},
		}
		configUpdates := make(map[string][]string)
		interrupt, _package := fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(interrupt.Services).To(BeEquivalentTo([]string{"bar", "ducks", "foo", "kittens", "log"}))
		Expect(_package).To(BeEquivalentTo("bar"))

		packages = []*v1alpha1.Package{
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "bar"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "bar",
					Version: "3.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"ducks", "kittens"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "buz",
					Version: "2.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "log"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name: "omg", Version: "1.2.3"},
				Interrupt: &v1alpha1.Interrupt{
					Type: v1alpha1.REBOOT,
				},
			},
		}

		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("omg"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.REBOOT))
		Expect(interrupt.Services).To(BeEmpty())

		packages = []*v1alpha1.Package{
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "bar"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "bar",
					Version: "3.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"ducks", "kittens"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "buz",
					Version: "2.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "log"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name: "omg", Version: "1.2.3"},
				Interrupt: &v1alpha1.Interrupt{
					Type: v1alpha1.REBOOT,
				},
			},
		}

		interrupts = map[string][]*v1alpha1.Interrupt{
			"foo": {
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"dogs"},
				},
			},
			"buz": {
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"cows"},
				},
			},
		}

		configUpdates = map[string][]string{
			"buz": {
				"foo",
			},
			"omg": {
				"bar",
			},
		}

		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("bar"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.SERVICE))
		Expect(interrupt.Services).To(BeEquivalentTo([]string{"bar", "cows", "ducks", "foo", "kittens"}))

		packages = []*v1alpha1.Package{
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "bar"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "bar",
					Version: "3.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"ducks", "kittens"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "buz",
					Version: "2.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type:     v1alpha1.SERVICE,
					Services: []string{"foo", "log"},
				},
			},
			{
				PackageRef: v1alpha1.PackageRef{
					Name: "omg", Version: "1.2.3"},
				Interrupt: &v1alpha1.Interrupt{
					Type: v1alpha1.REBOOT,
				},
			},
		}

		interrupts = map[string][]*v1alpha1.Interrupt{
			"foo": {
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"dogs"},
				},
			},
			"buz": {
				{
					Type:     v1alpha1.SERVICE,
					Services: []string{"cows"},
				},
			},
		}

		configUpdates = map[string][]string{
			"foo": {
				"foo",
			},
			"omg": {
				"bar",
			},
		}

		// configUpdate matches package so config interrupts are now added but not package interrupts
		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("bar"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.SERVICE))
		Expect(interrupt.Services).To(BeEquivalentTo([]string{"dogs", "ducks", "foo", "kittens", "log"}))
	})

	It("Should filter envs correctly", func() {
		envs := []corev1.EnvVar{
			{
				Name:  "DOGS",
				Value: "foobar",
			},
			{
				Name:  "CATS",
				Value: "foobar",
			},
			{
				Name:  "DUCKS",
				Value: "foobar",
			},
		}

		Expect(FilterEnv(envs, "NOTEXIST")).To(BeEquivalentTo([]corev1.EnvVar{
			{
				Name:  "DOGS",
				Value: "foobar",
			},
			{
				Name:  "CATS",
				Value: "foobar",
			},
			{
				Name:  "DUCKS",
				Value: "foobar",
			},
		}))

		Expect(FilterEnv(envs, "CATS")).To(BeEquivalentTo([]corev1.EnvVar{
			{
				Name:  "DOGS",
				Value: "foobar",
			},
			{
				Name:  "DUCKS",
				Value: "foobar",
			},
		}))

		Expect(FilterEnv(envs, "CATS", "DUCKS")).To(BeEquivalentTo([]corev1.EnvVar{
			{
				Name:  "DOGS",
				Value: "foobar",
			},
		}))

		Expect(FilterEnv(envs, "CATS", "DUCKS", "DOGS")).To(BeNil())
	})

	It("should pick highest priority interrupt", func() {
		packages := []*v1alpha1.Package{
			{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.2.1",
				},
				Interrupt: &v1alpha1.Interrupt{
					Type: v1alpha1.NOOP,
				},
			},
		}

		interrupts := make(map[string][]*v1alpha1.Interrupt)
		configUpdates := make(map[string][]string)
		interrupt, _package := fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.NOOP))
		Expect(_package).To(BeEquivalentTo("foo"))

		packages = append(packages, &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name: "bar", Version: "1.2.3"},
			Interrupt: &v1alpha1.Interrupt{
				Type:     v1alpha1.SERVICE,
				Services: []string{"foo", "bar"},
			},
		})

		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("bar"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.SERVICE))
		Expect(interrupt.Services).To(BeEquivalentTo([]string{"bar", "foo"}))

		packages = append(packages, &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name: "baz", Version: "1.2.3"},
			Interrupt: &v1alpha1.Interrupt{
				Type: v1alpha1.RESTART_ALL_SERVICES,
			},
		})

		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("baz"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.RESTART_ALL_SERVICES))
		Expect(interrupt.Services).To(BeEmpty())

		packages = append(packages, &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name: "omg", Version: "1.2.3"},
			Interrupt: &v1alpha1.Interrupt{
				Type: v1alpha1.REBOOT,
			},
		})

		interrupt, _package = fudgeInterruptWithPriority(packages, configUpdates, interrupts)
		Expect(interrupt).ToNot(BeNil())
		Expect(_package).To(BeEquivalentTo("omg"))
		Expect(interrupt.Type).To(BeEquivalentTo(v1alpha1.REBOOT))
		Expect(interrupt.Services).To(BeEmpty())
	})

	It("Check validations of skyhook options", func() {
		// good options
		opts := SkyhookOperatorOptions{
			Namespace:            "skyhook",
			MaxInterval:          time.Second * 61,
			ImagePullSecret:      "foo",
			CopyDirRoot:          "/tmp",
			ReapplyOnReboot:      true,
			RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
			AgentImage:           "foo:bar",
			PauseImage:           "foo:bar",
		}
		Expect(opts.Validate()).To(BeNil())

		// bad MaxInterval
		opts.MaxInterval = time.Second * 0
		Expect(opts.Validate()).ToNot(BeNil())

		// bad CopyDirRoot
		opts.MaxInterval = time.Second * 10
		opts.CopyDirRoot = "foo/bar"
		Expect(opts.Validate()).ToNot(BeNil())

		// bad RuntimeRequiredTaint
		opts.CopyDirRoot = "/tmp"
		opts.RuntimeRequiredTaint = "foo"
		Expect(opts.Validate()).ToNot(BeNil())

		// bad RuntimeRequiredTaint
		opts.RuntimeRequiredTaint = "foo=bar"
		Expect(opts.Validate()).ToNot(BeNil())

		// RuntimeRequiredTaint is a delete
		opts.RuntimeRequiredTaint = "skyhook.nvidia.com=runtime-required:NoExecute-"
		Expect(opts.Validate()).ToNot(BeNil())

		opts.AgentImage = ""
		Expect(opts.Validate()).ToNot(BeNil())

		opts.AgentImage = "foo"
		Expect(opts.Validate()).ToNot(BeNil())

		opts.PauseImage = ""
		Expect(opts.Validate()).ToNot(BeNil())

		opts.PauseImage = "bar"
		Expect(opts.Validate()).ToNot(BeNil())
	})
	It("Should group skyhooks by node correctly", func() {
		skyhooks := &v1alpha1.SkyhookList{
			Items: []v1alpha1.Skyhook{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "skyhook1",
					},
					Spec: v1alpha1.SkyhookSpec{
						NodeSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
							},
						},
						RuntimeRequired: true,
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "skyhook2",
					},
					Spec: v1alpha1.SkyhookSpec{
						NodeSelector: metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "foo",
									Operator: metav1.LabelSelectorOpExists,
								},
							},
						},
						RuntimeRequired: true,
					},
				},
			},
		}

		nodes := &corev1.NodeList{
			Items: []corev1.Node{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"foo": "bar",
						},
						UID: "node1",
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"foo": "baz",
						},
						UID: "node2",
					},
				},
			},
		}

		clusterState, err := BuildState(skyhooks, nodes)
		Expect(err).ToNot(HaveOccurred())

		node_to_skyhooks, _ := groupSkyhooksByNode(clusterState)
		Expect(node_to_skyhooks).To(HaveLen(2))
		Expect(node_to_skyhooks[nodes.Items[0].UID]).To(HaveLen(2))
		Expect(node_to_skyhooks[nodes.Items[1].UID]).To(HaveLen(1))
	})
	It("Should group skyhooks by node but ignore ones without runtime required", func() {
		skyhooks := &v1alpha1.SkyhookList{
			Items: []v1alpha1.Skyhook{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "skyhook1",
					},
					Spec: v1alpha1.SkyhookSpec{
						NodeSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
							},
						},
						RuntimeRequired: true,
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "skyhook2",
					},
					Spec: v1alpha1.SkyhookSpec{
						NodeSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"foo": "bar",
							},
						},
						RuntimeRequired: false,
					},
				},
			},
		}

		nodes := &corev1.NodeList{
			Items: []corev1.Node{
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				{
					ObjectMeta: v1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"foo": "baz",
						},
					},
				},
			},
		}

		clusterState, err := BuildState(skyhooks, nodes)
		Expect(err).ToNot(HaveOccurred())

		node_to_skyhooks, _ := groupSkyhooksByNode(clusterState)
		Expect(node_to_skyhooks).To(HaveLen(1))
		Expect(node_to_skyhooks[nodes.Items[0].UID]).To(HaveLen(1))
	})
	It("Should only select nodes to remove when all runtime required skyhooks have completed", func() {
		//node_mock := skyhookNodesMock.NewMockSkyhookNodes(GinkgoTestingT)
		skyhook_a_mock := skyhookNodesMock.MockSkyhookNodes{}
		skyhook_a_mock.EXPECT().IsComplete().Return(true)

		skyhook_b_mock := skyhookNodesMock.MockSkyhookNodes{}
		skyhook_b_mock.EXPECT().IsComplete().Return(false).Once()

		// skyhookNodes{
		// 	skyhook: &wrapper.Skyhook{
		// 		Updated: true,
		// 	},
		// 	nodes: []wrapper.SkyhookNode{},
		// }

		node1 := corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Name: "node1",
				UID:  "node1",
			},
		}

		node2 := corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Name: "node2",
				UID:  "node2",
			},
		}

		node_to_skyhooks := map[types.UID][]SkyhookNodes{
			node1.UID: {
				&skyhook_a_mock,
				&skyhook_b_mock,
			},
			node2.UID: {
				&skyhook_a_mock,
			},
		}

		node_map := map[types.UID]*corev1.Node{
			node1.UID: &node1,
			node2.UID: &node2,
		}

		to_remove := getRuntimeRequiredTaintCompleteNodes(node_to_skyhooks, node_map)
		Expect(to_remove).To(HaveLen(1))
		Expect(to_remove[0].UID).To(BeEquivalentTo(node2.UID))

		skyhook_b_mock.EXPECT().IsComplete().Return(true)
		to_remove = getRuntimeRequiredTaintCompleteNodes(node_to_skyhooks, node_map)
		Expect(to_remove).To(HaveLen(2))

	})
	It("CreateTolerationForTaint should tolerate the passed taint", func() {
		taint := corev1.Taint{
			Key:    "skyhook.nvidia.com",
			Value:  "runtime-required",
			Effect: "NoSchedule",
		}
		toleration := opts.GetRuntimeRequiredToleration()
		Expect(toleration.ToleratesTaint(&taint)).To(BeTrue())

	})
	It("Pods should always tolerate runtime required taint", func() {
		pod := operator.CreatePodFromPackage(
			&v1alpha1.Package{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.1.2",
				},
				Image: "foo/bar",
			},
			&wrapper.Skyhook{
				Skyhook: &v1alpha1.Skyhook{
					Spec: v1alpha1.SkyhookSpec{
						RuntimeRequired: true,
					},
				},
			},
			"node1",
			v1alpha1.StageApply,
		)
		found_toleration := false
		expected_toleration := opts.GetRuntimeRequiredToleration()
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.Key == expected_toleration.Key && toleration.Value == expected_toleration.Value && toleration.Effect == expected_toleration.Effect {
				found_toleration = true
				break
			}
		}
		Expect(found_toleration).To(BeTrue())
	})
	It("Interrupt pods should tolerate runtime required taint when it is runtime required", func() {
		pod := operator.CreateInterruptPodForPackage(
			&v1alpha1.Interrupt{
				Type: v1alpha1.REBOOT,
			},
			"argEncode",

			&v1alpha1.Package{
				PackageRef: v1alpha1.PackageRef{
					Name:    "foo",
					Version: "1.1.2",
				},
				Image: "foo/bar",
			},
			&wrapper.Skyhook{
				Skyhook: &v1alpha1.Skyhook{
					Spec: v1alpha1.SkyhookSpec{
						RuntimeRequired: true,
					},
				},
			},
			"node1",
		)
		found_toleration := false
		expected_toleration := opts.GetRuntimeRequiredToleration()
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.Key == expected_toleration.Key && toleration.Value == expected_toleration.Value && toleration.Effect == expected_toleration.Effect {
				found_toleration = true
				break
			}
		}
		Expect(found_toleration).To(BeTrue())
	})
})
