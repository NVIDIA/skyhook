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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("WebhookController", func() {
	var (
		secretName  string
		namespace   string
		serviceName string
	)

	BeforeEach(func() {
		secretName = "test-webhook-secret"
		namespace = "test-namespace"
		serviceName = "test-service"
	})

	Describe("generateCert", func() {
		It("should generate a valid certificate and key", func() {
			cert, err := generateCert(serviceName, namespace, 24*time.Second)
			Expect(err).ToNot(HaveOccurred())
			Expect(cert).ToNot(BeNil())
			Expect(cert.TLSCert).ToNot(BeEmpty())
			Expect(cert.TLSKey).ToNot(BeEmpty())
			Expect(cert.CABytes).ToNot(BeEmpty())
			Expect(cert.Expiration).ToNot(BeNil())
		})
	})

	Describe("Secret creation for webhook cert", func() {
		It("should create a Secret with the correct data", func() {
			cert, err := generateCert(serviceName, namespace, 24*time.Second)
			Expect(err).ToNot(HaveOccurred())
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
					Annotations: map[string]string{
						"expiration": cert.Expiration.Format(time.RFC3339),
					},
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"ca.crt":  cert.CABytes,
					"tls.crt": []byte(cert.TLSCert),
					"tls.key": []byte(cert.TLSKey),
				},
			}
			Expect(secret).ToNot(BeNil())
			Expect(secret.Data["ca.crt"]).To(Equal(cert.CABytes))
			Expect(secret.Data["tls.crt"]).To(Equal([]byte(cert.TLSCert)))
			Expect(secret.Data["tls.key"]).To(Equal([]byte(cert.TLSKey)))
			Expect(secret.Annotations["expiration"]).ToNot(BeEmpty())
		})
	})

	Describe("webhookValidatingWebhookConfiguration", func() {
		It("should create a ValidatingWebhookConfiguration with the correct CABundle", func() {
			cert, err := generateCert(serviceName, namespace, 24*time.Second)
			Expect(err).ToNot(HaveOccurred())
			secret := &corev1.Secret{
				Data: map[string][]byte{
					"ca.crt": cert.CABytes,
				},
			}
			conf := webhookValidatingWebhookConfiguration(namespace, serviceName, secret)
			Expect(conf).ToNot(BeNil())
			Expect(conf.Webhooks).ToNot(BeEmpty())
			Expect(conf.Webhooks[0].ClientConfig.CABundle).To(Equal(cert.CABytes))
		})
	})

	Describe("webhookMutatingWebhookConfiguration", func() {
		It("should create a MutatingWebhookConfiguration with the correct CABundle", func() {
			cert, err := generateCert(serviceName, namespace, 24*time.Hour)
			Expect(err).ToNot(HaveOccurred())
			secret := &corev1.Secret{
				Data: map[string][]byte{
					"ca.crt": cert.CABytes,
				},
			}
			conf := webhookMutatingWebhookConfiguration(namespace, serviceName, secret)
			Expect(conf).ToNot(BeNil())
			Expect(conf.Webhooks).ToNot(BeEmpty())
			Expect(conf.Webhooks[0].ClientConfig.CABundle).To(Equal(cert.CABytes))
		})
	})

	Describe(" webhook update logic", func() {

		It("should detect CABundle changes and non-changes for validating webhook", func() {
			tests := []struct {
				name          string
				oldCABundles  [][]byte
				newCABundles  [][]byte
				expectChanged bool
			}{
				{
					name:          "different CABundle",
					oldCABundles:  [][]byte{[]byte("old-ca")},
					newCABundles:  [][]byte{[]byte("new-ca")},
					expectChanged: true,
				},
				{
					name:          "same CABundle",
					oldCABundles:  [][]byte{[]byte("same-ca")},
					newCABundles:  [][]byte{[]byte("same-ca")},
					expectChanged: false,
				},
				{
					name:          "multiple webhooks, one changed",
					oldCABundles:  [][]byte{[]byte("ca1"), []byte("ca2")},
					newCABundles:  [][]byte{[]byte("ca1"), []byte("ca3")},
					expectChanged: true,
				},
				{
					name:          "different number of webhooks",
					oldCABundles:  [][]byte{[]byte("ca1")},
					newCABundles:  [][]byte{[]byte("ca1"), []byte("ca2")},
					expectChanged: true,
				},
			}

			for _, tt := range tests {
				oldConf := &admissionregistrationv1.ValidatingWebhookConfiguration{}
				for _, ca := range tt.oldCABundles {
					oldConf.Webhooks = append(oldConf.Webhooks, admissionregistrationv1.ValidatingWebhook{
						ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: ca},
					})
				}
				newConf := &admissionregistrationv1.ValidatingWebhookConfiguration{}
				for _, ca := range tt.newCABundles {
					newConf.Webhooks = append(newConf.Webhooks, admissionregistrationv1.ValidatingWebhook{
						ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: ca},
					})
				}
				changed := compareValidatingWebhookConfigurations(oldConf, newConf)
				Expect(changed).To(Equal(tt.expectChanged), "case: %s", tt.name)
			}
		})
		It("should detect CABundle changes and non-changes for mutating webhook", func() {
			tests := []struct {
				name          string
				oldCABundles  [][]byte
				newCABundles  [][]byte
				expectChanged bool
			}{
				{
					name:          "different CABundle",
					oldCABundles:  [][]byte{[]byte("old-ca")},
					newCABundles:  [][]byte{[]byte("new-ca")},
					expectChanged: true,
				},
				{
					name:          "same CABundle",
					oldCABundles:  [][]byte{[]byte("same-ca")},
					newCABundles:  [][]byte{[]byte("same-ca")},
					expectChanged: false,
				},
				{
					name:          "multiple webhooks, one changed",
					oldCABundles:  [][]byte{[]byte("ca1"), []byte("ca2")},
					newCABundles:  [][]byte{[]byte("ca1"), []byte("ca3")},
					expectChanged: true,
				},
				{
					name:          "different number of webhooks",
					oldCABundles:  [][]byte{[]byte("ca1")},
					newCABundles:  [][]byte{[]byte("ca1"), []byte("ca2")},
					expectChanged: true,
				},
			}

			for _, tt := range tests {
				oldConf := &admissionregistrationv1.MutatingWebhookConfiguration{}
				for _, ca := range tt.oldCABundles {
					oldConf.Webhooks = append(oldConf.Webhooks, admissionregistrationv1.MutatingWebhook{
						ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: ca},
					})
				}
				newConf := &admissionregistrationv1.MutatingWebhookConfiguration{}
				for _, ca := range tt.newCABundles {
					newConf.Webhooks = append(newConf.Webhooks, admissionregistrationv1.MutatingWebhook{
						ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: ca},
					})
				}
				changed := compareMutatingWebhookConfigurations(oldConf, newConf)
				Expect(changed).To(Equal(tt.expectChanged), "case: %s", tt.name)
			}
		})
	})
})
