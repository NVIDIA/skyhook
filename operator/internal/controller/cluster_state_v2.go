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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/NVIDIA/skyhook/api/v1alpha1"
	"github.com/NVIDIA/skyhook/internal/version"
	"github.com/NVIDIA/skyhook/internal/wrapper"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// tracks original objects
// useful for using patch api
// insert when u first see object
// later get a c
type ObjectTracker struct {
	objects map[string]client.Object
}

// GetOriginal will return prior tracked object if it exists, otherwise return nil
func (t *ObjectTracker) GetOriginal(obj client.Object) client.Object {
	key := fmt.Sprintf("%s|%s|%s-%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), obj.GetUID())

	if obj, ok := t.objects[key]; ok {
		return obj
	}
	return nil
}

// Track when calling Track, make sure to pass in a DeepCopy to make sure to save to a copy
func (t *ObjectTracker) Track(obj client.Object) {

	key := fmt.Sprintf("%s|%s|%s-%s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName(), obj.GetUID())

	_, ok := t.objects[key]
	if !ok { // was never inserted, so add it, else dont care
		t.objects[key] = obj
		return
	}
}

type clusterState struct {
	tracker  ObjectTracker
	skyhooks []*skyhookNodes
}

func BuildState(skyhooks *v1alpha1.SkyhookList, nodes *corev1.NodeList) (*clusterState, error) {

	ret := &clusterState{
		tracker:  ObjectTracker{objects: make(map[string]client.Object)},
		skyhooks: make([]*skyhookNodes, len(skyhooks.Items)),
		// nodes:    make(map[string][]*SkyhookNode),
	}

	for idx, skyhook := range skyhooks.Items {
		ret.tracker.Track(skyhook.DeepCopy())

		ret.skyhooks[idx] = &skyhookNodes{
			skyhook: wrapper.NewSkyhookWrapper(&skyhook),
			nodes:   make([]wrapper.SkyhookNode, 0),
		}
		for _, node := range nodes.Items {
			skyNode, err := wrapper.NewSkyhookNode(&node, &skyhook)
			if err != nil {
				return nil, err
			}

			selector, err := metav1.LabelSelectorAsSelector(&skyhook.Spec.NodeSelector)
			if err != nil {
				return nil, err
			}
			if selector.Matches(labels.Set(node.Labels)) { // note: if selector is empty, it selects all
				ret.tracker.Track(node.DeepCopy())
				ret.skyhooks[idx].nodes = append(ret.skyhooks[idx].nodes, skyNode)
			}
		}
	}

	sort.Slice(ret.skyhooks, func(i, j int) bool {
		return ret.skyhooks[i].skyhook.Name < ret.skyhooks[j].skyhook.Name
	})

	for _, skyhook := range ret.skyhooks {
		sort.Slice(skyhook.nodes, func(i, j int) bool {
			return skyhook.nodes[i].GetNode().CreationTimestamp.Before(&skyhook.nodes[j].GetNode().CreationTimestamp)
		})
	}

	return ret, nil
}

// SkyhookNodes wraps the skyhook and nodes that it pertains too
type SkyhookNodes interface {
	CollectNodeStatus() v1alpha1.Status
	GetSkyhook() *wrapper.Skyhook
	GetNodes() []wrapper.SkyhookNode
	GetNode(name string) (v1alpha1.Status, wrapper.SkyhookNode)
	IsComplete() bool
	NodeCount() int
	SetStatus(status v1alpha1.Status)
	Status() v1alpha1.Status
	// WasUpdated() bool
	UpdateCondition() bool
	ReportState()
	Migrate(logger logr.Logger) error
}

var _ SkyhookNodes = &skyhookNodes{}

// skyhookNodes impl's. SkyhookNodes
type skyhookNodes struct {
	skyhook     *wrapper.Skyhook
	nodes       []wrapper.SkyhookNode
	priorStatus v1alpha1.Status
}

func (s *skyhookNodes) GetNodes() []wrapper.SkyhookNode {
	return s.nodes
}

func (s *skyhookNodes) GetSkyhook() *wrapper.Skyhook {
	return s.skyhook
}

func (s *skyhookNodes) NodeCount() int {
	return len(s.nodes)
}

// func (s *skyhookNodes) WasUpdated() bool {
// 	return s.skyhook.WasUpdated()
// }

func (s *skyhookNodes) IsComplete() bool {
	for _, node := range s.nodes {
		if !node.IsComplete() {
			return false
		}
	}

	return true
}

func (s *skyhookNodes) Status() v1alpha1.Status {
	return s.skyhook.Status.Status
}

func (s *skyhookNodes) SetStatus(status v1alpha1.Status) {
	s.priorStatus = s.skyhook.Status.Status

	s.skyhook.SetStatus(status)
}

// CollectNodeStatus collects all the nodes current status
func (s *skyhookNodes) CollectNodeStatus() v1alpha1.Status {
	complete := 0
	status := v1alpha1.StatusUnknown

	for _, node := range s.nodes {
		if node.IsComplete() {
			complete += 1
			continue
		}
		switch node.Status() {
		case v1alpha1.StatusInProgress:
			status = v1alpha1.StatusInProgress
		case v1alpha1.StatusErroring:
			status = v1alpha1.StatusErroring
		case v1alpha1.StatusUnknown:
			// only one unknown means unknown
			return v1alpha1.StatusUnknown
		}
	}
	if complete == len(s.nodes) { // all need to be complete to be considered complete
		return v1alpha1.StatusComplete
	}
	return status
}

// Pick will grab node if exists
func (s *skyhookNodes) GetNode(name string) (v1alpha1.Status, wrapper.SkyhookNode) {

	for _, node := range s.nodes {
		if node.GetNode().Name == name {
			return node.Status(), node
		}
	}
	return v1alpha1.StatusUnknown, nil
}

func (s *skyhookNodes) UpdateCondition() bool { // TODO: might make sense to make this a ready, not what it is now

	// don't do this there was no change
	if s.skyhook.Updated && s.priorStatus != "" {
		if s.skyhook.Status.Conditions == nil {
			s.skyhook.Status.Conditions = make([]metav1.Condition, 0)
		}

		condType := fmt.Sprintf("%s/Transition", v1alpha1.METADATA_PREFIX)
		status := metav1.ConditionFalse
		if s.IsComplete() {
			status = metav1.ConditionTrue
		}
		new := metav1.Condition{
			Type:               condType,
			Status:             status,
			ObservedGeneration: s.skyhook.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(s.Status()),
			Message:            fmt.Sprintf("Transitioned [%s] -> [%s]", s.priorStatus, s.Status()),
		}

		for i, condition := range s.skyhook.Status.Conditions {
			if condition.Type == condType {
				// found it
				if condition.Reason == new.Reason && condition.Message == new.Message { // the reason is the same, then we are not
					return false
				}
				s.skyhook.Status.Conditions[i] = new // update it with the new condition
				s.skyhook.Updated = true
				return true // done
			}
		}

		s.skyhook.Updated = true
		s.skyhook.Status.Conditions = append(s.skyhook.Status.Conditions, new)
		return true
	}
	return false
}

type NodePicker struct {
	priorityNodes             map[string]time.Time
	runtimeRequiredToleration corev1.Toleration
}

func NewNodePicker(runtimeRequiredToleration corev1.Toleration) *NodePicker {
	return &NodePicker{
		priorityNodes:             make(map[string]time.Time),
		runtimeRequiredToleration: runtimeRequiredToleration,
	}
}

// primeAndPruneNodes add current priority from skyhook status, and check time removing old ones
func (s *NodePicker) primeAndPruneNodes(skyhook *wrapper.Skyhook) {

	now := time.Now()
	for n, t := range skyhook.Status.NodePriority {
		if now.Sub(t.Time) > skyhook.Spec.InterruptionBudget.Timeout.Duration {
			// prune
			// was not cleaned up before now, so it has timed out...
			// this could cause issues if the node is still in the cluster api,
			// but is bad, but seems like if that is true, there might be bigger problems
			// so removing from status
			delete(skyhook.Status.NodePriority, n)
			skyhook.Updated = true
		} else {
			s.priorityNodes[n] = t.Time
		}
	}

}

// upsertPick updates or inserts the node priority for a given name in the Skyhook object.
// If the node priority already exists, it updates the priority with the current time.
// If the node priority does not exist, it inserts a new priority with the current time.
// The updated Skyhook object is marked as "Updated".
//
// Parameters:
// - name: The name of the node.
// - skyhook: The Skyhook object to update.
func (s *NodePicker) upsertPick(name string, skyhook *wrapper.Skyhook) {

	if skyhook.Status.NodePriority == nil {
		skyhook.Status.NodePriority = make(map[string]metav1.Time)
	}

	if t, ok := skyhook.Status.NodePriority[name]; ok { // check if exists before inserting
		s.priorityNodes[name] = t.Time
		return
	}

	now := time.Now()
	s.priorityNodes[name] = now

	skyhook.Status.NodePriority[name] = metav1.NewTime(now)
	skyhook.Updated = true
}

func CheckTaintToleration(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
	// Must tolerate all taints.
	all_tolerated := true
	for _, taint := range taints {
		tolerated := false
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(&taint) {
				tolerated = true
				break
			}
		}
		all_tolerated = all_tolerated && tolerated
	}
	return all_tolerated
}

func (np *NodePicker) SelectNodes(s *skyhookNodes) []wrapper.SkyhookNode {

	np.primeAndPruneNodes(s.skyhook)

	nodes := make([]wrapper.SkyhookNode, 0)

	// Straight from skyhook_controller CreatePodForPackage
	tolerations := append([]corev1.Toleration{ // tolerate all cordon
		{
			Key:      TaintUnschedulable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      SkyhookTaintUnschedulable,
			Value:    s.skyhook.Name,
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, s.skyhook.Spec.AdditionalTolerations...)

	if s.skyhook.Spec.RuntimeRequired {
		tolerations = append(tolerations, np.runtimeRequiredToleration)
	}

	var nodeCount int
	if s.skyhook.Spec.InterruptionBudget.Percent != nil {
		limit := float64(*s.skyhook.Spec.InterruptionBudget.Percent) / 100
		nodeCount = max(1, int(float64(s.NodeCount())*limit))
	}
	if s.skyhook.Spec.InterruptionBudget.Count != nil {
		nodeCount = max(1, min(s.NodeCount(), *s.skyhook.Spec.InterruptionBudget.Count))
	}

	// if we don't have a setting still, set it to all
	if nodeCount == 0 {
		nodeCount = s.NodeCount()
	}

	// first check prior picks if we can
	for pnode := range np.priorityNodes {
		status, pick := s.GetNode(pnode)
		if status != v1alpha1.StatusComplete && pick != nil {
			if len(nodes) >= nodeCount {
				break
			}
			nodes = append(nodes, pick)
			// np.upsertPick(pick.GetNode().Name, s.skyhook) // track pick
		}
	}

	priority := []v1alpha1.Status{v1alpha1.StatusInProgress, v1alpha1.StatusUnknown, v1alpha1.StatusErroring}

	nodesWithTaintTolerationIssue := make([]string, 0)
	// look for progress first
	for _, order := range priority {
		for _, node := range s.nodes {

			if len(nodes) >= nodeCount {
				break
			}

			if node.Status() != order {
				continue
			}

			nodes = append(nodes, node)
			// np.upsertPick(node.GetNode().GetName(), s.skyhook)
		}
	}

	// loop through the selected node list and remove any nodes that are not tolerable
	final_nodes := make([]wrapper.SkyhookNode, 0)
	for _, node := range nodes {
		if CheckTaintToleration(tolerations, node.GetNode().Spec.Taints) {
			final_nodes = append(final_nodes, node)
			np.upsertPick(node.GetNode().GetName(), s.skyhook) // track pick
		} else {
			nodesWithTaintTolerationIssue = append(nodesWithTaintTolerationIssue, node.GetNode().Name)
		}
	}

	// if we have nodes that are not tolerable, we need to add a condition to the skyhook
	if len(nodesWithTaintTolerationIssue) > 0 {
		s.skyhook.AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/TaintNotTolerable", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionTrue,
			Reason:             "TaintNotTolerable",
			Message:            fmt.Sprintf("Node [%s] has taints that are not tolerable. Skipping.", strings.Join(nodesWithTaintTolerationIssue, ", ")),
			LastTransitionTime: metav1.Now(),
		})
	} else {
		s.skyhook.AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/TaintNotTolerable", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionFalse,
			Reason:             "TaintNotTolerable",
			Message:            "All nodes have tolerable taints.",
			LastTransitionTime: metav1.Now(),
		})
	}

	return final_nodes
}

// for node/package source of true, its on the node (we true to reflect this on the skyhook status)
// for SCR true, we need to look at all nodes and compare state to current SCR. This should be reflected in the SCR too.

// Introspect checks the current state of nodes, and SCR if they are in a bad mix, update to be correct
func IntrospectSkyhook(skyhook SkyhookNodes) bool {
	change := false

	scrStatus := skyhook.Status()
	collectNodeStatus := skyhook.CollectNodeStatus()

	if scrStatus != collectNodeStatus {
		skyhook.SetStatus(collectNodeStatus)
	}

	for _, node := range skyhook.GetNodes() {
		if IntrospectNode(node, skyhook) {
			change = true
		}
	}

	skyhook.UpdateCondition()
	if skyhook.GetSkyhook().Updated {
		change = true
	}
	return change
}

func IntrospectNode(node wrapper.SkyhookNode, skyhook SkyhookNodes) bool {

	nodeStatus := node.Status()
	node.SetStatus(nodeStatus)

	if nodeStatus != v1alpha1.StatusComplete && node.IsComplete() {
		node.SetStatus(v1alpha1.StatusComplete)
	}

	if nodeStatus == v1alpha1.StatusComplete && !node.IsComplete() {
		node.SetStatus(v1alpha1.StatusUnknown)
	}

	return node.Changed()
}

// ReportState collects the current state of the skyhook and reports it to the skyhook status for printer columns
func (skyhook *skyhookNodes) ReportState() {

	completeNodes, nodesInProgress, nodeCount := 0, 0, len(skyhook.nodes)

	// get current count of completed nodes
	for _, node := range skyhook.nodes {
		if node.IsComplete() {
			completeNodes++
		} else if node.Status() == v1alpha1.StatusInProgress {
			nodesInProgress++
		} else if node.Status() == v1alpha1.StatusErroring {
			nodesInProgress++
		}
	}

	// set current count of completed nodes
	nodeString := fmt.Sprintf("%d/%d", completeNodes, nodeCount)
	if nodeString != skyhook.skyhook.GetCompleteNodes() {
		skyhook.skyhook.SetCompleteNodes(nodeString)
		skyhook.skyhook.Updated = true
	}

	// set current nodes in progress
	if skyhook.skyhook.GetNodesInProgress() != nodesInProgress {
		skyhook.skyhook.SetNodesInProgress(nodesInProgress)
	}

	// get list of packages and versions in the skyhook
	packageNames := make([]string, 0)
	for _, _package := range skyhook.skyhook.Spec.Packages {
		packageNames = append(packageNames, fmt.Sprintf("%s:%s", _package.Name, _package.Version))
	}

	// turn the package list into a comma separated string
	sort.Strings(packageNames)
	packageList := strings.Join(packageNames, ",")
	if packageList != skyhook.skyhook.GetPackageList() {
		skyhook.skyhook.SetPackageList(packageList)
		skyhook.skyhook.Updated = true
	}
}

// Migrate is for tracking versions of the operator. If the version changes, we need to update the state of
// the skyhook and nodes to be valid for the new version. The pattern here is to check the versions if they match a version
// matrix we have 3 places to handle changes. Here and in the skyhook and node wrappers. The mirgrate function is called to compare
// version and then actual work are in the migration files prefixed with zz.migration and the version number.
func (skyhook *skyhookNodes) Migrate(logger logr.Logger) error {

	for _, node := range skyhook.nodes {
		if node.GetVersion() == version.VERSION {
			continue // already up to date
		}
		if err := node.Migrate(logger); err != nil {
			return fmt.Errorf("error migrating node [%s]: %w", node.GetNode().Name, err)
		}
	}

	from := skyhook.skyhook.GetVersion()
	to := version.VERSION

	if from == to {
		return nil
	}

	if err := skyhook.skyhook.Migrate(logger); err != nil {
		return fmt.Errorf("error migrating skyhook [%s]: %w", skyhook.skyhook.Name, err)
	}

	if from == "" { // before this was a thing v0.4.0 and before
		err := migrateSkyhookNodesTo_0_5_0(skyhook, logger)
		if err != nil {
			return err
		}
	}

	return nil
}
