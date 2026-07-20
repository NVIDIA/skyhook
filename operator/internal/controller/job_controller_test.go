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
	"context"
	"fmt"
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("JobReconcile", func() {
	const (
		skyhookName = "gpu-init"
		nodeName    = "worker-7"
		namespace   = "skyhook"
		image       = "ghcr.io/nvidia/skyhook-packages/tuning:1.0.0"
	)

	var pkgRef = v1alpha1.PackageRef{Name: "tuning", Version: "1.0.0"}

	validOpts := func() SkyhookOperatorOptions {
		return SkyhookOperatorOptions{
			Namespace:            namespace,
			CopyDirRoot:          "/var/lib/skyhook",
			AgentLogRoot:         "/var/log/skyhook",
			RuntimeRequiredTaint: "skyhook.nvidia.com=runtime-required:NoSchedule",
			AgentImage:           "ghcr.io/nvidia/skyhook/agent:1.2.3",
			PauseImage:           "registry.k8s.io/pause:3.10",
			MaxInterval:          10 * time.Minute,
			JobTTLSucceeded:      time.Hour,
			JobTTLFailed:         24 * time.Hour,
			JobStageTimeout:      time.Hour,
		}
	}

	// newReconciler builds an isolated reconciler over a fake client seeded with objects,
	// avoiding the background manager. The fake clientset serves the deadline log snapshot.
	newReconciler := func(objects ...client.Object) *SkyhookReconciler {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
		r, err := NewSkyhookReconciler(scheme, c, k8sfake.NewClientset(), events.NewFakeRecorder(50), validOpts())
		Expect(err).ToNot(HaveOccurred())
		return r
	}

	// nodeWithState returns a Node carrying node state for one package at (stage, state).
	nodeWithState := func(state v1alpha1.State, stage v1alpha1.Stage) *corev1.Node {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
		sn, err := wrapper.NewSkyhookNodeOnly(node, skyhookName)
		Expect(err).ToNot(HaveOccurred())
		Expect(sn.Upsert(pkgRef, image, state, stage, 0, "")).To(Succeed())
		return node
	}

	// packageJob returns a package/interrupt Job pinned to the node, carrying the package
	// annotation and the conditions supplied.
	packageJob := func(stage v1alpha1.Stage, interrupt bool, conditions ...batchv1.JobCondition) *batchv1.Job {
		lbls := map[string]string{fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): skyhookName}
		if interrupt {
			lbls[fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX)] = interruptLabelValue
		}
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "tuning-1-0-0-" + string(stage), Namespace: namespace, UID: "job-uid-1", Labels: lbls},
			Spec:       batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeName: nodeName}}},
			Status:     batchv1.JobStatus{Conditions: conditions},
		}
		Expect(SetPackages(job, &v1alpha1.NodeWright{ObjectMeta: metav1.ObjectMeta{Name: skyhookName}}, image, stage,
			&v1alpha1.Package{PackageRef: pkgRef, Image: image})).To(Succeed())
		return job
	}

	trueCondition := func(t batchv1.JobConditionType, reason string) batchv1.JobCondition {
		return batchv1.JobCondition{Type: t, Status: corev1.ConditionTrue, Reason: reason}
	}

	getNodeState := func(r *SkyhookReconciler) v1alpha1.NodeState {
		var node corev1.Node
		Expect(r.Get(ctx, types.NamespacedName{Name: nodeName}, &node)).To(Succeed())
		sn, err := wrapper.NewSkyhookNodeOnly(&node, skyhookName)
		Expect(err).ToNot(HaveOccurred())
		state, err := sn.State()
		Expect(err).ToNot(HaveOccurred())
		return state
	}

	getJob := func(r *SkyhookReconciler, name string) *batchv1.Job {
		var job batchv1.Job
		Expect(r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &job)).To(Succeed())
		return &job
	}

	// failedChildPod returns a Failed child pod of the Job, aged ageAgo, optionally a disruption
	// casualty (carrying DisruptionTarget).
	failedChildPod := func(job *batchv1.Job, name string, ageAgo time.Duration, disrupted bool) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: namespace,
				Labels:            map[string]string{batchControllerUIDLabel: string(job.UID)},
				CreationTimestamp: metav1.NewTime(time.Now().Add(-ageAgo)),
			},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		}
		if disrupted {
			pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.DisruptionTarget, Status: corev1.ConditionTrue}}
		}
		return pod
	}

	exists := func(r *SkyhookReconciler, name string) bool {
		err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &corev1.Pod{})
		if apierrors.IsNotFound(err) {
			return false
		}
		Expect(err).ToNot(HaveOccurred())
		return true
	}

	It("records a completed apply once and marks the Job with the success TTL", func() {
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageApply)
		job := packageJob(v1alpha1.StageApply, false, trueCondition(batchv1.JobComplete, ""))
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		status, ok := getNodeState(r)[pkgRef.GetUniqueName()]
		Expect(ok).To(BeTrue())
		Expect(status.State).To(Equal(v1alpha1.StateComplete))
		Expect(status.Stage).To(Equal(v1alpha1.StageApply))

		marked := getJob(r, job.Name)
		Expect(marked.Annotations).To(HaveKeyWithValue(annotationStateRecorded, annotationValueTrue))
		Expect(marked.Spec.TTLSecondsAfterFinished).ToNot(BeNil())
		Expect(*marked.Spec.TTLSecondsAfterFinished).To(BeEquivalentTo(int32(time.Hour.Seconds())))
	})

	It("is a no-op for an already-recorded Job (duplicate event)", func() {
		node := nodeWithState(v1alpha1.StateComplete, v1alpha1.StageConfig)
		job := packageJob(v1alpha1.StageConfig, false, trueCondition(batchv1.JobComplete, ""))
		job.Annotations[annotationStateRecorded] = annotationValueTrue
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())
		// unchanged
		Expect(getNodeState(r)[pkgRef.GetUniqueName()].Stage).To(Equal(v1alpha1.StageConfig))
	})

	It("does not regress a stage when a completion is re-served after the node advanced", func() {
		// The apply Job re-fires, but node state has already moved on to config.
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageConfig)
		job := packageJob(v1alpha1.StageApply, false, trueCondition(batchv1.JobComplete, ""))
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		// still at config, not regressed back to apply
		Expect(getNodeState(r)[pkgRef.GetUniqueName()].Stage).To(Equal(v1alpha1.StageConfig))
		Expect(getJob(r, job.Name).Annotations).To(HaveKeyWithValue(annotationStateRecorded, annotationValueTrue))
	})

	It("marks a completion but writes no state when the node is gone", func() {
		job := packageJob(v1alpha1.StageApply, false, trueCondition(batchv1.JobComplete, ""))
		r := newReconciler(job) // no node seeded

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())
		Expect(getJob(r, job.Name).Annotations).To(HaveKeyWithValue(annotationStateRecorded, annotationValueTrue))
	})

	It("parks a DeadlineExceeded Job as erroring with the failure TTL, leaving it in place", func() {
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageConfig)
		job := packageJob(v1alpha1.StageConfig, false,
			trueCondition(batchv1.JobFailed, batchv1.JobReasonDeadlineExceeded))
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		Expect(getNodeState(r)[pkgRef.GetUniqueName()].State).To(Equal(v1alpha1.StateErroring))

		parked := getJob(r, job.Name) // still present (park marker), marked with failure TTL
		Expect(parked.Annotations).To(HaveKeyWithValue(annotationStateRecorded, annotationValueTrue))
		Expect(*parked.Spec.TTLSecondsAfterFinished).To(BeEquivalentTo(int32((24 * time.Hour).Seconds())))
	})

	It("writes no state for a Failed Job with a non-deadline reason (backstop)", func() {
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageApply)
		job := packageJob(v1alpha1.StageApply, false,
			trueCondition(batchv1.JobFailed, "BackoffLimitExceeded"))
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		// unchanged (still in_progress), matching today's silent disruption behavior
		Expect(getNodeState(r)[pkgRef.GetUniqueName()].State).To(Equal(v1alpha1.StateInProgress))
		Expect(getJob(r, job.Name).Annotations).To(HaveKeyWithValue(annotationStateRecorded, annotationValueTrue))
	})

	It("deletes a Job whose package was invalidated", func() {
		job := packageJob(v1alpha1.StageApply, false, trueCondition(batchv1.JobComplete, ""))
		Expect(InvalidatePackage(job)).To(Succeed())
		r := newReconciler(job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		var got batchv1.Job
		err = r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: job.Name}, &got)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("snapshots the stuck container's logs on FailureTarget", func() {
		job := packageJob(v1alpha1.StageConfig, false, trueCondition(batchv1.JobFailureTarget, ""))
		stuckPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tuning-pod-1", Namespace: namespace,
				Labels: map[string]string{batchControllerUIDLabel: string(job.UID)},
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				InitContainerStatuses: []corev1.ContainerStatus{
					{Name: "init-copy", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
					{Name: "config", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
		}
		r := newReconciler(job, stuckPod)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		snap := getJob(r, job.Name).Annotations[annotationLastLogs]
		Expect(snap).To(ContainSubstring("config"))
		Expect(snap).To(ContainSubstring("fake logs"))
	})

	It("records the waiting reason when the stuck container never started", func() {
		job := packageJob(v1alpha1.StageConfig, false, trueCondition(batchv1.JobFailureTarget, ""))
		stuckPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "tuning-pod-2", Namespace: namespace,
				Labels: map[string]string{batchControllerUIDLabel: string(job.UID)},
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{
					{Name: "config", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: "ImagePullBackOff", Message: "back-off pulling image",
					}}},
				},
			},
		}
		r := newReconciler(job, stuckPod)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		Expect(getJob(r, job.Name).Annotations[annotationLastLogs]).To(ContainSubstring("ImagePullBackOff"))
	})

	It("keeps the first and most-recent genuine failures, pruning those in between", func() {
		job := packageJob(v1alpha1.StageApply, false) // active (no terminal condition)
		first := failedChildPod(job, "attempt-first", 3*time.Hour, false)
		middle := failedChildPod(job, "attempt-middle", 2*time.Hour, false)
		newest := failedChildPod(job, "attempt-newest", time.Hour, false)
		r := newReconciler(job, first, middle, newest)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		Expect(exists(r, "attempt-first")).To(BeTrue())   // root-cause archive
		Expect(exists(r, "attempt-middle")).To(BeFalse()) // pruned
		Expect(exists(r, "attempt-newest")).To(BeTrue())  // most-recent archive
	})

	It("keeps both archives when there are only two genuine failures", func() {
		job := packageJob(v1alpha1.StageApply, false)
		first := failedChildPod(job, "attempt-first", 2*time.Hour, false)
		newest := failedChildPod(job, "attempt-newest", time.Hour, false)
		r := newReconciler(job, first, newest)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		Expect(exists(r, "attempt-first")).To(BeTrue())
		Expect(exists(r, "attempt-newest")).To(BeTrue())
	})

	It("promotes interrupt-skipped packages on interrupt completion", func() {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
		sn, err := wrapper.NewSkyhookNodeOnly(node, skyhookName)
		Expect(err).ToNot(HaveOccurred())
		// the winning package ran the interrupt; a sibling was skipped during merge
		Expect(sn.Upsert(pkgRef, image, v1alpha1.StateInProgress, v1alpha1.StageInterrupt, 0, "")).To(Succeed())
		sibling := v1alpha1.PackageRef{Name: "other", Version: "2.0.0"}
		Expect(sn.Upsert(sibling, image, v1alpha1.StateSkipped, v1alpha1.StageInterrupt, 0, "")).To(Succeed())

		scr := &v1alpha1.NodeWright{
			ObjectMeta: metav1.ObjectMeta{Name: skyhookName},
			Spec: v1alpha1.NodeWrightSpec{Packages: v1alpha1.Packages{
				"tuning": {PackageRef: pkgRef, Image: image},
				"other":  {PackageRef: sibling, Image: image},
			}},
		}
		job := packageJob(v1alpha1.StageInterrupt, true, trueCondition(batchv1.JobComplete, ""))
		r := newReconciler(node, scr, job)

		_, err = r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		state := getNodeState(r)
		Expect(state[pkgRef.GetUniqueName()].State).To(Equal(v1alpha1.StateComplete))
		Expect(state[sibling.GetUniqueName()].State).To(Equal(v1alpha1.StateComplete))
	})

	It("records erroring (state only, no marker) for a stale FailureTarget on an unreachable node", func() {
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageConfig)
		job := packageJob(v1alpha1.StageConfig, false)
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)), // past the grace window
		}}
		r := newReconciler(node, job)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		Expect(getNodeState(r)[pkgRef.GetUniqueName()].State).To(Equal(v1alpha1.StateErroring))
		// state-only: no marker or TTL until the Job goes terminal
		Expect(getJob(r, job.Name).Annotations).ToNot(HaveKey(annotationStateRecorded))
	})

	It("requeues a fresh (not-yet-stale) FailureTarget so the stale check re-fires", func() {
		job := packageJob(v1alpha1.StageConfig, false)
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now()),
		}}
		r := newReconciler(job)

		res, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(failureTargetGrace))
	})

	It("does not count or delete disruption casualties when pruning", func() {
		job := packageJob(v1alpha1.StageApply, false)
		disruption := failedChildPod(job, "attempt-disrupted", 4*time.Hour, true)
		first := failedChildPod(job, "attempt-first", 3*time.Hour, false)
		middle := failedChildPod(job, "attempt-middle", 2*time.Hour, false)
		newest := failedChildPod(job, "attempt-newest", time.Hour, false)
		r := newReconciler(job, disruption, first, middle, newest)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())

		// disruption casualty untouched (no failure verdict, not counted); only the middle
		// genuine failure is pruned — first and newest remain.
		Expect(exists(r, "attempt-disrupted")).To(BeTrue())
		Expect(exists(r, "attempt-first")).To(BeTrue())
		Expect(exists(r, "attempt-middle")).To(BeFalse())
		Expect(exists(r, "attempt-newest")).To(BeTrue())
	})

	It("does not snapshot logs when a genuine failed archive already carries them", func() {
		job := packageJob(v1alpha1.StageConfig, false, trueCondition(batchv1.JobFailureTarget, ""))
		archive := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "archive-pod", Namespace: namespace,
				Labels: map[string]string{batchControllerUIDLabel: string(job.UID)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodFailed}, // genuine failure, no DisruptionTarget
		}
		r := newReconciler(job, archive)

		_, err := r.JobReconcile(ctx, job)
		Expect(err).ToNot(HaveOccurred())
		Expect(getJob(r, job.Name).Annotations).ToNot(HaveKey(annotationLastLogs))
	})

	It("returns an error (for a backoff retry) when stale-FailureTarget recording fails", func() {
		node := nodeWithState(v1alpha1.StateInProgress, v1alpha1.StageConfig)
		job := packageJob(v1alpha1.StageConfig, false)
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobFailureTarget, Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		}}

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(batchv1.AddToScheme(scheme)).To(Succeed())
		Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())
		// Fail the node patch so recordStaleFailureTarget errors — on an unreachable node
		// nothing else would retry the erroring write, so JobReconcile must surface the error.
		c := interceptor.NewClient(
			fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, job).Build(),
			interceptor.Funcs{
				Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
					return fmt.Errorf("simulated node patch conflict")
				},
			})
		r, err := NewSkyhookReconciler(scheme, c, k8sfake.NewClientset(), events.NewFakeRecorder(50), validOpts())
		Expect(err).ToNot(HaveOccurred())

		_, err = r.JobReconcile(ctx, job)
		Expect(err).To(HaveOccurred())
	})
})
