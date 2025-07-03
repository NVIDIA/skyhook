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
	"github.com/NVIDIA/skyhook/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	skyhook_node_target_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_target_count",
			Help: "Number of nodes in the cluster that the Skyhook Custom Resource is targeting",
		},
		[]string{"skyhook_name"},
	)

	skyhook_node_in_progress_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_in_progress_count",
			Help: "Number of nodes in the cluster that the Skyhook Custom Resource is currently working on",
		},
		[]string{"skyhook_name"},
	)

	skyhook_node_complete_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_complete_count",
			Help: "Number of nodes in the cluster that the Skyhook Custom Resource has finished working on",
		},
		[]string{"skyhook_name"},
	)

	skyhook_node_error_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_error_count",
			Help: "Number of nodes in the cluster that the Skyhook Custom Resource is erroring on",
		},
		[]string{"skyhook_name"},
	)

	skyhook_node_blocked_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_blocked_count",
			Help: "Number of nodes in the cluster that the Skyhook Custom Resource is blocked",
		},
		[]string{"skyhook_name"},
	)

	skyhook_complete_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_complete_count",
			Help: "A binary metric that is 1 if the Skyhook Custom Resource is complete, 0 otherwise",
		},
		[]string{"skyhook_name"},
	)

	skyhook_paused_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_paused_count",
			Help: "A binary metric that is 1 if the Skyhook Custom Resource is paused, 0 otherwise",
		},
		[]string{"skyhook_name"},
	)

	skyhook_disabled_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_disabled_count",
			Help: "A binary metric that is 1 if the Skyhook Custom Resource is disabled, 0 otherwise",
		},
		[]string{"skyhook_name"},
	)

	skyhook_package_in_progress_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_in_progress_count",
			Help: "Number of nodes in the cluster that are in progress for this package",
		},
		[]string{"skyhook_name", "package_name", "package_version"},
	)

	skyhook_package_error_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_error_count",
			Help: "Number of nodes in the cluster that have failed to apply this package",
		},
		[]string{"skyhook_name", "package_name", "package_version"},
	)

	skyhook_package_complete_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_complete_count",
			Help: "Number of nodes in the cluster that have applied this package",
		},
		[]string{"skyhook_name", "package_name", "package_version"},
	)

	skyhook_package_restarts_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_restarts_count",
			Help: "Number of restarts for this package",
		},
		[]string{"skyhook_name", "package_name", "package_version"},
	)

	// This should maybe a counter but ensuring the decrement is done correctly is tricky
	skyhook_package_stage_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_package_stage_count",
			Help: "Number of nodes in the cluster that are in this stage for this package",
		},
		[]string{"skyhook_name", "package_name", "package_version", "stage"},
	)

	skyhook_node_taint_tolerance_issue_count = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "skyhook_node_taint_tolerance_issue_count",
			Help: "Number of nodes in the cluster that have taint tolerance issues",
		},
		[]string{"skyhook_name"},
	)
)

func zeroOutSkyhookMetrics(skyhook SkyhookNodes) {
	skyhook_complete_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_paused_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_disabled_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_target_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_in_progress_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_complete_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_error_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_blocked_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	skyhook_node_taint_tolerance_issue_count.DeleteLabelValues(skyhook.GetSkyhook().Name)
	for _, _package := range skyhook.GetSkyhook().Spec.Packages {
		zeroOutSkyhookPackageMetrics(skyhook.GetSkyhook().Name, _package.Name, _package.Version)
	}
}

func zeroOutSkyhookPackageMetrics(skyhookName, packageName, packageVersion string) {
	skyhook_package_in_progress_count.DeleteLabelValues(skyhookName, packageName, packageVersion)
	skyhook_package_error_count.DeleteLabelValues(skyhookName, packageName, packageVersion)
	skyhook_package_complete_count.DeleteLabelValues(skyhookName, packageName, packageVersion)
	skyhook_package_restarts_count.DeleteLabelValues(skyhookName, packageName, packageVersion)
	for _, stage := range v1alpha1.Stages {
		skyhook_package_stage_count.DeleteLabelValues(skyhookName, packageName, packageVersion, string(stage))
	}
}

func init() {
	metrics.Registry.MustRegister(
		skyhook_node_target_count,
		skyhook_node_in_progress_count,
		skyhook_node_complete_count,
		skyhook_node_error_count,
		skyhook_node_blocked_count,
		skyhook_complete_count,
		skyhook_paused_count,
		skyhook_disabled_count,
		skyhook_package_in_progress_count,
		skyhook_package_error_count,
		skyhook_package_complete_count,
		skyhook_package_stage_count,
		skyhook_package_restarts_count,
		skyhook_node_taint_tolerance_issue_count,
	)
}
