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
	"fmt"
	"regexp"
	"strings"

	"github.com/NVIDIA/skyhook/internal/graph"
	semver "github.com/NVIDIA/skyhook/internal/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var (
	skyhooklog       = logf.Log.WithName("skyhook-resource")
	validPackageName = regexp.MustCompile(`(?m)^[a-z][-a-z0-9]{0,41}[a-z]$`)
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Skyhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=true,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=mskyhook.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Skyhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Skyhook) Default() {
	skyhooklog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	// Things we might want to default:
	//  - InterruptionBudget
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=false,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=vskyhook.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Skyhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Skyhook) ValidateCreate() (admission.Warnings, error) {
	skyhooklog.Info("validate create", "name", r.Name)

	return nil, r.Validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Skyhook) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	skyhooklog.Info("validate update", "name", r.Name)

	return nil, r.Validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Skyhook) ValidateDelete() (admission.Warnings, error) {
	skyhooklog.Info("validate delete", "name", r.Name)

	// I do yet know if we need to do any valuations on delete,
	// if so guessing they would be different than update and create anyways
	return nil, nil
}

func (r *Skyhook) Validate() error {

	if err := r.Spec.InterruptionBudget.Validate(); err != nil {
		return err
	}

	if _, err := metav1.LabelSelectorAsSelector(&r.Spec.NodeSelector); err != nil {
		return fmt.Errorf("node selectors are not valid: %w", err)
	}

	names := make(map[string]string)
	for name, v := range r.Spec.Packages {
		// test for package names to be unique and that the name and package key match
		if v.Name != name {
			return fmt.Errorf("error package %s's name was set to %s. Do not explicitly set the name in the package's definition", name, v.Name)
		}

		key := v.Name
		if val, ok := names[key]; ok {
			return fmt.Errorf("error duplicate packages different versions [%s:%s] and [%s:%s]", key, v.Version, key, val)
		}
		names[key] = v.Version

		// test name is valid RFC 1123
		if !validPackageName.MatchString(key) {
			return fmt.Errorf("package name [%s] is not valid. must match [%s]", key, validPackageName.String())
		}

		// test to make sure that the config interrupts are for valid packages
		for configMap := range v.ConfigInterrupts {
			if _, exists := v.ConfigMap[configMap]; !exists {
				return fmt.Errorf("error config interrupt for key that doesn't exist: %s doesn't exist as a configmap", configMap)
			}
		}

		image, version, found := strings.Cut(v.Image, ":")
		if found && version != v.Version {
			return fmt.Errorf(
				"error package %s's image tag was set to '%s' for '%s' and doesn't match the pacakge's version '%s'. Do not explicitly set the image's tag in the package's definition (The package version will be set as the tag)",
				name,
				version,
				image,
				v.Version,
			)
		}

		if !semver.IsValid(v.Version) {
			return fmt.Errorf("error version string for %s is invalid: %s", v.Name, v.Version)
		}
	}

	var graph graph.DependencyGraph[*Package]

	var err error
	graph, err = r.Spec.BuildGraph()
	if err != nil {
		return fmt.Errorf("error trying to validate skyhook spec building graph: %s", err)
	}

	err = graph.Valid()
	if err != nil {
		return fmt.Errorf("error trying to validate skyhook spec graph is invalid: %s", err)
	}

	return nil
}
