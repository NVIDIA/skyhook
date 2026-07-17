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

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// jobHandlerFunc maps Job events into the single reconcile queue as "job---<name>"
// requests, mirroring podHandlerFunc's "pod---<name>" routing so package-stage Jobs are
// serialized through the same SkyhookReconciler pass (the pseudo-controller pattern,
// issue #223 option A). Only Jobs this operator owns — carrying the skyhook name label —
// are enqueued; anything else in the namespace (e.g. a CronJob's Jobs) is ignored.
func jobHandlerFunc(_ context.Context, o client.Object) []reconcile.Request {
	job := o.(*batchv1.Job)

	if labels.Set(job.Labels).Has(fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX)) {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{
			Name:      fmt.Sprintf("job---%s", job.Name), // prefix distinguishes Job events from pod/skyhook events
			Namespace: job.Namespace,
		}}}
	}
	return nil
}
