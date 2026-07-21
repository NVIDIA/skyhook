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

package v1alpha1

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/NVIDIA/nodewright/operator/internal/graph"
	semver "github.com/NVIDIA/nodewright/operator/internal/version"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	validPackageName = regexp.MustCompile(`^[a-z][-a-z0-9]{0,41}[a-z]$`)
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Skyhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	skyhookWebhook := &SkyhookWebhook{
		Client: mgr.GetClient(),
	}
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(skyhookWebhook).
		WithValidator(skyhookWebhook).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=true,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=mskyhook.kb.io,admissionReviewVersions=v1

// SkyhookWebhook validates Skyhook resources at admission time.
// Includes a client for validating references to DeploymentPolicies.
// +kubebuilder:object:generate=false
type SkyhookWebhook struct {
	Client client.Client
}

var _ admission.Defaulter[*Skyhook] = &SkyhookWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SkyhookWebhook) Default(ctx context.Context, skyhook *Skyhook) error {

	logf.FromContext(ctx).Info("default", "name", skyhook.Name)

	// TODO(user): fill in your defaulting logic.
	// Things we might want to default:
	//  - InterruptionBudget
	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-skyhook-nvidia-com-v1alpha1-skyhook,mutating=false,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=skyhooks,verbs=create;update,versions=v1alpha1,name=vskyhook.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*Skyhook] = &SkyhookWebhook{}

// skyhookDeprecationWarning is surfaced on every create/update of a legacy Skyhook so
// operators are nudged toward the new group during the migration bridge. It deliberately
// does not name a specific removal release; the bridge is kept for a multi-release window.
const skyhookDeprecationWarning = "skyhook.nvidia.com/v1alpha1 Skyhook is deprecated; migrate to " +
	"nodewright.nvidia.com/v1alpha1 NodeWright (kubectl nodewright migrate). The Skyhook kind will be " +
	"removed in a future release."

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateCreate(ctx context.Context, skyhook *Skyhook) (admission.Warnings, error) {

	logf.FromContext(ctx).Info("validate create", "name", skyhook.Name)

	warnings := admission.Warnings{skyhookDeprecationWarning}

	if err := skyhook.Validate(); err != nil {
		return warnings, fmt.Errorf("validating skyhook %q: %w", skyhook.Name, err)
	}

	return warnings, r.validateDeploymentPolicyExists(ctx, skyhook)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateUpdate(ctx context.Context, oldSkyhook, newSkyhook *Skyhook) (admission.Warnings, error) {

	logf.FromContext(ctx).Info("validate update", "name", newSkyhook.Name)

	warnings := admission.Warnings{skyhookDeprecationWarning}

	// MIGRATION-SHIM: a migrated legacy Skyhook is frozen read-only; real edits go to the
	// NodeWright (see legacy_readonly_webhook.go, not mirrored into the nodewright group).
	// legacyReadOnlyError rejects every user-meaningful edit and allows only no-ops,
	// finalizer edits, and deletions. Those allow-listed updates must NOT be re-validated:
	// re-running Validate()/validateDeploymentPolicyExists on a frozen object would wrongly
	// reject a GitOps no-op re-apply or a finalizer strip (e.g. once a stricter rule ships,
	// or after the referenced legacy DeploymentPolicy is gone), stranding the object in
	// Terminating. All substantive validation — including the uninstall/downgrade rules —
	// now runs on the writable NodeWright (see nodewright_webhook.go).
	if oldSkyhook != nil {
		return warnings, legacyReadOnlyError(oldSkyhook, newSkyhook)
	}

	// oldSkyhook == nil is never a real update (the apiserver always supplies the prior
	// object); it is only reached by unit tests exercising create-style spec validation
	// through this entrypoint, so validate the spec and its policy reference as on create.
	if err := newSkyhook.Validate(); err != nil {
		return warnings, fmt.Errorf("validating skyhook %q: %w", newSkyhook.Name, err)
	}
	return warnings, r.validateDeploymentPolicyExists(ctx, newSkyhook)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SkyhookWebhook) ValidateDelete(ctx context.Context, skyhook *Skyhook) (admission.Warnings, error) {

	logf.FromContext(ctx).Info("validate delete", "name", skyhook.Name)

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
		return fmt.Errorf("interruption budget: %w", err)
	}

	if err := r.Spec.DrainConfig.Validate(); err != nil {
		return fmt.Errorf("drain config: %w", err)
	}

	// DeploymentPolicy and InterruptionBudget are mutually exclusive
	if r.Spec.DeploymentPolicy != "" && (r.Spec.InterruptionBudget.Percent != nil || r.Spec.InterruptionBudget.Count != nil) {
		return fmt.Errorf("deploymentPolicy and interruptionBudget are mutually exclusive")
	}

	if _, err := metav1.LabelSelectorAsSelector(&r.Spec.PodNonInterruptLabels); err != nil {
		return fmt.Errorf("pod non-interrupt labels are not valid: %w", err)
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

		// image must be a bare registry/repository reference. version is the operator's
		// ordering key (it drives upgrade/downgrade detection) and containerSHA pins the
		// exact bytes the kubelet pulls, so neither a tag nor a digest may be embedded in
		// image. Both are rejected rather than silently stripped: an inline tag was once
		// absorbed as a migration off the pre-semver scheme, but that migration is done,
		// so tags and digests now behave identically. tag/digest are non-nil whenever
		// their separator is present, so an empty separator ("repo@", "repo:") is
		// rejected too rather than slipping through to an invalid pull reference.
		// (Emptiness and whitespace are enforced declaratively by the CRD's Pattern.)
		_, tag, digest := splitImageReference(v.Image)
		if digest != nil {
			return fmt.Errorf(
				"error package %s's image '%s' contains an inline digest. Do not embed a digest in the image; pin the image bytes using the package's containerSHA field instead",
				name,
				v.Image,
			)
		}
		if tag != nil {
			return fmt.Errorf(
				"error package %s's image '%s' contains an inline tag '%s'. Do not embed a tag in the image; the package version supplies the tag",
				name,
				v.Image,
				*tag,
			)
		}

		if !semver.IsValid(v.Version) {
			return fmt.Errorf("error version string for %s is invalid: %s", v.Name, v.Version)
		}

		if err := validateResourceOverrides(name, v.Resources); err != nil {
			return err
		}

		// Validate uninstall configuration
		if v.Uninstall != nil && v.Uninstall.Apply && !v.Uninstall.Enabled {
			return fmt.Errorf("package %q: uninstall.apply requires uninstall.enabled to be true", name)
		}
	}

	var graph graph.DependencyGraph[*Package]

	var err error
	graph, err = r.Spec.BuildGraph()
	if err != nil {
		return fmt.Errorf("error trying to validate skyhook spec building graph: %w", err)
	}

	err = graph.Valid()
	if err != nil {
		return fmt.Errorf("error trying to validate skyhook spec graph is invalid: %w", err)
	}

	return nil
}

// validateDeploymentPolicyExists checks if the referenced DeploymentPolicy exists
func (r *SkyhookWebhook) validateDeploymentPolicyExists(ctx context.Context, skyhook *Skyhook) error {
	// Skip validation if no deployment policy is specified
	if skyhook.Spec.DeploymentPolicy == "" {
		return nil
	}

	// Check if the DeploymentPolicy exists (cluster-scoped, no namespace)
	policy := &DeploymentPolicy{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name: skyhook.Spec.DeploymentPolicy,
	}, policy)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("deploymentPolicy %q not found", skyhook.Spec.DeploymentPolicy)
		}
		return fmt.Errorf("error checking if deploymentPolicy %q exists: %w", skyhook.Spec.DeploymentPolicy, err)
	}

	return nil
}
