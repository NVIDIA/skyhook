/*
 * SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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
	"sort"
	"strings"
	"time"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/version"
	"github.com/NVIDIA/skyhook/operator/internal/wrapper"
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
	skyhooks []SkyhookNodes
}

func BuildState(skyhooks *v1alpha1.SkyhookList, nodes *corev1.NodeList, deploymentPolicies *v1alpha1.DeploymentPolicyList) (*clusterState, error) {

	ret := &clusterState{
		tracker:  ObjectTracker{objects: make(map[string]client.Object)},
		skyhooks: make([]SkyhookNodes, len(skyhooks.Items)),
		// nodes:    make(map[string][]*SkyhookNode),
	}

	for idx, skyhook := range skyhooks.Items {
		ret.tracker.Track(skyhook.DeepCopy())

		ret.skyhooks[idx] = &skyhookNodes{
			skyhook:      wrapper.NewSkyhookWrapper(&skyhook),
			nodes:        make([]wrapper.SkyhookNode, 0),
			compartments: make(map[string]*wrapper.Compartment),
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
				ret.skyhooks[idx].AddNode(skyNode)
			}
		}

		// find deployment policy and all compartments + the default one
		// Skip skyhooks that don't have a deployment policy
		if skyhook.Spec.DeploymentPolicy == "" {
			continue
		}

		for _, deploymentPolicy := range deploymentPolicies.Items {
			if deploymentPolicy.Name == skyhook.Spec.DeploymentPolicy {
				for _, compartment := range deploymentPolicy.Spec.Compartments {
					// Load persisted batch state if it exists
					var batchState *v1alpha1.BatchProcessingState
					if skyhook.Status.CompartmentBatchStates != nil {
						if state, exists := skyhook.Status.CompartmentBatchStates[compartment.Name]; exists {
							batchState = &state
						}
					}
					ret.skyhooks[idx].AddCompartment(compartment.Name, wrapper.NewCompartmentWrapper(&compartment, batchState))
				}
				// use policy default
				var defaultBatchState *v1alpha1.BatchProcessingState
				if skyhook.Status.CompartmentBatchStates != nil {
					if state, exists := skyhook.Status.CompartmentBatchStates[v1alpha1.DefaultCompartmentName]; exists {
						defaultBatchState = &state
					}
				}
				ret.skyhooks[idx].AddCompartment(v1alpha1.DefaultCompartmentName, wrapper.NewCompartmentWrapper(&v1alpha1.Compartment{
					Name:     v1alpha1.DefaultCompartmentName,
					Budget:   deploymentPolicy.Spec.Default.Budget,
					Strategy: deploymentPolicy.Spec.Default.Strategy,
				}, defaultBatchState))
			}
		}
	}

	// Sort by priority (ascending), then by name (ascending) if priorities are equal
	sort.Slice(ret.skyhooks, func(i, j int) bool {
		pi := ret.skyhooks[i].GetSkyhook().Spec.Priority
		pj := ret.skyhooks[j].GetSkyhook().Spec.Priority
		if pi != pj {
			return pi < pj
		}
		return ret.skyhooks[i].GetSkyhook().Name < ret.skyhooks[j].GetSkyhook().Name
	})

	for _, skyhook := range ret.skyhooks {
		sort.Slice(skyhook.GetNodes(), func(i, j int) bool {
			return skyhook.GetNodes()[i].GetNode().CreationTimestamp.Before(&skyhook.GetNodes()[j].GetNode().CreationTimestamp)
		})
	}

	return ret, nil
}

func GetNextSkyhook(skyhooks []SkyhookNodes) SkyhookNodes {
	for _, skyhook := range skyhooks {
		if skyhook.IsComplete() || skyhook.IsDisabled() {
			continue
		}
		return skyhook
	}
	// Always return the last non disabled skyhook to handle any final state logic
	// for i := len(skyhooks) - 1; i >= 0; i-- {
	// 	if !skyhooks[i].IsDisabled() {
	// 		return skyhooks[i]
	// 	}
	// }
	return nil
}

// SkyhookNodes wraps the skyhook and nodes that it pertains too
type SkyhookNodes interface {
	CollectNodeStatus() v1alpha1.Status
	GetSkyhook() *wrapper.Skyhook
	GetNodes() []wrapper.SkyhookNode
	GetNode(name string) (v1alpha1.Status, wrapper.SkyhookNode)
	AddNode(node wrapper.SkyhookNode)
	IsComplete() bool
	IsDisabled() bool
	IsPaused() bool
	NodeCount() int
	SetStatus(status v1alpha1.Status)
	Status() v1alpha1.Status
	GetPriorStatus() v1alpha1.Status
	// WasUpdated() bool
	UpdateCondition() bool
	ReportState()
	Migrate(logger logr.Logger) error

	GetCompartments() map[string]*wrapper.Compartment
	AddCompartment(name string, compartment *wrapper.Compartment)
	AddCompartmentNode(name string, node wrapper.SkyhookNode)
}

var _ SkyhookNodes = &skyhookNodes{}

// skyhookNodes impl's. SkyhookNodes
type skyhookNodes struct {
	skyhook      *wrapper.Skyhook
	nodes        []wrapper.SkyhookNode
	priorStatus  v1alpha1.Status
	compartments map[string]*wrapper.Compartment
}

func (s *skyhookNodes) GetPriorStatus() v1alpha1.Status {
	return s.priorStatus
}

func (s *skyhookNodes) GetNodes() []wrapper.SkyhookNode {
	return s.nodes
}

func (s *skyhookNodes) AddNode(node wrapper.SkyhookNode) {
	s.nodes = append(s.nodes, node)
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

func (s *skyhookNodes) IsDisabled() bool {
	return s.skyhook.IsDisabled()
}

func (s *skyhookNodes) IsPaused() bool {
	return s.skyhook.IsPaused()
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
			// only one erroring means erroring
			return v1alpha1.StatusErroring
		case v1alpha1.StatusBlocked:
			// only one blocked means blocked
			return v1alpha1.StatusBlocked
		case v1alpha1.StatusUnknown:
			// only one unknown means unknown
			return v1alpha1.StatusUnknown
		}
	}

	// all need to be complete to be considered complete
	if complete == len(s.nodes) {
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
func (s *NodePicker) primeAndPruneNodes(skyhook SkyhookNodes) {

	for n, t := range skyhook.GetSkyhook().Status.NodePriority {
		// prune
		// if the node is complete, remove it from the priority list
		if nodeStatus, _ := skyhook.GetNode(n); nodeStatus == v1alpha1.StatusComplete {
			delete(skyhook.GetSkyhook().Status.NodePriority, n)
			skyhook.GetSkyhook().Updated = true
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

func (np *NodePicker) SelectNodes(s SkyhookNodes) []wrapper.SkyhookNode {

	np.primeAndPruneNodes(s)

	// Straight from skyhook_controller CreatePodForPackage
	tolerations := append([]corev1.Toleration{ // tolerate all cordon
		{
			Key:      TaintUnschedulable,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, s.GetSkyhook().Spec.AdditionalTolerations...)

	if s.GetSkyhook().Spec.RuntimeRequired {
		tolerations = append(tolerations, np.runtimeRequiredToleration)
	}

	// Check if this skyhook uses deployment policies with compartments
	compartments := s.GetCompartments()
	if len(compartments) > 0 {
		return np.selectNodesWithCompartments(s, compartments, tolerations)
	}

	// Fallback to original logic for skyhooks without deployment policies
	return np.selectNodesLegacy(s, tolerations)
}

// selectNodesWithCompartments selects nodes using compartment-based batch processing
func (np *NodePicker) selectNodesWithCompartments(s SkyhookNodes, compartments map[string]*wrapper.Compartment, tolerations []corev1.Toleration) []wrapper.SkyhookNode {
	selectedNodes := make([]wrapper.SkyhookNode, 0)
	nodesWithTaintTolerationIssue := make([]string, 0)

	// Process each compartment according to its strategy
	for _, compartment := range compartments {
		batchNodes := compartment.GetNodesForNextBatch()

		for _, node := range batchNodes {
			// Check taint toleration
			if CheckTaintToleration(tolerations, node.GetNode().Spec.Taints) {
				selectedNodes = append(selectedNodes, node)
				np.upsertPick(node.GetNode().GetName(), s.GetSkyhook())
			} else {
				nodesWithTaintTolerationIssue = append(nodesWithTaintTolerationIssue, node.GetNode().Name)
				node.SetStatus(v1alpha1.StatusBlocked)
			}
		}
	}

	// Add condition about taint toleration issues
	np.updateTaintToleranceCondition(s, nodesWithTaintTolerationIssue)

	return selectedNodes
}

// PersistCompartmentBatchStates saves the current batch state for all compartments to the Skyhook status
func PersistCompartmentBatchStates(skyhook SkyhookNodes) bool {
	compartments := skyhook.GetCompartments()
	if len(compartments) == 0 {
		return false // No compartments, nothing to persist
	}

	// Initialize the batch states map if needed
	if skyhook.GetSkyhook().Status.CompartmentBatchStates == nil {
		skyhook.GetSkyhook().Status.CompartmentBatchStates = make(map[string]v1alpha1.BatchProcessingState)
	}

	changed := false
	for _, compartment := range compartments {
		// Always persist batch state to maintain cumulative counters
		batchState := compartment.GetBatchState()
		// Only persist if there's meaningful state (batch has started or there are nodes)
		if batchState.CurrentBatch > 0 || len(compartment.GetNodes()) > 0 {
			skyhook.GetSkyhook().Status.CompartmentBatchStates[compartment.GetName()] = batchState
			changed = true
		}
	}

	if changed {
		skyhook.GetSkyhook().Updated = true
	}

	return changed
}

// selectNodesLegacy implements the original node selection logic for backward compatibility
func (np *NodePicker) selectNodesLegacy(s SkyhookNodes, tolerations []corev1.Toleration) []wrapper.SkyhookNode {
	nodes := make([]wrapper.SkyhookNode, 0)

	var nodeCount int
	if s.GetSkyhook().Spec.InterruptionBudget.Percent != nil {
		limit := float64(*s.GetSkyhook().Spec.InterruptionBudget.Percent) / 100
		nodeCount = max(1, int(float64(s.NodeCount())*limit))
	}
	if s.GetSkyhook().Spec.InterruptionBudget.Count != nil {
		nodeCount = max(1, min(s.NodeCount(), *s.GetSkyhook().Spec.InterruptionBudget.Count))
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

	priority := []v1alpha1.Status{v1alpha1.StatusInProgress, v1alpha1.StatusUnknown, v1alpha1.StatusBlocked, v1alpha1.StatusErroring}

	nodesWithTaintTolerationIssue := make([]string, 0)
	// look for progress first
	for _, order := range priority {
		for _, node := range s.GetNodes() {

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
			np.upsertPick(node.GetNode().GetName(), s.GetSkyhook()) // track pick
		} else {
			nodesWithTaintTolerationIssue = append(nodesWithTaintTolerationIssue, node.GetNode().Name)

			// Set status here (not in IntrospectNode) to avoid recalculating
			// taints/tolerations on each introspection
			node.SetStatus(v1alpha1.StatusBlocked)
		}
	}

	// if we have nodes that are not tolerable, we need to add a condition to the skyhook
	np.updateTaintToleranceCondition(s, nodesWithTaintTolerationIssue)

	return final_nodes
}

// updateTaintToleranceCondition updates the taint tolerance condition on the skyhook
func (np *NodePicker) updateTaintToleranceCondition(s SkyhookNodes, nodesWithTaintTolerationIssue []string) {
	if len(nodesWithTaintTolerationIssue) > 0 {
		s.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/TaintNotTolerable", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionTrue,
			Reason:             "TaintNotTolerable",
			Message:            fmt.Sprintf("Node [%s] has taints that are not tolerable. Skipping.", strings.Join(nodesWithTaintTolerationIssue, ", ")),
			LastTransitionTime: metav1.Now(),
		})
	} else {
		s.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/TaintNotTolerable", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionFalse,
			Reason:             "TaintNotTolerable",
			Message:            "All nodes have tolerable taints.",
			LastTransitionTime: metav1.Now(),
		})
	}
}

// for node/package source of true, its on the node (we true to reflect this on the skyhook status)
// for SCR true, we need to look at all nodes and compare state to current SCR. This should be reflected in the SCR too.

// IntrospectSkyhook checks the current state of nodes, and SCR if they are in a bad mix, update to be correct
func IntrospectSkyhook(skyhook SkyhookNodes, allSkyhooks []SkyhookNodes) bool {
	change := false

	scrStatus := skyhook.Status()
	collectNodeStatus := skyhook.CollectNodeStatus()

	// override the node status if the skyhook is in a skyhook controlled state. (e.g. disabled, paused, waiting)
	if collectNodeStatus != v1alpha1.StatusComplete {
		switch {
		case skyhook.IsDisabled():
			collectNodeStatus = v1alpha1.StatusDisabled

		case skyhook.IsPaused():
			collectNodeStatus = v1alpha1.StatusPaused

		default:
			if nextSkyhook := GetNextSkyhook(allSkyhooks); nextSkyhook != nil && nextSkyhook != skyhook {
				collectNodeStatus = v1alpha1.StatusWaiting
			}
		}
	}

	if scrStatus != collectNodeStatus {
		skyhook.SetStatus(collectNodeStatus)
	}

	for _, node := range skyhook.GetNodes() {
		if IntrospectNode(node, skyhook) {
			change = true
		}
	}

	// Evaluate completed batches for compartments with deployment policies
	if evaluateCompletedBatches(skyhook) {
		change = true
	}

	skyhook.UpdateCondition()
	if skyhook.GetSkyhook().Updated {
		change = true
	}
	return change
}

// evaluateCompletedBatches checks if any compartment batches are complete and evaluates them
func evaluateCompletedBatches(skyhook SkyhookNodes) bool {
	compartments := skyhook.GetCompartments()
	if len(compartments) == 0 {
		return false // No compartments to evaluate
	}

	changed := false
	for _, compartment := range compartments {
		if isComplete, successCount, failureCount := compartment.EvaluateCurrentBatch(); isComplete {
			batchSize := successCount + failureCount

			// Update the compartment's batch state using strategy logic
			compartment.EvaluateAndUpdateBatchState(batchSize, successCount, failureCount)

			// Persist the updated batch state to the skyhook status
			if skyhook.GetSkyhook().Status.CompartmentBatchStates == nil {
				skyhook.GetSkyhook().Status.CompartmentBatchStates = make(map[string]v1alpha1.BatchProcessingState)
			}
			skyhook.GetSkyhook().Status.CompartmentBatchStates[compartment.GetName()] = compartment.GetBatchState()
			skyhook.GetSkyhook().Updated = true
			changed = true
		}
	}

	return changed
}

func IntrospectNode(node wrapper.SkyhookNode, skyhook SkyhookNodes) bool {
	skyhookStatus := skyhook.Status()

	nodeStatus := node.Status()
	node.SetStatus(nodeStatus)

	// Check if skyhook status should override node status
	if isSkyhookControlledNodeStatus(skyhookStatus) {
		if nodeStatus != skyhookStatus {
			node.SetStatus(skyhookStatus)
		}
		return node.Changed()
	}

	// need to move node out of Skyhook controlled status
	if isSkyhookControlledNodeStatus(nodeStatus) {
		if node.IsComplete() {
			node.SetStatus(v1alpha1.StatusComplete)
		} else {
			// node will update to it's correct status on next reconcile
			node.SetStatus(v1alpha1.StatusUnknown)
		}
		return node.Changed()
	}

	// For normal node state transitions
	if nodeStatus != v1alpha1.StatusComplete && node.IsComplete() {
		node.SetStatus(v1alpha1.StatusComplete)
	}

	if nodeStatus == v1alpha1.StatusComplete && !node.IsComplete() {
		node.SetStatus(v1alpha1.StatusUnknown)
	}

	return node.Changed()
}

func isSkyhookControlledNodeStatus(status v1alpha1.Status) bool {
	return status == v1alpha1.StatusDisabled ||
		status == v1alpha1.StatusPaused ||
		status == v1alpha1.StatusWaiting
}

func UpdateSkyhookPauseStatus(skyhook SkyhookNodes) bool {
	changed := false
	if skyhook.IsPaused() && skyhook.Status() != v1alpha1.StatusPaused {
		skyhook.SetStatus(v1alpha1.StatusPaused)

		for _, node := range skyhook.GetNodes() {
			node.SetStatus(v1alpha1.StatusPaused)
		}

		changed = true
	}

	return changed
}

// ReportState collects the current state of the skyhook and reports it to the skyhook status for printer columns
func (skyhook *skyhookNodes) ReportState() {
	CleanupRemovedNodes(skyhook)

	nodeCount := len(skyhook.nodes)
	skyhookName := skyhook.GetSkyhook().Name

	// Initialize status and metrics maps
	nodeStatusCounts := make(map[v1alpha1.Status]int, len(v1alpha1.Statuses))
	for _, status := range v1alpha1.Statuses {
		nodeStatusCounts[status] = 0
	}

	packageRestarts := make(map[string]map[string]int32)
	packageStateStageCounts := make(map[string]map[string]map[v1alpha1.State]map[v1alpha1.Stage]int)

	// Collect node and package stats
	for _, node := range skyhook.nodes {
		nodeStatusCounts[node.Status()]++

		for _, _package := range node.GetSkyhook().Spec.Packages {
			packageStatus, found := node.PackageStatus(_package.GetUniqueName())
			if !found {
				continue
			}

			// Nested map initialization
			if packageStateStageCounts[_package.Name] == nil {
				packageStateStageCounts[_package.Name] = make(map[string]map[v1alpha1.State]map[v1alpha1.Stage]int)
			}
			if packageStateStageCounts[_package.Name][_package.Version] == nil {
				packageStateStageCounts[_package.Name][_package.Version] = make(map[v1alpha1.State]map[v1alpha1.Stage]int)
			}
			if packageStateStageCounts[_package.Name][_package.Version][packageStatus.State] == nil {
				packageStateStageCounts[_package.Name][_package.Version][packageStatus.State] = make(map[v1alpha1.Stage]int)
			}
			packageStateStageCounts[_package.Name][_package.Version][packageStatus.State][packageStatus.Stage]++

			if packageRestarts[_package.Name] == nil {
				packageRestarts[_package.Name] = make(map[string]int32)
			}
			packageRestarts[_package.Name][_package.Version] += packageStatus.Restarts
		}
	}

	// reset metrics to zero
	ResetSkyhookMetricsToZero(skyhook)

	// Set skyhook status metrics
	SetSkyhookStatusMetrics(skyhookName, skyhook.Status(), true)

	// Set target count and node status metrics
	SetNodeTargetCountMetrics(skyhookName, float64(nodeCount))
	for status, count := range nodeStatusCounts {
		SetNodeStatusMetrics(skyhookName, status, float64(count))
	}

	// Set package state and stage metrics
	for _package, versions := range packageStateStageCounts {
		for version, states := range versions {
			for state, stages := range states {
				for stage, count := range stages {
					SetPackageStateMetrics(skyhookName, _package, version, state, float64(count))
					SetPackageStageMetrics(skyhookName, _package, version, stage, float64(count))
				}
			}
		}
	}

	// Set package restarts metrics
	for _package, versions := range packageRestarts {
		for version, restarts := range versions {
			SetPackageRestartsMetrics(skyhookName, _package, version, restarts)
		}
	}

	// Set current count of completed nodes
	completeNodes := fmt.Sprintf("%d/%d", nodeStatusCounts[v1alpha1.StatusComplete], nodeCount)
	if completeNodes != skyhook.skyhook.GetCompleteNodes() {
		skyhook.skyhook.SetCompleteNodes(completeNodes)
		skyhook.skyhook.Updated = true
	}

	// Update nodes in progress count if changed
	inProgress := nodeStatusCounts[v1alpha1.StatusInProgress] + nodeStatusCounts[v1alpha1.StatusErroring]
	if skyhook.skyhook.GetNodesInProgress() != inProgress {
		skyhook.skyhook.SetNodesInProgress(inProgress)
	}

	// Get and set sorted package list
	packageNames := make([]string, 0, len(skyhook.skyhook.Spec.Packages))
	for _, _package := range skyhook.skyhook.Spec.Packages {
		packageNames = append(packageNames, fmt.Sprintf("%s:%s", _package.Name, _package.Version))
	}
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

func (skyhook *skyhookNodes) GetCompartments() map[string]*wrapper.Compartment {
	return skyhook.compartments
}

func (skyhook *skyhookNodes) AddCompartment(name string, compartment *wrapper.Compartment) {
	skyhook.compartments[name] = compartment
}

func (skyhook *skyhookNodes) AddCompartmentNode(name string, node wrapper.SkyhookNode) {
	skyhook.compartments[name].AddNode(node)
}

// cleanupNodeMap removes nodes from the given map that no longer exist in currentNodes
// Returns false if nodeMap is nil, otherwise returns true if any nodes were removed
func cleanupNodeMap[T any](nodeMap map[string]T, currentNodes map[string]struct{}) bool {
	if nodeMap == nil {
		return false
	}

	change := false
	for nodeName := range nodeMap {
		if _, ok := currentNodes[nodeName]; !ok {
			delete(nodeMap, nodeName)
			change = true
		}
	}
	return change
}

// CleanupRemovedNodes removes nodes from the Skyhook status that no longer exist in the cluster
// or no longer match the node selector. This ensures that only nodes that exist in the cluster
// are tracked in the status section of the Custom Resource.
func CleanupRemovedNodes(skyhook SkyhookNodes) {
	// Get all current node names from the cluster using struct{} for O(1) lookup
	currentNodeNames := make(map[string]struct{})
	for _, node := range skyhook.GetNodes() {
		currentNodeNames[node.GetNode().Name] = struct{}{}
	}

	status := skyhook.GetSkyhook().Status

	// Check and remove nodes from all status maps
	change := cleanupNodeMap(status.NodeState, currentNodeNames)
	change = cleanupNodeMap(status.NodeStatus, currentNodeNames) || change
	change = cleanupNodeMap(status.NodeBootIds, currentNodeNames) || change
	change = cleanupNodeMap(status.NodePriority, currentNodeNames) || change

	// Only set Updated flag if there were changes
	if change {
		skyhook.GetSkyhook().Updated = true
	}
}
