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

package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	nwv1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// Webhook names
	// MIGRATION-SHIM: the skyhook* names/rules below (and their getMutating/
	// getValidatingWebhookRules cases) manage the legacy skyhook.nvidia.com webhooks.
	// Drop the skyhook* cases when the legacy group is removed; the nodewright webhooks
	// are chart-owned (operator only injects their caBundle).
	skyhookValidatingWebhookName          = "validate-skyhook.nvidia.com"
	deploymentPolicyValidatingWebhookName = "validate-deploymentpolicy.nvidia.com"
	skyhookMutatingWebhookName            = "mutate-skyhook.nvidia.com"
	deploymentPolicyMutatingWebhookName   = "mutate-deploymentpolicy.nvidia.com"

	// webhookConfigLabelKey marks the chart-owned webhook configurations this operator
	// injects a caBundle into. They are looked up by this label rather than by name so
	// that renaming them in the chart does not break the operator binary: a name-based
	// lookup turns a rename into a hard error on the running (old) leader, which then
	// never goes Ready, never releases the bootstrap lease, and wedges the rolling
	// update. See docs/designs/webhook-bootstrap-lease.md.
	//
	// This buys name-independence for the *lookup* only. The manager ClusterRole still
	// scopes `update` to these objects by resourceNames (deliberate least privilege --
	// see the CKV_K8S_155 skip on chart/templates/manager-rbac.yaml), so the chart must
	// keep that list and the object names in lockstep. Both render from the same
	// chart.{validating,mutating}WebhookName helpers, and helm-template-test asserts they
	// agree. If they ever diverge, the symptom is a Forbidden on Update, which
	// updateWebhookConfigurationsErr below annotates rather than leaving opaque.
	webhookConfigLabelKey = nwv1.METADATA_PREFIX + "/webhook-config"

	// Certificate management
	certRotationThreshold    = 168 * time.Hour      // 7 days
	certValidityDurationYear = 365 * 24 * time.Hour // 1 year

	// Both of these stay on the LEGACY prefix, unlike webhookConfigLabelKey above, because
	// they are not ours to choose: webhookCert.ToSecret (cert_utils.go) has written both
	// keys under v1alpha1.METADATA_PREFIX since cert-manager was dropped, and every
	// webhook-cert Secret in the field already carries them. Reading under the nodewright
	// prefix would miss on every existing Secret and leave the writer and the reader
	// disagreeing. Move them together with ToSecret when the legacy group is removed.
	expirationAnnotationKey = v1alpha1.METADATA_PREFIX + "/expiration"
	serviceAnnotationKey    = v1alpha1.METADATA_PREFIX + "/service"
)

// This project used to use cert-manager to generate the webhook certificates.
// This removes the dependency on cert-manager and simplifies the deployment.
// This also removes the need to have a specific issuer, and just uses a self-signed cert.
//
// ServiceName is chart-supplied rather than a constant because the chart templates the
// Service name off its fullname; it goes in the serving cert's SAN, so a mismatch fails
// admission closed. The webhook configurations are deliberately absent here: they are
// found by label (see webhookConfigLabelKey), not by a configured name.
type WebhookControllerOptions struct { // prefix these with WEBHOOK_
	SecretName  string `env:"WEBHOOK_SECRET_NAME, default=webhook-cert"`
	ServiceName string `env:"WEBHOOK_SERVICE_NAME, default=skyhook-operator-webhook-service"`
}

type WebhookController struct {
	client.Client
	cache     runtimecache.Cache
	namespace string
	certDir   string
	opts      WebhookControllerOptions
}

func NewWebhookController(client client.Client, cache runtimecache.Cache, namespace, certDir string, opts WebhookControllerOptions) (*WebhookController, error) {
	if err := ensureDummyCert(certDir); err != nil {
		return nil, err
	}

	return &WebhookController{
		Client:    client,
		cache:     cache,
		namespace: namespace,
		certDir:   certDir,
		opts:      opts,
	}, nil
}

// Start implements the Runnable interface to ensure certificates are set up before the webhook server starts
func (r *WebhookController) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Setting up webhook certificates")

	// wait for the cache to sync
	if cache := r.cache.WaitForCacheSync(ctx); !cache {
		return fmt.Errorf("failed to wait for cache to sync")
	}
	// starts the reconcile process off
	_, err := r.GetOrCreateWebhookCertSecret(ctx, r.opts.SecretName, r.namespace)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil // ignore this special case, it just needs to exist
		}
		return err
	}

	logger.Info("Webhook certificates setup complete")
	return nil
}

// NeedLeaderElection implements the Runnable interface; runs only on the leader of
// whichever manager this controller is registered with. WebhookController is intentionally
// hosted on a dedicated manager (webhookBootstrapLeaseID, see cmd/manager/main.go) rather
// than the main reconcile manager, so that an old-version leader holding the reconcile
// lease cannot block a new pod from bootstrapping the webhook serving cert and patching
// the (Mutating|Validating)WebhookConfiguration caBundle (see
// docs/designs/webhook-bootstrap-lease.md).
func (r *WebhookController) NeedLeaderElection() bool {
	return true
}

func (r *WebhookController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetNamespace() == r.namespace && obj.GetName() == r.opts.SecretName
		}))).
		// Watch webhook configurations so the leader detects external changes (e.g. Helm upgrade
		// resetting caBundle) and fixes them immediately instead of waiting for the 24h requeue.
		Watches(&admissionregistrationv1.ValidatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.webhookConfigToSecret),
			builder.WithPredicates(predicate.NewPredicateFuncs(hasWebhookConfigLabel))).
		Watches(&admissionregistrationv1.MutatingWebhookConfiguration{},
			handler.EnqueueRequestsFromMapFunc(r.webhookConfigToSecret),
			builder.WithPredicates(predicate.NewPredicateFuncs(hasWebhookConfigLabel))).
		Complete(r)
}

// webhookConfigToSecret maps webhook config change events back to the cert Secret,
// so the existing Reconcile() can verify and fix the caBundle.
func (r *WebhookController) webhookConfigToSecret(_ context.Context, _ client.Object) []reconcile.Request {
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{Name: r.opts.SecretName, Namespace: r.namespace},
	}}
}

// permissions
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// The only Secret the operator touches is its own webhook serving cert, in its own namespace,
// so this is namespaced rather than cluster-wide — the operator has no business reading every
// Secret in the cluster. The webhookBootstrapMgr cache in main.go is scoped to match; the two
// must move together, since a cluster-wide Secret informer under this Role would be rejected
// and the webhook would never bootstrap.
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete,namespace=skyhook

// Reconcile is the main function that reconciles the webhook controller
func (r *WebhookController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling webhook controller")

	// if its deleted, skip reconciliation, this is for cleanup
	obj := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		// handle not found, etc.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If the object is being deleted, skip reconciliation
	if !obj.ObjectMeta.DeletionTimestamp.IsZero() {
		// Optionally: handle finalizers here if you want
		return ctrl.Result{}, nil
	}

	// 1. Get or create/update the Secret with certs
	// 2. Get or create/update the webhook configurations

	// Example: check if secret exists
	secret, err := r.GetOrCreateWebhookCertSecret(ctx, r.opts.SecretName, r.namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = r.CheckOrUpdateWebhookCertSecret(ctx, secret)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = r.CheckOrUpdateWebhookConfigurations(ctx, secret)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled webhook controller")
	return reconcile.Result{RequeueAfter: 24 * time.Hour}, nil // requeue for periodic rotation/check
}

// GetOrCreateWebhookCertSecret returns a new secret with the given name and the given CA and cert.
func (r *WebhookController) GetOrCreateWebhookCertSecret(ctx context.Context, secretName, namespace string) (*corev1.Secret, error) {

	// get the secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// not found, create it
			webhookCert, err := generateCert(r.opts.ServiceName, r.namespace, certValidityDurationYear)
			if err != nil {
				return nil, err
			}

			// Write cert and key to disk if CertDir is set
			if r.certDir != "" {
				_ = writeCertAndKey([]byte(webhookCert.TLSCert), []byte(webhookCert.TLSKey), r.certDir)
			}

			secret = webhookCert.ToSecret(secretName, namespace, r.opts.ServiceName)

			if err := r.Create(ctx, secret); err != nil {
				return nil, err
			}

			return secret, nil
		}
		return nil, err
	}

	// found, return it
	return secret, nil
}

// CheckOrUpdateWebhookCertSecret checks if the webhook secret is going to expire in the next 7 days,
// if the cert on disk is different from the secret, or if the cert was minted for a different Service
// than the one currently configured. If any of those hold, it generates a new cert and updates the secret.
func (r *WebhookController) CheckOrUpdateWebhookCertSecret(ctx context.Context, secret *corev1.Secret) (bool, error) {
	equal, err := compareCertOnDiskToSecret(r.certDir, secret)
	if err != nil {
		return false, fmt.Errorf("comparing cert on disk to secret %s/%s: %w", r.namespace, secret.Name, err)
	}

	// The Secret outlives the chart objects: helm does not own it, so renaming the webhook
	// Service leaves a perfectly valid, unexpired cert whose SAN no longer matches the DNS
	// name the API server dials, and admission fails closed with an x509 error until the
	// cert expires (a year). Treat a missing annotation as a mismatch: it can only come from
	// a cert minted before the annotation existed, and reminting is cheap.
	serviceChanged := secret.Annotations[serviceAnnotationKey] != r.opts.ServiceName

	// check if the secret is going to expire in the next 7 days or if the cert on disk is different from the secret
	if !equal || serviceChanged || secret.Annotations[expirationAnnotationKey] < time.Now().Add(certRotationThreshold).Format(time.RFC3339) {
		// expired, generate a new cert
		webhookCert, err := generateCert(r.opts.ServiceName, r.namespace, certValidityDurationYear)
		if err != nil {
			return false, err
		}

		// Write cert and key to disk if CertDir is set
		if r.certDir != "" {
			_ = writeCertAndKey([]byte(webhookCert.TLSCert), []byte(webhookCert.TLSKey), r.certDir)
		}

		secret.Data["ca.crt"] = webhookCert.CABytes
		secret.Data["tls.crt"] = []byte(webhookCert.TLSCert)
		secret.Data["tls.key"] = []byte(webhookCert.TLSKey)
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[expirationAnnotationKey] = webhookCert.Expiration.Format(time.RFC3339)
		secret.Annotations[serviceAnnotationKey] = r.opts.ServiceName

		return true, r.Update(ctx, secret)
	}

	return false, nil
}

// CheckOrUpdateWebhookConfigurations checks if the webhook configurations are need to be updated with the new cert
// if it is, it will update the webhook configurations
func (r *WebhookController) CheckOrUpdateWebhookConfigurations(ctx context.Context, secret *corev1.Secret) (bool, error) {
	caBundle := secret.Data["ca.crt"]

	validatingChanged, err := r.updateValidatingWebhookConfiguration(ctx, caBundle)
	if err != nil {
		return false, err
	}

	mutatingChanged, err := r.updateMutatingWebhookConfiguration(ctx, caBundle)
	if err != nil {
		return false, err
	}

	return validatingChanged || mutatingChanged, nil
}

// hasWebhookConfigLabel reports whether an object carries the chart's webhook-config marker.
func hasWebhookConfigLabel(obj client.Object) bool {
	_, ok := obj.GetLabels()[webhookConfigLabelKey]
	return ok
}

// dialsThisOperator reports whether a single webhook's clientConfig points at the Service
// this operator issues its serving certificate for.
//
// This is THE ownership rule, deliberately in one place: it decides both which
// configurations are ours and which individual webhooks inside them we may write. Those two
// must agree — the caBundle only signs the cert for r.opts.ServiceName, so injecting it into
// a webhook that dials some other Service actively breaks that Service's admission rather
// than merely being untidy. Keeping the predicate single-sourced is also why the validating
// and mutating paths, which are otherwise near-identical, cannot drift apart on the part
// that matters.
func (r *WebhookController) dialsThisOperator(svc *admissionregistrationv1.ServiceReference) bool {
	return svc != nil && svc.Name == r.opts.ServiceName && svc.Namespace == r.namespace
}

// ownedValidatingWebhookConfigurations returns the chart-owned ValidatingWebhookConfigurations
// that point at this operator's namespace.
func (r *WebhookController) ownedValidatingWebhookConfigurations(ctx context.Context) ([]admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	list := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	if err := r.List(ctx, list, client.HasLabels{webhookConfigLabelKey}); err != nil {
		return nil, fmt.Errorf("listing ValidatingWebhookConfigurations labelled %s: %w", webhookConfigLabelKey, err)
	}

	owned := make([]admissionregistrationv1.ValidatingWebhookConfiguration, 0, len(list.Items))
	for _, conf := range list.Items {
		for i := range conf.Webhooks {
			if r.dialsThisOperator(conf.Webhooks[i].ClientConfig.Service) {
				owned = append(owned, conf)
				break
			}
		}
	}

	if len(owned) == 0 {
		return nil, fmt.Errorf("no ValidatingWebhookConfiguration labelled %s targets namespace %q; creation is handled by the Helm chart. Ensure the chart is installed and webhooks are enabled", webhookConfigLabelKey, r.namespace)
	}
	return owned, nil
}

// ownedMutatingWebhookConfigurations returns the chart-owned MutatingWebhookConfigurations
// that point at this operator's namespace.
func (r *WebhookController) ownedMutatingWebhookConfigurations(ctx context.Context) ([]admissionregistrationv1.MutatingWebhookConfiguration, error) {
	list := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := r.List(ctx, list, client.HasLabels{webhookConfigLabelKey}); err != nil {
		return nil, fmt.Errorf("listing MutatingWebhookConfigurations labelled %s: %w", webhookConfigLabelKey, err)
	}

	owned := make([]admissionregistrationv1.MutatingWebhookConfiguration, 0, len(list.Items))
	for _, conf := range list.Items {
		for i := range conf.Webhooks {
			if r.dialsThisOperator(conf.Webhooks[i].ClientConfig.Service) {
				owned = append(owned, conf)
				break
			}
		}
	}

	if len(owned) == 0 {
		return nil, fmt.Errorf("no MutatingWebhookConfiguration labelled %s targets namespace %q; creation is handled by the Helm chart. Ensure the chart is installed and webhooks are enabled", webhookConfigLabelKey, r.namespace)
	}
	return owned, nil
}

// updateValidatingWebhookConfiguration updates every owned ValidatingWebhookConfiguration with the provided CABundle
func (r *WebhookController) updateValidatingWebhookConfiguration(ctx context.Context, caBundle []byte) (bool, error) {
	owned, err := r.ownedValidatingWebhookConfigurations(ctx)
	if err != nil {
		return false, err
	}

	changed := false
	var errs []error
	for c := range owned {
		existingValidating := &owned[c]

		needUpdate := false
		for i := range existingValidating.Webhooks {
			// Ownership is per-webhook, matching how it was decided: a configuration is
			// claimed because AT LEAST ONE webhook dials our Service, so skip any that
			// does not. The chart points all of them at one Service and this is a no-op,
			// but injecting a CA that does not sign a foreign Service's cert is the exact
			// breakage dialsThisOperator exists to prevent.
			if !r.dialsThisOperator(existingValidating.Webhooks[i].ClientConfig.Service) {
				continue
			}
			// getValidatingWebhookRules returns nil for webhooks the operator does not
			// own (e.g. the chart-defined nodewright mirror webhooks); those keep their
			// chart rules and only have their caBundle reconciled.
			expectedRules := r.getValidatingWebhookRules(existingValidating.Webhooks[i].Name)
			if validatingWebhookNeedsUpdate(&existingValidating.Webhooks[i], caBundle, expectedRules) {
				needUpdate = true
			}
		}

		if needUpdate {
			// Keep going rather than returning: during a rename window two owned
			// configurations legitimately coexist, and bailing on the first would
			// silently defer the second to a later requeue with no admission in
			// between. Errors are joined so every failure still reaches the queue.
			if err := r.Update(ctx, existingValidating); err != nil {
				errs = append(errs, updateWebhookConfigurationsErr("ValidatingWebhookConfiguration", existingValidating.Name, err))
				continue
			}
			changed = true
		}
	}

	return changed, errors.Join(errs...)
}

// updateMutatingWebhookConfiguration updates every owned MutatingWebhookConfiguration with the provided CABundle
func (r *WebhookController) updateMutatingWebhookConfiguration(ctx context.Context, caBundle []byte) (bool, error) {
	owned, err := r.ownedMutatingWebhookConfigurations(ctx)
	if err != nil {
		return false, err
	}

	changed := false
	var errs []error
	for c := range owned {
		existingMutating := &owned[c]

		needUpdate := false
		for i := range existingMutating.Webhooks {
			// See updateValidatingWebhookConfiguration: skip webhooks that dial a
			// different Service than the one this operator's caBundle signs for.
			if !r.dialsThisOperator(existingMutating.Webhooks[i].ClientConfig.Service) {
				continue
			}
			// getMutatingWebhookRules returns nil for webhooks the operator does not own
			// (e.g. the chart-defined nodewright mirror webhooks); those keep their chart
			// rules and only have their caBundle reconciled.
			expectedRules := r.getMutatingWebhookRules(existingMutating.Webhooks[i].Name)
			if mutatingWebhookNeedsUpdate(&existingMutating.Webhooks[i], caBundle, expectedRules) {
				needUpdate = true
			}
		}

		if needUpdate {
			// See updateValidatingWebhookConfiguration: continue rather than return so a
			// failure on one owned configuration does not defer the others.
			if err := r.Update(ctx, existingMutating); err != nil {
				errs = append(errs, updateWebhookConfigurationsErr("MutatingWebhookConfiguration", existingMutating.Name, err))
				continue
			}
			changed = true
		}
	}

	return changed, errors.Join(errs...)
}

// updateWebhookConfigurationsErr annotates a failed caBundle write. A Forbidden here is
// almost always the one coupling label-based discovery does not remove: the operator finds
// a configuration by label that the manager ClusterRole's resourceNames does not cover, so
// say that outright instead of surfacing a bare RBAC error in a requeue loop.
func updateWebhookConfigurationsErr(kind, name string, err error) error {
	if apierrors.IsForbidden(err) {
		return fmt.Errorf("updating %s %s: not permitted; the manager ClusterRole scopes update by resourceNames and %q is not in that list, so the chart's webhook configuration names and its RBAC have drifted: %w", kind, name, name, err)
	}
	return fmt.Errorf("updating %s %s: %w", kind, name, err)
}

// getValidatingWebhookRules returns the expected rules for a validating webhook by name
func (r *WebhookController) getValidatingWebhookRules(webhookName string) []admissionregistrationv1.RuleWithOperations {
	switch webhookName {
	case skyhookValidatingWebhookName:
		return skyhookRules()
	case deploymentPolicyValidatingWebhookName:
		return deploymentPolicyValidatingRules()
	default:
		return nil
	}
}

// getMutatingWebhookRules returns the expected rules for a mutating webhook by name
func (r *WebhookController) getMutatingWebhookRules(webhookName string) []admissionregistrationv1.RuleWithOperations {
	switch webhookName {
	case skyhookMutatingWebhookName:
		return skyhookRules()
	case deploymentPolicyMutatingWebhookName:
		return deploymentPolicyMutatingRules()
	default:
		return nil
	}
}

func compareMutatingWebhookConfigurations(a, b *admissionregistrationv1.MutatingWebhookConfiguration) bool {
	if len(a.Webhooks) != len(b.Webhooks) {
		return true
	}
	for i := range a.Webhooks {
		if !bytes.Equal(a.Webhooks[i].ClientConfig.CABundle, b.Webhooks[i].ClientConfig.CABundle) {
			return true
		}
	}
	return false
}

func compareValidatingWebhookConfigurations(a, b *admissionregistrationv1.ValidatingWebhookConfiguration) bool {
	if len(a.Webhooks) != len(b.Webhooks) {
		return true
	}
	for i := range a.Webhooks {
		if !bytes.Equal(a.Webhooks[i].ClientConfig.CABundle, b.Webhooks[i].ClientConfig.CABundle) {
			return true
		}
	}
	return false
}

func skyhookRules() []admissionregistrationv1.RuleWithOperations {
	return []admissionregistrationv1.RuleWithOperations{
		{
			Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
			Rule: admissionregistrationv1.Rule{
				APIGroups:   []string{v1alpha1.GroupVersion.Group},
				APIVersions: []string{v1alpha1.GroupVersion.Version},
				Resources:   []string{"skyhooks"},
			},
		},
	}
}

// deploymentPolicyValidatingRules adds the delete operation to the mutating webhook rules, otherwise they are the same
func deploymentPolicyValidatingRules() []admissionregistrationv1.RuleWithOperations {
	mutrules := deploymentPolicyMutatingRules()
	oprs := mutrules[0].Operations
	newops := make([]admissionregistrationv1.OperationType, len(oprs), len(oprs)+1)
	copy(newops, oprs)
	newops = append(newops, admissionregistrationv1.Delete)
	mutrules[0].Operations = newops
	return mutrules
}

func deploymentPolicyMutatingRules() []admissionregistrationv1.RuleWithOperations {
	return []admissionregistrationv1.RuleWithOperations{
		{
			Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
			Rule: admissionregistrationv1.Rule{
				APIGroups:   []string{v1alpha1.GroupVersion.Group},
				APIVersions: []string{v1alpha1.GroupVersion.Version},
				Resources:   []string{"deploymentpolicies"},
			},
		},
	}
}

// validatingWebhookNeedsUpdate checks if a validating webhook needs to be updated with new CABundle or Rules
// Returns true if updates were made to the webhook
func validatingWebhookNeedsUpdate(webhook *admissionregistrationv1.ValidatingWebhook, caBundle []byte, expectedRules []admissionregistrationv1.RuleWithOperations) bool {
	needUpdate := false

	// Check if CABundle needs to be set or updated (catches both empty and stale values)
	if !bytes.Equal(webhook.ClientConfig.CABundle, caBundle) {
		webhook.ClientConfig.CABundle = caBundle
		needUpdate = true
	}

	// Only reconcile rules for webhooks the operator owns (expectedRules != nil).
	// Chart-defined mirror webhooks keep their own rules.
	if expectedRules != nil && !reflect.DeepEqual(webhook.Rules, expectedRules) {
		webhook.Rules = expectedRules
		needUpdate = true
	}

	return needUpdate
}

// mutatingWebhookNeedsUpdate checks if a mutating webhook needs to be updated
func mutatingWebhookNeedsUpdate(webhook *admissionregistrationv1.MutatingWebhook, caBundle []byte, expectedRules []admissionregistrationv1.RuleWithOperations) bool {
	needUpdate := false

	// Check if CABundle needs to be set or updated (catches both empty and stale values)
	if !bytes.Equal(webhook.ClientConfig.CABundle, caBundle) {
		webhook.ClientConfig.CABundle = caBundle
		needUpdate = true
	}

	// Only reconcile rules for webhooks the operator owns (expectedRules != nil).
	// Chart-defined mirror webhooks keep their own rules.
	if expectedRules != nil && !reflect.DeepEqual(webhook.Rules, expectedRules) {
		webhook.Rules = expectedRules
		needUpdate = true
	}

	return needUpdate
}

// WebhookSecretReadyzCheck is a readyz check for the webhook secret, if it does not exist, it will return an error
// if it exists, it will wait for the secret to be ready, this makes sure that we don't start the operator
// if the webhook secret is not ready
func (r *WebhookController) WebhookSecretReadyzCheck(req *http.Request) error {
	// Inherit the probe's deadline so these reads are cancelled when kubelet gives up on
	// the request, rather than outliving it. controller-runtime always passes a request
	// here; the nil guard is for direct calls in tests.
	ctx := context.Background()
	if req != nil {
		ctx = req.Context()
	}

	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      r.opts.SecretName,
		Namespace: r.namespace,
	}, secret)

	if err != nil {
		return err
	}

	equal, err := compareCertOnDiskToSecret(r.certDir, secret)
	if err != nil {
		return err
	}

	if !equal {
		return fmt.Errorf("webhook secret is not ready")
	}

	// Check EVERY webhook, not just the first. The update path injects the caBundle into
	// all of them, and each is dialled independently by the API server, so a stale bundle
	// on webhook[1] fails those admission requests while a first-entry-only check reports
	// the pod ready.
	validating, err := r.ownedValidatingWebhookConfigurations(ctx)
	if err != nil {
		return err
	}
	for i := range validating {
		for j := range validating[i].Webhooks {
			// Same per-webhook ownership rule the update path uses: readiness must not
			// gate on a webhook this operator deliberately does not write.
			if !r.dialsThisOperator(validating[i].Webhooks[j].ClientConfig.Service) {
				continue
			}
			if !bytes.Equal(validating[i].Webhooks[j].ClientConfig.CABundle, secret.Data["ca.crt"]) {
				return fmt.Errorf("webhook secret is not ready, ca bundle is not equal to webhook %s of ValidatingWebhookConfiguration %s",
					validating[i].Webhooks[j].Name, validating[i].Name)
			}
		}
	}

	mutating, err := r.ownedMutatingWebhookConfigurations(ctx)
	if err != nil {
		return err
	}
	for i := range mutating {
		for j := range mutating[i].Webhooks {
			if !r.dialsThisOperator(mutating[i].Webhooks[j].ClientConfig.Service) {
				continue
			}
			if !bytes.Equal(mutating[i].Webhooks[j].ClientConfig.CABundle, secret.Data["ca.crt"]) {
				return fmt.Errorf("webhook secret is not ready, ca bundle is not equal to webhook %s of MutatingWebhookConfiguration %s",
					mutating[i].Webhooks[j].Name, mutating[i].Name)
			}
		}
	}

	return nil
}
