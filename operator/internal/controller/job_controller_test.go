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
	"fmt"

	"github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("job controller", func() {

	It("maps only Jobs we own to job---<name> requests", func() {
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-init-tuning-1-0-0-apply-worker-7",
				Namespace: "skyhook",
				Labels: map[string]string{
					fmt.Sprintf("%s/name", v1alpha1.METADATA_PREFIX): "gpu-init",
				},
			},
		}

		ret := jobHandlerFunc(ctx, job)
		Expect(ret).To(HaveLen(1))
		Expect(ret[0].Name).To(BeEquivalentTo("job---gpu-init-tuning-1-0-0-apply-worker-7"))
		Expect(ret[0].Namespace).To(Equal("skyhook"))

		// a Job without our name label (e.g. a CronJob's Job) is ignored
		job.Labels = map[string]string{"foo": "bar"}
		Expect(jobHandlerFunc(ctx, job)).To(BeNil())
	})
})
