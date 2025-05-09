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

package controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/NVIDIA/skyhook/api/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WebhookControllerOptions struct { // prefix these with WEBHOOK_
	SecretName  string `env:"WEBHOOK_SECRET_NAME, default=webhook-cert"`
	ServiceName string `env:"WEBHOOK_SERVICE_NAME, default=skyhook-operator-webhook-service"`
}

type WebhookController struct {
	client.Client
	namespace string
	certDir   string
	opts      WebhookControllerOptions
}

// Start implements the Runnable interface to ensure certificates are set up before the webhook server starts
func (r *WebhookController) Start(ctx context.Context) error {
	logger := log.FromContext(ctx)
	logger.Info("Setting up webhook certificates")

	// Sleep for a short time to ensure cache and client are ready
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}

	// Reconcile the webhook controller will do what we need before the webhook server starts
	_, err := r.Reconcile(ctx, reconcile.Request{})
	if err != nil {
		return err
	}

	logger.Info("Webhook certificates setup complete")
	return nil
}

func NewWebhookController(client client.Client, namespace, certDir string, opts WebhookControllerOptions) *WebhookController {
	return &WebhookController{
		Client:    client,
		namespace: namespace,
		certDir:   certDir,
		opts:      opts,
	}
}

func (r *WebhookController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetNamespace() == r.namespace && obj.GetName() == r.opts.SecretName
		}))).
		Complete(r)
}

func (r *WebhookController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling webhook controller")

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

// secret permissions
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;create;update;patch;delete

// Write cert and key to disk if CertDir is set, make sure the directory exists, and the files are 600. Will also sync from the secret to the disk.
func writeCertAndKey(certPEM, keyPEM []byte, certDir string) error {
	if certDir == "" {
		return nil
	}
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return err
	}
	certPath := filepath.Join(certDir, "tls.crt")
	keyPath := filepath.Join(certDir, "tls.key")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
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

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
					Annotations: map[string]string{
						fmt.Sprintf("%s/service", v1alpha1.METADATA_PREFIX):    r.opts.ServiceName,
						fmt.Sprintf("%s/expiration", v1alpha1.METADATA_PREFIX): webhookCert.Expiration.Format(time.RFC3339), // we can monitor this to trigger a new cert to be created
					},
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"ca.crt":  webhookCert.CABytes,
					"tls.crt": []byte(webhookCert.TLSCert),
					"tls.key": []byte(webhookCert.TLSKey),
				},
			}

			// // set the owner reference to the webhook controller
			// if err := ctrl.SetControllerReference(r, secret, r.Scheme); err != nil {
			// 	return nil, err
			// }

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
	// check if the secret is going to expire in the next 24 hours
	if secret.Annotations[fmt.Sprintf("%s/expiration", v1alpha1.METADATA_PREFIX)] < time.Now().Add(24*time.Hour).Format(time.RFC3339) {
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
	// ValidatingWebhookConfiguration
	validatingWebhookConfiguration := webhookValidatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret)
	existingValidatingWebhookConfiguration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	err := r.Get(ctx, types.NamespacedName{Name: validatingWebhookConfiguration.Name}, existingValidatingWebhookConfiguration)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, r.Create(ctx, validatingWebhookConfiguration)
		}
		return false, err
	} else {
		if compareValidatingWebhookConfigurations(existingValidatingWebhookConfiguration, validatingWebhookConfiguration) {
			existingValidatingWebhookConfiguration.Webhooks = validatingWebhookConfiguration.Webhooks
			return true, r.Update(ctx, existingValidatingWebhookConfiguration)
		}
	}

	// MutatingWebhookConfiguration
	mutatingWebhookConfiguration := webhookMutatingWebhookConfiguration(r.namespace, r.opts.ServiceName, secret)
	existingMutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{}
	err = r.Get(ctx, types.NamespacedName{Name: mutatingWebhookConfiguration.Name}, existingMutatingWebhookConfiguration)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, r.Create(ctx, mutatingWebhookConfiguration)
		}
		return false, err
	} else {
		if compareMutatingWebhookConfigurations(existingMutatingWebhookConfiguration, mutatingWebhookConfiguration) {
			existingMutatingWebhookConfiguration.Webhooks = mutatingWebhookConfiguration.Webhooks
			return true, r.Update(ctx, existingMutatingWebhookConfiguration)
		}
	}

	return false, nil
}

// webhookCert contains the certificate data and expiration for webhook TLS
type webhookCert struct {
	CABytes         []byte
	TLSCert, TLSKey string
	Expiration      time.Time
}

// This project used to use cert-manager to generate the webhook certificates.
// This removes the dependency on cert-manager and simplifies the deployment.

// GenerateCert generates a new CA and a new cert signed by the CA.
// The CA is valid for the given duration.
// The cert is valid for the given duration and has the given DNS names.
func generateCert(serviceName, namespace string, duration time.Duration) (*webhookCert, error) {

	// create a new CA
	serialNumber, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	timeNow := time.Now()
	ca := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"NVIDIA"},
			CommonName:   "webhooks-ca",
		},
		SignatureAlgorithm: x509.SHA256WithRSA,
		NotBefore:          timeNow,
		NotAfter:           timeNow.Add(duration),
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	caCert, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	caPEM := new(bytes.Buffer)
	if err := pem.Encode(caPEM, &pem.Block{Type: "CERTIFICATE", Bytes: caCert}); err != nil {
		return nil, err
	}

	caPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(caPrivKeyPEM, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey)}); err != nil {
		return nil, err
	}

	// create a new cert from the CA
	serialNumber, err = rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	dnsNames := []string{
		fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace),
		fmt.Sprintf("%s.%s.svc", serviceName, namespace),
	}
	cert := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"NVIDIA"},
			CommonName:   dnsNames[0],
		},
		NotBefore:   timeNow,
		NotAfter:    timeNow.Add(duration),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		DNSNames:    dnsNames,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return nil, err
	}

	certPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(certPrivKeyPEM, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey)}); err != nil {
		return nil, err
	}

	return &webhookCert{
		CABytes:    caPEM.Bytes(),
		TLSCert:    certPEM.String(),
		TLSKey:     certPrivKeyPEM.String(),
		Expiration: cert.NotAfter,
	}, nil
}

// validating webhook permissions
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;create;update;patch;delete

// webhookValidatingWebhookConfiguration returns a new validating webhook configuration.
func webhookValidatingWebhookConfiguration(namespace, serviceName string, secret *corev1.Secret) *admissionregistrationv1.ValidatingWebhookConfiguration {
	conf := admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "skyhook-operator-validating-webhook",
			Namespace: namespace,
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
			Name:      "skyhook-operator-mutating-webhook",
			Namespace: namespace,
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
