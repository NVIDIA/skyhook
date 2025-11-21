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

		// Setup compartments for this skyhook
		if err := setupCompartments(ret.skyhooks[idx], &skyhook, deploymentPolicies); err != nil {
			return nil, err
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

// setupCompartments initializes compartments for a skyhook. Always ensures a default compartment exists.
// The default compartment is created from:
// 1. DeploymentPolicy if specified and found
// 2. Legacy InterruptionBudget if no DeploymentPolicy is specified, or if DeploymentPolicy is specified but not found
// 3. Safe fallback defaults if neither DeploymentPolicy nor InterruptionBudget is available
func setupCompartments(skyhookNodes SkyhookNodes, skyhook *v1alpha1.Skyhook, deploymentPolicies *v1alpha1.DeploymentPolicyList) error {
	var defaultCompartment *v1alpha1.Compartment
	var defaultBatchState *v1alpha1.BatchProcessingState

	// Load persisted batch state for default compartment if it exists
	if skyhook.Status.CompartmentStatuses != nil {
		if status, exists := skyhook.Status.CompartmentStatuses[v1alpha1.DefaultCompartmentName]; exists && status.BatchState != nil {
			defaultBatchState = status.BatchState
		}
	}

	// Try to find and apply deployment policy if specified
	if skyhook.Spec.DeploymentPolicy != "" {
		policyFound := false
		for _, deploymentPolicy := range deploymentPolicies.Items {
			if deploymentPolicy.Name == skyhook.Spec.DeploymentPolicy {
				policyFound = true

				// Check for orphaned batch state from renamed compartments
				checkForOrphanedBatchState(skyhookNodes, skyhook, &deploymentPolicy)

				// Add all compartments from the policy
				for _, compartment := range deploymentPolicy.Spec.Compartments {
					var batchState *v1alpha1.BatchProcessingState
					if skyhook.Status.CompartmentStatuses != nil {
						if status, exists := skyhook.Status.CompartmentStatuses[compartment.Name]; exists && status.BatchState != nil {
							batchState = status.BatchState
						}
					}
					skyhookNodes.AddCompartment(compartment.Name, wrapper.NewCompartmentWrapper(&compartment, batchState))
				}
				// Use policy's default compartment specification
				defaultCompartment = &v1alpha1.Compartment{
					Name:     v1alpha1.DefaultCompartmentName,
					Budget:   deploymentPolicy.Spec.Default.Budget,
					Strategy: deploymentPolicy.Spec.Default.Strategy,
				}
				// Clear condition if policy is found
				skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
					Type:               fmt.Sprintf("%s/DeploymentPolicyNotFound", v1alpha1.METADATA_PREFIX),
					Status:             metav1.ConditionFalse,
					Reason:             "DeploymentPolicyFound",
					Message:            fmt.Sprintf("DeploymentPolicy %q found and applied", skyhook.Spec.DeploymentPolicy),
					LastTransitionTime: metav1.Now(),
					ObservedGeneration: skyhook.Generation,
				})
				break
			}
		}

		// If policy not found, fall back to legacy InterruptionBudget-based compartment
		if !policyFound {
			// Add condition to warn user
			skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
				Type:               fmt.Sprintf("%s/DeploymentPolicyNotFound", v1alpha1.METADATA_PREFIX),
				Status:             metav1.ConditionTrue,
				Reason:             "DeploymentPolicyNotFound",
				Message:            fmt.Sprintf("DeploymentPolicy %q not found, falling back to InterruptionBudget", skyhook.Spec.DeploymentPolicy),
				LastTransitionTime: metav1.Now(),
				ObservedGeneration: skyhook.Generation,
			})
			nodeCount := len(skyhookNodes.GetNodes())
			defaultCompartment = createLegacyDefaultCompartment(skyhook.Spec, nodeCount)
		}
	} else {
		// No deployment policy specified - use legacy InterruptionBudget-based compartment
		// Clear condition if it exists (user intentionally not using deployment policy)
		skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/DeploymentPolicyNotFound", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionFalse,
			Reason:             "NoDeploymentPolicySpecified",
			Message:            "No DeploymentPolicy specified, using InterruptionBudget",
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: skyhook.Generation,
		})
		nodeCount := len(skyhookNodes.GetNodes())
		defaultCompartment = createLegacyDefaultCompartment(skyhook.Spec, nodeCount)
	}

	// Always add the default compartment
	skyhookNodes.AddCompartment(v1alpha1.DefaultCompartmentName, wrapper.NewCompartmentWrapper(defaultCompartment, defaultBatchState))

	// For legacy mode (no deployment policy), assign all nodes to default compartment
	// Node assignment happens here for legacy mode, and later in partitionNodesIntoCompartments for policy mode
	if skyhook.Spec.DeploymentPolicy == "" {
		for _, node := range skyhookNodes.GetNodes() {
			skyhookNodes.AddCompartmentNode(v1alpha1.DefaultCompartmentName, node)
		}
	}
	// Note: If using deployment policy, nodes are NOT assigned here
	// They will be assigned in partitionNodesIntoCompartments

	return nil
}

// checkForOrphanedBatchState detects when compartment names have changed (renaming)
// and warns the user that batch state may be lost
func checkForOrphanedBatchState(skyhookNodes SkyhookNodes, skyhook *v1alpha1.Skyhook, policy *v1alpha1.DeploymentPolicy) {
	if len(skyhook.Status.CompartmentStatuses) == 0 {
		return // No previous state to check
	}

	// Build set of current compartment names from policy
	currentCompartments := make(map[string]bool)
	for _, compartment := range policy.Spec.Compartments {
		currentCompartments[compartment.Name] = true
	}
	currentCompartments[v1alpha1.DefaultCompartmentName] = true // Always exists

	// Find orphaned compartment statuses (in status but not in current policy)
	orphanedNames := make([]string, 0)
	orphanedWithBatchState := make([]string, 0)

	for name, status := range skyhook.Status.CompartmentStatuses {
		if !currentCompartments[name] {
			orphanedNames = append(orphanedNames, name)
			// Check if this orphaned compartment has active batch state that will be lost
			if status.BatchState != nil &&
				(status.BatchState.CurrentBatch > 1 || status.InProgress > 0 || status.BatchState.CompletedNodes > 0) {
				orphanedWithBatchState = append(orphanedWithBatchState, name)
			}
		}
	}

	// Warn if compartments with active rollout state are missing
	if len(orphanedWithBatchState) > 0 {
		skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/CompartmentBatchStateLost", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionTrue,
			Reason:             "CompartmentRenamed",
			Message:            fmt.Sprintf("Compartments %v from previous policy not found in current policy. Batch state will be lost. This typically happens when compartments are renamed.", orphanedWithBatchState),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: skyhook.Generation,
		})
	} else if len(orphanedNames) > 0 {
		// Info-level: compartments removed but no active state lost
		skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/CompartmentBatchStateLost", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionFalse,
			Reason:             "CompartmentsRemoved",
			Message:            fmt.Sprintf("Compartments %v from previous policy removed (no active rollout state lost)", orphanedNames),
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: skyhook.Generation,
		})
	} else {
		// Clear condition if no issues
		skyhookNodes.GetSkyhook().AddCondition(metav1.Condition{
			Type:               fmt.Sprintf("%s/CompartmentBatchStateLost", v1alpha1.METADATA_PREFIX),
			Status:             metav1.ConditionFalse,
			Reason:             "NoOrphanedState",
			Message:            "All compartments match between policy and status",
			LastTransitionTime: metav1.Now(),
			ObservedGeneration: skyhook.Generation,
		})
	}
}

// createLegacyDefaultCompartment creates a synthetic default compartment for backwards compatibility.
// It translates the legacy InterruptionBudget into a FixedStrategy compartment that behaves the same way.
// Used when no DeploymentPolicy is specified, or when DeploymentPolicy is specified but not found.
func createLegacyDefaultCompartment(spec v1alpha1.SkyhookSpec, nodeCount int) *v1alpha1.Compartment {
	// Create a synthetic budget from InterruptionBudget
	// If InterruptionBudget is not set, default to 100% (all nodes at once)
	var budget v1alpha1.DeploymentBudget
	if spec.InterruptionBudget.Percent != nil {
		budget.Percent = spec.InterruptionBudget.Percent
	} else if spec.InterruptionBudget.Count != nil {
		budget.Count = spec.InterruptionBudget.Count
	} else {
		// Default to 100% for backwards compatibility (process all nodes at once)
		budget.Percent = ptr(100)
	}

	// Calculate the ceiling to maintain backwards-compatible batch size
	// This ensures the FixedStrategy processes the same number of nodes per batch
	// as the legacy InterruptionBudget behavior
	var initialBatch int
	if budget.Count != nil {
		// Count budget: use the count directly
		initialBatch = max(1, min(nodeCount, *budget.Count))
	} else if budget.Percent != nil {
		// Percent budget: calculate based on total nodes
		if nodeCount > 0 {
			limit := float64(*budget.Percent) / 100
			initialBatch = max(1, int(float64(nodeCount)*limit))
		} else {
			initialBatch = 1
		}
	} else {
		initialBatch = 1
	}

	// Create a FixedStrategy with InitialBatch matching the legacy ceiling behavior
	fixedStrategy := &v1alpha1.FixedStrategy{}
	fixedStrategy.Default()
	fixedStrategy.InitialBatch = &initialBatch

	return &v1alpha1.Compartment{
		Name:   v1alpha1.DefaultCompartmentName,
		Budget: budget,
		Strategy: &v1alpha1.DeploymentStrategy{
			Fixed: fixedStrategy,
		},
	}
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
	AssignNodeToCompartment(node wrapper.SkyhookNode) (string, error)
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

	// All skyhooks now use compartments (with a default 100% compartment if none specified)
	compartments := s.GetCompartments()
	return np.selectNodesWithCompartments(s, compartments, tolerations)
}

// selectNodesWithCompartments selects nodes using compartment-based batch processing
func (np *NodePicker) selectNodesWithCompartments(s SkyhookNodes, compartments map[string]*wrapper.Compartment, tolerations []corev1.Toleration) []wrapper.SkyhookNode {
	selectedNodes := make([]wrapper.SkyhookNode, 0)
	nodesWithTaintTolerationIssue := make([]string, 0)

	// First, check ALL nodes for taint issues to set the condition correctly
	// This ensures the condition reflects the true state even when no batch is being processed
	for _, compartment := range compartments {
		for _, node := range compartment.GetNodes() {
			if !CheckTaintToleration(tolerations, node.GetNode().Spec.Taints) {
				nodesWithTaintTolerationIssue = append(nodesWithTaintTolerationIssue, node.GetNode().Name)
			}
		}
	}

	// Process each compartment according to its strategy
	for _, compartment := range compartments {
		batchNodes := compartment.GetNodesForNextBatch()

		for _, node := range batchNodes {
			// Check taint toleration
			if CheckTaintToleration(tolerations, node.GetNode().Spec.Taints) {
				selectedNodes = append(selectedNodes, node)
				np.upsertPick(node.GetNode().GetName(), s.GetSkyhook())
			} else {
				node.SetStatus(v1alpha1.StatusBlocked)
			}
		}
	}

	// Add condition about taint toleration issues
	np.updateTaintToleranceCondition(s, nodesWithTaintTolerationIssue)

	return selectedNodes
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

			// Skip evaluation if batch size is zero (no nodes actually processed)
			// This can happen when nodes transition to Blocked or other non-terminal states
			if batchSize == 0 {
				continue
			}

			// Update the compartment's batch state using strategy logic
			compartment.EvaluateAndUpdateBatchState(batchSize, successCount, failureCount)

			// Persist the updated batch state to the skyhook status immediately
			if skyhook.GetSkyhook().Status.CompartmentStatuses == nil {
				skyhook.GetSkyhook().Status.CompartmentStatuses = make(map[string]v1alpha1.CompartmentStatus)
			}
			// Build and persist the compartment status with the updated batch state
			newStatus := buildCompartmentStatus(compartment)
			skyhook.GetSkyhook().Status.CompartmentStatuses[compartment.GetName()] = newStatus

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

// compartmentStatusEqual compares two CompartmentStatus for equality
func compartmentStatusEqual(a, b v1alpha1.CompartmentStatus) bool {
	if a.Matched != b.Matched || a.Ceiling != b.Ceiling || a.InProgress != b.InProgress ||
		a.Completed != b.Completed || a.ProgressPercent != b.ProgressPercent {
		return false
	}

	// Compare BatchState if present
	if (a.BatchState == nil) != (b.BatchState == nil) {
		return false
	}
	if a.BatchState != nil && b.BatchState != nil {
		return *a.BatchState == *b.BatchState
	}
	return true
}

// buildCompartmentStatus creates a CompartmentStatus for a given compartment
func buildCompartmentStatus(compartment *wrapper.Compartment) v1alpha1.CompartmentStatus {
	matched := len(compartment.GetNodes())
	ceiling := wrapper.CalculateCeiling(compartment.Budget, matched)

	// Count inProgress and completed nodes
	inProgress := 0
	completed := 0
	for _, node := range compartment.GetNodes() {
		if node.Status() == v1alpha1.StatusInProgress {
			inProgress++
		}
		if node.IsComplete() {
			completed++
		}
	}

	// Calculate progress percentage
	progressPercent := 0
	if matched > 0 {
		progressPercent = (completed * 100) / matched
	}

	// Get batch state
	batchState := compartment.GetBatchState()

	// Copy batch state for status
	var batchStateCopy *v1alpha1.BatchProcessingState
	if compartment.Strategy != nil {
		batchStateCopy = &v1alpha1.BatchProcessingState{
			CurrentBatch:        batchState.CurrentBatch,
			ConsecutiveFailures: batchState.ConsecutiveFailures,
			CompletedNodes:      batchState.CompletedNodes,
			FailedNodes:         batchState.FailedNodes,
			ShouldStop:          batchState.ShouldStop,
			LastBatchSize:       batchState.LastBatchSize,
			LastBatchFailed:     batchState.LastBatchFailed,
		}
	}

	return v1alpha1.CompartmentStatus{
		Matched:         matched,
		Ceiling:         ceiling,
		InProgress:      inProgress,
		Completed:       completed,
		ProgressPercent: progressPercent,
		BatchState:      batchStateCopy,
	}
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

	// Update compartment statuses if compartments exist
	if len(skyhook.compartments) > 0 {
		if skyhook.skyhook.Status.CompartmentStatuses == nil {
			skyhook.skyhook.Status.CompartmentStatuses = make(map[string]v1alpha1.CompartmentStatus)
		}

		// Track which compartments are currently active
		activeCompartments := make(map[string]bool)
		for name, compartment := range skyhook.compartments {
			activeCompartments[name] = true
			newStatus := buildCompartmentStatus(compartment)
			if existing, ok := skyhook.skyhook.Status.CompartmentStatuses[name]; !ok || !compartmentStatusEqual(existing, newStatus) {
				skyhook.skyhook.Status.CompartmentStatuses[name] = newStatus
				skyhook.skyhook.Updated = true
			}
		}

		// Remove statuses for compartments that no longer exist
		for name := range skyhook.skyhook.Status.CompartmentStatuses {
			if !activeCompartments[name] {
				delete(skyhook.skyhook.Status.CompartmentStatuses, name)
				skyhook.skyhook.Updated = true
			}
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

	// Set rollout metrics for each compartment (follows same pattern as other metrics)
	if len(skyhook.compartments) > 0 {
		policyName := skyhook.GetSkyhook().Spec.DeploymentPolicy
		if policyName == "" {
			policyName = LegacyPolicyName
		}

		for name, compartment := range skyhook.compartments {
			if status, ok := skyhook.skyhook.Status.CompartmentStatuses[name]; ok {
				strategy := getStrategyType(compartment)
				SetRolloutMetrics(skyhookName, policyName, name, strategy, status)
			}
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

// compartmentMatch represents a compartment that matches a node
type compartmentMatch struct {
	name         string
	strategyType v1alpha1.StrategyType
	capacity     int
}

// countMatchingNodes counts how many nodes from allNodes match the given selector
func (skyhook *skyhookNodes) countMatchingNodes(selector metav1.LabelSelector) (int, error) {
	labelSelector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, node := range skyhook.nodes {
		if labelSelector.Matches(labels.Set(node.GetNode().Labels)) {
			count++
		}
	}
	return count, nil
}

// AssignNodeToCompartment assigns a single node to the appropriate compartment using overlap resolution.
// When a node matches multiple compartments, it resolves using:
// 1. Strategy safety order: Fixed is safer than Linear, which is safer than Exponential
// 2. Tie-break on same strategy: Choose compartment with smaller effective ceiling (window)
// 3. Final tie-break: Lexicographically by compartment name for determinism
// Assignments are recalculated fresh on every reconcile based on current cluster state.
func (skyhook *skyhookNodes) AssignNodeToCompartment(node wrapper.SkyhookNode) (string, error) {
	nodeLabels := labels.Set(node.GetNode().Labels)

	matches := []compartmentMatch{}

	// Collect all matching compartments (excluding default)
	for _, compartment := range skyhook.compartments {
		// Skip the default compartment - it's a fallback
		if compartment.Name == v1alpha1.DefaultCompartmentName {
			continue
		}

		selector, err := metav1.LabelSelectorAsSelector(&compartment.Selector)
		if err != nil {
			return "", fmt.Errorf("invalid selector for compartment %s: %w", compartment.Name, err)
		}

		if selector.Matches(nodeLabels) {
			// Count how many nodes in total match this compartment's selector
			matchedCount, err := skyhook.countMatchingNodes(compartment.Selector)
			if err != nil {
				return "", fmt.Errorf("error counting matching nodes for compartment %s: %w", compartment.Name, err)
			}

			// Ensure at least 1 node for capacity calculation
			if matchedCount == 0 {
				matchedCount = 1
			}

			stratType := wrapper.GetStrategyType(compartment.Strategy)
			capacity := wrapper.CalculateCeiling(compartment.Budget, matchedCount)

			matches = append(matches, compartmentMatch{
				name:         compartment.Name,
				strategyType: stratType,
				capacity:     capacity,
			})
		}
	}

	// No matches - assign to default
	if len(matches) == 0 {
		return v1alpha1.DefaultCompartmentName, nil
	}

	// Single match - return it
	if len(matches) == 1 {
		return matches[0].name, nil
	}

	// Multiple matches - apply overlap resolution
	// Sort matches using the safety heuristic
	sort.Slice(matches, func(i, j int) bool {
		// 1. Strategy safety order: Fixed > Linear > Exponential
		if matches[i].strategyType != matches[j].strategyType {
			return wrapper.StrategyIsSafer(matches[i].strategyType, matches[j].strategyType)
		}

		// 2. Tie-break on same strategy: smaller window (capacity)
		if matches[i].capacity != matches[j].capacity {
			return matches[i].capacity < matches[j].capacity
		}

		// 3. Final tie-break: lexicographically by name for determinism
		return matches[i].name < matches[j].name
	})

	// Return the safest compartment
	return matches[0].name, nil
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
