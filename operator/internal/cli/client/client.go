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

// Package client provides Kubernetes client functionality for the Skyhook CLI.
//
// This package is separate from the main cli package to avoid import cycles and to
// provide a clean separation between CLI command structure and Kubernetes API access.
package client

import (
	"fmt"
	"sync"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const userAgent = "skyhook-plugin"

// Client wraps the Kubernetes clients that the plugin uses to interact with clusters.
//
// It provides two types of clients:
//   - Kubernetes (typed): For standard K8s resources (Pods, Deployments, Nodes, etc.)
//   - Dynamic (untyped): For Custom Resources like Skyhook CRs that don't have Go types
type Client struct {
	restConfig    *rest.Config
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
}

// New builds a new Client from the provided ConfigFlags.
func New(flags *genericclioptions.ConfigFlags) (*Client, error) {
	if flags == nil {
		return nil, fmt.Errorf("config flags are required")
	}

	restConfig, err := flags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("building rest config: %w", err)
	}

	cfg := rest.CopyConfig(restConfig)
	if cfg.UserAgent == "" {
		cfg.UserAgent = fmt.Sprintf("%s %s", rest.DefaultKubernetesUserAgent(), userAgent)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}

	return &Client{
		restConfig:    cfg,
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
	}, nil
}

// Kubernetes returns the typed Kubernetes clientset.
func (c *Client) Kubernetes() kubernetes.Interface {
	return c.kubeClient
}

// Dynamic returns the dynamic client.
func (c *Client) Dynamic() dynamic.Interface {
	return c.dynamicClient
}

// RESTConfig returns a copy of the REST configuration used by the client.
func (c *Client) RESTConfig() *rest.Config {
	return rest.CopyConfig(c.restConfig)
}

// Factory lazily constructs shared clients for commands that only need one instance per execution.
type Factory struct {
	flags  *genericclioptions.ConfigFlags
	once   sync.Once
	client *Client
	err    error
}

// NewFactory returns a new Factory.
func NewFactory(flags *genericclioptions.ConfigFlags) *Factory {
	return &Factory{flags: flags}
}

// Client returns a singleton Client instance built from the factory's flags.
func (f *Factory) Client() (*Client, error) {
	f.once.Do(func() {
		f.client, f.err = New(f.flags)
	})
	return f.client, f.err
}

// Reset clears the cached client (mainly useful for tests).
func (f *Factory) Reset() {
	f.once = sync.Once{}
	f.client = nil
	f.err = nil
}
