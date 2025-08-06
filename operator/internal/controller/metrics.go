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
	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// skyhook metrics
	skyhook_status = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_status",
			Help: "Binary metric indicating the status of the Skyhook Custom Resource (1 if in that status, 0 otherwise)",
		},
		[]string{"skyhook_name", "status"},
	)

	// node metrics
	skyhook_node_status_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_status_count",
			Help: "Number of nodes in the cluster by status for the Skyhook Custom Resource",
		},
		[]string{"skyhook_name", "status"},
	)

	skyhook_node_target_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_target_count",
			Help: "Total number of nodes targeted by this Skyhook Custom Resource",
		},
		[]string{"skyhook_name"},
	)

	// package metrics
	skyhook_package_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_count",
			Help: "Number of nodes in the cluster by state for this package",
		},
		[]string{"skyhook_name", "package_name", "package_version", "state", "stage"},
	)

	skyhook_package_restarts_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_restarts_count",
			Help: "Number of restarts for this package on this node",
		},
		[]string{"skyhook_name", "package_name", "package_version"},
	)
)

func zeroOutSkyhookMetrics(skyhook SkyhookNodes) {
	// Clean up abstracted metrics
	skyhookName := skyhook.GetSkyhook().Name

	// Clean up node status metrics
	for _, status := range v1alpha1.Statuses {
		skyhook_node_status_count.DeleteLabelValues(skyhookName, string(status))
	}

	// Clean up target count metric
	skyhook_node_target_count.DeleteLabelValues(skyhookName)

	// Clean up skyhook state metrics
	for _, state := range v1alpha1.States {
		skyhook_status.DeleteLabelValues(skyhookName, string(state))
	}

	for _, _package := range skyhook.GetSkyhook().Spec.Packages {
		zeroOutSkyhookPackageMetrics(skyhook.GetSkyhook().Name, _package.Name, _package.Version)
	}
}

func zeroOutSkyhookPackageMetrics(skyhookName, packageName, packageVersion string) {
	skyhook_package_restarts_count.DeleteLabelValues(skyhookName, packageName, packageVersion)

	for _, state := range v1alpha1.States {
		for _, stage := range v1alpha1.Stages {
			skyhook_package_count.DeleteLabelValues(skyhookName, packageName, packageVersion, string(state), string(stage))
		}
	}
}

func SetNodeStatusMetrics(skyhookName string, status v1alpha1.Status, count float64) {
	skyhook_node_status_count.WithLabelValues(skyhookName, string(status)).Set(count)
}

func SetSkyhookStatusMetrics(skyhookName string, state v1alpha1.Status, active bool) {
	value := float64(0)
	if active {
		value = 1
	}
	skyhook_status.WithLabelValues(skyhookName, string(state)).Set(value)
}

func SetPackageStateMetrics(skyhookName, packageName, packageVersion string, state v1alpha1.State, stage v1alpha1.Stage, count float64) {
	skyhook_package_count.WithLabelValues(skyhookName, packageName, packageVersion, string(state), string(stage)).Set(count)
}

func SetPackageRestartsMetrics(skyhookName, packageName, packageVersion string, restarts int32) {
	skyhook_package_restarts_count.WithLabelValues(skyhookName, packageName, packageVersion).Set(float64(restarts))
}

func SetNodeTargetCountMetrics(skyhookName string, count float64) {
	skyhook_node_target_count.WithLabelValues(skyhookName).Set(count)
}

func init() {
	metrics.Registry.MustRegister(
		skyhook_status,
		skyhook_node_status_count,
		skyhook_node_target_count,
		skyhook_package_count,
		skyhook_package_restarts_count,
	)
}
