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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	runtimecache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MetricsCertControllerOptions struct {
	SecretName  string `env:"METRICS_SECRET_NAME, default=metrics-cert"`
	ServiceName string `env:"METRICS_SERVICE_NAME, default=skyhook-operator-controller-manager-metrics-service"`
}

type MetricsCertController struct {
	client.Client
	cache     runtimecache.Cache
	namespace string
	opts      MetricsCertControllerOptions
}

func NewMetricsCertController(client client.Client, cache runtimecache.Cache, namespace string, opts MetricsCertControllerOptions) *MetricsCertController {
	return &MetricsCertController{Client: client, cache: cache, namespace: namespace, opts: opts}
}

func (r *MetricsCertController) Start(ctx context.Context) error {
	if synced := r.cache.WaitForCacheSync(ctx); !synced {
		return fmt.Errorf("waiting for metrics certificate cache sync")
	}
	if _, err := r.getOrCreateCertSecret(ctx); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating metrics certificate secret: %w", err)
	}
	return nil
}

func (r *MetricsCertController) NeedLeaderElection() bool {
	return true
}

func (r *MetricsCertController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("metrics-certificate").
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			return obj.GetNamespace() == r.namespace && obj.GetName() == r.opts.SecretName
		}))).
		Complete(r)
}

//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func (r *MetricsCertController) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	secret, err := r.getOrCreateCertSecret(ctx)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting or creating metrics certificate secret: %w", err)
	}
	if err := r.rotateCertSecretIfNeeded(ctx, secret); err != nil {
		return reconcile.Result{}, fmt.Errorf("rotating metrics certificate secret: %w", err)
	}
	return reconcile.Result{RequeueAfter: 24 * time.Hour}, nil
}

func (r *MetricsCertController) getOrCreateCertSecret(ctx context.Context) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: r.opts.SecretName, Namespace: r.namespace}
	if err := r.Get(ctx, key, secret); err != nil {
		if !errors.IsNotFound(err) {
			return nil, fmt.Errorf("getting metrics certificate secret %s: %w", key, err)
		}
		cert, err := generateCert(r.opts.ServiceName, r.namespace, certValidityDurationYear)
		if err != nil {
			return nil, fmt.Errorf("generating metrics certificate: %w", err)
		}
		secret = cert.ToSecret(r.opts.SecretName, r.namespace, r.opts.ServiceName)
		if err := r.Create(ctx, secret); err != nil {
			return nil, fmt.Errorf("creating metrics certificate secret %s: %w", key, err)
		}
	}
	return secret, nil
}

func (r *MetricsCertController) rotateCertSecretIfNeeded(ctx context.Context, secret *corev1.Secret) error {
	expiration, err := time.Parse(time.RFC3339, secret.Annotations[expirationAnnotationKey])
	serviceName := secret.Annotations[serviceAnnotationKey]
	if err == nil && expiration.After(time.Now().Add(certRotationThreshold)) && serviceName == r.opts.ServiceName {
		return nil
	}

	cert, err := generateCert(r.opts.ServiceName, r.namespace, certValidityDurationYear)
	if err != nil {
		return fmt.Errorf("generating replacement metrics certificate: %w", err)
	}
	replacement := cert.ToSecret(secret.Name, secret.Namespace, r.opts.ServiceName)
	secret.Data = replacement.Data
	secret.Annotations = replacement.Annotations
	if err := r.Update(ctx, secret); err != nil {
		return fmt.Errorf("updating metrics certificate secret: %w", err)
	}
	return nil
}
