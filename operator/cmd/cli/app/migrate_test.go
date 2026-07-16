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

package app

import (
	"bytes"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/NVIDIA/nodewright/operator/internal/cli/context"
	"sigs.k8s.io/yaml"
)

var _ = Describe("Migrate Command", func() {
	// runMigrate feeds input on stdin via the offline `-f -` path, so no cluster
	// is required.
	runMigrate := func(input string, verbose bool) (string, error) {
		cliCtx := context.NewCLIContext(nil)
		cliCtx.GlobalFlags.Verbose = verbose
		cmd := NewMigrateCmd(cliCtx)
		out := &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(out)
		cmd.SetIn(strings.NewReader(input))
		cmd.SetArgs([]string{"-f", "-"})
		err := cmd.Execute()
		return out.String(), err
	}

	Describe("NewMigrateCmd", func() {
		It("should create command with correct properties", func() {
			cmd := NewMigrateCmd(context.NewCLIContext(nil))
			Expect(cmd.Use).To(Equal("migrate"))
			Expect(cmd.Short).To(ContainSubstring("NodeWright"))

			filenameFlag := cmd.Flags().Lookup("filename")
			Expect(filenameFlag).NotTo(BeNil())
			Expect(filenameFlag.Shorthand).To(Equal("f"))
		})
	})

	Describe("offline conversion via -f -", func() {
		const legacySkyhook = `apiVersion: skyhook.nvidia.com/v1alpha1
kind: Skyhook
metadata:
  name: gpu-init
  resourceVersion: "12345"
  uid: 11111111-2222-3333-4444-555555555555
  creationTimestamp: "2026-01-01T00:00:00Z"
  finalizers:
    - skyhook.nvidia.com/skyhook
  annotations:
    skyhook.nvidia.com/pause: "true"
spec:
  packages:
    tuning:
      version: "1.2.3"
      image: ghcr.io/nvidia/skyhook-packages/shellscript
  interruptionBudget:
    count: 1
status:
  status: complete
`

		const legacyDeploymentPolicy = `apiVersion: skyhook.nvidia.com/v1alpha1
kind: DeploymentPolicy
metadata:
  name: rollout
spec:
  default:
    budget:
      count: 2
`

		It("converts a Skyhook to a NodeWright and strips server-managed fields", func() {
			out, err := runMigrate(legacySkyhook, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(out).To(ContainSubstring("apiVersion: nodewright.nvidia.com/v1alpha1"))
			Expect(out).To(ContainSubstring("kind: NodeWright"))

			// spec is preserved
			Expect(out).To(ContainSubstring("tuning"))
			Expect(out).To(ContainSubstring("1.2.3"))
			Expect(out).To(ContainSubstring("ghcr.io/nvidia/skyhook-packages/shellscript"))

			// prefix-keyed annotations carry over to the new group
			Expect(out).To(ContainSubstring("nodewright.nvidia.com/pause"))

			// server-managed metadata and status are stripped
			Expect(out).NotTo(ContainSubstring("status:"))
			Expect(out).NotTo(ContainSubstring("resourceVersion"))
			Expect(out).NotTo(ContainSubstring("uid:"))
			Expect(out).NotTo(ContainSubstring("creationTimestamp"))
			// the legacy finalizer must not be inherited
			Expect(out).NotTo(ContainSubstring("finalizers"))
		})

		It("emits valid YAML that round-trips through a decoder", func() {
			out, err := runMigrate(legacySkyhook, false)
			Expect(err).NotTo(HaveOccurred())

			var decoded map[string]any
			Expect(yaml.Unmarshal([]byte(out), &decoded)).To(Succeed())
			Expect(decoded["apiVersion"]).To(Equal("nodewright.nvidia.com/v1alpha1"))
			Expect(decoded["kind"]).To(Equal("NodeWright"))
		})

		It("converts a DeploymentPolicy, keeping the kind and moving the group", func() {
			out, err := runMigrate(legacyDeploymentPolicy, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(out).To(ContainSubstring("apiVersion: nodewright.nvidia.com/v1alpha1"))
			Expect(out).To(ContainSubstring("kind: DeploymentPolicy"))
			Expect(out).To(ContainSubstring("rollout"))
		})

		It("converts a multi-document stream and separates docs with ---", func() {
			input := legacySkyhook + "---\n" + legacyDeploymentPolicy
			out, err := runMigrate(input, false)
			Expect(err).NotTo(HaveOccurred())

			Expect(out).To(ContainSubstring("kind: NodeWright"))
			Expect(out).To(ContainSubstring("kind: DeploymentPolicy"))
			Expect(out).To(ContainSubstring("\n---\n"))
		})

		It("passes through unrelated kinds unchanged", func() {
			const configMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: unrelated
data:
  foo: bar
`
			out, err := runMigrate(configMap, true)
			Expect(err).NotTo(HaveOccurred())

			Expect(out).To(ContainSubstring("kind: ConfigMap"))
			Expect(out).To(ContainSubstring("name: unrelated"))
			Expect(out).To(ContainSubstring("Passing through unchanged"))
		})

		It("passes through objects that are already in the nodewright group", func() {
			const alreadyNodeWright = `apiVersion: nodewright.nvidia.com/v1alpha1
kind: NodeWright
metadata:
  name: already
spec:
  packages: {}
`
			out, err := runMigrate(alreadyNodeWright, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("apiVersion: nodewright.nvidia.com/v1alpha1"))
			Expect(out).To(ContainSubstring("name: already"))
		})
	})
})
