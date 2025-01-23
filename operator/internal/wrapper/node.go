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

package wrapper

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"gitlab-master.nvidia.com/dgx/infra/skyhook-operator/api/v1alpha1"
	"gitlab-master.nvidia.com/dgx/infra/skyhook-operator/internal/graph"
	"gitlab-master.nvidia.com/dgx/infra/skyhook-operator/internal/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// there are 2 interface to reflect functions that need a skyhook and node
// and ones that just need a node

// SkyhookNode wraps a node with a supporting skyhook
type SkyhookNode interface {
	SkyhookNodeOnly
	GetSkyhook() *Skyhook
	GetComplete() []string
	SetStatus(status v1alpha1.Status)
	IsComplete() bool
	ProgressSkipped()
	IsPackageComplete(_package v1alpha1.Package) bool
	RunNext() ([]*v1alpha1.Package, error)
	NextStage(_package *v1alpha1.Package) *v1alpha1.Stage
	HasInterrupt(_package v1alpha1.Package) bool
	UpdateCondition()
}

// SkyhookNodeOnly wraps the node with just a skyhook name
type SkyhookNodeOnly interface {
	Status() v1alpha1.Status
	// SetStatus is in both interfaces, does more if skyhook is not nil
	SetStatus(status v1alpha1.Status)
	PackageStatus(name string) (*v1alpha1.PackageStatus, bool)
	SetVersion()
	GetVersion() string
	Migrate(logger logr.Logger) error
	State() (v1alpha1.NodeState, error)
	SetState(state v1alpha1.NodeState) error
	RemoveState(_package v1alpha1.PackageRef) error
	Upsert(_package v1alpha1.PackageRef, image string, state v1alpha1.State, stage v1alpha1.Stage, restarts int32) error
	GetNode() *corev1.Node
	Taint(key string)
	RemoveTaint(key string)
	Cordon()
	Uncordon()
	Reset()
	Changed() bool
}

var _ SkyhookNode = &skyhookNode{}

// most of use cases for the wrapper just needs name, so this stub is for making helpers for those use cases,
// should help reduce calls to api, and not leak stubbed skyhooks with just name set
func NewSkyhookNodeOnly(node *corev1.Node, skyhookName string) (SkyhookNodeOnly, error) {
	ret := &skyhookNode{
		Node:        node,
		skyhookName: skyhookName,
	}
	state, err := ret.State()
	if err != nil {
		return nil, fmt.Errorf("error creating skyhookNode: %w", err)
	}
	ret.nodeState = state
	return ret, nil
}

// Convert will upgrade this to be the full interface if you have a skyhook
func Convert(node SkyhookNodeOnly, skyhook *v1alpha1.Skyhook) (SkyhookNode, error) {
	ret := node.(*skyhookNode)
	ret.skyhook = &Skyhook{Skyhook: skyhook}

	graph, err := skyhook.Spec.BuildGraph()
	if err != nil {
		return nil, err
	}

	ret.graph = graph

	return ret, nil
}

func NewSkyhookNode(node *corev1.Node, skyhook *v1alpha1.Skyhook) (SkyhookNode, error) {

	t, err := NewSkyhookNodeOnly(node, skyhook.Name)
	if err != nil {
		return nil, err
	}

	return Convert(t, skyhook)
}

type skyhookNode struct {
	*corev1.Node
	skyhookName string
	skyhook     *Skyhook
	nodeState   v1alpha1.NodeState
	graph       graph.DependencyGraph[*v1alpha1.Package]
	updated     bool
}

// GetSkyhook implements sskyhookNode.
func (node *skyhookNode) GetSkyhook() *Skyhook {
	return node.skyhook
}

// GetNode implements sskyhookNode.
func (node *skyhookNode) GetNode() *corev1.Node {
	return node.Node
}

func (node *skyhookNode) SetStatus(status v1alpha1.Status) {

	s, ok := node.Annotations[fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !ok || s != string(status) {
		if node.Annotations == nil {
			node.Annotations = make(map[string]string)
		}
		if node.Labels == nil {
			node.Labels = make(map[string]string)
		}
		node.updated = true
		node.Annotations[fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)] = string(status)
		node.Labels[fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)] = string(status)
	}

	if status == v1alpha1.StatusComplete {
		node.Uncordon()
	}

	if node.skyhook != nil {
		node.skyhook.SetNodeStatus(node.Node.Name, status)
		node.skyhook.SetNodeState(node.Node.Name, node.nodeState)
	}
}

func (node *skyhookNode) Status() v1alpha1.Status {
	status, ok := node.Annotations[fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !ok {
		return v1alpha1.StatusUnknown
	}
	return v1alpha1.GetStatus(status)
}

func (node *skyhookNode) State() (v1alpha1.NodeState, error) {

	if node.nodeState != nil {
		return node.nodeState, nil
	}

	if node == nil {
		return nil, nil
	}
	s, ok := node.Annotations[fmt.Sprintf("%s/nodeState_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !ok {
		return nil, nil
	}

	ret := v1alpha1.NodeState{}
	err := json.Unmarshal([]byte(s), &ret)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling node state: %w", err)
	}

	return ret, nil
}

func (node *skyhookNode) PackageStatus(name string) (*v1alpha1.PackageStatus, bool) {
	packageStatus := node.nodeState.Get(name)
	if packageStatus != nil {
		return packageStatus, true
	}

	return nil, false
}

func (node *skyhookNode) SetVersion() {

	current := node.GetVersion()
	if current == version.VERSION { // if has not changed, do nothing and not set updated
		return
	}

	if version.VERSION == "" { // was not compiled with version, so do nothing
		return
	}

	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}

	node.Annotations[fmt.Sprintf("%s/version_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)] = version.VERSION
	node.updated = true
}

func (node *skyhookNode) GetVersion() string {
	version, ok := node.Annotations[fmt.Sprintf("%s/version_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !ok {
		return ""
	}
	return version
}

func (node *skyhookNode) Migrate(logger logr.Logger) error {

	from := node.GetVersion()
	to := version.VERSION

	if from == to {
		return nil
	}

	if from == "" { // from before versioning, so is empty
		err := migrateNodeTo_0_5_0(node, logger)
		if err != nil {
			return err
		}
		node.SetVersion()
		return nil
	}

	return nil
}

func (node *skyhookNode) SetState(state v1alpha1.NodeState) error {
	if node == nil || state == nil {
		return nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("error marshalling node state: %w", err)
	}

	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}

	s, ok := node.Annotations[fmt.Sprintf("%s/nodeState_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !ok || s != string(data) {
		node.Annotations[fmt.Sprintf("%s/nodeState_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)] = string(data)
		node.nodeState = state
		node.updated = true
	}

	return nil
}

func (node *skyhookNode) RemoveState(_package v1alpha1.PackageRef) error {
	changed := node.nodeState.RemoveState(_package)
	if changed {
		return node.SetState(node.nodeState)
	}

	return nil
}

func (node *skyhookNode) Upsert(_package v1alpha1.PackageRef, image string, state v1alpha1.State, stage v1alpha1.Stage, restarts int32) error {
	changed := node.nodeState.Upsert(_package, image, state, stage, restarts)
	if changed {
		if node.skyhook != nil {
			node.skyhook.Updated = true
		}

		return node.SetState(node.nodeState)
	}
	return nil
}

func (node *skyhookNode) IsPackageComplete(_package v1alpha1.Package) bool {
	return node.nodeState.IsPackageComplete(_package, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates())
}

func (node *skyhookNode) IsComplete() bool {
	return node.nodeState.IsComplete(node.skyhook.Spec.Packages, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates())
}

func (node *skyhookNode) GetComplete() []string {
	return node.nodeState.GetComplete(node.skyhook.Spec.Packages, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates())
}

func (node *skyhookNode) ProgressSkipped() {
	if node.nodeState.ProgressSkipped(node.skyhook.Spec.Packages, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates()) {
		node.skyhook.Updated = true
		node.updated = true
	}
}

func (node *skyhookNode) RunNext() ([]*v1alpha1.Package, error) {
	complete := node.GetComplete()

	var next []string
	var err error
	if len(complete) == 0 { // base case, start from leaves
		next, err = node.graph.Next()
	} else {
		next, err = node.graph.Next(complete...)
	}

	// this case is if we updated the SCR, and we have new leafs that are getting skipped because we have some complete
	if !node.IsComplete() && len(next) == 0 {
		next, err = node.graph.Next()

		// now might have some completed ones, so remove those
		temp := next[:0]
		for _, item := range next {
			completed := false
			for _, done := range complete {
				if done == item {
					completed = true
				}
			}
			if !completed {
				temp = append(temp, item)
			}
		}
		next = temp
	}

	if err != nil {
		return nil, err
	}

	toRun := node.graph.Get(next...)

	// make sure they are always in the same order, make things deterministic
	sort.Slice(toRun, func(i, j int) bool {
		return toRun[i].Name < toRun[j].Name
	})

	return toRun, nil
}

func (node *skyhookNode) NextStage(_package *v1alpha1.Package) *v1alpha1.Stage {
	return node.nodeState.NextStage(_package, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates())
}

func (node *skyhookNode) Changed() bool {
	return node.updated
}

func (node *skyhookNode) HasInterrupt(_package v1alpha1.Package) bool {
	return node.nodeState.HasInterrupt(_package, node.skyhook.GetConfigInterrupts(), node.skyhook.GetConfigUpdates())
}

func (node *skyhookNode) Taint(key string) {

	// dont add it if it exists already, dups will error
	for _, t := range node.Spec.Taints {
		if t.Key == key {
			return
		}
	}

	if node.Spec.Taints == nil {
		node.Spec.Taints = make([]corev1.Taint, 0)
	}

	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
		Key:    key,
		Value:  node.GetSkyhook().Name,
		Effect: corev1.TaintEffectNoSchedule,
	})
	node.updated = true
}

func (node *skyhookNode) RemoveTaint(key string) {

	if len(node.Spec.Taints) == 0 {
		return
	}

	temp := node.Spec.Taints[:0]
	for _, t := range node.Spec.Taints {
		if t.Key != key {
			temp = append(temp, t)
		}
	}

	if len(temp) < len(node.Spec.Taints) {
		node.Spec.Taints = temp
		node.updated = true
	}
}

func (node *skyhookNode) Cordon() {
	_, ok := node.Annotations[fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if !node.Spec.Unschedulable || !ok {
		node.Spec.Unschedulable = true
		node.Annotations[fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)] = "true"
		node.updated = true
	}
}

func (node *skyhookNode) Uncordon() {

	other := false
	for key := range node.Annotations {
		if strings.HasPrefix(key, fmt.Sprintf("%s/cordon_", v1alpha1.METADATA_PREFIX)) &&
			key != fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName) {
			other = true
			break
		}
	}

	if other { // if others also hold a cordon, just remove our self
		delete(node.Annotations, fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName))
		node.updated = true
		return
	}

	// if we hold a cordon remove it, also we dont want to remove a cordon if we dont have one...
	_, ok := node.Annotations[fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName)]
	if ok {
		node.Spec.Unschedulable = false
		delete(node.Annotations, fmt.Sprintf("%s/cordon_%s", v1alpha1.METADATA_PREFIX, node.skyhookName))
		node.updated = true
	}
}

func (node *skyhookNode) Reset() {

	delete(node.skyhook.Status.NodeState, node.Name)
	delete(node.skyhook.Status.NodeStatus, node.Name)
	node.skyhook.Status.Status = v1alpha1.StatusUnknown
	node.skyhook.Updated = true

	delete(node.Annotations, fmt.Sprintf("%s/cordon_", v1alpha1.METADATA_PREFIX))
	delete(node.Annotations, fmt.Sprintf("%s/nodeState_%s", v1alpha1.METADATA_PREFIX, node.skyhook.Name))
	delete(node.Annotations, fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhook.Name))

	delete(node.Labels, fmt.Sprintf("%s/status_%s", v1alpha1.METADATA_PREFIX, node.skyhook.Name))
	node.updated = true
}

func (node *skyhookNode) UpdateCondition() {
	readyReason, errorReason := "Incomplete", "Not Erroring"
	errorCondFound, condFound := false, false

	if node.Node.Status.Conditions == nil {
		node.Node.Status.Conditions = make([]corev1.NodeCondition, 0)
	}

	errorStatus, condStatus := corev1.ConditionFalse, corev1.ConditionTrue
	if node.IsComplete() {
		readyReason = "Complete"
		condStatus = corev1.ConditionFalse
	}

	for _, packageStatus := range node.nodeState {
		switch packageStatus.State {
		case v1alpha1.StateErroring, v1alpha1.StateUnknown:
			errorReason = "Package(s) Erroring or Unknown"
			errorStatus = corev1.ConditionTrue
		}
	}

	cond := corev1.NodeCondition{
		Type:               corev1.NodeConditionType(fmt.Sprintf("%s/%s/NotReady", v1alpha1.METADATA_PREFIX, node.skyhookName)),
		Status:             condStatus,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             readyReason,
		Message:            fmt.Sprintf("Skyhook %s Ready", node.skyhookName),
	}

	errorCond := corev1.NodeCondition{
		Type:               corev1.NodeConditionType(fmt.Sprintf("%s/%s/Erroring", v1alpha1.METADATA_PREFIX, node.skyhookName)),
		Status:             errorStatus,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             errorReason,
		Message:            fmt.Sprintf("Package Erroring or Unknown for %s", node.skyhookName),
	}

	for i, condition := range node.Node.Status.Conditions {
		if condition.Type == errorCond.Type {
			errorCondFound = true
			if condition.Reason != errorCond.Reason && condition.Message == errorCond.Message {
				node.Node.Status.Conditions[i] = errorCond // update it with the new condition
				node.updated = true
			}
		} else if condition.Type == cond.Type {
			condFound = true
			if condition.Reason != cond.Reason && condition.Message == cond.Message {
				node.Node.Status.Conditions[i] = cond // update it with the new condition
				node.updated = true
			}
		}
	}

	if !errorCondFound {
		node.Node.Status.Conditions = append([]corev1.NodeCondition{errorCond}, node.Node.Status.Conditions...)
		node.updated = true
	}
	if !condFound {
		node.Node.Status.Conditions = append([]corev1.NodeCondition{cond}, node.Node.Status.Conditions...)
		node.updated = true
	}
}
