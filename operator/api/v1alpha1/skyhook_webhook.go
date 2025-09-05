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

package v1alpha1

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/NVIDIA/skyhook/operator/internal/graph"
	semver "github.com/NVIDIA/skyhook/operator/internal/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var (
	skyhooklog       = logf.Log.WithName("skyhook-resource")
	validPackageName = regexp.MustCompile(`(?m)^[a-z][-a-z0-9]{0,41}[a-z]$`)
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Skyhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	skyhookWebhook := &SkyhookWebhook{}
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithDefaulter(skyhookWebhook).
		WithValidator(skyhookWebhook).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=true,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=mskyhook.kb.io,admissionReviewVersions=v1

type SkyhookWebhook struct {
}

var _ admission.CustomDefaulter = &SkyhookWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SkyhookWebhook) Default(ctx context.Context, obj runtime.Object) error {

	skyhook, ok := obj.(*Skyhook)
	if !ok {
		return fmt.Errorf("object is not a Skyhook")
	}

	skyhooklog.Info("default", "name", skyhook.Name)

	// TODO(user): fill in your defaulting logic.
	// Things we might want to default:
	//  - InterruptionBudget
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=false,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=vskyhook.kb.io,admissionReviewVersions=v1

var _ admission.CustomValidator = &SkyhookWebhook{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {

	skyhook, ok := obj.(*Skyhook)
	if !ok {
		return nil, fmt.Errorf("object is not a Skyhook")
	}

	skyhooklog.Info("validate create", "name", skyhook.Name)

	return nil, skyhook.Validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {

	skyhook, ok := newObj.(*Skyhook)
	if !ok {
		return nil, fmt.Errorf("object is not a Skyhook")
	}

	skyhooklog.Info("validate update", "name", skyhook.Name)

	return nil, skyhook.Validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	skyhook, ok := obj.(*Skyhook)
	if !ok {
		return nil, fmt.Errorf("object is not a Skyhook")
	}

	skyhooklog.Info("validate delete", "name", skyhook.Name)

	// I do yet know if we need to do any valuations on delete,
	// if so guessing they would be different than update and create anyways
	return nil, nil
}

func validateResourceOverrides(name string, res *ResourceRequirements) error {
	if res == nil {
		return nil
	}
	anySet := !res.CPURequest.IsZero() || !res.CPULimit.IsZero() || !res.MemoryRequest.IsZero() || !res.MemoryLimit.IsZero()
	allSet := !res.CPURequest.IsZero() && !res.CPULimit.IsZero() && !res.MemoryRequest.IsZero() && !res.MemoryLimit.IsZero()
	if anySet && !allSet {
		return fmt.Errorf("package %q: if any resource override is set, all of cpuRequest, cpuLimit, memoryRequest, memoryLimit must be set", name)
	}
	if allSet {
		if res.CPULimit.Cmp(res.CPURequest) < 0 {
			return fmt.Errorf("package %q: cpuLimit (%s) must be >= cpuRequest (%s)", name, res.CPULimit.String(), res.CPURequest.String())
		}
		if res.MemoryLimit.Cmp(res.MemoryRequest) < 0 {
			return fmt.Errorf("package %q: memoryLimit (%s) must be >= memoryRequest (%s)", name, res.MemoryLimit.String(), res.MemoryRequest.String())
		}
		if res.CPURequest.Sign() <= 0 || res.CPULimit.Sign() <= 0 || res.MemoryRequest.Sign() <= 0 || res.MemoryLimit.Sign() <= 0 {
			return fmt.Errorf("package %q: all resource values must be positive", name)
		}
	}
	return nil
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
		for pattern := range v.ConfigInterrupts {
			// exact key present
			if _, exists := v.ConfigMap[pattern]; exists {
				continue
			}

			// Only '*' is supported as a glob meta character
			isGlob := strings.Contains(pattern, "*")
			if isGlob {
				matchedAny := false
				for key := range v.ConfigMap {
					if ok, err := filepath.Match(pattern, key); err == nil && ok {
						matchedAny = true
						break
					}
				}
				if matchedAny {
					continue
				}
				return fmt.Errorf("error config interrupt glob %q does not match any configMap keys", pattern)
			}

			// not a glob and not an exact key
			return fmt.Errorf("error config interrupt for key that doesn't exist: %s doesn't exist as a configmap", pattern)
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

		if err := validateResourceOverrides(name, v.Resources); err != nil {
			return err
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
