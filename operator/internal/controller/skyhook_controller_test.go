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
	"testing"
	"time"

	"github.com/NVIDIA/skyhook/api/v1alpha1"
	skyhookNodesMock "github.com/NVIDIA/skyhook/internal/controller/mock"
	"github.com/NVIDIA/skyhook/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("skyhook controller tests", func() {

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

	It("Ensure all the config env vars are set", func() {
		opts := SkyhookOperatorOptions{
			Namespace:            "skyhook",
			MaxInterval:          time.Second * 61,
			ImagePullSecret:      "foo",
			CopyDirRoot:          "/tmp",
			ReapplyOnReboot:      true,
			RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
			AgentImage:           "foo:bar",
			PauseImage:           "foo:bar",
			AgentLogRoot:         "/log",
		}
		Expect(opts.Validate()).To(BeNil())

		envs := getAgentConfigEnvVars(opts, "package", "version", "id", "skyhook_name")
		expected := []corev1.EnvVar{
			{
				Name:  "SKYHOOK_LOG_DIR",
				Value: "/log/skyhook_name",
			},
			{
				Name:  "SKYHOOK_ROOT_DIR",
				Value: "/tmp/skyhook_name",
			},
			{
				Name:  "COPY_RESOLV",
				Value: "false",
			},
			{
				Name:  "SKYHOOK_RESOURCE_ID",
				Value: "id_package_version",
			},
		}
		Expect(envs).To(BeEquivalentTo(expected))
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
		pod := createPodFromPackage(
			operator.opts,
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
		pod := createInterruptPodForPackage(
			operator.opts,
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

	It("should generate deterministic pod names", func() {
		// Setup basic test data
		skyhook := &wrapper.Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
			},
		}

		package1 := &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name:    "test-package",
				Version: "1.2.3",
			},
		}

		package2 := &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name:    "test-package",
				Version: "1.2.4",
			},
		}

		nodeName := "test-node"
		nodeName2 := "test-node-2"

		// Create a function to generate the namePrefix in the same way the controller does
		createNamePrefix := func(skyhookName, pkgName, pkgVersion, stage string) string {
			return fmt.Sprintf("%s-%s-%s-%s", skyhookName, pkgName, pkgVersion, stage)
		}

		// Test 1: Deterministic behavior (same inputs = same output)
		prefix1 := createNamePrefix(skyhook.Name, package1.Name, package1.Version, string(v1alpha1.StageApply))
		name1 := generateSafeName(63, prefix1, nodeName)
		name2 := generateSafeName(63, prefix1, nodeName)
		Expect(name1).To(Equal(name2), "Generated pod names should be deterministic")

		// Test 2: Uniqueness with different inputs
		// Different stage
		prefixApply := createNamePrefix(skyhook.Name, package1.Name, package1.Version, string(v1alpha1.StageApply))
		prefixConfig := createNamePrefix(skyhook.Name, package1.Name, package1.Version, string(v1alpha1.StageConfig))
		nameApply := generateSafeName(63, prefixApply, nodeName)
		nameConfig := generateSafeName(63, prefixConfig, nodeName)
		Expect(nameApply).NotTo(Equal(nameConfig), "Different stages should produce different pod names")

		// Different package version
		prefix2 := createNamePrefix(skyhook.Name, package2.Name, package2.Version, string(v1alpha1.StageApply))
		nameVersion1 := generateSafeName(63, prefix1, nodeName)
		nameVersion2 := generateSafeName(63, prefix2, nodeName)
		Expect(nameVersion1).NotTo(Equal(nameVersion2), "Different package versions should produce different pod names")

		// Different node
		nameNode1 := generateSafeName(63, prefix1, nodeName)
		nameNode2 := generateSafeName(63, prefix1, nodeName2)
		Expect(nameNode1).NotTo(Equal(nameNode2), "Different nodes should produce different pod names")

		// Test for uninstall pods with timestamp
		uninstallPrefix1 := fmt.Sprintf("%s-uninstall-123456789", prefixApply)
		uninstallPrefix2 := fmt.Sprintf("%s-uninstall-987654321", prefixApply)
		uninstallName1 := generateSafeName(63, uninstallPrefix1, nodeName)
		uninstallName2 := generateSafeName(63, uninstallPrefix2, nodeName)
		Expect(uninstallName1).NotTo(Equal(uninstallName2), "Uninstall pods with different timestamps should have different names")
		Expect(uninstallName1).NotTo(Equal(nameApply), "Uninstall pod name should be different from regular pod name")

		// Test 3: Length constraints
		longSkyhookName := "this-is-a-very-long-skyhook-name-that-exceeds-kubernetes-naming-limits-by-a-significant-margin"
		longPackageName := "this-is-a-very-long-package-name-that-also-exceeds-kubernetes-naming-limits"
		longPackageVersion := "1.2.3.4.5.6.7.8.9.10"
		longPrefix := createNamePrefix(longSkyhookName, longPackageName, longPackageVersion, string(v1alpha1.StageApply))
		longName := generateSafeName(63, longPrefix, "node1")
		Expect(len(longName)).To(BeNumerically("<=", 63), "Pod name should not exceed Kubernetes 63 character limit")
		Expect(longName).To(MatchRegexp(`-[0-9a-f]+$`), "Pod name should end with a hash component")
	})

	It("should correctly identify if a pod matches a package", func() {

		// Create a test package
		testPackage := &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name:    "test-package",
				Version: "1.2.3",
			},
			Image: "test-image:1.2.3",
			Resources: &v1alpha1.ResourceRequirements{
				CPURequest:    resource.MustParse("100m"),
				CPULimit:      resource.MustParse("200m"),
				MemoryRequest: resource.MustParse("64Mi"),
				MemoryLimit:   resource.MustParse("128Mi"),
			},
		}

		// Create a test skyhook
		testSkyhook := &wrapper.Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
				Spec: v1alpha1.SkyhookSpec{
					Packages: v1alpha1.Packages{
						"test-package": *testPackage,
					},
				},
			},
		}

		// Stage to test
		testStage := v1alpha1.StageApply

		// Create actual pods that would be created by the operator functions
		// First using CreatePodFromPackage
		actualPod := createPodFromPackage(operator.opts, testPackage, testSkyhook, "test-node", testStage)

		// Verify that the pod matches the package according to PodMatchesPackage
		matches := podMatchesPackage(operator.opts, testPackage, *actualPod, testSkyhook, testStage)
		Expect(matches).To(BeTrue(), "PodMatchesPackage should recognize the pod it created")

		// Now let's modify the package version and see if it correctly identifies non-matches
		modifiedPackage := testPackage.DeepCopy()
		modifiedPackage.Version = "1.2.4"

		matches = podMatchesPackage(operator.opts, modifiedPackage, *actualPod, testSkyhook, testStage)
		Expect(matches).To(BeFalse(), "PodMatchesPackage should not match when package version changed")

		// Test with different stage
		matches = podMatchesPackage(operator.opts, testPackage, *actualPod, testSkyhook, v1alpha1.StageConfig)
		Expect(matches).To(BeFalse(), "PodMatchesPackage should not match when stage changed")

		// Test with interrupt pods
		interruptPod := createInterruptPodForPackage(
			operator.opts,
			&v1alpha1.Interrupt{
				Type: v1alpha1.REBOOT,
			},
			"argEncode",
			testPackage,
			testSkyhook,
			"test-node",
		)

		// Verify that the interrupt pod matches the package
		matches = podMatchesPackage(operator.opts, testPackage, *interruptPod, testSkyhook, testStage)
		Expect(matches).To(BeTrue(), "PodMatchesPackage should recognize the interrupt pod it created")
	})

	It("should generate valid volume names", func() {
		tests := []struct {
			name        string
			prefix      string
			nodeName    string
			expectedLen int
			shouldMatch string
			description string
		}{
			{
				name:        "short name",
				prefix:      "metadata",
				nodeName:    "node1",
				expectedLen: 23, // "metadata-node1-" + 8 char hash
				description: "should handle short names",
			},
			{
				name:        "very long node name",
				prefix:      "metadata",
				nodeName:    "very-long-node-name-that-exceeds-kubernetes-limits-and-needs-to-be-truncated-to-something-shorter",
				expectedLen: 63,
				description: "should handle long names by hashing",
			},
			{
				name:        "consistent hashing",
				prefix:      "metadata",
				nodeName:    "node1",
				shouldMatch: generateSafeName(63, "metadata", "node1"),
				description: "should generate consistent names for the same input",
			},
		}

		for _, tt := range tests {
			result := generateSafeName(63, tt.prefix, tt.nodeName)

			if tt.expectedLen > 0 {
				Expect(len(result)).To(Equal(tt.expectedLen), tt.description)
			}
			if tt.shouldMatch != "" {
				Expect(result).To(Equal(tt.shouldMatch), tt.description)
			}
			Expect(len(result)).To(BeNumerically("<=", 63), "volume name should never exceed 63 characters")
			Expect(result).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`), "volume name should match kubernetes naming requirements")
		}
	})

	It("should generate valid configmap names", func() {
		tests := []struct {
			name        string
			skyhookName string
			nodeName    string
			expectedLen int
			shouldMatch string
			description string
		}{
			{
				name:        "short names",
				skyhookName: "skyhook1",
				nodeName:    "node1",
				expectedLen: 32, // "skyhook1-node1-metadata-" + 8 char hash
				description: "should handle short names",
			},
			{
				name:        "very long names",
				skyhookName: "very-long-skyhook-name",
				nodeName:    "very-long-node-name-that-exceeds-kubernetes-limits-and-needs-to-be-truncated",
				expectedLen: 63,
				description: "should handle long names by truncating and hashing",
			},
			{
				name:        "consistent hashing",
				skyhookName: "skyhook1",
				nodeName:    "node1",
				shouldMatch: generateSafeName(63, "skyhook1", "node1", "metadata"),
				description: "should generate consistent names for the same input",
			},
			{
				name:        "handles dots in names",
				skyhookName: "skyhook.1",
				nodeName:    "node.1",
				expectedLen: 34,
				description: "should handle dots in names consistently",
			},
		}

		for _, tt := range tests {
			result := generateSafeName(63, tt.skyhookName, tt.nodeName, "metadata")

			if tt.expectedLen > 0 {
				Expect(len(result)).To(Equal(tt.expectedLen), tt.description)
			}
			if tt.shouldMatch != "" {
				Expect(result).To(Equal(tt.shouldMatch), tt.description)
			}
			Expect(len(result)).To(BeNumerically("<=", 63), "configmap name should never exceed 63 characters")
			Expect(result).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`), "configmap name should match kubernetes naming requirements")
		}
	})
})

var _ = Describe("Resource Comparison", func() {
	var (
		expectedPod *corev1.Pod
		actualPod   *corev1.Pod
		skyhook     *wrapper.Skyhook
		package_    *v1alpha1.Package
	)

	BeforeEach(func() {
		// Setup common test objects
		nodeName := "testNode"
		stage := v1alpha1.StageApply
		package_ = &v1alpha1.Package{
			PackageRef: v1alpha1.PackageRef{
				Name:    "test-package",
				Version: "1.0.0",
			},
			Image: "test-image",
		}

		skyhook = &wrapper.Skyhook{
			Skyhook: &v1alpha1.Skyhook{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-skyhook",
				},
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"test-package": *package_,
					},
				},
			},
		}

		// Create base pod structure, to much work to do it again
		expectedPod = createPodFromPackage(operator.opts, package_, skyhook, nodeName, stage)
		actualPod = expectedPod.DeepCopy()
	})

	It("should match when resources are identical", func() {
		// Setup: Add resources to package and expected pod
		newPackage := *package_
		newPackage.Resources = &v1alpha1.ResourceRequirements{
			CPURequest:    resource.MustParse("100m"),
			CPULimit:      resource.MustParse("200m"),
			MemoryRequest: resource.MustParse("128Mi"),
			MemoryLimit:   resource.MustParse("256Mi"),
		}
		skyhook.Spec.Packages["test-package"] = newPackage

		expectedResources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		}

		// Set resources for all init containers in expected pod
		for i := range expectedPod.Spec.InitContainers {
			expectedPod.Spec.InitContainers[i].Resources = expectedResources
		}

		// Test: Set actual pod resources to match expected
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Resources = expectedResources
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeTrue())
	})

	It("should not match when resources differ", func() {
		// Setup: Add resources to package and expected pod
		newPackage := *package_
		newPackage.Resources = &v1alpha1.ResourceRequirements{
			CPURequest:    resource.MustParse("100m"),
			CPULimit:      resource.MustParse("200m"),
			MemoryRequest: resource.MustParse("128Mi"),
			MemoryLimit:   resource.MustParse("256Mi"),
		}
		skyhook.Spec.Packages["test-package"] = newPackage

		expectedResources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		}

		// Set resources for all init containers in expected pod
		for i := range expectedPod.Spec.InitContainers {
			expectedPod.Spec.InitContainers[i].Resources = expectedResources
		}

		// Test: Set different CPU request in actual pod for all init containers
		differentResources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"), // Different CPU request
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		}
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Resources = differentResources
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeFalse())
	})

	It("should match when no resources are specified and pod has no overrides", func() {
		// Setup: Ensure no resources in package
		newPackage := *package_
		newPackage.Resources = nil
		skyhook.Spec.Packages["test-package"] = newPackage

		// Test: Ensure pod has no resource overrides for any init container
		emptyResources := corev1.ResourceRequirements{}
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Resources = emptyResources
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeTrue())
	})

	It("should not match when no resources are specified but pod has requests", func() {
		// Setup: Ensure no resources in package
		newPackage := *package_
		newPackage.Resources = nil
		skyhook.Spec.Packages["test-package"] = newPackage

		// Test: Add resource requests to all init containers
		requestResources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Resources = requestResources
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeFalse())
	})

	It("should not match when no resources are specified but pod has limits", func() {
		// Setup: Ensure no resources in package
		newPackage := *package_
		newPackage.Resources = nil
		skyhook.Spec.Packages["test-package"] = newPackage

		// Test: Add resource limits to all init containers
		limitResources := corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		}
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Resources = limitResources
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeFalse())
	})

	It("should ignore SKYHOOK_RESOURCE_ID env var", func() {
		newPackage := *package_
		newPackage.Resources = nil
		skyhook.Spec.Packages["test-package"] = newPackage

		// Setup: Add SKYHOOK_RESOURCE_ID env var to all init containers
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Env = append(actualPod.Spec.InitContainers[i].Env, corev1.EnvVar{
				Name:  "SKYHOOK_RESOURCE_ID",
				Value: "SOME_VALUE",
			})
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeTrue())
	})

	It("should not ignore non static env vars", func() {
		newPackage := *package_
		newPackage.Resources = nil
		skyhook.Spec.Packages["test-package"] = newPackage

		// Setup: Add SKYHOOK_RESOURCE_ID env var to all init containers
		for i := range actualPod.Spec.InitContainers {
			actualPod.Spec.InitContainers[i].Env = append(actualPod.Spec.InitContainers[i].Env, corev1.EnvVar{
				Name:  "SOME_ENV_VAR",
				Value: "SOME_VALUE",
			})
		}

		// Set the package in the pod annotations
		err := SetPackages(actualPod, skyhook.Skyhook, newPackage.Image, v1alpha1.StageApply, &newPackage)
		Expect(err).ToNot(HaveOccurred())

		Expect(podMatchesPackage(operator.opts, &newPackage, *actualPod, skyhook, v1alpha1.StageApply)).To(BeFalse())
	})
})

func TestGenerateValidPodNames(t *testing.T) {
	g := NewWithT(t)

	// Test short name
	name := generateSafeName(63, "test", "node1")
	g.Expect(len(name)).To(Equal(19)) // "test-node1-" + 8 char hash
	g.Expect(name).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`))

	// Test very long name
	name = generateSafeName(63, "test-very-long-name-that-should-be-truncated", "node1")
	g.Expect(len(name)).To(Equal(59))
	g.Expect(name).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`))

	// Test consistent hashing
	name1 := generateSafeName(63, "test", "node1")
	name2 := generateSafeName(63, "test", "node1")
	g.Expect(name1).To(Equal(name2))

	// Test dots in name
	name = generateSafeName(63, "test.name", "node.1")
	g.Expect(name).To(MatchRegexp(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`))
	g.Expect(len(name)).To(Equal(25)) // "test-name-node-1-" + 8 char hash
}
