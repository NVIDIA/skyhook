/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package controller

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/NVIDIA/skyhook/api/v1alpha1"
	"github.com/NVIDIA/skyhook/internal/dal"
	"github.com/NVIDIA/skyhook/internal/version"
	"github.com/NVIDIA/skyhook/internal/wrapper"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/pkg/util/taints"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	EventsReasonSkyhookApply       = "Apply"
	EventsReasonSkyhookInterrupt   = "Interrupt"
	EventsReasonSkyhookDrain       = "Drain"
	EventsReasonSkyhookStateChange = "State"
	EventsReasonNodeReboot         = "Reboot"
	EventTypeNormal                = "Normal"
	// EventTypeWarning = "Warning"
	TaintUnschedulable        = corev1.TaintNodeUnschedulable
	SkyhookTaintUnschedulable = "skyhook.nvidia.com/unschedulable"
	InterruptContainerName    = "interrupt"

	SkyhookFinalizer = "skyhook.nvidia.com/skyhook"
)

type SkyhookOperatorOptions struct {
	Namespace            string        `env:"NAMESPACE, default=skyhook"`
	MaxInterval          time.Duration `env:"DEFAULT_INTERVAL, default=10m"`
	ImagePullSecret      string        `env:"IMAGE_PULL_SECRET, default=node-init-secret"` //TODO: should this be defaulted?
	CopyDirRoot          string        `env:"COPY_DIR_ROOT, default=/tmp"`
	ReapplyOnReboot      bool          `env:"REAPPLY_ON_REBOOT, default=false"`
	RuntimeRequiredTaint string        `env:"RUNTIME_REQUIRED_TAINT, default=skyhook.nvidia.com=runtime-required:NoSchedule"`
	PauseImage           string        `env:"PAUSE_IMAGE, default=registry.k8s.io/pause:3.10"`
	AgentImage           string        `env:"AGENT_IMAGE, default=nvcr.io/nvidian/swgpu-baseos/skyhook-agent:latest"` // TODO: this needs to be updated with a working default
}

func (o *SkyhookOperatorOptions) Validate() error {

	messages := make([]string, 0)
	if o.Namespace == "" {
		messages = append(messages, "namespace must be set")
	}
	if o.CopyDirRoot == "" {
		messages = append(messages, "copy dir root must be set")
	}
	if o.RuntimeRequiredTaint == "" {
		messages = append(messages, "runtime required taint must be set")
	}
	if o.MaxInterval < time.Minute {
		messages = append(messages, "max interval must be at least 1 minute")
	}

	// CopyDirRoot must start with /
	if !strings.HasPrefix(o.CopyDirRoot, "/") {
		messages = append(messages, "copy dir root must start with /")
	}

	// RuntimeRequiredTaint must be parsable and must not be a deletion
	_, delete, err := taints.ParseTaints([]string{o.RuntimeRequiredTaint})
	if err != nil {
		messages = append(messages, fmt.Sprintf("runtime required taint is invalid: %s", err.Error()))
	}
	if len(delete) > 0 {
		messages = append(messages, "runtime required taint must not be a deletion")
	}

	if o.AgentImage == "" {
		messages = append(messages, "agent image must be set")
	}

	if !strings.Contains(o.AgentImage, ":") {
		messages = append(messages, "agent image must contain a tag")
	}

	if o.PauseImage == "" {
		messages = append(messages, "pause image must be set")
	}

	if !strings.Contains(o.PauseImage, ":") {
		messages = append(messages, "pause image must contain a tag")
	}

	if len(messages) > 0 {
		return errors.New(strings.Join(messages, ", "))
	}

	return nil
}

func (o *SkyhookOperatorOptions) GetRuntimeRequiredTaint() corev1.Taint {
	to_add, _, _ := taints.ParseTaints([]string{o.RuntimeRequiredTaint})
	return to_add[0]
}

func (o *SkyhookOperatorOptions) GetRuntimeRequiredToleration() corev1.Toleration {
	taint := o.GetRuntimeRequiredTaint()
	return corev1.Toleration{
		Key:      taint.Key,
		Operator: corev1.TolerationOpEqual,
		Value:    taint.Value,
		Effect:   taint.Effect,
	}
}

// force type checking against this interface
var _ reconcile.Reconciler = &SkyhookReconciler{}

// NOTE: if get a few more settings, we might want to encapsulate them
func NewSkyhookReconciler(schema *runtime.Scheme, c client.Client, recorder record.EventRecorder, opts SkyhookOperatorOptions) (*SkyhookReconciler, error) {

	err := opts.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid skyhook operator options: %w", err)
	}

	return &SkyhookReconciler{
		Client:   c,
		scheme:   schema,
		recorder: recorder,
		opts:     opts,
		dal:      dal.New(c),
	}, nil
}

// SkyhookReconciler reconciles a Skyhook object
type SkyhookReconciler struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	opts     SkyhookOperatorOptions
	dal      dal.DAL
}

// SetupWithManager sets up the controller with the Manager.
func (r *SkyhookReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// indexes allow for query on fields to use the local cache
	indexer := mgr.GetFieldIndexer()
	err := indexer.
		IndexField(context.TODO(), &corev1.Pod{}, "spec.nodeName", func(o client.Object) []string {
			pod, ok := o.(*corev1.Pod)
			if !ok {
				return nil
			}
			return []string{pod.Spec.NodeName}
		})

	if err != nil {
		return err
	}

	ehandler := &eventHandler{
		logger: mgr.GetLogger(),
		dal:    dal.New(r.Client),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Skyhook{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(podHandlerFunc),
		).
		Watches(
			&corev1.Node{},
			ehandler,
		).
		Complete(r)
}

// CRD Permissions
//+kubebuilder:rbac:groups=skyhook.nvidia.com,resources=skyhooks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=skyhook.nvidia.com,resources=skyhooks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=skyhook.nvidia.com,resources=skyhooks/finalizers,verbs=update

// core permissions
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;update;patch;watch
//+kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=pods/eviction,verbs=create
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *SkyhookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// split off requests for pods
	if strings.HasPrefix(req.Name, "pod---") {
		name := strings.Split(req.Name, "pod---")[1]
		pod, err := r.dal.GetPod(ctx, req.Namespace, name)
		if err == nil && pod != nil { // if pod, then call other wise not a pod
			return r.PodReconcile(ctx, pod)
		}
		return ctrl.Result{}, err
	}

	// get all skyhooks (SCR)
	skyhooks, err := r.dal.GetSkyhooks(ctx)
	if err != nil {
		// error, going to requeue and backoff
		logger.Error(err, "error getting skyhooks")
		return ctrl.Result{}, err
	}

	// if there are no skyhooks, so actually nothing to do, so don't requeue
	if skyhooks == nil || len(skyhooks.Items) == 0 {
		return ctrl.Result{}, nil
	}

	// get all nodes
	nodes, err := r.dal.GetNodes(ctx)
	if err != nil {
		// error, going to requeue and backoff
		logger.Error(err, "error getting nodes")
		return ctrl.Result{}, err
	}

	// if no nodes, well not work to do either
	if nodes == nil || len(nodes.Items) == 0 {
		// no nodes, so nothing to do
		return ctrl.Result{}, nil
	}

	// TODO: this build state could error in a lot of ways, and I think we might want to move towards partial state
	// mean if we cant get on SCR state, great, process that one and error

	// BUILD cluster state from all skyhooks, and all nodes
	// this filters and pairs up nodes to skyhooks, also provides help methods for introspection and mutation
	clusterState, err := BuildState(skyhooks, nodes)
	if err != nil {
		// error, going to requeue and backoff
		logger.Error(err, "error building cluster state")
		return ctrl.Result{}, err
	}

	if yes, result, err := shouldReturn(r.HandleMigrations(ctx, clusterState)); yes {
		return result, err
	}

	if yes, result, err := shouldReturn(r.TrackReboots(ctx, clusterState)); yes {
		return result, err
	}

	// node picker is for selecting nodes to do work, tries maintain a prior of nodes between SCRs
	nodePicker := NewNodePicker(r.opts.GetRuntimeRequiredToleration())

	errs := make([]error, 0)
	// requeueNeeded := false
	var result *ctrl.Result
	for _, skyhook := range clusterState.skyhooks {

		if yes, result, err := shouldReturn(r.HandleFinalizer(ctx, skyhook)); yes {
			return result, err
		}

		if yes, result, err := shouldReturn(r.ValidateRunningPackages(ctx, skyhook)); yes {
			return result, err
		}

		if yes, result, err := shouldReturn(r.ValidateNodeConfigmaps(ctx, skyhook.skyhook.Name, skyhook.nodes)); yes {
			return result, err
		}

		if yes, result, err := shouldReturn(r.UpsertConfigmaps(ctx, skyhook, clusterState)); yes {
			return result, err
		}

		if yes, result, err := shouldReturn(r.ReportState(ctx, clusterState, skyhook)); yes {
			return result, err
		}

		ret, err := r.RunSkyhookPackages(ctx, clusterState, nodePicker, skyhook)
		if err != nil {
			logger.Error(err, "error processing skyhook", "skyhook", skyhook.skyhook.Name)
			errs = append(errs, err)
		}

		// NOT a fan of this logic
		if ret != nil {
			result = PickResults(ret, result)
		}
	}

	err = r.HandleRuntimeRequired(ctx, clusterState)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		err := utilerrors.NewAggregate(errs)
		return ctrl.Result{}, err
	}

	if result != nil {
		return *result, nil
	}

	// default happy retry after max
	return ctrl.Result{RequeueAfter: r.opts.MaxInterval}, nil
}

func shouldReturn(updates bool, err error) (bool, ctrl.Result, error) {
	if err != nil {
		return true, ctrl.Result{}, err
	}
	if updates {
		return true, ctrl.Result{Requeue: true}, nil
	}
	return false, ctrl.Result{}, nil
}

func (r *SkyhookReconciler) HandleMigrations(ctx context.Context, clusterState *clusterState) (bool, error) {

	updates := false

	if version.VERSION == "" {
		// this means the binary was complied without version information
		return false, nil
	}

	logger := log.FromContext(ctx)
	errors := make([]error, 0)
	for _, skyhook := range clusterState.skyhooks {

		err := skyhook.Migrate(logger)
		if err != nil {
			return false, fmt.Errorf("error migrating skyhook [%s]: %w", skyhook.skyhook.Name, err)
		}

		if err := skyhook.skyhook.Skyhook.Validate(); err != nil {
			return false, fmt.Errorf("error validating skyhook [%s]: %w", skyhook.skyhook.Name, err)
		}

		for _, node := range skyhook.nodes {
			if node.Changed() {
				err := r.Status().Patch(ctx, node.GetNode(), client.MergeFrom(clusterState.tracker.GetOriginal(node.GetNode())))
				if err != nil {
					errors = append(errors, fmt.Errorf("error patching node [%s]: %w", node.GetNode().Name, err))
				}

				err = r.Patch(ctx, node.GetNode(), client.MergeFrom(clusterState.tracker.GetOriginal(node.GetNode())))
				if err != nil {
					errors = append(errors, fmt.Errorf("error patching node [%s]: %w", node.GetNode().Name, err))
				}
				updates = true
			}
		}

		if skyhook.skyhook.Updated {
			// need to do this because SaveNodesAndSkyhook only saves skyhook status, not the main skyhook object where the annotations are
			// additionally it needs to be an update, a patch nils out the annotations for some reason, which the save function does a patch

			if err = r.Status().Update(ctx, skyhook.skyhook.Skyhook); err != nil {
				return false, fmt.Errorf("error updating during migration skyhook status [%s]: %w", skyhook.skyhook.Name, err)
			}

			// because of conflict issues (409) we need to do things a bit differently here.
			// We might be able to use server side apply in the future, but for now we need to do this
			// https://kubernetes.io/docs/reference/using-api/server-side-apply/
			// https://github.com/kubernetes-sigs/controller-runtime/issues/347

			// work around for now is to grab a new copy of the object, and then patch it

			newskyhook, err := r.dal.GetSkyhook(ctx, skyhook.skyhook.Name)
			if err != nil {
				return false, fmt.Errorf("error getting skyhook to migrate [%s]: %w", skyhook.skyhook.Name, err)
			}
			newPatch := client.MergeFrom(newskyhook.DeepCopy())

			// set version
			wrapper.NewSkyhookWrapper(newskyhook).SetVersion()

			if err = r.Patch(ctx, newskyhook, newPatch); err != nil {
				return false, fmt.Errorf("error updating during migration skyhook [%s]: %w", skyhook.skyhook.Name, err)
			}

			updates = true
		}
	}

	if len(errors) > 0 {
		return false, utilerrors.NewAggregate(errors)
	}

	return updates, nil
}

// ReportState computes and puts important information into the skyhook status so that monitoring tools such as k9s
// can see the information at a glance. For example, the number of completed nodes and the list of packages in the skyhook.
func (r *SkyhookReconciler) ReportState(ctx context.Context, clusterState *clusterState, skyhook *skyhookNodes) (bool, error) {

	// save updated state to skyhook status
	skyhook.ReportState()

	if skyhook.skyhook.Updated {
		_, errs := r.SaveNodesAndSkyhook(ctx, clusterState, skyhook)
		if len(errs) > 0 {
			return false, utilerrors.NewAggregate(errs)
		}
		return true, nil
	}

	return false, nil
}

func (r *SkyhookReconciler) TrackReboots(ctx context.Context, clusterState *clusterState) (bool, error) {

	updates := false
	errs := make([]error, 0)

	for _, skyhook := range clusterState.skyhooks {
		if skyhook.GetSkyhook().Status.NodeBootIds == nil {
			skyhook.GetSkyhook().Status.NodeBootIds = make(map[string]string)
		}

		for _, node := range skyhook.nodes {
			id, ok := skyhook.GetSkyhook().Status.NodeBootIds[node.GetNode().Name]

			if !ok { // new node
				skyhook.GetSkyhook().Status.NodeBootIds[node.GetNode().Name] = node.GetNode().Status.NodeInfo.BootID
				skyhook.skyhook.Updated = true
			}

			if id != "" && id != node.GetNode().Status.NodeInfo.BootID { // node rebooted
				if r.opts.ReapplyOnReboot {
					r.recorder.Eventf(skyhook.GetSkyhook().Skyhook, EventTypeNormal, EventsReasonNodeReboot, "detected reboot, resetting node [%s] to be reapplied", node.GetNode().Name)
					r.recorder.Eventf(node.GetNode(), EventTypeNormal, EventsReasonNodeReboot, "detected reboot, resetting node for [%s] to be reapplied", node.GetSkyhook().Name)
					node.Reset()
				}
				skyhook.GetSkyhook().Status.NodeBootIds[node.GetNode().Name] = node.GetNode().Status.NodeInfo.BootID
				skyhook.skyhook.Updated = true
			}

			if node.Changed() { // update
				updates = true
				err := r.Update(ctx, node.GetNode())
				if err != nil {
					errs = append(errs, fmt.Errorf("error updating node after reboot [%s]: %w", node.GetNode().Name, err))
				}
			}
		}
		if skyhook.skyhook.Updated { // update
			updates = true
			err := r.Status().Update(ctx, skyhook.GetSkyhook().Skyhook)
			if err != nil {
				errs = append(errs, fmt.Errorf("error updating skyhook status after reboot [%s]: %w", skyhook.skyhook.Name, err))
			}
		}
	}

	return updates, utilerrors.NewAggregate(errs)
}

func PickResults(left, right *ctrl.Result) *ctrl.Result {

	// handle nils
	if left == nil && right == nil {
		return nil
	}
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}

	if left.Requeue {
		return left
	}
	if right.Requeue {
		return right
	}
	if left.RequeueAfter > right.RequeueAfter {
		return left
	}

	return right
}

// Runs all skyhook packages then saves and requeues if changes were made
func (r *SkyhookReconciler) RunSkyhookPackages(ctx context.Context, clusterState *clusterState, nodePicker *NodePicker, skyhook *skyhookNodes) (*ctrl.Result, error) {

	logger := log.FromContext(ctx)
	requeue := false

	if skyhook.skyhook.Spec.Pause {
		// continue, paused
		return nil, nil
	}

	toUninstall, err := HandleVersionChange(skyhook)
	if err != nil {
		return nil, fmt.Errorf("error getting packages to uninstall: %w", err)
	}

	changed := IntrospectSkyhook(skyhook)
	if !changed && skyhook.IsComplete() {
		return nil, nil
	}

	selectedNode := nodePicker.SelectNodes(skyhook)
	for _, node := range selectedNode {

		if node.IsComplete() && !node.Changed() {
			continue
		}

		toRun, err := node.RunNext()
		if err != nil {
			return nil, fmt.Errorf("error getting next packages to run: %w", err)
		}

		// prepend the uninstall packages so they are ran first
		toRun = append(toUninstall, toRun...)

		interrupt, pack := fudgeInterruptWithPriority(toRun, skyhook.skyhook.GetConfigUpdates(), skyhook.skyhook.GetConfigInterrupts())

		for _, f := range toRun {

			ok, err := r.ProcessInterrupt(ctx, node, f, interrupt, interrupt != nil && f.Name == pack)
			if err != nil {
				// TODO: error handle
				return nil, fmt.Errorf("error processing if we should interrupt [%s:%s]: %w", f.Name, f.Version, err)
			}
			if !ok {
				requeue = true
				continue
			}

			err = r.ApplyPackage(ctx, logger, clusterState, node, f, interrupt != nil && f.Name == pack)
			if err != nil {
				return nil, fmt.Errorf("error applying package [%s:%s]: %w", f.Name, f.Version, err)
			}

			// process one package at a time
			if skyhook.skyhook.Spec.Serial {
				return &ctrl.Result{Requeue: true}, nil
			}
		}
	}

	saved, errs := r.SaveNodesAndSkyhook(ctx, clusterState, skyhook)
	if len(errs) > 0 {
		return &ctrl.Result{}, utilerrors.NewAggregate(errs)
	}
	if saved {
		requeue = true
	}

	if !skyhook.IsComplete() || requeue {
		return &ctrl.Result{RequeueAfter: time.Second * 2}, nil // not sure this is better then just requeue bool
	}

	return nil, utilerrors.NewAggregate(errs)
}

// SaveNodesAndSkyhook saves nodes and skyhook and will update the events if the skyhook status changes
func (r *SkyhookReconciler) SaveNodesAndSkyhook(ctx context.Context, clusterState *clusterState, skyhook *skyhookNodes) (bool, []error) {
	saved := false
	errs := make([]error, 0)

	for _, node := range skyhook.nodes {
		patch := client.StrategicMergeFrom(clusterState.tracker.GetOriginal(node.GetNode()))
		if node.Changed() {
			err := r.Patch(ctx, node.GetNode(), patch)
			if err != nil {
				errs = append(errs, fmt.Errorf("error patching node [%s]: %w", node.GetNode().Name, err))
			}
			saved = true

			err = r.UpsertNodeLabelsAnnotations(ctx, skyhook.skyhook, node.GetNode())
			if err != nil {
				errs = append(errs, fmt.Errorf("error upserting labels and annotations config map for node [%s]: %w", node.GetNode().Name, err))
			}

			if node.IsComplete() {
				r.recorder.Eventf(node.GetNode(), EventTypeNormal, EventsReasonSkyhookStateChange, "Skyhook [%s] complete.", skyhook.skyhook.Name)

				// since node is complete remove from priority
				if _, ok := skyhook.skyhook.Status.NodePriority[node.GetNode().Name]; ok {
					delete(skyhook.skyhook.Status.NodePriority, node.GetNode().Name)
					skyhook.skyhook.Updated = true
				}
			}
		}

		// updates node's condition
		node.UpdateCondition()
		if node.Changed() {
			// conditions are in status
			err := r.Status().Patch(ctx, node.GetNode(), patch)
			if err != nil {
				errs = append(errs, fmt.Errorf("error patching node status [%s]: %w", node.GetNode().Name, err))
			}
			saved = true
		}

		if node.GetSkyhook() != nil && node.GetSkyhook().Updated {
			skyhook.skyhook.Updated = true
		}
	}

	if skyhook.skyhook.Updated {
		patch := client.MergeFrom(clusterState.tracker.GetOriginal(skyhook.skyhook.Skyhook))
		err := r.Status().Patch(ctx, skyhook.skyhook.Skyhook, patch)
		if err != nil {
			errs = append(errs, err)
		}
		saved = true

		if skyhook.priorStatus != "" && skyhook.priorStatus != skyhook.Status() {
			// we transitioned, fire event
			r.recorder.Eventf(skyhook.skyhook, EventTypeNormal, EventsReasonSkyhookStateChange, "Skyhook transitioned [%s] -> [%s]", skyhook.priorStatus, skyhook.Status())
		}
	}

	if len(errs) > 0 {
		saved = false
	}
	return saved, errs
}

// Updates the state for the node or skyhook if a version is changed on a package
func HandleVersionChange(skyhook *skyhookNodes) ([]*v1alpha1.Package, error) {
	toUninstall := make([]*v1alpha1.Package, 0)

	for _, node := range skyhook.nodes {
		nodeState, err := node.State()
		if err != nil {
			return nil, err
		}

		for _, packageStatus := range nodeState {
			upgrade := false

			_package, exists := skyhook.skyhook.Spec.Packages[packageStatus.Name]
			if exists && _package.Version == packageStatus.Version {
				continue // no uninstall needed for package
			}

			packageStatusRef := v1alpha1.PackageRef{
				Name:    packageStatus.Name,
				Version: packageStatus.Version,
			}

			if !exists && packageStatus.Stage != v1alpha1.StageUninstall {
				// Start uninstall of old package
				err := node.Upsert(packageStatusRef, packageStatus.Image, v1alpha1.StateInProgress, v1alpha1.StageUninstall, 0)
				if err != nil {
					return nil, fmt.Errorf("error updating node status: %w", err)
				}
			} else if exists && _package.Version != packageStatus.Version {
				comparison := version.Compare(_package.Version, packageStatus.Version)
				if comparison == -2 {
					return nil, errors.New("error comparing package versions: invalid version string provided enabling webhooks validates versions before being applied")
				}

				if comparison == 1 {
					_packageStatus, found := node.PackageStatus(_package.GetUniqueName())
					if found && _packageStatus.Stage == v1alpha1.StageUpgrade {
						continue
					}

					// start upgrade of package
					err := node.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateInProgress, v1alpha1.StageUpgrade, 0)
					if err != nil {
						return nil, fmt.Errorf("error updating node status: %w", err)
					}

					upgrade = true
				} else if comparison == -1 && packageStatus.Stage != v1alpha1.StageUninstall {
					// Start uninstall of old package
					err := node.Upsert(packageStatusRef, packageStatus.Image, v1alpha1.StateInProgress, v1alpha1.StageUninstall, 0)
					if err != nil {
						return nil, fmt.Errorf("error updating node status: %w", err)
					}

					// If version changed then update new version to wait
					err = node.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateSkipped, v1alpha1.StageUninstall, 0)
					if err != nil {
						return nil, fmt.Errorf("error updating node status: %w", err)
					}
				}
			}

			// only need to create a feaux package for uninstall since it won't be in the DAG (Upgrade will)
			newPackageStatus, found := node.PackageStatus(packageStatusRef.GetUniqueName())
			if !upgrade && found && newPackageStatus.Stage == v1alpha1.StageUninstall && newPackageStatus.State == v1alpha1.StateInProgress {
				// create fake package with the info we can salvage from the node state
				newPackage := &v1alpha1.Package{
					PackageRef: packageStatusRef,
					Image:      packageStatus.Image,
					Resources: v1alpha1.ResourceRequirements{
						CPURequest:    resource.MustParse("50m"),
						CPULimit:      resource.MustParse("250m"),
						MemoryRequest: resource.MustParse("64Mi"),
						MemoryLimit:   resource.MustParse("256Mi"),
					},
				}

				// Add package to uninstall list if it's not already present
				found := false
				for _, uninstallPackage := range toUninstall {
					if reflect.DeepEqual(uninstallPackage, newPackage) {
						found = true
					}
				}

				if !found {
					toUninstall = append(toUninstall, newPackage)
				}
			}

			// remove all config updates for the package since it's being uninstalled or
			// upgraded. NOTE: The config updates must be removed whenever the version changes
			// or else the package interrupt may be skipped if there is one
			skyhook.skyhook.RemoveConfigUpdates(_package.Name)

			// set the node and skyhook status to in progress
			node.SetStatus(v1alpha1.StatusInProgress)
		}
	}

	return toUninstall, nil
}

// helper for get a point to a ref
func ptr[E any](e E) *E {
	return &e
}

func (r *SkyhookReconciler) UpsertNodeLabelsAnnotations(ctx context.Context, skyhook *wrapper.Skyhook, node *corev1.Node) error {
	// No work to do if there is no labels or annotations for node
	if len(node.Labels) == 0 && len(node.Annotations) == 0 {
		return nil
	}

	annotations, err := json.Marshal(node.Annotations)
	if err != nil {
		return fmt.Errorf("error converting annotations into byte array: %w", err)
	}

	labels, err := json.Marshal(node.Labels)
	if err != nil {
		return fmt.Errorf("error converting labels into byte array: %w", err)
	}

	// node names in different CSPs might include dots which isn't allowed in configmap names
	// so we have to replace all dots with dashes
	newCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhook.Name, node.Name), ".", "-"),
			Namespace: r.opts.Namespace,
			Labels: map[string]string{
				fmt.Sprintf("%s/skyhook-node-meta", v1alpha1.METADATA_PREFIX): skyhook.Name,
			},
			Annotations: map[string]string{
				fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):      skyhook.Name,
				fmt.Sprintf("%s/Node.name", v1alpha1.METADATA_PREFIX): node.Name,
			},
		},
		Data: map[string]string{
			"annotations.json": string(annotations),
			"labels.json":      string(labels),
		},
	}

	if err := ctrl.SetControllerReference(skyhook.Skyhook, newCM, r.scheme); err != nil {
		return fmt.Errorf("error setting ownership: %w", err)
	}

	existingConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Namespace: r.opts.Namespace, Name: strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhook.Name, node.Name), ".", "-")}, existingConfigMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// create
			err := r.Create(ctx, newCM)
			if err != nil {
				return fmt.Errorf("error creating config map [%s]: %w", newCM.Name, err)
			}
		} else {
			return fmt.Errorf("error getting config map: %w", err)
		}
	} else {
		if !reflect.DeepEqual(existingConfigMap.Data, newCM.Data) {
			// update
			err := r.Update(ctx, newCM)
			if err != nil {
				return fmt.Errorf("error updating config map [%s]: %w", newCM.Name, err)
			}
		}
	}

	return nil
}

// HandleConfigUpdates checks whether the configMap on a package was updated and if it was the configmap will
// be updated and the package will be put into config mode if the package is complete or erroring
func (r *SkyhookReconciler) HandleConfigUpdates(ctx context.Context, clusterState *clusterState, skyhook *skyhookNodes, _package v1alpha1.Package, oldConfigMap, newConfigMap *corev1.ConfigMap) (bool, error) {
	completedNodes, nodeCount := 0, len(skyhook.nodes)
	erroringNode := false

	// if configmap changed
	if !reflect.DeepEqual(oldConfigMap.Data, newConfigMap.Data) {
		for _, node := range skyhook.nodes {
			exists, err := r.PodExists(ctx, node.GetNode().Name, skyhook.skyhook.GetName(), &_package)
			if err != nil {
				return false, err
			}

			if !exists && node.IsPackageComplete(_package) {
				completedNodes++
			}

			// if we have an erroring node in the config, interrupt, or post-interrupt mode
			// then we will restart the config changes
			if packageStatus, found := node.PackageStatus(_package.GetUniqueName()); found {
				switch packageStatus.Stage {
				case v1alpha1.StageConfig, v1alpha1.StageInterrupt, v1alpha1.StagePostInterrupt:
					if packageStatus.State == v1alpha1.StateErroring {
						erroringNode = true

						// delete the erroring pod from the node so that it can be recreated
						// with the updated configmap
						pods, err := r.dal.GetPods(ctx,
							client.MatchingFields{
								"spec.nodeName": node.GetNode().Name,
							},
							client.MatchingLabels{
								fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):    skyhook.skyhook.Name,
								fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX): fmt.Sprintf("%s-%s", _package.Name, _package.Version),
							},
						)
						if err != nil {
							return false, err
						}

						if pods != nil {
							for _, pod := range pods.Items {
								err := r.Delete(ctx, &pod)
								if err != nil {
									return false, err
								}
							}
						}
					}
				}
			}
		}

		// if the update is complete or there is an erroring node put the package back into
		// the config mode and update the config map
		if completedNodes == nodeCount || erroringNode {
			// get the keys in the configmap that changed
			newConfigUpdates := make([]string, 0)
			for key, new_val := range newConfigMap.Data {
				if old_val, exists := oldConfigMap.Data[key]; !exists || old_val != new_val {
					newConfigUpdates = append(newConfigUpdates, key)
				}
			}

			// if updates completed then clear out old config updates as they are finished
			if completedNodes == nodeCount {
				skyhook.skyhook.RemoveConfigUpdates(_package.Name)
			}

			// Add the new changed keys to the config updates
			skyhook.skyhook.AddConfigUpdates(_package.Name, newConfigUpdates...)

			for _, node := range skyhook.nodes {
				err := node.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateInProgress, v1alpha1.StageConfig, 0)
				if err != nil {
					return false, fmt.Errorf("error upserting node status [%s]: %w", node.GetNode().Name, err)
				}

				node.SetStatus(v1alpha1.StatusInProgress)
			}

			_, errs := r.SaveNodesAndSkyhook(ctx, clusterState, skyhook)
			if len(errs) > 0 {
				return false, utilerrors.NewAggregate(errs)
			}

			// update config map
			err := r.Update(ctx, newConfigMap)
			if err != nil {
				return false, fmt.Errorf("error updating config map [%s]: %w", newConfigMap.Name, err)
			}

			return true, nil
		}
	}

	return false, nil
}

func (r *SkyhookReconciler) UpsertConfigmaps(ctx context.Context, skyhook *skyhookNodes, clusterState *clusterState) (bool, error) {
	updated := false

	var list corev1.ConfigMapList
	err := r.List(ctx, &list, client.InNamespace(r.opts.Namespace), client.MatchingLabels{fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): skyhook.skyhook.Name})
	if err != nil {
		return false, fmt.Errorf("error listing config maps while upserting: %w", err)
	}

	existingCMs := make(map[string]corev1.ConfigMap)
	for _, cm := range list.Items {
		existingCMs[cm.Name] = cm
	}

	// clean up from an update
	shouldExist := make(map[string]struct{})
	for _, _package := range skyhook.skyhook.Spec.Packages {
		shouldExist[strings.ToLower(fmt.Sprintf("%s-%s-%s", skyhook.skyhook.Name, _package.Name, _package.Version))] = struct{}{}
	}

	for k, v := range existingCMs {
		if _, ok := shouldExist[k]; !ok {
			// delete
			err := r.Delete(ctx, &v)
			if err != nil {
				return false, fmt.Errorf("error deleting existing config map [%s] while upserting: %w", v.Name, err)
			}
		}
	}

	for _, _package := range skyhook.skyhook.Spec.Packages {
		if len(_package.ConfigMap) > 0 {

			newCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.ToLower(fmt.Sprintf("%s-%s-%s", skyhook.skyhook.Name, _package.Name, _package.Version)),
					Namespace: r.opts.Namespace,
					Labels: map[string]string{
						fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): skyhook.skyhook.Name,
					},
					Annotations: map[string]string{
						fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):            skyhook.skyhook.Name,
						fmt.Sprintf("%s/Package.Name", v1alpha1.METADATA_PREFIX):    _package.Name,
						fmt.Sprintf("%s/Package.Version", v1alpha1.METADATA_PREFIX): _package.Version,
					},
				},
				Data: _package.ConfigMap,
			}
			// set owner of CM to the SCR, which will clean up the CM in delete of the SCR
			if err := ctrl.SetControllerReference(skyhook.skyhook.Skyhook, newCM, r.scheme); err != nil {
				return false, fmt.Errorf("error setting ownership of cm: %w", err)
			}

			if existingCM, ok := existingCMs[strings.ToLower(fmt.Sprintf("%s-%s-%s", skyhook.skyhook.Name, _package.Name, _package.Version))]; ok {
				updatedConfigMap, err := r.HandleConfigUpdates(ctx, clusterState, skyhook, _package, &existingCM, newCM)
				if err != nil {
					return false, fmt.Errorf("error updating config map [%s]: %s", newCM.Name, err)
				}
				if updatedConfigMap {
					updated = true
				}
			} else {
				// create
				err := r.Create(ctx, newCM)
				if err != nil {
					return false, fmt.Errorf("error creating config map [%s]: %w", newCM.Name, err)
				}
			}
		}
	}

	return updated, nil
}

func (r *SkyhookReconciler) IsDrained(ctx context.Context, skyhookNode wrapper.SkyhookNode) (bool, error) {

	pods, err := r.dal.GetPods(ctx, client.MatchingFields{
		"spec.nodeName": skyhookNode.GetNode().Name,
	})
	if err != nil {
		return false, err
	}

	if pods == nil || len(pods.Items) == 0 {
		return true, nil
	}

	// checking for any running or pending pods with no toleration to unschedulable
	// if its has an unschedulable toleration we can ignore
	for _, pod := range pods.Items {

		if ShouldEvict(&pod) {
			return false, nil
		}

	}

	return true, nil
}

func ShouldEvict(pod *corev1.Pod) bool {
	switch pod.Status.Phase {
	case corev1.PodRunning, corev1.PodPending:

		for _, taint := range pod.Spec.Tolerations {
			switch taint.Key {
			case "node.kubernetes.io/unschedulable": // ignoring
				return false
			}
		}

		if len(pod.ObjectMeta.OwnerReferences) > 1 {
			for _, owner := range pod.ObjectMeta.OwnerReferences {
				if owner.Kind == "DaemonSet" { // ignoring
					return false
				}
			}
		}

		if pod.GetNamespace() == "kube-system" {
			return false
		}

		return true
	}
	return false
}

// HandleFinalizer returns true only if we container is deleted and we handled it completely, else false
func (r *SkyhookReconciler) HandleFinalizer(ctx context.Context, skyhook *skyhookNodes) (bool, error) {
	if skyhook.skyhook.DeletionTimestamp.IsZero() { // if not deleted, and does not have our finalizer, add it
		if !controllerutil.ContainsFinalizer(skyhook.skyhook.Skyhook, SkyhookFinalizer) {
			controllerutil.AddFinalizer(skyhook.skyhook.Skyhook, SkyhookFinalizer)

			if err := r.Update(ctx, skyhook.skyhook.Skyhook); err != nil {
				return false, fmt.Errorf("error updating skyhook to add finalizer: %w", err)
			}
		}
	} else { // being delete, time to handle our
		if controllerutil.ContainsFinalizer(skyhook.skyhook.Skyhook, SkyhookFinalizer) {

			errs := make([]error, 0)

			for _, node := range skyhook.nodes {
				patch := client.StrategicMergeFrom(node.GetNode().DeepCopy())

				node.Uncordon()
				node.RemoveTaint(SkyhookTaintUnschedulable)

				// if this doesn't change the node then don't patch
				if !node.Changed() {
					continue
				}

				err := r.Patch(ctx, node.GetNode(), patch)
				if err != nil {
					errs = append(errs, fmt.Errorf("error patching node [%s] in finalizer: %w", node.GetNode().Name, err))
				}
			}

			if len(errs) > 0 { // we errored, so we need to return error, otherwise we would release the skyhook when we didnt finish
				return false, utilerrors.NewAggregate(errs)
			}

			controllerutil.RemoveFinalizer(skyhook.skyhook.Skyhook, SkyhookFinalizer)
			if err := r.Update(ctx, skyhook.skyhook.Skyhook); err != nil {
				return false, fmt.Errorf("error updating skyhook removing finalizer: %w", err)
			}
			// should be 1, and now 2. we want to set ObservedGeneration up to not trigger an logic from this update adding the finalizer
			skyhook.skyhook.Status.ObservedGeneration = skyhook.skyhook.Status.ObservedGeneration + 1

			if err := r.Status().Update(ctx, skyhook.skyhook.Skyhook); err != nil {
				return false, fmt.Errorf("error updating skyhook status: %w", err)
			}

			return true, nil
		}
	}
	return false, nil
}

// HasNonInterruptWork returns true if pods are running on the node that are either packages, or matches the SCR selector
func (r *SkyhookReconciler) HasNonInterruptWork(ctx context.Context, skyhookNode wrapper.SkyhookNode) (bool, error) {

	selector, err := metav1.LabelSelectorAsSelector(&skyhookNode.GetSkyhook().Spec.PodNonInteruptLabels)
	if err != nil {
		return false, fmt.Errorf("error creating selector: %w", err)
	}

	if selector.Empty() { // when selector is empty it does not do any selecting, ie will return all pods on node.
		return false, nil
	}

	pods, err := r.dal.GetPods(ctx,
		client.MatchingLabelsSelector{Selector: selector},
		client.MatchingFields{
			"spec.nodeName": skyhookNode.GetNode().Name,
		},
	)
	if err != nil {
		return false, fmt.Errorf("error getting pods: %w", err)
	}

	if pods == nil || len(pods.Items) == 0 {
		return false, nil
	}

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning, corev1.PodPending:
			return true, nil
		}
	}

	return false, nil
}

func (r *SkyhookReconciler) HasRunningPackages(ctx context.Context, skyhookNode wrapper.SkyhookNode) (bool, error) {
	pods, err := r.dal.GetPods(ctx,
		client.HasLabels{fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX)},
		client.MatchingFields{
			"spec.nodeName": skyhookNode.GetNode().Name,
		},
	)
	if err != nil {
		return false, fmt.Errorf("error getting pods: %w", err)
	}

	return pods != nil && len(pods.Items) > 0, nil
}

func (r *SkyhookReconciler) DrainNode(ctx context.Context, skyhookNode wrapper.SkyhookNode) error {

	// cordon node
	skyhookNode.Cordon()

	// adding this taint should help preventing things coming back, including new packages
	// NOTE: might cause issues if we boot off something that really had to be there...
	skyhookNode.Taint(SkyhookTaintUnschedulable)

	pods, err := r.dal.GetPods(ctx, client.MatchingFields{
		"spec.nodeName": skyhookNode.GetNode().Name,
	})
	if err != nil {
		return err
	}

	if pods == nil || len(pods.Items) == 0 {
		return nil
	}

	errs := make([]error, 0)
	for _, pod := range pods.Items {

		if ShouldEvict(&pod) {
			eviction := policyv1.Eviction{}
			err := r.Client.SubResource("eviction").Create(ctx, &pod, &eviction)
			if err != nil {
				errs = append(errs, fmt.Errorf("error evicting pod [%s:%s]: %w", pod.Namespace, pod.Name, err))
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Interrupt should not be called unless safe to do so, IE already cordoned and drained
func (r *SkyhookReconciler) Interrupt(ctx context.Context, skyhookNode wrapper.SkyhookNode, _package *v1alpha1.Package, _interrupt *v1alpha1.Interrupt) error {

	skyhookNode.Taint(SkyhookTaintUnschedulable) // adding this to prevent race conditions between SCR instances

	exists, err := r.PodExists(ctx, skyhookNode.GetNode().Name, skyhookNode.GetSkyhook().Name, _package)
	if err != nil {
		return err
	}
	if exists {
		// nothing to do here, already running
		return nil
	}

	argEncode, err := _interrupt.ToArgs()
	if err != nil {
		return fmt.Errorf("error creating interrupt args: %w", err)
	}

	pod := r.CreateInterruptPodForPackage(_interrupt, argEncode, _package, skyhookNode.GetSkyhook(), skyhookNode.GetNode().Name)

	if err := SetPackages(pod, skyhookNode.GetSkyhook().Skyhook, _package.Image, v1alpha1.StageInterrupt, _package); err != nil {
		return fmt.Errorf("error setting package on interrupt: %w", err)
	}

	if err := ctrl.SetControllerReference(skyhookNode.GetSkyhook().Skyhook, pod, r.scheme); err != nil {
		return fmt.Errorf("error setting ownership: %w", err)
	}

	if err := r.Create(ctx, pod); err != nil {
		return fmt.Errorf("error creating interruption pod: %w", err)
	}

	_ = skyhookNode.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateInProgress, v1alpha1.StageInterrupt, 0)

	r.recorder.Eventf(skyhookNode.GetSkyhook().Skyhook, EventTypeNormal, EventsReasonSkyhookInterrupt,
		"Interrupting node [%s] package [%s:%s] from [skyhook:%s]",
		skyhookNode.GetNode().Name,
		_package.Name,
		_package.Version,
		skyhookNode.GetSkyhook().Name)

	return nil
}

// fudgeInterruptWithPriority takes a list of packages, interrupts, and configUpdates and returns the correct merged interrupt to run to handle all the packages
func fudgeInterruptWithPriority(next []*v1alpha1.Package, configUpdates map[string][]string, interrupts map[string][]*v1alpha1.Interrupt) (*v1alpha1.Interrupt, string) {
	var ret *v1alpha1.Interrupt
	var pack string

	// map interrupt to priority
	// A lower priority value means a higher priority and will be used in favor of anything with a higher value
	var priorities = map[v1alpha1.InterruptType]int{
		v1alpha1.REBOOT:               0,
		v1alpha1.RESTART_ALL_SERVICES: 1,
		v1alpha1.SERVICE:              2,
		v1alpha1.NOOP:                 3,
	}

	for _, _package := range next {

		if len(configUpdates[_package.Name]) == 0 {
			interrupts[_package.Name] = []*v1alpha1.Interrupt{}
			if _package.HasInterrupt() {
				interrupts[_package.Name] = append(interrupts[_package.Name], _package.Interrupt)
			}
		}
	}

	packageNames := make([]string, 0)
	for _, pkg := range next {
		packageNames = append(packageNames, pkg.Name)
	}
	sort.Strings(packageNames)

	for _, _package := range packageNames {
		_interrupts, ok := interrupts[_package]
		if !ok {
			continue
		}

		for _, interrupt := range _interrupts {
			if ret == nil { // prime ret, base case
				ret = interrupt
				pack = _package
			}

			// short circuit, reboot has highest priority
			switch interrupt.Type {
			case v1alpha1.REBOOT:
				return interrupt, _package
			}

			// check if interrupt is higher priority using the priority_order
			// A lower priority value means a higher priority
			if priorities[interrupt.Type] < priorities[ret.Type] {
				ret = interrupt
				pack = _package
			} else if priorities[interrupt.Type] == priorities[ret.Type] {
				mergeInterrupt(ret, interrupt)
			}
		}
	}

	return ret, pack // return merged interrupt and package
}

func mergeInterrupt(left, right *v1alpha1.Interrupt) {

	// make sure both are of type service
	if left.Type != v1alpha1.SERVICE || right.Type != v1alpha1.SERVICE {
		return
	}

	left.Services = merge(left.Services, right.Services)
}

func merge[T cmp.Ordered](left, right []T) []T {
	for _, r := range right {
		if !slices.Contains(left, r) {
			left = append(left, r)
		}
	}
	slices.Sort(left)
	return left
}

// Validate that there are no orphaned or stale config maps for a node
func (r *SkyhookReconciler) ValidateNodeConfigmaps(ctx context.Context, skyhookName string, nodes []wrapper.SkyhookNode) (bool, error) {
	var list corev1.ConfigMapList
	err := r.List(ctx, &list, client.InNamespace(r.opts.Namespace), client.MatchingLabels{fmt.Sprintf("%s/skyhook-node-meta", v1alpha1.METADATA_PREFIX): skyhookName})
	if err != nil {
		return false, fmt.Errorf("error listing config maps: %w", err)
	}

	// No configmaps created by this skyhook, no work needs to be done
	if len(list.Items) == 0 {
		return false, nil
	}

	existingCMs := make(map[string]corev1.ConfigMap)
	for _, cm := range list.Items {
		existingCMs[cm.Name] = cm
	}

	shouldExist := make(map[string]struct{})
	for _, node := range nodes {
		shouldExist[strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhookName, node.GetNode().Name), ".", "-")] = struct{}{}
	}

	update := false
	errs := make([]error, 0)
	for k, v := range existingCMs {
		if _, ok := shouldExist[k]; !ok {
			update = true
			err := r.Delete(ctx, &v)
			if err != nil {
				errs = append(errs, fmt.Errorf("error deleting existing config map [%s]: %w", v.Name, err))
			}
		}
	}

	return update, utilerrors.NewAggregate(errs)
}

// PodExists tests if this package is exists on a node.
func (r *SkyhookReconciler) PodExists(ctx context.Context, nodeName, skyhookName string, _package *v1alpha1.Package) (bool, error) {

	pods, err := r.dal.GetPods(ctx,
		client.MatchingFields{
			"spec.nodeName": nodeName,
		},
		client.MatchingLabels{
			fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):    skyhookName,
			fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX): fmt.Sprintf("%s-%s", _package.Name, _package.Version),
		},
	)
	if err != nil {
		return false, fmt.Errorf("error check from existing pods: %w", err)
	}

	if pods == nil || len(pods.Items) == 0 {
		return false, nil
	}
	return true, nil
}

// CreateInterruptPodForPackage returns the pod spec for an interrupt pod given an package
func (r *SkyhookReconciler) CreateInterruptPodForPackage(_interrupt *v1alpha1.Interrupt, argEncode string, _package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string) *corev1.Pod {
	copyDir := fmt.Sprintf("%s/%s/%s-%s-%s-%d",
		r.opts.CopyDirRoot,
		skyhook.Name,
		_package.Name,
		_package.Version,
		skyhook.UID,
		skyhook.Generation,
	)

	volumes := []corev1.Volume{
		{
			Name: "root-mount",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
		{
			// node names in different CSPs might include dots which isn't allowed in volume names
			// so we have to replace all dots with dashes
			Name: strings.ReplaceAll(fmt.Sprintf("%s-metadata", nodeName), ".", "-"),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhook.Name, nodeName), ".", "-"),
					},
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:             "root-mount",
			MountPath:        "/root",
			MountPropagation: ptr(corev1.MountPropagationHostToContainer),
		},
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: strings.ToLower(fmt.Sprintf("%s-interrupt-%s-", skyhook.Name, _interrupt.Type)), //TODO - RFC 1123 subdomain must consist of lower case alphanumeric
			Namespace:    r.opts.Namespace,
			Labels: map[string]string{
				fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):      skyhook.Name,
				fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX):   fmt.Sprintf("%s-%s", _package.Name, _package.Version),
				fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX): "True",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyOnFailure,
			InitContainers: []corev1.Container{
				{
					Name:  InterruptContainerName,
					Image: getAgentImage(r.opts, _package),
					Args:  []string{"interrupt", "/root", copyDir, argEncode},
					Env: []corev1.EnvVar{
						{
							Name:  "COPY_RESOLV",
							Value: "false",
						},
						{
							Name:  "SKYHOOK_RESOURCE_ID",
							Value: fmt.Sprintf("%s_%s_%s", skyhook.ResourceID(), _package.Name, _package.Version),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "pause",
					Image: r.opts.PauseImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: r.opts.ImagePullSecret,
				},
			},
			HostPID:     true,
			HostNetwork: true,
			// If you change these go change the SelectNode toleration in cluster_state.go
			Tolerations: append([]corev1.Toleration{ // tolerate all cordon
				{
					Key:      TaintUnschedulable,
					Operator: corev1.TolerationOpExists,
				},
				{
					Key:      SkyhookTaintUnschedulable,
					Value:    skyhook.Name,
					Operator: corev1.TolerationOpEqual,
					Effect:   corev1.TaintEffectNoSchedule,
				},
				r.opts.GetRuntimeRequiredToleration(),
			}, skyhook.Spec.AdditionalTolerations...),
			Volumes: volumes,
		},
	}
}

func trunstr(str string, length int) string {
	if len(str) > length {
		return str[:length]
	}
	return str
}

func getAgentImage(opts SkyhookOperatorOptions, _package *v1alpha1.Package) string {
	if _package.AgentImageOverride != "" {
		return _package.AgentImageOverride
	}
	return opts.AgentImage
}

// CreatePodFromPackage creates a pod spec for a skyhook pod for a given package
func (r *SkyhookReconciler) CreatePodFromPackage(_package *v1alpha1.Package, skyhook *wrapper.Skyhook, nodeName string, stage v1alpha1.Stage) *corev1.Pod {
	volumes := []corev1.Volume{
		{
			Name: "root-mount",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
		{
			// node names in different CSPs might include dots which isn't allowed in volume names
			// so we have to replace all dots with dashes
			Name: strings.ReplaceAll(fmt.Sprintf("%s-metadata", nodeName), ".", "-"),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: strings.ReplaceAll(fmt.Sprintf("%s-%s-metadata", skyhook.Name, nodeName), ".", "-"),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:             "root-mount",
			MountPath:        "/root",
			MountPropagation: ptr(corev1.MountPropagationHostToContainer),
		},
		{
			// node names in different CSPs might include dots which isn't allowed in volume mount names
			// so we have to replace all dots with dashes
			Name:      strings.ReplaceAll(fmt.Sprintf("%s-metadata", nodeName), ".", "-"),
			MountPath: "/skyhook-package/node-metadata",
		},
	}

	if len(_package.ConfigMap) > 0 {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      _package.Name,
			MountPath: "/skyhook-package/configmaps",
		})

		volumes = append(volumes, corev1.Volume{
			Name: _package.Name,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: strings.ToLower(fmt.Sprintf("%s-%s-%s", skyhook.Name, _package.Name, _package.Version)),
					},
				},
			},
		})
	}

	copyDir := fmt.Sprintf("%s/%s/%s-%s-%s-%d",
		r.opts.CopyDirRoot,
		skyhook.Name,
		_package.Name,
		_package.Version,
		skyhook.UID,
		skyhook.Generation,
	)
	applyargs := []string{strings.ToLower(string(stage)), "/root", copyDir}
	checkargs := []string{strings.ToLower(string(stage) + "-check"), "/root", copyDir}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: strings.ToLower(fmt.Sprintf("%s-%s-%s-%s-", skyhook.Name, _package.Name, _package.Version, stage)),
			Namespace:    r.opts.Namespace,
			Labels: map[string]string{
				fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX):    skyhook.Name,
				fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX): fmt.Sprintf("%s-%s", _package.Name, _package.Version),
			},
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			RestartPolicy: corev1.RestartPolicyOnFailure,
			InitContainers: []corev1.Container{
				{
					Name:            fmt.Sprintf("%s-init", trunstr(_package.Name, 43)),
					Image:           fmt.Sprintf("%s:%s", _package.Image, _package.Version),
					ImagePullPolicy: "Always",
					Command:         []string{"/bin/sh"},
					Args: []string{
						"-c",
						"mkdir -p /root/${SKYHOOK_DIR} && cp -r /skyhook-package/* /root/${SKYHOOK_DIR}",
					},
					Env: []corev1.EnvVar{
						{
							Name:  "SKYHOOK_DIR",
							Value: copyDir,
						},
					},
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
					Resources: corev1.ResourceRequirements{ // setting this
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
				{
					Name:            fmt.Sprintf("%s-%s", trunstr(_package.Name, 43), stage),
					Image:           getAgentImage(r.opts, _package),
					ImagePullPolicy: "Always",
					Args:            applyargs,
					Env: append(_package.Env, []corev1.EnvVar{
						{
							Name:  "COPY_RESOLV",
							Value: "false",
						},
						{
							Name:  "SKYHOOK_RESOURCE_ID",
							Value: fmt.Sprintf("%s_%s_%s", skyhook.ResourceID(), _package.Name, _package.Version),
						}}...),
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    _package.Resources.CPULimit,
							corev1.ResourceMemory: _package.Resources.MemoryLimit,
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    _package.Resources.CPURequest,
							corev1.ResourceMemory: _package.Resources.MemoryRequest,
						},
					},
				},
				{
					Name:            fmt.Sprintf("%s-%scheck", trunstr(_package.Name, 43), stage),
					Image:           getAgentImage(r.opts, _package),
					ImagePullPolicy: "Always",
					Args:            checkargs,
					Env: append(_package.Env, []corev1.EnvVar{
						{
							Name:  "COPY_RESOLV",
							Value: "false",
						},
						{
							Name:  "SKYHOOK_RESOURCE_ID",
							Value: fmt.Sprintf("%s_%s_%s", skyhook.ResourceID(), _package.Name, _package.Version),
						}}...),
					SecurityContext: &corev1.SecurityContext{
						Privileged: ptr(true),
					},
					VolumeMounts: volumeMounts,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    _package.Resources.CPULimit,
							corev1.ResourceMemory: _package.Resources.MemoryLimit,
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    _package.Resources.CPURequest,
							corev1.ResourceMemory: _package.Resources.MemoryRequest,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "pause",
					Image: r.opts.PauseImage,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("20Mi"),
						},
					},
				},
			},
			ImagePullSecrets: []corev1.LocalObjectReference{
				{
					Name: r.opts.ImagePullSecret,
				},
			},
			Volumes:     volumes,
			HostPID:     true,
			HostNetwork: true,
			// If you change these go change the SelectNode toleration in cluster_state.go
			Tolerations: append([]corev1.Toleration{ // tolerate all cordon
				{
					Key:      TaintUnschedulable,
					Operator: corev1.TolerationOpExists,
				},
				r.opts.GetRuntimeRequiredToleration(),
			}, skyhook.Spec.AdditionalTolerations...),
		},
	}

	return pod
}

// filterEnv removes the environment variables passed into exlude
func FilterEnv(envs []corev1.EnvVar, exclude ...string) []corev1.EnvVar {
	var filteredEnv []corev1.EnvVar

	// build map of exclude strings for faster lookup
	excludeMap := make(map[string]struct{})
	for _, name := range exclude {
		excludeMap[name] = struct{}{}
	}

	// If the environment variable name is in the exclude list, skip it
	// otherwise append it to the final list
	for _, env := range envs {
		if _, found := excludeMap[env.Name]; !found {
			filteredEnv = append(filteredEnv, env)
		}
	}

	return filteredEnv
}

// PodMatchesPackage asserts that a given pod matches the given pod spec
func (r *SkyhookReconciler) PodMatchesPackage(_package *v1alpha1.Package, pod corev1.Pod, skyhook *wrapper.Skyhook, stage v1alpha1.Stage) bool {
	var expectedPod *corev1.Pod

	// need to differentiate whether the pod is for an interrupt or not so we know
	// what to expect and how to compare them
	if pod.Labels[fmt.Sprintf("%s/interrupt", v1alpha1.METADATA_PREFIX)] == "True" {
		expectedPod = r.CreateInterruptPodForPackage(&v1alpha1.Interrupt{}, "", _package, skyhook, "")
	} else {
		expectedPod = r.CreatePodFromPackage(_package, skyhook, "", stage)
	}

	actualPod := pod.DeepCopy()

	// check to see whether the name or the version of the package changed
	packageLabel := fmt.Sprintf("%s/package", v1alpha1.METADATA_PREFIX)
	if actualPod.Labels[packageLabel] != expectedPod.Labels[packageLabel] {
		return false
	}

	// compare initContainers since this is where a lot of the important info lives
	for i := range actualPod.Spec.InitContainers {
		expectedContainer := expectedPod.Spec.InitContainers[i]
		actualContainer := actualPod.Spec.InitContainers[i]

		if expectedContainer.Name != actualContainer.Name {
			return false
		}

		if expectedContainer.Image != actualContainer.Image {
			return false
		}

		// compare the containers env vars except for the ones that are inserted
		// by the operator by default as the SKYHOOK_RESOURCE_ID will change every
		// time the skyhook is updated and would cause every pod to be removed
		excludeEnvs := []string{"SKYHOOK_RESOURCE_ID", "NODEOS_DO_NOT_UPDATE_LABEL", "COPY_RESOLV"}
		expectedFilteredEnv := FilterEnv(expectedContainer.Env, excludeEnvs...)
		actualFilteredEnv := FilterEnv(actualContainer.Env, excludeEnvs...)
		if !reflect.DeepEqual(expectedFilteredEnv, actualFilteredEnv) {
			return false
		}

		// compare resource requests and limits (CPU, memory, etc.)
		if !reflect.DeepEqual(expectedContainer.Resources, actualContainer.Resources) {
			return false
		}
	}

	return true
}

// ValidateRunningPackages deletes pods that don't match the current spec
func (r *SkyhookReconciler) ValidateRunningPackages(ctx context.Context, skyhook *skyhookNodes) (bool, error) {

	pods, err := r.dal.GetPods(ctx,
		client.MatchingLabels{
			fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): skyhook.skyhook.Name,
		},
	)
	if err != nil {
		return false, fmt.Errorf("error getting pods while validating packages: %w", err)
	}
	if pods == nil || len(pods.Items) == 0 {
		return false, nil // nothing running for this skyhook on this node
	}

	errs := make([]error, 0)
	update := false

	for _, pod := range pods.Items {
		found := false

		f, err := GetPackage(&pod)
		if err != nil {
			errs = append(errs, fmt.Errorf("error getting package from pod [%s:%s] while validating packages: %w", pod.Namespace, pod.Name, err))
		}

		for _, v := range skyhook.skyhook.Spec.Packages {
			if r.PodMatchesPackage(&v, pod, skyhook.skyhook, f.Stage) {
				found = true
			}
		}

		if f.Stage == v1alpha1.StageUninstall {
			found = true
		}

		if !found {
			update = true

			err = r.Delete(ctx, &pod)
			if err != nil {
				errs = append(errs, fmt.Errorf("error deleting invalid pod [%s:%s] while validating packages: %w", pod.Namespace, pod.Name, err))
			}
		}
	}

	return update, utilerrors.NewAggregate(errs)
}

// ProcessInterrupt will check and do the interrupt if need, and returns
// false means we are waiting
// true means we are good to proceed
func (r *SkyhookReconciler) ProcessInterrupt(ctx context.Context, skyhookNode wrapper.SkyhookNode, _package *v1alpha1.Package, interrupt *v1alpha1.Interrupt, runInterrupt bool) (bool, error) {

	if !skyhookNode.HasInterrupt(*_package) {
		return true, nil
	}

	// cordon node
	skyhookNode.Cordon()

	hasWork, err := r.HasNonInterruptWork(ctx, skyhookNode)
	if err != nil {
		return false, err
	}

	if hasWork { // has work, so we need to keep waiting to do
		return false, nil
	}

	stage := v1alpha1.StageApply // default starting stage
	nextStage := skyhookNode.NextStage(_package)
	if nextStage != nil {
		stage = *nextStage
	}

	// wait tell this is done if its happening
	status, found := skyhookNode.PackageStatus(_package.GetUniqueName())
	if found && status.State == v1alpha1.StateSkipped {
		return false, nil
	}

	if nextStage != nil && *nextStage == v1alpha1.StageInterrupt && runInterrupt { // time to do the interrupt

		hasWork, err := r.HasNonInterruptWork(ctx, skyhookNode)
		if err != nil {
			return false, err
		}
		if hasWork { // keep waiting...
			return false, nil
		}

		hasPackagesRunning, err := r.HasRunningPackages(ctx, skyhookNode)
		if err != nil {
			return false, err
		}

		if hasPackagesRunning { // keep waiting...
			return false, nil
		}

		drained, err := r.IsDrained(ctx, skyhookNode)
		if err != nil {
			return false, err
		}

		// TODO: NOTE this could be a dead lock if IsDrained always returns true if it happens to not be correct
		if !drained {
			err := r.DrainNode(ctx, skyhookNode)
			if err != nil {
				return false, fmt.Errorf("error draining node [%s]: %w", skyhookNode.GetNode().Name, err)
			}

			r.recorder.Eventf(skyhookNode.GetNode(), EventTypeNormal, EventsReasonSkyhookInterrupt,
				"draining node [%s] package [%s:%s] from [skyhook:%s]",
				skyhookNode.GetNode().Name,
				_package.Name,
				_package.Version,
				skyhookNode.GetSkyhook().Name,
			)
			return false, nil // need to wait for it to drain
		}

		err = r.Interrupt(ctx, skyhookNode, _package, interrupt)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	//skipping
	if nextStage != nil && *nextStage == v1alpha1.StageInterrupt && !runInterrupt {
		err := skyhookNode.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateSkipped, stage, 0)
		if err != nil {
			return false, fmt.Errorf("error upserting to skip interrupt: %w", err)
		}
		return false, nil
	}

	// wait tell this is done if its happening
	if status != nil && status.Stage == v1alpha1.StageInterrupt && status.State != v1alpha1.StateComplete {
		return false, nil
	}

	return true, nil
}

// ApplyPackage starts a pod on node for the package
func (r *SkyhookReconciler) ApplyPackage(ctx context.Context, logger logr.Logger, clusterState *clusterState, skyhookNode wrapper.SkyhookNode, _package *v1alpha1.Package, runInterrupt bool) error {

	if _package == nil {
		return errors.New("can not apply nil package")
	}

	// default starting stage
	stage := v1alpha1.StageApply

	// These modes don't have anything that comes before them so we must specify them as the
	// starting point. The next stage function will return nil until these modes complete.
	// Config is a special case as sometimes apply will come before it and other times it wont
	// which is why it needs to be here as well
	if packageStatus, found := skyhookNode.PackageStatus(_package.GetUniqueName()); found {
		switch packageStatus.Stage {
		case v1alpha1.StageConfig:
			stage = v1alpha1.StageConfig
		case v1alpha1.StageUpgrade:
			stage = v1alpha1.StageUpgrade
		case v1alpha1.StageUninstall:
			stage = v1alpha1.StageUninstall
		}
	}

	nextStage := skyhookNode.NextStage(_package)
	if nextStage != nil {
		stage = *nextStage
	}

	// test if pod exists, if so, bailout
	exists, err := r.PodExists(ctx, skyhookNode.GetNode().Name, skyhookNode.GetSkyhook().Name, _package)
	if err != nil {
		return err
	}

	// wait tell this is done if its happening
	status, found := skyhookNode.PackageStatus(_package.GetUniqueName())

	if found && status.State == v1alpha1.StateSkipped { // skipped, so nothing to do
		return nil
	}

	if found && status.State == v1alpha1.StateInProgress { // running, so do nothing atm
		if exists {
			return nil
		}
	}

	if exists {
		// nothing to do here, already running
		return nil
	}

	pod := r.CreatePodFromPackage(_package, skyhookNode.GetSkyhook(), skyhookNode.GetNode().Name, stage)

	if err := SetPackages(pod, skyhookNode.GetSkyhook().Skyhook, _package.Image, stage, _package); err != nil {
		return fmt.Errorf("error setting package on pod: %w", err)
	}

	// setup ownership of the pod we created
	// helps run time know what to do when something happens to this pod we are about to create
	if err := ctrl.SetControllerReference(skyhookNode.GetSkyhook().Skyhook, pod, r.scheme); err != nil {
		return fmt.Errorf("error setting ownership: %w", err)
	}

	if err := r.Create(ctx, pod); err != nil {
		return fmt.Errorf("error creating pod: %w", err)
	}

	if err = skyhookNode.Upsert(_package.PackageRef, _package.Image, v1alpha1.StateInProgress, stage, 0); err != nil {
		err = fmt.Errorf("error upserting package: %w", err) // want to keep going in this case, but don't want to lose the err
	}

	skyhookNode.SetStatus(v1alpha1.StatusInProgress)

	skyhookNode.GetSkyhook().AddCondition(metav1.Condition{
		Type:               fmt.Sprintf("%s/ApplyPackage", v1alpha1.METADATA_PREFIX),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: skyhookNode.GetSkyhook().Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ApplyPackage",
		Message:            fmt.Sprintf("Applying package [%s:%s] to node [%s]", _package.Name, _package.Version, skyhookNode.GetNode().Name),
	})

	r.recorder.Eventf(skyhookNode.GetNode(), EventTypeNormal, EventsReasonSkyhookApply, "Applying package [%s:%s] from [skyhook:%s] stage [%s]", _package.Name, _package.Version, skyhookNode.GetSkyhook().Name, stage)
	r.recorder.Eventf(skyhookNode.GetSkyhook(), EventTypeNormal, EventsReasonSkyhookApply, "Applying package [%s:%s] to node [%s] stage [%s]", _package.Name, _package.Version, skyhookNode.GetNode().Name, stage)

	skyhookNode.GetSkyhook().Updated = true

	return err
}

// Find any nodes for which all runtime required Skyhooks are complete and remove their runtime required taint
// Will return an error if the patching of the nodes is not possible
func (r *SkyhookReconciler) HandleRuntimeRequired(ctx context.Context, clusterState *clusterState) error {
	node_to_skyhooks, skyhook_node_map := groupSkyhooksByNode(clusterState)
	to_remove := getRuntimeRequiredTaintCompleteNodes(node_to_skyhooks, skyhook_node_map)
	// Remove the runtime required taint from nodes in to_remove
	taint_to_remove := r.opts.GetRuntimeRequiredTaint()
	errs := make([]error, 0)
	for _, node := range to_remove {
		// check before removing taint that it even exists to begin with
		if !taints.TaintExists(node.Spec.Taints, &taint_to_remove) {
			continue
		}
		// RemoveTaint will ALWAYS return nil for its error so no need to check it
		new_node, updated, _ := taints.RemoveTaint(node, &taint_to_remove)
		if updated {
			err := r.Patch(ctx, new_node, client.MergeFrom(node))
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}
	return nil
}

// Group Skyhooks by what node they target
func groupSkyhooksByNode(clusterState *clusterState) (map[types.UID][]SkyhookNodes, map[types.UID]*corev1.Node) {
	node_to_skyhooks := make(map[types.UID][]SkyhookNodes)
	nodes := make(map[types.UID]*corev1.Node)
	for _, skyhook := range clusterState.skyhooks {
		// Ignore skyhooks that don't have runtime required
		if !skyhook.skyhook.Skyhook.Spec.RuntimeRequired {
			continue
		}
		for _, node := range skyhook.nodes {
			if _, ok := node_to_skyhooks[node.GetNode().UID]; !ok {
				node_to_skyhooks[node.GetNode().UID] = make([]SkyhookNodes, 0)
				nodes[node.GetNode().UID] = node.GetNode()
			}
			node_to_skyhooks[node.GetNode().UID] = append(node_to_skyhooks[node.GetNode().UID], skyhook)
		}

	}
	return node_to_skyhooks, nodes
}

// Get the nodes to remove runtime required taint from node that all skyhooks targeting that node have completed
func getRuntimeRequiredTaintCompleteNodes(node_to_skyhooks map[types.UID][]SkyhookNodes, nodes map[types.UID]*corev1.Node) []*corev1.Node {
	to_remove := make([]*corev1.Node, 0)
	for node_uid, skyhooks := range node_to_skyhooks {
		all_complete := true
		for _, skyhook := range skyhooks {
			if !skyhook.IsComplete() {
				all_complete = false
				break
			}
		}
		if all_complete {
			to_remove = append(to_remove, nodes[node_uid])
		}
	}
	return to_remove
}
