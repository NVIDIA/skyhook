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
	"fmt"
	"strings"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Jobs execution swap", func() {
	const (
		skyhookName = "gpu-init"
		nodeName    = "worker-1"
		namespace   = "skyhook"
		image       = "ghcr.io/nvidia/skyhook-packages/tuning:1.0.0"
	)
	pkg := &v1alpha1.Package{PackageRef: v1alpha1.PackageRef{Name: "tuning", Version: "1.0.0"}, Image: image}

	nameLabel := fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX)
	packageLabel := fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)
	nodeLabel := fmt.Sprintf("%s/node", v1alpha1.METADATA_PREFIX)
	stageLabel := fmt.Sprintf("%s/stage", v1alpha1.METADATA_PREFIX)

	validOpts := func() SkyhookOperatorOptions {
		return SkyhookOperatorOptions{
			Namespace:            namespace,
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
		}
	}

	newReconciler := func(objects ...client.Object) (*SkyhookReconciler, client.WithWatch) {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).
			WithIndex(&corev1.Pod{}, fieldSelectorNodeName, func(obj client.Object) []string {
				pod, ok := obj.(*corev1.Pod)
				if !ok {
					return nil
				}
				return []string{pod.Spec.NodeName}
			}).Build()
		r, err := NewSkyhookReconciler(scheme, c, k8sfake.NewClientset(), events.NewFakeRecorder(50), validOpts())
		Expect(err).ToNot(HaveOccurred())
		return r, c
	}
	newPodWatch := func(objects ...client.Object) (*PodReconciler, client.WithWatch) {
		_, c := newReconciler(objects...)
		return NewPodReconciler(c, c, k8sfake.NewClientset(), events.NewFakeRecorder(50)), c
	}

	stageJob := func(stage v1alpha1.Stage, conditions ...batchv1.JobCondition) *batchv1.Job {
		return &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tuning-1-0-0-" + string(stage), Namespace: namespace, UID: "job-uid",
				Labels: map[string]string{
					nameLabel:    skyhookName,
					packageLabel: "tuning-1.0.0",
					nodeLabel:    nodeName,
				},
			},
			Spec:   batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeName: nodeName}}},
			Status: batchv1.JobStatus{Conditions: conditions},
		}
	}

	jobOwnedPod := func(name string, statuses ...corev1.ContainerStatus) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: namespace,
				Labels: map[string]string{
					nameLabel:         skyhookName,
					packageLabel:      "tuning-1.0.0",
					batchJobNameLabel: "tuning-1-0-0-apply",
				},
			},
			Spec:   corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{InitContainerStatuses: statuses},
		}
		// Child pods inherit the package annotation from the Job pod template; the in-flight
		// erroring path reads it off the pod, so the fixture must carry it.
		Expect(SetPackages(pod, &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName}}, image, v1alpha1.StageApply, pkg)).To(Succeed())
		return pod
	}

	It("creates a package Job (not a raw pod) for a fresh stage", func() {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
		scr := &v1alpha1.NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: skyhookName, Namespace: namespace, Generation: 1},
			Spec:       v1alpha1.NodeWrightSpec{Packages: v1alpha1.Packages{"tuning": *pkg}},
		}
		r, c := newReconciler(node, scr)
		skyhookNode, err := wrapper.NewSkyhookNode(node, scr)
		Expect(err).ToNot(HaveOccurred())

		Expect(r.ApplyPackage(ctx, GinkgoLogr, nil, skyhookNode, pkg, false)).To(Succeed())

		var jobs batchv1.JobList
		Expect(c.List(ctx, &jobs, client.InNamespace(namespace))).To(Succeed())
		Expect(jobs.Items).To(HaveLen(1))
		Expect(jobs.Items[0].Labels).To(HaveKeyWithValue(nameLabel, skyhookName))
		Expect(jobs.Items[0].Labels).To(HaveKeyWithValue(stageLabel, string(v1alpha1.StageApply)))

		// no raw package pod is created by the swap
		var pods corev1.PodList
		Expect(c.List(ctx, &pods, client.InNamespace(namespace))).To(Succeed())
		Expect(pods.Items).To(BeEmpty())
	})

	Describe("JobExists", func() {
		It("counts an unfinished Job", func() {
			r, _ := newReconciler(stageJob(v1alpha1.StageApply))
			exists, err := r.JobExists(ctx, nodeName, skyhookName, pkg)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("does not count a finished Job (so the stage can re-run once it is cleaned)", func() {
			r, _ := newReconciler(stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}))
			exists, err := r.JobExists(ctx, nodeName, skyhookName, pkg)
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Describe("pod watch", func() {
		It("does not delete a Job-owned pod on success (JobReconcile owns completion)", func() {
			pod := jobOwnedPod("tuning-pod-ok", corev1.ContainerStatus{
				Name: "apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}},
			})
			r, c := newPodWatch(pod)
			_, err := r.PodReconcile(ctx, pod)
			Expect(err).ToNot(HaveOccurred())
			// still present: completion and cleanup belong to JobReconcile, never this watch
			Expect(c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: pod.Name}, &corev1.Pod{})).To(Succeed())
		})

		It("stays silent for a Job-owned pod killed by a disruption", func() {
			pod := jobOwnedPod("tuning-pod-evicted", corev1.ContainerStatus{
				Name: "apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 137}},
			})
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue}}
			r, c := newPodWatch(pod)
			_, err := r.PodReconcile(ctx, pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: pod.Name}, &corev1.Pod{})).To(Succeed())
		})

		It("records erroring from a Job-owned pod's genuine step failure", func() {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
			sn, err := wrapper.NewSkyhookNodeOnly(node, skyhookName)
			Expect(err).ToNot(HaveOccurred())
			Expect(sn.Upsert(pkg.PackageRef, image, v1alpha1.StateInProgress, v1alpha1.StageApply, 0, "")).To(Succeed())

			pod := jobOwnedPod("tuning-pod-failed", corev1.ContainerStatus{
				Name: "apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
			})
			r, c := newPodWatch(node, pod)
			_, err = r.PodReconcile(ctx, pod)
			Expect(err).ToNot(HaveOccurred())

			// the pod carries the package annotation (from the template), so node state records erroring
			var got corev1.Node
			Expect(c.Get(ctx, types.NamespacedName{Name: nodeName}, &got)).To(Succeed())
			gsn, err := wrapper.NewSkyhookNodeOnly(&got, skyhookName)
			Expect(err).ToNot(HaveOccurred())
			state, err := gsn.State()
			Expect(err).ToNot(HaveOccurred())
			Expect(state[pkg.GetUniqueName()].State).To(Equal(v1alpha1.StateErroring))
		})

		// A failing Job under restartPolicy Never mints a fresh pod per attempt, so every one of
		// these lands on the watch. If the watch could write an entry the reset just cleared, the
		// package would be re-pinned to the cleared stage and the reset could never take effect.
		DescribeTable("never creates or regresses a node-state entry",
			func(seed func(wrapper.SkyhookNodeOnly)) {
				node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
				sn, err := wrapper.NewSkyhookNodeOnly(node, skyhookName)
				Expect(err).ToNot(HaveOccurred())
				seed(sn)

				pod := jobOwnedPod("tuning-pod-failed", corev1.ContainerStatus{
					Name: "apply", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}},
				})
				r, c := newPodWatch(node, pod)
				_, err = r.PodReconcile(ctx, pod)
				Expect(err).ToNot(HaveOccurred())

				var got corev1.Node
				Expect(c.Get(ctx, types.NamespacedName{Name: nodeName}, &got)).To(Succeed())
				gsn, err := wrapper.NewSkyhookNodeOnly(&got, skyhookName)
				Expect(err).ToNot(HaveOccurred())
				after, err := gsn.State()
				Expect(err).ToNot(HaveOccurred())

				before, err := sn.State()
				Expect(err).ToNot(HaveOccurred())
				Expect(after).To(Equal(before))
			},
			Entry("entry cleared by a node reset", func(wrapper.SkyhookNodeOnly) {}),
			Entry("entry already moved past the pod's stage", func(sn wrapper.SkyhookNodeOnly) {
				Expect(sn.Upsert(pkg.PackageRef, image, v1alpha1.StateInProgress, v1alpha1.StageConfig, 0, "")).To(Succeed())
			}),
			Entry("entry already recorded complete for this stage", func(sn wrapper.SkyhookNodeOnly) {
				Expect(sn.Upsert(pkg.PackageRef, image, v1alpha1.StateComplete, v1alpha1.StageApply, 0, "")).To(Succeed())
			}),
		)
	})

	Describe("rerun predicate", func() {
		pkgSky := &PackageSkyhook{PackageRef: pkg.PackageRef, Skyhook: skyhookName, Stage: v1alpha1.StageApply}
		processed := func() *batchv1.Job {
			job := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue})
			job.Annotations = map[string]string{annotationStateRecorded: annotationValueTrue}
			return job
		}
		erroringEntry := v1alpha1.NodeState{pkg.GetUniqueName(): {Name: "tuning", Version: "1.0.0", Stage: v1alpha1.StageApply, State: v1alpha1.StateErroring}}
		completeEntry := v1alpha1.NodeState{pkg.GetUniqueName(): {Name: "tuning", Version: "1.0.0", Stage: v1alpha1.StageApply, State: v1alpha1.StateComplete}}

		It("deletes a processed finished Job once its stage is no longer recorded done", func() {
			r, _ := newReconciler()
			Expect(r.shouldDeleteFinishedJob(processed(), pkgSky, v1alpha1.NodeState{})).To(BeTrue())
		})
		It("keeps a processed finished Job while its stage is recorded complete", func() {
			r, _ := newReconciler()
			Expect(r.shouldDeleteFinishedJob(processed(), pkgSky, completeEntry)).To(BeFalse())
		})
		It("never deletes an unprocessed Complete Job (JobReconcile owns it)", func() {
			r, _ := newReconciler()
			unprocessed := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue})
			Expect(r.shouldDeleteFinishedJob(unprocessed, pkgSky, v1alpha1.NodeState{})).To(BeFalse())
		})
		It("keeps a parked DeadlineExceeded Job while its entry sits erroring", func() {
			r, _ := newReconciler()
			parked := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: batchv1.JobReasonDeadlineExceeded})
			parked.Annotations = map[string]string{annotationStateRecorded: annotationValueTrue}
			Expect(r.shouldDeleteFinishedJob(parked, pkgSky, erroringEntry)).To(BeFalse())
		})
	})

	Describe("pause suspend cascade", func() {
		// buildSkyhookNodes wraps a NodeWright (with no packages, so any Job is stale) as the
		// SkyhookNodes the suspend/resume helpers take. The nodes matter only for the
		// ValidateRunningPackages ordering test; the plain helpers read only the Skyhook name.
		buildSkyhookNodes := func(nodes ...corev1.Node) SkyhookNodes {
			scr := v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName, Namespace: namespace}}
			state, err := BuildState(
				&v1alpha1.NodeWrightList{Items: []v1alpha1.NodeWright{scr}},
				&corev1.NodeList{Items: nodes},
				&v1alpha1.DeploymentPolicyList{},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.skyhooks).To(HaveLen(1))
			return state.skyhooks[0]
		}

		suspendVal := func(c client.WithWatch, name string) *bool {
			var got batchv1.Job
			Expect(c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &got)).To(Succeed())
			return got.Spec.Suspend
		}

		invalidate := func(job *batchv1.Job) {
			Expect(SetPackages(job, &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName}}, image, v1alpha1.StageApply, pkg)).To(Succeed())
			Expect(InvalidatePackage(job)).To(Succeed())
		}

		It("suspends an unfinished Job when the Skyhook is paused", func() {
			job := stageJob(v1alpha1.StageApply)
			r, c := newReconciler(job)
			Expect(r.suspendUnfinishedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			Expect(suspendVal(c, job.Name)).To(HaveValue(BeTrue()))
		})

		It("does not suspend a finished Job", func() {
			job := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue})
			r, c := newReconciler(job)
			Expect(r.suspendUnfinishedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			Expect(suspendVal(c, job.Name)).To(BeNil())
		})

		It("does not suspend an invalid Job (already being reaped)", func() {
			job := stageJob(v1alpha1.StageApply)
			invalidate(job)
			r, c := newReconciler(job)
			Expect(r.suspendUnfinishedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			Expect(suspendVal(c, job.Name)).To(BeNil())
		})

		It("is idempotent — leaves an already-suspended Job unchanged", func() {
			job := stageJob(v1alpha1.StageApply)
			job.Spec.Suspend = ptr(true)
			r, c := newReconciler(job)
			Expect(r.suspendUnfinishedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			Expect(suspendVal(c, job.Name)).To(HaveValue(BeTrue()))
		})

		It("resumes a suspended Job when the Skyhook is no longer paused", func() {
			job := stageJob(v1alpha1.StageApply)
			job.Spec.Suspend = ptr(true)
			r, c := newReconciler(job)
			Expect(r.resumeSuspendedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			Expect(suspendVal(c, job.Name)).To(HaveValue(BeFalse()))
		})

		It("leaves an invalid suspended Job suspended (validation reaps it before resume clears it)", func() {
			job := stageJob(v1alpha1.StageApply)
			job.Spec.Suspend = ptr(true)
			invalidate(job)
			r, c := newReconciler(job)
			Expect(r.resumeSuspendedJobs(ctx, buildSkyhookNodes())).To(Succeed())
			// still suspended — resume must not un-suspend a stale-spec Job before it is reaped
			Expect(suspendVal(c, job.Name)).To(HaveValue(BeTrue()))
		})

		It("is a no-op with only a legacy pod present (legacy pods cannot suspend)", func() {
			legacy := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "legacy-tuning", Namespace: namespace,
					Labels: map[string]string{nameLabel: skyhookName, packageLabel: "tuning-1.0.0"},
				},
				Spec: corev1.PodSpec{NodeName: nodeName},
			}
			r, _ := newReconciler(legacy)
			Expect(r.suspendUnfinishedJobs(ctx, buildSkyhookNodes())).To(Succeed())
		})

		It("invalidates a stale suspended Job without clearing suspend (resume ordering guard)", func() {
			// A suspended Job whose package is no longer in spec (edited while paused). Validation must
			// mark it invalid and return update=true — which makes the reconcile loop early-return
			// BEFORE resumeSuspendedJobs runs — and must not clear suspend, so no stale-spec attempt
			// launches before JobReconcile reaps it.
			job := stageJob(v1alpha1.StageApply)
			job.Spec.Suspend = ptr(true)
			Expect(SetPackages(job, &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName}}, image, v1alpha1.StageApply, pkg)).To(Succeed())

			node := corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
			r, c := newReconciler(job)
			// empty Packages spec => the Job matches nothing => stale
			update, err := r.ValidateRunningPackages(ctx, buildSkyhookNodes(node))
			Expect(err).ToNot(HaveOccurred())
			Expect(update).To(BeTrue())

			var got batchv1.Job
			Expect(c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: job.Name}, &got)).To(Succeed())
			invalid, err := IsInvalidPackage(&got)
			Expect(err).ToNot(HaveOccurred())
			Expect(invalid).To(BeTrue())                     // validation marked it invalid...
			Expect(got.Spec.Suspend).To(HaveValue(BeTrue())) // ...and left suspend untouched
		})
	})

	Describe("executor metric classification", func() {
		It("maps a Job to its executor_state", func() {
			running := stageJob(v1alpha1.StageApply)
			Expect(executorState(running)).To(Equal(executorStateRunning))

			suspended := stageJob(v1alpha1.StageApply)
			suspended.Spec.Suspend = ptr(true)
			Expect(executorState(suspended)).To(Equal(executorStateSuspended))

			parked := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: batchv1.JobReasonDeadlineExceeded})
			Expect(executorState(parked)).To(Equal(executorStateParked))

			// finished-but-not-parked Jobs are not active executors
			complete := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobComplete, Status: corev1.ConditionTrue})
			Expect(executorState(complete)).To(BeEmpty())
			failed := stageJob(v1alpha1.StageApply, batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"})
			Expect(executorState(failed)).To(BeEmpty())
		})
	})

	Describe("executor metric counting", func() {
		fooPkg := func(version string) *v1alpha1.Package {
			return &v1alpha1.Package{PackageRef: v1alpha1.PackageRef{Name: "foo", Version: version}, Image: "ghcr.io/nvidia/skyhook-packages/foo"}
		}
		// executorJob builds a Job carrying foo/<version>'s package annotation + name label.
		executorJob := func(version string, mutate func(*batchv1.Job)) batchv1.Job {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo-" + strings.ReplaceAll(version, ".", "-") + "-apply", Namespace: namespace,
					Labels: map[string]string{nameLabel: skyhookName},
				},
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeName: nodeName}}},
			}
			Expect(SetPackages(job, &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName}}, image, v1alpha1.StageApply, fooPkg(version))).To(Succeed())
			if mutate != nil {
				mutate(job)
			}
			return *job
		}

		It("tallies executors per package version (keyed off the Job's annotation), skipping finished Jobs", func() {
			suspend := func(j *batchv1.Job) { j.Spec.Suspend = ptr(true) }
			complete := func(j *batchv1.Job) {
				j.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
			}

			// Two versions coexist during an in-place upgrade — they must count under distinct
			// versions, and a Complete Job is not an active executor.
			counts := executorCounts([]batchv1.Job{
				executorJob("1.0.0", nil),
				executorJob("1.0.0", suspend),
				executorJob("2.0.0", nil),
				executorJob("2.0.0", complete),
			})

			Expect(counts["foo"]["1.0.0"][executorStateRunning]).To(Equal(1.0))
			Expect(counts["foo"]["1.0.0"][executorStateSuspended]).To(Equal(1.0))
			Expect(counts["foo"]["2.0.0"][executorStateRunning]).To(Equal(1.0))
			Expect(counts["foo"]["2.0.0"]).To(HaveLen(1)) // the Complete Job contributed nothing
		})
	})
})
