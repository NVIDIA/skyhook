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
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("SecretCertWatcher", Ordered, func() {
	var (
		watcher *SecretCertWatcher
		tmpDir  string
		secret  *corev1.Secret
		cert    *webhookCert
	)

	BeforeAll(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "webhook-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Create a test secret with valid cert data
		cert, err = generateCert("test-service", "test-namespace", 24*time.Hour)
		Expect(err).NotTo(HaveOccurred())

		secret = cert.ToSecret("test-webhook-secret", "test-namespace", "test-service")
	})

	AfterAll(func() {
		_ = os.RemoveAll(tmpDir)
	})

	BeforeEach(func() {
		watcher = NewSecretCertWatcher(
			fake.NewClientBuilder().WithObjects(secret).Build(),
			nil, // Mock cache
			"test-namespace",
			"test-webhook-secret",
			tmpDir,
		)
	})

	It("should sync secret to disk on initial start", func() {
		err := watcher.syncSecretToDisk(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Verify files were written
		certData, err := os.ReadFile(filepath.Join(tmpDir, "tls.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(certData).To(Equal(secret.Data["tls.crt"]))

		keyData, err := os.ReadFile(filepath.Join(tmpDir, "tls.key"))
		Expect(err).NotTo(HaveOccurred())
		Expect(keyData).To(Equal(secret.Data["tls.key"]))
	})

	It("should handle secret updates", func() {
		// Initial sync
		err := watcher.syncSecretToDisk(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Generate new cert
		newCert, err := generateCert("test-service", "test-namespace", 24*time.Hour)
		Expect(err).NotTo(HaveOccurred())

		// Update secret
		secret = newCert.ToSecret("test-webhook-secret", "test-namespace", "test-service")
		err = watcher.client.Update(context.Background(), secret)
		Expect(err).NotTo(HaveOccurred())

		// Simulate update event
		err = watcher.syncSecretToDisk(context.Background())
		Expect(err).NotTo(HaveOccurred())

		// Verify files were updated
		certData, err := os.ReadFile(filepath.Join(tmpDir, "tls.crt"))
		Expect(err).NotTo(HaveOccurred())
		Expect(certData).To(Equal(secret.Data["tls.crt"]))

		keyData, err := os.ReadFile(filepath.Join(tmpDir, "tls.key"))
		Expect(err).NotTo(HaveOccurred())
		Expect(keyData).To(Equal(secret.Data["tls.key"]))
	})

	It("should handle missing secret gracefully", func() {
		// Create watcher with empty client
		watcher.client = fake.NewClientBuilder().Build()

		err := watcher.syncSecretToDisk(context.Background())
		Expect(err).NotTo(HaveOccurred()) // Should not error, just log
	})

	It("should handle filesystem errors", func() {
		// Remove write permissions from directory
		err := os.Chmod(tmpDir, 0o444)
		Expect(err).NotTo(HaveOccurred())

		err = watcher.syncSecretToDisk(context.Background())
		Expect(err).To(HaveOccurred())

		// Restore permissions
		err = os.Chmod(tmpDir, 0o755)
		Expect(err).NotTo(HaveOccurred())
	})
})
