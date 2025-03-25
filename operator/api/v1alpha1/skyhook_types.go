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

package v1alpha1

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NVIDIA/skyhook/internal/graph"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SkyhookSpec defines the desired state of Skyhook
type SkyhookSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Serial tells skyhook if it allowed to run in parallel or not when applying packages
	//+kubebuilder:default=false
	Serial bool `json:"serial,omitempty"`

	// Pause halt the operator from proceeding. THIS is for admin use to stop skyhook if there is an issue or
	// concert without needing to delete to ad in discovery of the issue.
	//+kubebuilder:default=false
	Pause bool `json:"pause,omitempty"`

	// PodNonInterruptLabels are a set of labels we want to monitor pods for whether they Interruptible
	PodNonInterruptLabels metav1.LabelSelector `json:"podNonInterruptLabels,omitempty"`

	// NodeSelector are a set of labels we want to monitor nodes for applying packages too
	NodeSelector metav1.LabelSelector `json:"nodeSelectors,omitempty"`

	// InterruptionBudget configures how many nodes that match node selectors that allowed to be interrupted at once.
	InterruptionBudget InterruptionBudget `json:"interruptionBudget,omitempty"`

	// Packages are the DAG of packages to be applied to nodes.
	//+kubebuilder:validation:Required
	Packages Packages `json:"packages,omitempty"`

	// AdditionalTolerations adds tolerations to all packages
	AdditionalTolerations []corev1.Toleration `json:"additionalTolerations,omitempty"`

	// This skyhook is required to have been completed before any workloads can start
	//+kubebuilder:default=false
	RuntimeRequired bool `json:"runtimeRequired,omitempty"`
}

// BuildGraph turns packages in the a graph of dependencies
func (spec *SkyhookSpec) BuildGraph() (graph.DependencyGraph[*Package], error) {
	dependencyGraph := graph.New[*Package]()
	for _, _package := range spec.Packages {

		deps := make([]string, 0)
		for dep, ver := range _package.DependsOn {
			if ver == "" {
				return nil, fmt.Errorf("DependsOn version is empty for [%s]", dep)
			}
			deps = append(deps, fmt.Sprintf("%s|%s", dep, ver))
		}

		err := dependencyGraph.Add(_package.GetUniqueName(), &_package, deps...)
		if err != nil {
			return nil, fmt.Errorf("error building graph from packages: %w", err)
		}
	}

	// return dependencyGraph, nil
	return dependencyGraph, nil
}

// Packages are set of packages to apply
type Packages map[string]Package

// set the names on packages if not set
func (f Packages) Names() {
	for k := range f {
		m := f[k]
		if m.Name == "" {
			m.Name = k
			f[k] = m
		}
	}
}

// Remove the image tag if set on image
func (f Packages) Images() {
	for k := range f {
		m := f[k]
		image, _, found := strings.Cut(m.Image, ":")
		if found {
			m.Image = image
		}

		f[k] = m
	}
}

func (f *Packages) UnmarshalJSON(data []byte) error {

	var ret map[string]Package
	err := json.Unmarshal(data, &ret)
	if err != nil {
		return err
	}

	*f = Packages(ret)
	f.Images()
	f.Names()
	return nil
}

type InterruptionBudget struct {
	// Percent of nodes that match node selectors that allowed to be interrupted at once.
	// Percent and count are mutually exclusive settings
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=100
	//+nullable
	Percent *int `json:"percent,omitempty"`

	// Count is number of nodes that match node selectors that allowed to be interrupted at once.
	// Percent and count are mutually exclusive settings
	//+kubebuilder:validation:Minimum=0
	//+nullable
	Count *int `json:"count,omitempty"`
}

func (i *InterruptionBudget) Validate() error {
	if i.Count != nil && i.Percent != nil {
		return errors.New("error InterruptionBudget is not valid, both percent and count can not be set at the same time")
	}
	return nil
}

type PackageRef struct {
	// Name of the package. Do not set unless you know what your doing. Comes from map key.
	//+optional
	//+kubebuilder:validation:Pattern=`^[a-z][-a-z0-9]{0,41}[a-z]$`
	//+kubebuilder:validation:MaxLength=43
	Name string `json:"name"`
	// Version is the version of the package
	//+kubebuilder:validation:Required
	Version string `json:"version"`
}

func (p *PackageRef) GetUniqueName() string {
	return fmt.Sprintf("%s|%s", p.Name, p.Version)
}

type ResourceRequirements struct {
	// +kubebuilder:default="500m"
	CPURequest resource.Quantity `json:"cpuRequest,omitempty"`
	// +kubebuilder:default="500m"
	CPULimit resource.Quantity `json:"cpuLimit,omitempty"`
	// +kubebuilder:default="256Mi"
	MemoryRequest resource.Quantity `json:"memoryRequest,omitempty"`
	// +kubebuilder:default="256Mi"
	MemoryLimit resource.Quantity `json:"memoryLimit,omitempty"`
}

// Package is a container that contains the skyhook agent plus some work to do, plus any dependencies to be run first.
type Package struct {
	PackageRef `json:",inline"`

	// Image is the container image to run. Do not included the tag, that is set in the version.
	//+kubebuilder:example="alpine"
	//+kubebuilder:validation:Required
	Image string `json:"image"`

	// Agent Image Override is the container image to override at the package level. Full qualified image with tag.
	// This overrides the image provided via ENV to the operator.
	//+kubebuilder:example="alpine:3.21.0"
	AgentImageOverride string `json:"agentImageOverride,omitempty"`

	// Interrupt if supplied is the type of interrupt
	//+optional
	Interrupt *Interrupt `json:"interrupt,omitempty"`

	// DependsOn is a map of name:version of dependencies.
	// NOTE: we need to deal with version
	//+optional
	DependsOn map[string]string `json:"dependsOn,omitempty"`

	// ConfigInterrupts is a map for whether an interrupt is needed for a configmap key
	// +optional
	ConfigInterrupts map[string]Interrupt `json:"configInterrupts,omitempty"`

	// ConfigMap contains the configuration data.
	// Each key must consist of alphanumeric characters, '-', '_' or '.'.
	// Values must be UTF-8 byte sequences.
	// The keys stored in Data must not overlap with the keys in
	// the BinaryData field, this is enforced during validation process.
	// +optional
	ConfigMap map[string]string `json:"configMap,omitempty"`

	// Env are the environment variables for the package
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources lets you set the cpu and memory limits and requests for this package.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +kubebuilder:default={}
	Resources ResourceRequirements `json:"resources,omitempty"`
}

func (f *Package) HasInterrupt() bool {
	return f.Interrupt != nil
}

type InterruptType string
type Interrupt struct {
	// Type of interrupt. Reboot, Service, All Services, or Noop
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=service;reboot;noop;restartAllServices
	Type InterruptType `json:"type"`
	// List of systemd services to restart
	//+optional
	Services []string `json:"services,omitempty"`
}

// ToArgs base64 encoded json of self
func (i *Interrupt) ToArgs() (string, error) {

	// HACK: choosing to do it this way so the CRD interface is not tied to the agent
	clone := i.DeepCopy() // make copy as to not alter this

	switch clone.Type { // update type to match what the agent is expecting
	case REBOOT:
		clone.Type = InterruptType("node_restart")
	case SERVICE:
		clone.Type = InterruptType("service_restart")
	case NOOP:
		clone.Type = InterruptType("no_op")
	case RESTART_ALL_SERVICES:
		clone.Type = InterruptType("restart_all_services")
	}

	data, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

const (
	REBOOT               InterruptType = "reboot"
	SERVICE              InterruptType = "service"
	NOOP                 InterruptType = "noop"
	RESTART_ALL_SERVICES InterruptType = "restartAllServices"
)

// SkyhookStatus defines the observed state of Skyhook
type SkyhookStatus struct {

	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.observedGeneration is 9, then status is out of date
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// NodeState is the detailed state of each node
	NodeState map[string]NodeState `json:"nodeState,omitempty"`

	// NodeStatus tracks by node the status of the node
	NodeStatus map[string]Status `json:"nodeStatus,omitempty"`

	//Status is the roll of this instance of skyhook and all nodes status.
	//+kubebuilder:validation:Enum=unknown;complete;in_progress;erroring
	Status Status `json:"status,omitempty"`

	// Represents the observations of a skyhook's current state.
	// Known .status.conditions.type are: "Available", "Progressing", and "Degraded" // TODO
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NodeBootIds tracks the boot ids of nodes for triggering on reboot
	NodeBootIds map[string]string `json:"nodeBootIds,omitempty"`

	// NodePriority tracks what nodes we are working on. This is makes the interrupts budgets sticky.
	NodePriority map[string]metav1.Time `json:"nodePriority,omitempty"`

	// ConfigUpdates tracks config updates
	ConfigUpdates map[string][]string `json:"configUpdates,omitempty"`

	// +kubebuilder:example=3
	// +kubebuilder:default=0
	// NodesInProgress displays the number of nodes that are currently in progress and is
	// only used for printer columns.
	NodesInProgress int `json:"nodesInProgress,omitempty"`

	// +kubebuilder:example="3/5"
	// +kubebuilder:default="0/0"
	// CompleteNodes is a string that displays the amount of nodes that are complete
	// out of the total nodes the skyhook is being applied to and is only used for
	// a printer column.
	CompleteNodes string `json:"completeNodes,omitempty"`

	// +kubebuilder:example="dexter,spencer,foobar"
	// +kubebuilder:default=""
	// PackageList is a comma separated list of package names from the skyhook spec and
	// is only used for a printer column.
	PackageList string `json:"packageList,omitempty"`
}

type NodeState map[string]PackageStatus

// Adds or updates specified state for package in the node state
func (ns *NodeState) Upsert(_package PackageRef, image string, state State, stage Stage, restarts int32) bool {

	if *ns == nil {
		*ns = make(map[string]PackageStatus)
	}

	status := PackageStatus{
		Name:     _package.Name,
		Version:  _package.Version,
		State:    state,
		Image:    image,
		Stage:    stage,
		Restarts: restarts,
	}

	existing, ok := (*ns)[_package.GetUniqueName()]
	if ok && status.Equal(&existing) {
		return false
	}

	(*ns)[_package.GetUniqueName()] = status

	return true
}

// Removes specified package from Node State
func (ns *NodeState) RemoveState(_package PackageRef) bool {
	if *ns == nil {
		return false
	}

	_, ok := (*ns)[_package.GetUniqueName()]
	if ok {
		delete(*ns, _package.GetUniqueName())
		return true
	}

	return false
}

func (ns *NodeState) Get(name string) *PackageStatus {
	if s, ok := (*ns)[name]; ok {
		return &s
	}
	return nil
}

// IsComplete checks if the number of complete frames is equal to total packages,
// and that the set of packages contain the same packages
func (ns *NodeState) IsComplete(packages Packages, interrupt map[string][]*Interrupt, config map[string][]string) bool {
	if len(packages) <= len(ns.GetComplete(packages, interrupt, config)) { // is greater than because if we change packages in CSR
		// If there is still an uninstall package then the node isn't complete
		for _, packageStatus := range *ns {
			if packageStatus.Stage == StageUninstall {
				return false
			}
		}

		return ns.Contains(packages)
	}

	return false
}

// Determines whether package has an interrupt based off of config updates and interrupts passed to it
func (ns *NodeState) HasInterrupt(_package Package, interrupt map[string][]*Interrupt, config map[string][]string) bool {
	var hasInterrupt bool

	if len(config[_package.Name]) > 0 {
		hasInterrupt = len(interrupt[_package.Name]) > 0
	} else {
		hasInterrupt = _package.HasInterrupt()
	}

	return hasInterrupt
}

func (ns *NodeState) NextStage(_package *Package, interrupt map[string][]*Interrupt, config map[string][]string) *Stage {

	state, ok := (*ns)[_package.GetUniqueName()]
	if !ok || state.State != StateComplete {
		return nil
	}

	nextStage := map[Stage]Stage{
		StageUninstall: StageApply,
		StageApply:     StageConfig,
		StageUpgrade:   StageConfig,
	}

	hasInterrupt := (*ns).HasInterrupt(*_package, interrupt, config)
	if hasInterrupt {
		nextStage = map[Stage]Stage{
			StageUpgrade:   StageConfig,
			StageUninstall: StageApply,
			StageApply:     StageConfig,
			StageConfig:    StageInterrupt,
			StageInterrupt: StagePostInterrupt,
		}
	}

	if next, exists := nextStage[state.Stage]; exists {
		return &next
	}

	return nil
}

// Equal return true if node state contains the same packages as the spec including versions
func (ns *NodeState) Contains(packages Packages) bool {

	if len(*ns) < len(packages) { // same as above, ns can be longer, but not shorter
		return false
	}

	for _, v := range packages {
		v2, ok := (*ns)[v.GetUniqueName()]
		if !ok {
			return false
		}
		if v2.Version != v.Version {
			return false
		}
	}

	return true
}

func (left *NodeState) Equal(right *NodeState) bool {
	return reflect.DeepEqual(left, right)
}

// GetComplete returns a list of packages that are complete
func (ns *NodeState) GetComplete(packages Packages, interrupt map[string][]*Interrupt, config map[string][]string) []string {

	ret := make([]string, 0)

	for _, packageStatus := range *ns {
		_package, found := packages[packageStatus.Name]
		if found && _package.Version == packageStatus.Version && packageStatus.State == StateComplete {
			hasInterrupt := (*ns).HasInterrupt(_package, interrupt, config)

			if hasInterrupt && packageStatus.Stage == StagePostInterrupt {
				ret = append(ret, fmt.Sprintf("%s|%s", packageStatus.Name, packageStatus.Version))
			} else if !hasInterrupt && packageStatus.Stage == StageConfig {
				ret = append(ret, fmt.Sprintf("%s|%s", packageStatus.Name, packageStatus.Version))
			}
		}
	}

	sort.Strings(ret)

	return ret
}

// IsPackageComplete checks if a package is complete
func (ns *NodeState) IsPackageComplete(_package Package, interrupt map[string][]*Interrupt, config map[string][]string) bool {

	packageStatus, found := (*ns)[_package.GetUniqueName()]
	if found && _package.Version == packageStatus.Version && packageStatus.State == StateComplete {
		hasInterrupt := (*ns).HasInterrupt(_package, interrupt, config)

		if hasInterrupt && packageStatus.Stage == StagePostInterrupt {
			return true
		} else if !hasInterrupt && packageStatus.Stage == StageConfig {
			return true
		}
	}

	return false
}

// ProgressSkipped checks if a package is skipped and should be progressed to complete
func (ns *NodeState) ProgressSkipped(packages Packages, interrupt map[string][]*Interrupt, config map[string][]string) bool {
	ret := false
	for _, s := range *ns {
		f, ok := packages[s.Name]
		if !ok {
			continue
		}

		if (*ns).HasInterrupt(f, interrupt, config) && s.Stage == StageInterrupt && s.State == StateSkipped {
			s.State = StateComplete
			(*ns)[f.GetUniqueName()] = s
			ret = true
		}
	}
	return ret
}

type PackageStatus struct {
	// Name is the name of the package
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Version is the version of the package
	//+kubebuilder:validation:Required
	Version string `json:"version"`

	// Image for the package
	//+kubebuilder:validation:Required
	Image string `json:"image"`

	// Stage is where in the package install process is currently for a node.
	// these stages encapsulate checks. Both Apply and PostInterrupt also run checks,
	// these are all or nothing, meaning both need to be successful in order to transition
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=apply;interrupt;post-interrupt;config;uninstall;upgrade
	Stage Stage `json:"stage"`

	// State is the current state of this package
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Enum=complete;in_progress;skipped;erroring;unknown
	State State `json:"state,omitempty"`

	// Restarts are the number of times a package restarted
	Restarts int32 `json:"restarts,omitempty"`
}

// Equal checks name, version, state, state (not restarts)
func (left *PackageStatus) Equal(right *PackageStatus) bool {
	return left.Name == right.Name &&
		left.Version == right.Version &&
		left.Stage == right.Stage &&
		left.State == right.State
}

type Stage string

const (
	StageUninstall     Stage = "uninstall"
	StageUpgrade       Stage = "upgrade"
	StageApply         Stage = "apply"
	StageInterrupt     Stage = "interrupt"
	StagePostInterrupt Stage = "post-interrupt"
	StageConfig        Stage = "config"
)

type State string

const (
	METADATA_PREFIX string = "skyhook.nvidia.com"
	StateComplete   State  = "complete"
	StateInProgress State  = "in_progress" // this means its actually running, pod started
	StateSkipped    State  = "skipped"     // this means this package, stage are skipped mostly for some parts of the lifecycle
	StateErroring   State  = "erroring"
	StateUnknown    State  = "unknown"
)

type Status string

// TODO: seems like we might be missing a waiting or queued status for nodes, not sure it makes sense at the upper level
// unless we add a limit of number parallel packages, might be another good idea.
const (
	StatusComplete   Status = "complete"
	StatusInProgress Status = "in_progress"
	StatusErroring   Status = "erroring"
	StatusUnknown    Status = "unknown"
)

func GetStatus(s string) Status {
	switch Status(s) {
	case StatusComplete:
		return StatusComplete
	case StatusInProgress:
		return StatusInProgress
	case StatusErroring:
		return StatusErroring
	default:
		return StatusUnknown
	}
}

func StateToStatus(s State) Status {
	switch s {
	case StateComplete:
		return StatusComplete
	case StateErroring:
		return StatusErroring
	case StateInProgress:
		return StatusInProgress
	default:
		return StatusUnknown
	}
}

//+kubebuilder:object:root=true
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.status"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Nodes In-Progress",type=integer,JSONPath=".status.nodesInProgress"
//+kubebuilder:printcolumn:name="Complete Nodes",type=string,JSONPath=".status.completeNodes"
//+kubebuilder:printcolumn:name="Packages",type=string,JSONPath=".status.packageList"
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// Skyhook is the Schema for the skyhooks API
type Skyhook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SkyhookSpec   `json:"spec,omitempty"`
	Status SkyhookStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SkyhookList contains a list of Skyhook
type SkyhookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Skyhook `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Skyhook{}, &SkyhookList{})
}

// WasUpdated returns true if this instance of skyhook has been updated
// func (s *Skyhook) WasUpdated() bool {
// 	return s.Generation > 1 && s.Generation > s.Status.ObservedGeneration
// }
