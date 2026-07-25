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
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("MetricsCertController", func() {
	const (
		namespace   = "test-namespace"
		secretName  = "metrics-cert"
		serviceName = "test-metrics-service"
	)

	newController := func(objects ...*corev1.Secret) *MetricsCertController {
		builder := fake.NewClientBuilder()
		for _, object := range objects {
			builder = builder.WithObjects(object)
		}
		return &MetricsCertController{
			Client:    builder.Build(),
			namespace: namespace,
			opts: MetricsCertControllerOptions{
				SecretName:  secretName,
				ServiceName: serviceName,
			},
		}
	}

	It("creates a serving certificate for the metrics service", func() {
		controller := newController()
		secret, err := controller.getOrCreateCertSecret(context.Background())
		Expect(err).NotTo(HaveOccurred())

		block, _ := pem.Decode(secret.Data["tls.crt"])
		Expect(block).NotTo(BeNil())
		certificate, err := x509.ParseCertificate(block.Bytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(certificate.VerifyHostname(serviceName + "." + namespace + ".svc")).To(Succeed())
		Expect(secret.Annotations[serviceAnnotationKey]).To(Equal(serviceName))
	})

	It("rotates an expiring certificate", func() {
		cert, err := generateCert(serviceName, namespace, time.Hour)
		Expect(err).NotTo(HaveOccurred())
		secret := cert.ToSecret(secretName, namespace, serviceName)
		originalCert := string(secret.Data["tls.crt"])
		controller := newController(secret)

		Expect(controller.rotateCertSecretIfNeeded(context.Background(), secret)).To(Succeed())

		updated := &corev1.Secret{}
		Expect(controller.Get(context.Background(), types.NamespacedName{
			Name: secretName, Namespace: namespace,
		}, updated)).To(Succeed())
		Expect(string(updated.Data["tls.crt"])).NotTo(Equal(originalCert))
	})

	It("rotates a certificate when the service name changes", func() {
		cert, err := generateCert("old-service", namespace, certValidityDurationYear)
		Expect(err).NotTo(HaveOccurred())
		secret := cert.ToSecret(secretName, namespace, "old-service")
		controller := newController(secret)

		Expect(controller.rotateCertSecretIfNeeded(context.Background(), secret)).To(Succeed())
		Expect(secret.Annotations[serviceAnnotationKey]).To(Equal(serviceName))
	})
})
