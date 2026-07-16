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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	deploymentPolicylog = logf.Log.WithName("deployment-policy-resource")
)

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *DeploymentPolicy) SetupWebhookWithManager(mgr ctrl.Manager) error {
	deploymentPolicyWebhook := &DeploymentPolicyWebhook{
		Client: mgr.GetClient(),
	}
	return ctrl.NewWebhookManagedBy(mgr, r).
		WithDefaulter(deploymentPolicyWebhook).
		WithValidator(deploymentPolicyWebhook).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-skyhook-nvidia-com-v1alpha1-deploymentpolicy,mutating=true,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=deploymentpolicies,verbs=create;update,versions=v1alpha1,name=mdeploymentpolicy.kb.io,admissionReviewVersions=v1

// DeploymentPolicyWebhook validates DeploymentPolicy resources at admission time.
// Includes a client to check if any Skyhooks reference this policy before allowing deletion.
// +kubebuilder:object:generate=false
type DeploymentPolicyWebhook struct {
	Client client.Client
}

var _ admission.Defaulter[*DeploymentPolicy] = &DeploymentPolicyWebhook{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *DeploymentPolicyWebhook) Default(ctx context.Context, deploymentPolicy *DeploymentPolicy) error {

	deploymentPolicylog.Info(DefaultCompartmentName, "name", deploymentPolicy.Name)

	// Apply defaults to the default strategy
	if deploymentPolicy.Spec.Default.Strategy != nil {
		deploymentPolicy.Spec.Default.Strategy.Default()
	}

	// Apply defaults to compartment strategies
	for i := range deploymentPolicy.Spec.Compartments {
		compartment := &deploymentPolicy.Spec.Compartments[i]

		// Apply defaults to compartment strategy
		if compartment.Strategy != nil {
			compartment.Strategy.Default()
		}
	}

	return nil
}

//+kubebuilder:webhook:path=/validate-skyhook-nvidia-com-v1alpha1-deploymentpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=skyhook.nvidia.com,resources=deploymentpolicies,verbs=create;update;delete,versions=v1alpha1,name=vdeploymentpolicy.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*DeploymentPolicy] = &DeploymentPolicyWebhook{}

// deploymentPolicyDeprecationWarning is surfaced on every create/update of a legacy
// DeploymentPolicy during the migration bridge. It does not name a specific removal release.
const deploymentPolicyDeprecationWarning = "skyhook.nvidia.com/v1alpha1 DeploymentPolicy is deprecated; " +
	"migrate to nodewright.nvidia.com/v1alpha1 DeploymentPolicy (kubectl nodewright migrate). The " +
	"skyhook.nvidia.com group will be removed in a future release."

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentPolicyWebhook) ValidateCreate(ctx context.Context, deploymentPolicy *DeploymentPolicy) (admission.Warnings, error) {

	deploymentPolicylog.Info("validate create", "name", deploymentPolicy.Name)

	return admission.Warnings{deploymentPolicyDeprecationWarning}, deploymentPolicy.Validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentPolicyWebhook) ValidateUpdate(ctx context.Context, oldDeploymentPolicy, deploymentPolicy *DeploymentPolicy) (admission.Warnings, error) {

	deploymentPolicylog.Info("validate update", "name", deploymentPolicy.Name)

	return admission.Warnings{deploymentPolicyDeprecationWarning}, deploymentPolicy.Validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentPolicyWebhook) ValidateDelete(ctx context.Context, deploymentPolicy *DeploymentPolicy) (admission.Warnings, error) {

	deploymentPolicylog.Info("validate delete", "name", deploymentPolicy.Name)

	// Check if any Skyhooks are still referencing this policy
	skyhooks := &SkyhookList{}
	if err := r.Client.List(ctx, skyhooks); err != nil {
		return nil, fmt.Errorf("failed to list skyhooks to check for references: %w", err)
	}

	referencingSkyhooks := []string{}
	for _, skyhook := range skyhooks.Items {
		if skyhook.Spec.DeploymentPolicy == deploymentPolicy.Name {
			referencingSkyhooks = append(referencingSkyhooks, skyhook.Name)
		}
	}

	if len(referencingSkyhooks) > 0 {
		return nil, fmt.Errorf("cannot delete DeploymentPolicy %q: still referenced by %d Skyhook(s): %v",
			deploymentPolicy.Name, len(referencingSkyhooks), referencingSkyhooks)
	}

	return nil, nil
}

func (r *DeploymentPolicy) Validate() error {
	// Validate default budget
	if err := r.Spec.Default.Budget.Validate(); err != nil {
		return fmt.Errorf("default budget: %w", err)
	}

	// Validate default strategy
	if r.Spec.Default.Strategy != nil {
		if err := r.Spec.Default.Strategy.Validate(); err != nil {
			return fmt.Errorf("default strategy: %w", err)
		}
	}

	// Track compartment names for uniqueness
	names := make(map[string]bool)
	selectors := make(map[string]metav1.LabelSelector)

	for _, compartment := range r.Spec.Compartments {
		// Validate compartment name is not "__default__" (reserved)
		if compartment.Name == DefaultCompartmentName {
			return fmt.Errorf("compartment name %q is reserved and cannot be used", compartment.Name)
		}

		// Validate unique names
		if names[compartment.Name] {
			return fmt.Errorf("compartment name %q is not unique", compartment.Name)
		}
		names[compartment.Name] = true

		// Validate the compartment itself
		if err := compartment.Validate(); err != nil {
			return fmt.Errorf("compartment %q: %w", compartment.Name, err)
		}

		// Check for identical selectors
		for existingName, existingSelector := range selectors {
			if reflect.DeepEqual(compartment.Selector, existingSelector) {
				return fmt.Errorf("compartment %q has identical selector to compartment %q", compartment.Name, existingName)
			}
		}
		selectors[compartment.Name] = compartment.Selector
	}

	return nil
}
