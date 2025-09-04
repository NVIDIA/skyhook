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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// This project used to use cert-manager to generate the webhook certificates.
// This removes the dependency on cert-manager and simplifies the deployment.
// This also removes the need to have a specific issuer, and just uses a self-signed cert.
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
		if errors.IsAlreadyExists(err) {
			return nil // ignore this special case, it just needs to exist
		}
		return err
	}

	logger.Info("Webhook certificates setup complete")
	return nil
}

// NeedLeaderElection implements the Runnable interface, runs only on leader
func (r *WebhookController) NeedLeaderElection() bool {
	return true
}

func (r *WebhookController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetNamespace() == r.namespace && obj.GetName() == r.opts.SecretName
		}))).
		Complete(r)
}

// permissions
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

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
		if errors.IsNotFound(err) {
			// not found, create it
			webhookCert, err := generateCert(r.opts.ServiceName, r.namespace, 365*24*time.Hour) // TODO: this should be configured
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

func (r *WebhookController) CheckOrUpdateWebhookCertSecret(ctx context.Context, secret *corev1.Secret) (bool, error) {
	equal, err := compareCertOnDiskToSecret(r.certDir, secret)
	if err != nil {
		return false, err
	}

	// check if the secret is going to expire in the next 168 hours or if the cert on disk is different from the secret
	if !equal || secret.Annotations[fmt.Sprintf("%s/expiration", v1alpha1.METADATA_PREFIX)] < time.Now().Add(168*time.Hour).Format(time.RFC3339) {
		// expired, generate a new cert
		webhookCert, err := generateCert(r.opts.ServiceName, r.namespace, 365*24*time.Hour) // TODO: this should be configured
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
		secret.Annotations[fmt.Sprintf("%s/expiration", v1alpha1.METADATA_PREFIX)] = webhookCert.Expiration.Format(time.RFC3339)

		return true, r.Update(ctx, secret)
	}

	return false, nil
}

func (r *WebhookController) CheckOrUpdateWebhookConfigurations(ctx context.Context, secret *corev1.Secret) (bool, error) {
	// Update only CABundle fields of existing webhook configurations created by Helm
	caBundle := secret.Data["ca.crt"]
	changed := false

	// ValidatingWebhookConfiguration
	validatingName := webhookValidatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret).GetName()
	existingValidating := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := r.Get(ctx, types.NamespacedName{Name: validatingName}, existingValidating); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	needUpdate := false
	for i := range existingValidating.Webhooks {
		if len(existingValidating.Webhooks[i].ClientConfig.CABundle) == 0 {
			existingValidating.Webhooks[i].ClientConfig.CABundle = caBundle
			needUpdate = true
		}
	}
	if needUpdate {
		if err := r.Update(ctx, existingValidating); err != nil {
			return false, err
		} else {
			changed = true
		}
	}

	// MutatingWebhookConfiguration
	mutatingName := webhookMutatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret).GetName()
	existingMutating := &admissionregistrationv1.MutatingWebhookConfiguration{}
	if err := r.Get(ctx, types.NamespacedName{Name: mutatingName}, existingMutating); err != nil {
		if errors.IsNotFound(err) {
			return changed, nil
		}
		return false, err
	}

	needUpdate = false
	for i := range existingMutating.Webhooks {
		if len(existingMutating.Webhooks[i].ClientConfig.CABundle) == 0 {
			existingMutating.Webhooks[i].ClientConfig.CABundle = caBundle
			needUpdate = true
		}
	}
	if needUpdate {
		if err := r.Update(ctx, existingMutating); err != nil {
			return false, err
		} else {
			changed = true
		}
	}

	return changed, nil
}

// webhookValidatingWebhookConfiguration returns a new validating webhook configuration.
func webhookValidatingWebhookConfiguration(namespace, serviceName string, secret *corev1.Secret) *admissionregistrationv1.ValidatingWebhookConfiguration {
	conf := admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "skyhook-operator-validating-webhook",
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{
				Name:                    "validate-skyhook.nvidia.com",
				ClientConfig:            webhookClient(serviceName, namespace, "/validate-skyhook-nvidia-com-v1alpha1-skyhook", secret),
				FailurePolicy:           ptr(admissionregistrationv1.Fail),
				Rules:                   webhookRule(),
				SideEffects:             ptr(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	return &conf
}

// webhookMutatingWebhookConfiguration returns a new mutating webhook configuration.
func webhookMutatingWebhookConfiguration(namespace, serviceName string, secret *corev1.Secret) *admissionregistrationv1.MutatingWebhookConfiguration {
	conf := admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "skyhook-operator-mutating-webhook",
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name:                    "mutate-skyhook.nvidia.com",
				ClientConfig:            webhookClient(serviceName, namespace, "/mutate-skyhook-nvidia-com-v1alpha1-skyhook", secret),
				FailurePolicy:           ptr(admissionregistrationv1.Fail),
				Rules:                   webhookRule(),
				SideEffects:             ptr(admissionregistrationv1.SideEffectClassNone),
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	return &conf
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

func webhookClient(serviceName, namespace, path string, secret *corev1.Secret) admissionregistrationv1.WebhookClientConfig {
	return admissionregistrationv1.WebhookClientConfig{
		Service: &admissionregistrationv1.ServiceReference{
			Name:      serviceName,
			Namespace: namespace,
			Path:      ptr(path),
		},
		CABundle: secret.Data["ca.crt"],
	}
}

func webhookRule() []admissionregistrationv1.RuleWithOperations {
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

// WebhookSecretReadyzCheck is a readyz check for the webhook secret, if it does not exist, it will return an error
// if it exists, it will wait for the secret to be ready, this makes sure that we don't start the operator
// if the webhook secret is not ready
func (r *WebhookController) WebhookSecretReadyzCheck(_ *http.Request) error {
	secret := &corev1.Secret{}
	err := r.Client.Get(context.Background(), types.NamespacedName{
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

	// check for the webhook configurations
	validatingWebhookName := webhookValidatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret).GetName()
	validatingWebhookConfiguration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	err = r.Get(context.Background(), types.NamespacedName{Name: validatingWebhookName}, validatingWebhookConfiguration)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("ValidatingWebhookConfiguration %q not found. Either disable webhooks (not recommended) or reinstall the operator via the Helm chart to provision webhooks", validatingWebhookName)
		}
		return err
	}

	if !bytes.Equal(validatingWebhookConfiguration.Webhooks[0].ClientConfig.CABundle, secret.Data["ca.crt"]) {
		return fmt.Errorf("webhook secret is not ready, ca bundle is not equal to the validating webhook configuration")
	}

	mutatingWebhookConfiguration := webhookMutatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret)
	err = r.Get(context.Background(), types.NamespacedName{Name: mutatingWebhookConfiguration.Name}, mutatingWebhookConfiguration)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("MutatingWebhookConfiguration %q not found. Either disable webhooks (not recommended) or reinstall the operator via the Helm chart to provision webhooks", mutatingWebhookConfiguration.Name)
		}
		return err
	}

	if !bytes.Equal(mutatingWebhookConfiguration.Webhooks[0].ClientConfig.CABundle, secret.Data["ca.crt"]) {
		return fmt.Errorf("webhook secret is not ready, ca bundle is not equal to the mutating webhook configuration")
	}

	return nil
}
