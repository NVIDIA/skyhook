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

package pkg

import (
	"bytes"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/NVIDIA/skyhook/operator/api/v1alpha1"
	"github.com/NVIDIA/skyhook/operator/internal/cli/context"
)

var _ = Describe("Package Status Command", func() {
	Describe("formatPackageList", func() {
		It("should format single package", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"pkg1": {PackageRef: v1alpha1.PackageRef{Version: "1.0.0"}},
					},
				},
			}
			result := formatPackageList(skyhook)
			Expect(result).To(Equal("pkg1:1.0.0"))
		})

		It("should format multiple packages sorted alphabetically", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"zebra": {PackageRef: v1alpha1.PackageRef{Version: "1.0"}},
						"alpha": {PackageRef: v1alpha1.PackageRef{Version: "2.0"}},
						"beta":  {PackageRef: v1alpha1.PackageRef{Version: "3.0"}},
					},
				},
			}
			result := formatPackageList(skyhook)
			Expect(result).To(Equal("alpha:2.0, beta:3.0, zebra:1.0"))
		})

		It("should handle empty packages", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{},
				},
			}
			result := formatPackageList(skyhook)
			Expect(result).To(BeEmpty())
		})
	})

	Describe("outputJSON", func() {
		It("should output valid JSON", func() {
			statuses := []nodePackageStatus{
				{NodeName: "node1", PackageName: "pkg1", Version: "1.0", Stage: "apply", State: "complete"},
			}
			output := &bytes.Buffer{}

			err := outputJSON(output, statuses)
			Expect(err).NotTo(HaveOccurred())

			var result []nodePackageStatus
			err = json.Unmarshal(output.Bytes(), &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(1))
			Expect(result[0].NodeName).To(Equal("node1"))
		})

		It("should include all fields in JSON output", func() {
			statuses := []nodePackageStatus{
				{
					NodeName:    "node1",
					PackageName: "pkg1",
					Version:     "1.0",
					Stage:       "apply",
					State:       "complete",
					Image:       "nginx:latest",
				},
			}
			output := &bytes.Buffer{}

			err := outputJSON(output, statuses)
			Expect(err).NotTo(HaveOccurred())

			Expect(output.String()).To(ContainSubstring(`"nodeName": "node1"`))
			Expect(output.String()).To(ContainSubstring(`"packageName": "pkg1"`))
			Expect(output.String()).To(ContainSubstring(`"version": "1.0"`))
			Expect(output.String()).To(ContainSubstring(`"stage": "apply"`))
			Expect(output.String()).To(ContainSubstring(`"state": "complete"`))
			Expect(output.String()).To(ContainSubstring(`"image": "nginx:latest"`))
		})

		It("should handle empty statuses", func() {
			output := &bytes.Buffer{}
			err := outputJSON(output, []nodePackageStatus{})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).To(ContainSubstring("[]"))
		})
	})

	Describe("outputTable", func() {
		It("should output table format with headers", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"pkg1": {PackageRef: v1alpha1.PackageRef{Version: "1.0"}},
					},
				},
			}
			skyhook.Name = "my-skyhook"

			statuses := []nodePackageStatus{
				{NodeName: "node1", PackageName: "pkg1", Version: "1.0", Stage: "apply", State: "complete"},
			}
			output := &bytes.Buffer{}

			err := outputTable(output, skyhook, statuses)
			Expect(err).NotTo(HaveOccurred())

			result := output.String()
			Expect(result).To(ContainSubstring("Skyhook: my-skyhook"))
			Expect(result).To(ContainSubstring("NODE"))
			Expect(result).To(ContainSubstring("PACKAGE"))
			Expect(result).To(ContainSubstring("VERSION"))
			Expect(result).To(ContainSubstring("STAGE"))
			Expect(result).To(ContainSubstring("STATE"))
			Expect(result).To(ContainSubstring("node1"))
			Expect(result).To(ContainSubstring("pkg1"))
		})

		It("should include packages header", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"pkg1": {PackageRef: v1alpha1.PackageRef{Version: "1.0"}},
						"pkg2": {PackageRef: v1alpha1.PackageRef{Version: "2.0"}},
					},
				},
			}
			skyhook.Name = "my-skyhook"

			output := &bytes.Buffer{}
			err := outputTable(output, skyhook, []nodePackageStatus{})
			Expect(err).NotTo(HaveOccurred())

			Expect(output.String()).To(ContainSubstring("Packages:"))
		})
	})

	Describe("outputWide", func() {
		It("should include IMAGE column", func() {
			skyhook := &v1alpha1.Skyhook{
				Spec: v1alpha1.SkyhookSpec{
					Packages: map[string]v1alpha1.Package{
						"pkg1": {PackageRef: v1alpha1.PackageRef{Version: "1.0"}},
					},
				},
			}
			skyhook.Name = "my-skyhook"

			statuses := []nodePackageStatus{
				{NodeName: "node1", PackageName: "pkg1", Version: "1.0", Stage: "apply", State: "complete", Image: "nginx:latest"},
			}
			output := &bytes.Buffer{}

			err := outputWide(output, skyhook, statuses)
			Expect(err).NotTo(HaveOccurred())

			result := output.String()
			Expect(result).To(ContainSubstring("IMAGE"))
			Expect(result).To(ContainSubstring("nginx:latest"))
		})
	})

	Describe("NewStatusCmd", func() {
		var statusCmd *cobra.Command

		BeforeEach(func() {
			testCtx := context.NewCLIContext(nil)
			statusCmd = NewStatusCmd(testCtx)
		})

		It("should require --skyhook flag", func() {
			statusCmd.SetArgs([]string{"pkg1"})
			err := statusCmd.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("skyhook"))
		})

		It("should have output flag with shorthand", func() {
			outputFlag := statusCmd.Flags().Lookup("output")
			Expect(outputFlag).NotTo(BeNil())
			Expect(outputFlag.Shorthand).To(Equal("o"))
			Expect(outputFlag.DefValue).To(Equal("table"))
		})

		It("should have node flag", func() {
			nodeFlag := statusCmd.Flags().Lookup("node")
			Expect(nodeFlag).NotTo(BeNil())
		})
	})
})
