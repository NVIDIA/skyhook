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
	"sort"
	"time"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/wrapper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// jobHandlerFunc maps Job events into the single reconcile queue as "job---<name>"
// requests, mirroring podHandlerFunc's "pod---<name>" routing so package-stage Jobs are
// serialized through the same SkyhookReconciler pass (the pseudo-controller pattern,
// issue #223 option A). Only Jobs this operator owns, carrying the skyhook name label,
// are enqueued; anything else in the namespace (e.g. a CronJob's Jobs) is ignored.
func jobHandlerFunc(_ context.Context, o client.Object) []reconcile.Request {
	job := o.(*batchv1.Job)

	if labels.Set(job.Labels).Has(fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX)) {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{
			Name:      fmt.Sprintf("job---%s", job.Name), // prefix distinguishes Job events from pod/skyhook events
			Namespace: job.Namespace,
		}}}
	}
	return nil
}

// jobMatchesPackage reports whether an existing stage Job still matches what the operator
// would build for this package+stage now. It is the Job analogue of podMatchesPackage, used
// to decide on an AlreadyExists race or a validation sweep whether a Job is stale and must
// be replaced.
//
// podMatchesPackage compares only the package label, the interrupt label (to pick which
// expected executor to build), and per-init-container name/image/env/resources, all of
// which live on parts of the pod the Job builder copies through unchanged (the init-container
// chain and the template labels). The Job-specific differences (exit-0 main container,
// restartPolicy, extra tolerations, podFailurePolicy) are pod-level fields podMatchesPackage
// never reads, so evaluating it on the Job's pod template gives the same answer as on the
// equivalent raw pod. Reuse it rather than duplicate (and risk drifting) the compare.
func jobMatchesPackage(opts SkyhookOperatorOptions, _package *v1alpha1.Package, job batchv1.Job, skyhook *wrapper.Skyhook, stage v1alpha1.Stage) bool {
	templatePod := corev1.Pod{
		ObjectMeta: job.Spec.Template.ObjectMeta,
		Spec:       job.Spec.Template.Spec,
	}
	return podMatchesPackage(opts, _package, templatePod, skyhook, stage)
}

const (
	// annotationStateRecorded marks a finished Job whose completion the operator has
	// already written to node state. Pods now linger past completion, so delete-on-success
	// can no longer be the processed-once marker; this persisted annotation replaces it and
	// makes a duplicate terminal event for a marked Job a no-op.
	annotationStateRecorded = v1alpha1.METADATA_PREFIX + "/state-recorded"
	annotationValueTrue     = "true"

	// annotationLastLogs holds a best-effort tail of a deadline-killed stage's stuck
	// container — or, when the container never started, its waiting reason — so a parked
	// tombstone still names the problem after the kubelet garbage-collects the pod's logs.
	annotationLastLogs = v1alpha1.METADATA_PREFIX + "/last-logs"

	// batchControllerUIDLabel selects a Job's own child pods. Job names are deterministic
	// and reused across reruns, so a same-named prior Job's pod can still be terminating;
	// the controller UID is the only unambiguous parent link (job-name alone is not).
	batchControllerUIDLabel = "batch.kubernetes.io/controller-uid"

	// lastLogsMaxBytes caps the deadline log snapshot. Annotations share a per-object
	// metadata budget, so the tail stays small.
	lastLogsMaxBytes = 16 * 1024

	// failureTargetGrace bounds how long a Job may sit at FailureTarget without going
	// terminal before its stuck stage is treated as erroring evidence — the unreachable-node
	// case, where the Job controller cannot delete the pod to finalize the failure.
	failureTargetGrace = 5 * time.Minute
)

// JobReconcile records the outcome of a package/interrupt stage Job into node state,
// exactly once, and manages the finished Job's failure archive and deadline snapshot.
// It is the Job analogue of PodReconcile and the completion authority for the Jobs
// execution path (the Pod watch keeps reporting only in-flight erroring for Job-owned
// pods). Not wired into Reconcile yet — the swap lands in #303.
func (r *SkyhookReconciler) JobReconcile(ctx context.Context, job *batchv1.Job) (ctrl.Result, error) {
	// A package invalidated mid-flight (spec drift) is torn down, mirroring
	// HandleInvalidPackage for raw pods. Foreground so the deterministic name frees only
	// once the child pods are gone.
	if invalid, err := IsInvalidPackage(job); err != nil {
		return ctrl.Result{}, fmt.Errorf("checking invalid package on job %s: %w", job.Name, err)
	} else if invalid {
		return ctrl.Result{}, r.deleteJobForeground(ctx, job)
	}

	// Already processed: a persisted marker — not pod deletion — is the processed-once
	// guard now that completed pods linger, so a re-served terminal event is harmless.
	if jobProcessed(job) {
		return ctrl.Result{}, nil
	}

	if hasJobCondition(job, batchv1.JobComplete) {
		return r.handleCompleteJob(ctx, job)
	}
	if failed, reason := jobFailure(job); failed {
		return r.handleFailedJob(ctx, job, reason)
	}
	return r.handleActiveJob(ctx, job)
}

// handleCompleteJob records a successful stage completion once, then marks the Job with
// the success TTL. The record is skipped (only marked) when the node is gone or the
// completion is already reflected, so a re-served event after a crash between the two
// writes cannot double-apply a non-idempotent transition.
func (r *SkyhookReconciler) handleCompleteJob(ctx context.Context, job *batchv1.Job) (ctrl.Result, error) {
	pkg, err := GetPackage(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting package from job %s: %w", job.Name, err)
	}
	if pkg == nil {
		// A Job we own with no package annotation cannot be recorded; mark it so it TTLs out
		// instead of re-serving forever. Once #303 wires creation the annotation is always set,
		// so log it — this surfaces a wiring regression rather than silently losing completions.
		log.FromContext(ctx).WithName("job-reconcile").Info("completed job has no package annotation; marking without recording", "job", job.Name)
		return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLSucceeded)
	}

	node, err := r.dal.GetNode(ctx, jobNodeName(job))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting node for job %s: %w", job.Name, err)
	}
	// Node gone → there is no state to record; mark processed rather than error-loop on
	// a node that will never return.
	if node == nil {
		return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLSucceeded)
	}

	record, err := r.shouldRecordCompletion(job, pkg, node)
	if err != nil {
		return ctrl.Result{}, err
	}
	if record {
		if err := r.recordJobCompletion(ctx, job, pkg, node); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLSucceeded)
}

// shouldRecordCompletion is the re-serve guard. It records a completion only when the
// package's node-state entry is still at this Job's stage and not yet complete — a valid
// forward transition. An absent entry (uninstalled or node reset), an already-complete
// entry, or an entry that has advanced past this stage all mean recording would resurrect,
// duplicate, or regress state; the marker alone suffices. Interrupt completions also promote
// packages skipped during interrupt sequencing, so they re-run while any remain skipped.
func (r *SkyhookReconciler) shouldRecordCompletion(job *batchv1.Job, pkg *PackageSkyhook, node *corev1.Node) (bool, error) {
	skyhookNode, err := wrapper.NewSkyhookNodeOnly(node, pkg.Skyhook)
	if err != nil {
		return false, fmt.Errorf("creating node wrapper for job %s: %w", job.Name, err)
	}
	state, err := skyhookNode.State()
	if err != nil {
		return false, fmt.Errorf("reading node state for job %s: %w", job.Name, err)
	}

	if isInterruptJob(job) {
		// ProgressSkipped promotes only StageInterrupt-skipped packages, so re-run the record
		// only while such a package remains — matching what the promotion can actually advance.
		for _, s := range state {
			if s.Stage == v1alpha1.StageInterrupt && s.State == v1alpha1.StateSkipped {
				return true, nil
			}
		}
	}

	status, present := state[pkg.GetUniqueName()]
	return present && status.Stage == pkg.Stage && status.State != v1alpha1.StateComplete, nil
}

// recordJobCompletion writes the StateComplete transition through the existing node-state
// path (HandleCompletePod carries the upgrade/uninstall/interrupt special cases unchanged),
// sourced from the Job rather than a pod so it still works after the child pod is GC'd.
func (r *SkyhookReconciler) recordJobCompletion(ctx context.Context, job *batchv1.Job, pkg *PackageSkyhook, node *corev1.Node) error {
	patch := client.StrategicMergeFrom(node.DeepCopy())

	skyhookNode, err := wrapper.NewSkyhookNodeOnly(node, pkg.Skyhook)
	if err != nil {
		return fmt.Errorf("creating node wrapper for job %s: %w", job.Name, err)
	}

	// HandleCompletePod only compares containerName against InterruptContainerName, so the
	// Job's interrupt label determines it — no child pod needed. For non-interrupt stages the
	// exact name is cosmetic; read it from the succeeded pod when present, else leave it empty.
	containerName := ""
	if isInterruptJob(job) {
		containerName = InterruptContainerName
	} else {
		containerName = r.succeededContainerName(ctx, job)
	}

	updated, err := r.HandleCompletePod(ctx, skyhookNode, pkg, containerName)
	if err != nil {
		return fmt.Errorf("recording completion for job %s: %w", job.Name, err)
	}
	if !updated {
		if err := skyhookNode.Upsert(pkg.PackageRef, pkg.Image, v1alpha1.StateComplete, pkg.Stage, job.Status.Failed, pkg.ContainerSHA); err != nil {
			return fmt.Errorf("upserting complete state for job %s: %w", job.Name, err)
		}
	}

	if skyhookNode.Changed() {
		if err := r.Patch(ctx, node, patch); err != nil {
			return fmt.Errorf("patching node %s for job %s completion: %w", node.Name, job.Name, err)
		}
		r.recorder.Eventf(node, nil, EventTypeNormal, EventsReasonSkyhookStateChange, "JobComplete",
			"Package [%s:%s] state %s on [skyhook:%s]", pkg.Name, pkg.Version, v1alpha1.StateComplete, pkg.Skyhook)
	}
	return nil
}

// handleFailedJob handles a terminal Failed Job. DeadlineExceeded is a genuine failure:
// record erroring, mark with the failure TTL, and leave the Job in place as the park marker
// so the main pass does not recreate the stage until a rerun/reset/config-change/TTL clears
// it. Any other reason is a backstop that unlimited backoff plus the deadline should make
// unreachable — no node-state write (a vanished pod self-heals invisibly, as today), just
// the marker and failure TTL.
func (r *SkyhookReconciler) handleFailedJob(ctx context.Context, job *batchv1.Job, reason string) (ctrl.Result, error) {
	if reason != batchv1.JobReasonDeadlineExceeded {
		return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLFailed)
	}

	pkg, err := GetPackage(job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting package from job %s: %w", job.Name, err)
	}
	if pkg == nil {
		return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLFailed)
	}

	node, err := r.dal.GetNode(ctx, jobNodeName(job))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting node for job %s: %w", job.Name, err)
	}
	if node == nil {
		return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLFailed)
	}

	if err := r.recordJobErroring(ctx, job, pkg, node); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.markJobProcessed(ctx, job, r.opts.JobTTLFailed)
}

// recordJobErroring parks the stage at (stage, erroring). It is guarded like the completion
// path: only the stage the Job actually ran is touched, and never regressed to a later stage
// or resurrected onto a removed package. This is also the state-only write for a stale
// FailureTarget on an unreachable node (no marker/TTL there — those wait for terminal Failed).
func (r *SkyhookReconciler) recordJobErroring(ctx context.Context, job *batchv1.Job, pkg *PackageSkyhook, node *corev1.Node) error {
	patch := client.StrategicMergeFrom(node.DeepCopy())

	skyhookNode, err := wrapper.NewSkyhookNodeOnly(node, pkg.Skyhook)
	if err != nil {
		return fmt.Errorf("creating node wrapper for job %s: %w", job.Name, err)
	}
	state, err := skyhookNode.State()
	if err != nil {
		return fmt.Errorf("reading node state for job %s: %w", job.Name, err)
	}

	status, present := state[pkg.GetUniqueName()]
	if !present || status.Stage != pkg.Stage || status.State == v1alpha1.StateErroring {
		return nil
	}

	if err := skyhookNode.Upsert(pkg.PackageRef, pkg.Image, v1alpha1.StateErroring, pkg.Stage, job.Status.Failed, pkg.ContainerSHA); err != nil {
		return fmt.Errorf("upserting erroring state for job %s: %w", job.Name, err)
	}
	skyhookNode.SetStatus(v1alpha1.StatusErroring)

	if skyhookNode.Changed() {
		if err := r.Patch(ctx, node, patch); err != nil {
			return fmt.Errorf("patching node %s for job %s deadline: %w", node.Name, job.Name, err)
		}
		r.recorder.Eventf(node, nil, EventTypeNormal, EventsReasonSkyhookStateChange, "JobDeadlineExceeded",
			"Package [%s:%s] exceeded its stage deadline on [skyhook:%s]", pkg.Name, pkg.Version, pkg.Skyhook)
	}
	return nil
}

// handleActiveJob runs on a non-terminal Job: it takes the best-effort deadline log snapshot
// when the Job is at FailureTarget (and records erroring if that state has gone stale on an
// unreachable node), then prunes failed attempts to a single archive. Completion itself waits
// for the terminal Complete/Failed condition.
func (r *SkyhookReconciler) handleActiveJob(ctx context.Context, job *batchv1.Job) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("job-reconcile")

	result := ctrl.Result{}
	if hasJobCondition(job, batchv1.JobFailureTarget) {
		if err := r.snapshotFailureLogs(ctx, job); err != nil {
			logger.Error(err, "error snapshotting failure logs", "job", job.Name)
		}
		if failureTargetStale(job) {
			// A real state write. On an unreachable node no further Job event retries it, so
			// let the error escape to the work queue for a backoff retry rather than deferring
			// the erroring evidence to the next informer resync.
			if err := r.recordStaleFailureTarget(ctx, job); err != nil {
				return ctrl.Result{}, fmt.Errorf("recording erroring for stale failure target on job %s: %w", job.Name, err)
			}
		} else {
			// An unreachable node emits no further Job event to drive the stale→erroring
			// transition, so schedule the re-check ourselves rather than wait for a resync.
			result.RequeueAfter = failureTargetGrace
		}
	}

	if err := r.pruneFailedAttempts(ctx, job); err != nil {
		logger.Error(err, "error pruning failed attempts", "job", job.Name)
	}

	return result, nil
}

// recordStaleFailureTarget records erroring (state only) for a Job stuck at FailureTarget on
// an unreachable node, where the Job controller can never delete the pod to reach terminal
// Failed. Terminal handling — marker, TTL, park — still waits for Failed.
func (r *SkyhookReconciler) recordStaleFailureTarget(ctx context.Context, job *batchv1.Job) error {
	pkg, err := GetPackage(job)
	if err != nil || pkg == nil {
		return err
	}
	node, err := r.dal.GetNode(ctx, jobNodeName(job))
	if err != nil || node == nil {
		return err
	}
	return r.recordJobErroring(ctx, job, pkg, node)
}

// snapshotFailureLogs captures the last evidence of a deadline-bound stage before its pod is
// deleted. It is skipped when a failed-attempt archive already exists (that pod carries full
// logs and survives the deadline) or when the snapshot is already taken. A never-started
// container has no logs, so its waiting reason/message is recorded instead. Any error is
// swallowed by the caller — this must never delay the erroring/park path.
func (r *SkyhookReconciler) snapshotFailureLogs(ctx context.Context, job *batchv1.Job) error {
	if _, ok := job.Annotations[annotationLastLogs]; ok {
		return nil
	}

	pods, err := r.childPods(ctx, job)
	if err != nil {
		return err
	}

	var target *corev1.Pod
	for i := range pods {
		// A genuine failed archive already holds full logs — nothing to snapshot.
		if pods[i].Status.Phase == corev1.PodFailed && !hasDisruptionTarget(&pods[i]) {
			return nil
		}
		if target == nil && (pods[i].Status.Phase == corev1.PodRunning || pods[i].Status.Phase == corev1.PodPending) {
			target = &pods[i]
		}
	}
	if target == nil {
		return nil
	}

	container, waitingReason, waitingMessage := stuckInitContainer(target)
	if container == "" {
		return nil
	}

	var snapshot string
	if waitingReason != "" {
		snapshot = fmt.Sprintf("%s: %s: %s", container, waitingReason, waitingMessage)
	} else {
		logs, err := r.dal.GetPodLogTail(ctx, target.Namespace, target.Name, container, lastLogsMaxBytes)
		if err != nil {
			return nil
		}
		snapshot = fmt.Sprintf("%s:\n%s", container, logs)
	}

	if job.Annotations == nil {
		job.Annotations = map[string]string{}
	}
	job.Annotations[annotationLastLogs] = snapshot
	if err := r.Update(ctx, job); err != nil {
		return fmt.Errorf("annotating job %s with failure logs: %w", job.Name, err)
	}
	return nil
}

// pruneFailedAttempts keeps two archives — the first genuine failure (most likely the root
// cause, before cascading errors obscure it) and the most recent — and deletes the genuine
// failures in between. Only genuinely-Failed pods count: a disruption casualty (carrying
// DisruptionTarget) has no failure verdict and must neither be kept nor deleted, so it must
// not shadow a real one. Normal deletion only: the Job-tracking finalizer guarantees a failure
// is counted and policy-classified before its pod is removed.
func (r *SkyhookReconciler) pruneFailedAttempts(ctx context.Context, job *batchv1.Job) error {
	pods, err := r.childPods(ctx, job)
	if err != nil {
		return err
	}

	archives := make([]corev1.Pod, 0, len(pods))
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodFailed && !hasDisruptionTarget(&pods[i]) && pods[i].DeletionTimestamp == nil {
			archives = append(archives, pods[i])
		}
	}
	if len(archives) <= 2 {
		return nil
	}

	// Name breaks CreationTimestamp ties (second granularity) so the kept pair is deterministic.
	sort.Slice(archives, func(i, j int) bool {
		if archives[i].CreationTimestamp.Equal(&archives[j].CreationTimestamp) {
			return archives[i].Name < archives[j].Name
		}
		return archives[i].CreationTimestamp.Before(&archives[j].CreationTimestamp)
	})

	// Keep the first and the last; delete the middle.
	for i := 1; i < len(archives)-1; i++ {
		if err := r.Delete(ctx, &archives[i]); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("pruning failed attempt %s for job %s: %w", archives[i].Name, job.Name, err)
		}
	}
	return nil
}

// deleteJobForeground deletes a Job with foreground propagation so its deterministic name is
// freed only after the child pods are gone.
func (r *SkyhookReconciler) deleteJobForeground(ctx context.Context, job *batchv1.Job) error {
	if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationForeground)); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("deleting job %s: %w", job.Name, err)
	}
	return nil
}

// markJobProcessed sets the state-recorded marker and the outcome TTL in one Update, run after
// the node-state write so a crash can never mark a Job whose state is still in_progress. TTL is
// unset at creation, so the TTL controller cannot race this.
func (r *SkyhookReconciler) markJobProcessed(ctx context.Context, job *batchv1.Job, ttl time.Duration) error {
	if job.Annotations == nil {
		job.Annotations = map[string]string{}
	}
	job.Annotations[annotationStateRecorded] = annotationValueTrue
	seconds := int32(ttl.Seconds())
	job.Spec.TTLSecondsAfterFinished = &seconds

	if err := r.Update(ctx, job); err != nil {
		return fmt.Errorf("marking job %s processed: %w", job.Name, err)
	}
	return nil
}

// succeededContainerName returns the last init-container name of the Job's succeeded child pod
// (selected by controller UID, never job-name), or "" if the pod was already GC'd.
func (r *SkyhookReconciler) succeededContainerName(ctx context.Context, job *batchv1.Job) string {
	pods, err := r.childPods(ctx, job)
	if err != nil {
		return ""
	}
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodSucceeded {
			statuses := pods[i].Status.InitContainerStatuses
			if len(statuses) > 0 {
				return statuses[len(statuses)-1].Name
			}
		}
	}
	return ""
}

// childPods lists a Job's own pods by controller UID (job-name is ambiguous across reruns of a
// deterministic name).
func (r *SkyhookReconciler) childPods(ctx context.Context, job *batchv1.Job) ([]corev1.Pod, error) {
	list, err := r.dal.GetPods(ctx, client.InNamespace(job.Namespace), client.MatchingLabels{batchControllerUIDLabel: string(job.UID)})
	if err != nil {
		return nil, fmt.Errorf("listing child pods for job %s: %w", job.Name, err)
	}
	if list == nil {
		return nil, nil
	}
	return list.Items, nil
}

// jobProcessed reports whether the Job's completion has already been recorded.
func jobProcessed(job *batchv1.Job) bool {
	return job.Annotations[annotationStateRecorded] == annotationValueTrue
}

// jobNodeName is the authoritative full node name the Job's pod is pinned to; the node label
// may be a hash for >63-char node names, so the pod template's nodeName is the source of truth.
func jobNodeName(job *batchv1.Job) string {
	return job.Spec.Template.Spec.NodeName
}

// isInterruptJob reports whether the Job runs an interrupt stage (carries the interrupt label).
func isInterruptJob(job *batchv1.Job) bool {
	return job.Labels[fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX)] == interruptLabelValue
}

// hasJobCondition reports whether the Job has the given condition set True.
func hasJobCondition(job *batchv1.Job, condition batchv1.JobConditionType) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == condition && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// jobFailure reports whether the Job is terminally Failed and, if so, its reason.
func jobFailure(job *batchv1.Job) (bool, string) {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true, c.Reason
		}
	}
	return false, ""
}

// failureTargetStale reports whether the Job has sat at FailureTarget past the grace window
// without going terminal — the unreachable-node case.
func failureTargetStale(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailureTarget && c.Status == corev1.ConditionTrue {
			return time.Since(c.LastTransitionTime.Time) > failureTargetGrace
		}
	}
	return false
}

// hasDisruptionTarget reports whether the pod carries the DisruptionTarget condition — a pod
// lost to eviction/preemption/PodGC rather than a genuine step failure.
func hasDisruptionTarget(pod *corev1.Pod) bool {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.DisruptionTarget && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// stuckInitContainer returns the first init container that has not exited 0 — the one that hung
// or crashed under the deadline. When that container never started (unpullable image, missing
// configmap) its waiting reason and message are returned instead, since there are no logs.
func stuckInitContainer(pod *corev1.Pod) (name, waitingReason, waitingMessage string) {
	for _, s := range pod.Status.InitContainerStatuses {
		if s.State.Terminated != nil && s.State.Terminated.ExitCode == 0 {
			continue
		}
		if s.State.Waiting != nil {
			return s.Name, s.State.Waiting.Reason, s.State.Waiting.Message
		}
		return s.Name, "", ""
	}
	return "", "", ""
}
