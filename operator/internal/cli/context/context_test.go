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

package context

import (
	"bytes"
	gocontext "context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context CLI Tests Suite")
}

var _ = Describe("CLI Context", func() {
	Describe("GlobalFlags", func() {
		Describe("NewGlobalFlags", func() {
			It("should initialize with default namespace", func() {
				flags := NewGlobalFlags()
				Expect(flags.ConfigFlags.Namespace).NotTo(BeNil())
				Expect(*flags.ConfigFlags.Namespace).To(Equal("nodewright"))
			})

			It("should initialize with default output format", func() {
				flags := NewGlobalFlags()
				Expect(flags.OutputFormat).To(Equal("table"))
			})
		})

		Describe("AddFlags", func() {
			It("should register output flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				outputFlag := flagset.Lookup("output")
				Expect(outputFlag).NotTo(BeNil())
				Expect(outputFlag.Shorthand).To(Equal("o"))
			})

			It("should register verbose flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				verboseFlag := flagset.Lookup("verbose")
				Expect(verboseFlag).NotTo(BeNil())
				Expect(verboseFlag.Shorthand).To(Equal("v"))
			})

			It("should register dry-run flag", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				dryRunFlag := flagset.Lookup("dry-run")
				Expect(dryRunFlag).NotTo(BeNil())
			})

			It("should bind flags to struct fields", func() {
				flags := NewGlobalFlags()
				flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
				flags.AddFlags(flagset)

				Expect(flagset.Set("output", "json")).To(Succeed())
				Expect(flagset.Set("verbose", "true")).To(Succeed())
				Expect(flagset.Set("dry-run", "true")).To(Succeed())

				Expect(flags.OutputFormat).To(Equal("json"))
				Expect(flags.Verbose).To(BeTrue())
				Expect(flags.DryRun).To(BeTrue())
			})
		})

		Describe("Validate", func() {
			It("should accept valid output formats", func() {
				for _, format := range []string{"json", "yaml", "table", "wide"} {
					flags := NewGlobalFlags()
					flags.OutputFormat = format
					Expect(flags.Validate()).To(Succeed())
				}
			})

			It("should be case insensitive", func() {
				flags := NewGlobalFlags()
				flags.OutputFormat = "JSON"
				Expect(flags.Validate()).To(Succeed())
				Expect(flags.OutputFormat).To(Equal("json"))
			})

			It("should reject invalid output formats", func() {
				flags := NewGlobalFlags()
				flags.OutputFormat = "invalid"
				err := flags.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid output format"))
			})
		})

		Describe("Namespace", func() {
			It("should return default namespace when not set", func() {
				flags := NewGlobalFlags()
				Expect(flags.Namespace()).To(Equal("nodewright"))
			})

			It("should return custom namespace when set", func() {
				flags := NewGlobalFlags()
				ns := "custom-ns"
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("custom-ns"))
			})

			It("should return default namespace for empty string", func() {
				flags := NewGlobalFlags()
				ns := ""
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("nodewright"))
			})

			It("should return default namespace for whitespace", func() {
				flags := NewGlobalFlags()
				ns := "  "
				flags.ConfigFlags.Namespace = &ns
				Expect(flags.Namespace()).To(Equal("nodewright"))
			})

			It("should return default namespace when nil", func() {
				flags := NewGlobalFlags()
				flags.ConfigFlags.Namespace = nil
				Expect(flags.Namespace()).To(Equal("nodewright"))
			})
		})
	})

	Describe("CLIConfig", func() {
		It("should create config with default writers", func() {
			config := NewCLIConfig()
			Expect(config.OutputWriter).NotTo(BeNil())
			Expect(config.ErrorWriter).NotTo(BeNil())
		})

		It("should allow custom output writer", func() {
			buf := &bytes.Buffer{}
			config := NewCLIConfig(WithOutputWriter(buf))
			Expect(config.OutputWriter).To(Equal(buf))
		})

		It("should allow custom error writer", func() {
			buf := &bytes.Buffer{}
			config := NewCLIConfig(WithErrorWriter(buf))
			Expect(config.ErrorWriter).To(Equal(buf))
		})
	})

	Describe("CLIContext", func() {
		It("should create context with default config when nil", func() {
			ctx := NewCLIContext(nil)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.GlobalFlags).NotTo(BeNil())
			Expect(ctx.Config()).NotTo(BeNil())
		})

		It("should create context with provided config", func() {
			config := NewCLIConfig()
			ctx := NewCLIContext(config)
			Expect(ctx.Config()).To(Equal(config))
		})

		Describe("ResolveNamespace", func() {
			operatorIn := func(namespace string) *appsv1.Deployment {
				return &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "controller-manager",
						Namespace: namespace,
						Labels:    map[string]string{"control-plane": "controller-manager"},
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "ghcr.io/nvidia/nodewright/operator:v1"}}},
						},
					},
				}
			}

			// The command must have the flag registered for Changed() to be meaningful.
			newCmd := func(cliCtx *CLIContext, args ...string) (*cobra.Command, *bytes.Buffer) {
				stderr := &bytes.Buffer{}
				cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
				cliCtx.GlobalFlags.AddFlags(cmd.Flags())
				cmd.SetErr(stderr)
				// Must be non-nil: cobra falls back to os.Args (which carries go test's
				// own -test.* flags) when SetArgs is given nil.
				cmd.SetArgs(append([]string{}, args...))
				Expect(cmd.Execute()).To(Succeed())
				return cmd, stderr
			}

			It("honors an explicit --namespace without touching the cluster", func() {
				cliCtx := NewCLIContext(nil)
				cmd, stderr := newCmd(cliCtx, "--namespace", "team-platform")

				Expect(cliCtx.ResolveNamespace(gocontext.Background(), cmd, fake.NewClientset(operatorIn("nodewright")))).
					To(Equal("team-platform"))
				Expect(stderr.String()).To(BeEmpty())
			})

			It("discovers the nodewright namespace without a note", func() {
				cliCtx := NewCLIContext(nil)
				cmd, stderr := newCmd(cliCtx)

				Expect(cliCtx.ResolveNamespace(gocontext.Background(), cmd, fake.NewClientset(operatorIn("nodewright")))).
					To(Equal("nodewright"))
				Expect(stderr.String()).To(BeEmpty())
			})

			It("discovers a legacy skyhook install and notes it once", func() {
				cliCtx := NewCLIContext(nil)
				cmd, stderr := newCmd(cliCtx)
				kube := fake.NewClientset(operatorIn("skyhook"))

				Expect(cliCtx.ResolveNamespace(gocontext.Background(), cmd, kube)).To(Equal("skyhook"))
				Expect(stderr.String()).To(ContainSubstring("legacy \"skyhook\" namespace"))

				// Cached: a second call must not re-probe or re-warn.
				stderr.Reset()
				Expect(cliCtx.ResolveNamespace(gocontext.Background(), cmd, kube)).To(Equal("skyhook"))
				Expect(stderr.String()).To(BeEmpty())
			})

			It("falls back to the default when no operator is found", func() {
				cliCtx := NewCLIContext(nil)
				cmd, stderr := newCmd(cliCtx)

				Expect(cliCtx.ResolveNamespace(gocontext.Background(), cmd, fake.NewClientset())).To(Equal("nodewright"))
				Expect(stderr.String()).To(BeEmpty())
			})
		})
	})
})
