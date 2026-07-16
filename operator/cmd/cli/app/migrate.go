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

// migrate.go bridges the legacy skyhook.nvidia.com API group to the new
// nodewright.nvidia.com group, so it is the one CLI file that legitimately
// imports both api packages: skyhookv1 (legacy, disposable) and nwv1 (new,
// durable). The dual import is expected here and nowhere else.
package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	nwv1 "github.com/NVIDIA/nodewright/operator/api/nodewright/v1alpha1"
	skyhookv1 "github.com/NVIDIA/nodewright/operator/api/v1alpha1"
	"github.com/NVIDIA/nodewright/operator/internal/cli/client"
	cliContext "github.com/NVIDIA/nodewright/operator/internal/cli/context"
)

var (
	legacySkyhookGVR          = skyhookv1.GroupVersion.WithResource("skyhooks")
	legacyDeploymentPolicyGVR = skyhookv1.GroupVersion.WithResource("deploymentpolicies")
)

// migrateOptions holds the options for the migrate command.
type migrateOptions struct {
	filenames []string
}

// NewMigrateCmd creates the migrate command.
func NewMigrateCmd(ctx *cliContext.CLIContext) *cobra.Command {
	opts := &migrateOptions{}

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Convert legacy Skyhook objects to NodeWright YAML",
		Long: `Convert legacy skyhook.nvidia.com objects (Skyhook, DeploymentPolicy) into the
equivalent nodewright.nvidia.com objects and print them as YAML to stdout.

The output is a multi-document YAML stream suitable for 'kubectl apply -f -',
committing to git, or handing to a GitOps tool such as Argo CD. Server-managed
fields (status, resourceVersion, uid, creationTimestamp) are stripped so the
result is a clean, apply-able manifest.

Two input modes:
  - With --filename, objects are read from files (or stdin) and converted
    offline; no cluster connection is required.
  - Without --filename, the legacy Skyhook and DeploymentPolicy objects are
    listed from the current cluster and converted.

Documents that are already nodewright.nvidia.com objects, or an unrelated kind,
are passed through unchanged.`,
		Example: `  # Convert a file and apply the result
  kubectl skyhook migrate -f skyhook.yaml | kubectl apply -f -

  # Convert everything piped in on stdin
  cat skyhooks.yaml | kubectl skyhook migrate -f -

  # Convert every legacy object in the cluster and save for GitOps
  kubectl skyhook migrate > nodewright.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(opts.filenames) > 0 {
				return runMigrateFromFiles(cmd, opts.filenames, ctx)
			}

			clientFactory := client.NewFactory(ctx.GlobalFlags.ConfigFlags)
			kubeClient, err := clientFactory.Client()
			if err != nil {
				return fmt.Errorf("initializing kubernetes client: %w", err)
			}
			return runMigrateFromCluster(cmd.Context(), cmd, kubeClient)
		},
	}

	cmd.Flags().StringSliceVarP(&opts.filenames, "filename", "f", nil,
		"Path to a YAML file with legacy objects (repeatable, comma-separated, or '-' for stdin). Offline; no cluster needed.")

	return cmd
}

// yamlEmitter writes a multi-document YAML stream, inserting a '---' separator
// between documents.
type yamlEmitter struct {
	w     io.Writer
	count int
}

func (e *yamlEmitter) writeDoc(data []byte) error {
	if e.count > 0 {
		if _, err := io.WriteString(e.w, "---\n"); err != nil {
			return fmt.Errorf("writing document separator: %w", err)
		}
	}
	e.count++
	if _, err := e.w.Write(data); err != nil {
		return fmt.Errorf("writing document: %w", err)
	}
	return nil
}

// emitObject marshals a converted object to YAML for apply. Although the shared
// converters carry Status over (the mirror controller needs it), a manifest
// destined for 'kubectl apply' / git must not carry the status subresource or a
// server-managed creationTimestamp, so both are stripped here.
func (e *yamlEmitter) emitObject(obj any) error {
	raw, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("marshaling converted object: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("normalizing converted object: %w", err)
	}
	delete(m, "status")
	if meta, ok := m["metadata"].(map[string]any); ok {
		delete(meta, "creationTimestamp")
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshaling converted object to yaml: %w", err)
	}
	return e.writeDoc(data)
}

// emitRaw passes a document through unchanged, ensuring it ends with a newline.
func (e *yamlEmitter) emitRaw(doc []byte) error {
	if len(doc) > 0 && doc[len(doc)-1] != '\n' {
		doc = append(doc, '\n')
	}
	return e.writeDoc(doc)
}

func runMigrateFromFiles(cmd *cobra.Command, filenames []string, cliCtx *cliContext.CLIContext) error {
	emitter := &yamlEmitter{w: cmd.OutOrStdout()}

	for _, filename := range filenames {
		var reader io.Reader
		if filename == "-" {
			reader = cmd.InOrStdin()
		} else {
			f, err := os.Open(filename)
			if err != nil {
				return fmt.Errorf("opening %q: %w", filename, err)
			}
			// read-only handle: close error is not load-bearing
			defer func() { _ = f.Close() }()
			reader = f
		}

		if err := migrateStream(reader, emitter, cmd, cliCtx); err != nil {
			return fmt.Errorf("migrating %q: %w", filename, err)
		}
	}
	return nil
}

func migrateStream(reader io.Reader, emitter *yamlEmitter, cmd *cobra.Command, cliCtx *cliContext.CLIContext) error {
	yamlReader := utilyaml.NewYAMLReader(bufio.NewReader(reader))
	for {
		doc, err := yamlReader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading yaml document: %w", err)
		}
		if strings.TrimSpace(string(doc)) == "" {
			continue
		}
		if err := migrateDoc(doc, emitter, cmd, cliCtx); err != nil {
			return err
		}
	}
}

func migrateDoc(doc []byte, emitter *yamlEmitter, cmd *cobra.Command, cliCtx *cliContext.CLIContext) error {
	var tm metav1.TypeMeta
	if err := yaml.Unmarshal(doc, &tm); err != nil {
		return fmt.Errorf("decoding document type metadata: %w", err)
	}

	if tm.APIVersion == skyhookv1.GroupVersion.String() {
		switch tm.Kind {
		case "Skyhook":
			var in skyhookv1.Skyhook
			if err := yaml.Unmarshal(doc, &in); err != nil {
				return fmt.Errorf("decoding Skyhook: %w", err)
			}
			var out nwv1.NodeWright
			if err := skyhookv1.Convert_Skyhook_To_NodeWright(&in, &out); err != nil {
				return fmt.Errorf("converting Skyhook %q: %w", in.Name, err)
			}
			return emitter.emitObject(&out)
		case "DeploymentPolicy":
			var in skyhookv1.DeploymentPolicy
			if err := yaml.Unmarshal(doc, &in); err != nil {
				return fmt.Errorf("decoding DeploymentPolicy: %w", err)
			}
			var out nwv1.DeploymentPolicy
			if err := skyhookv1.Convert_DeploymentPolicy_To_NodeWright(&in, &out); err != nil {
				return fmt.Errorf("converting DeploymentPolicy %q: %w", in.Name, err)
			}
			return emitter.emitObject(&out)
		}
	}

	if cliCtx.GlobalFlags.Verbose {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Passing through unchanged: apiVersion=%q kind=%q\n", tm.APIVersion, tm.Kind)
	}
	return emitter.emitRaw(doc)
}

func runMigrateFromCluster(ctx context.Context, cmd *cobra.Command, kubeClient *client.Client) error {
	emitter := &yamlEmitter{w: cmd.OutOrStdout()}

	// The legacy list types (skyhookv1.SkyhookList / skyhookv1.DeploymentPolicyList)
	// have different element and destination types, so each object is decoded and
	// converted through a single generic helper rather than two near-identical loops.
	if err := listConvertEmit(ctx, kubeClient, legacySkyhookGVR, emitter, skyhookv1.Convert_Skyhook_To_NodeWright); err != nil {
		return err
	}
	if err := listConvertEmit(ctx, kubeClient, legacyDeploymentPolicyGVR, emitter, skyhookv1.Convert_DeploymentPolicy_To_NodeWright); err != nil {
		return err
	}

	if emitter.count == 0 {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No legacy Skyhook or DeploymentPolicy objects found in the cluster")
	}
	return nil
}

// listConvertEmit lists a legacy resource, then decodes, converts, and emits each
// object in name order. S is the legacy source type, D the NodeWright-group
// destination type; convert bridges the two.
func listConvertEmit[S, D any](
	ctx context.Context,
	kubeClient *client.Client,
	gvr schema.GroupVersionResource,
	emitter *yamlEmitter,
	convert func(*S, *D) error,
) error {
	list, err := kubeClient.Dynamic().Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing legacy %s: %w", gvr.Resource, err)
	}

	items := list.Items
	sort.Slice(items, func(i, j int) bool { return items[i].GetName() < items[j].GetName() })

	for i := range items {
		name := items[i].GetName()
		raw, err := items[i].MarshalJSON()
		if err != nil {
			return fmt.Errorf("marshaling %s %q: %w", gvr.Resource, name, err)
		}
		var src S
		if err := json.Unmarshal(raw, &src); err != nil {
			return fmt.Errorf("decoding %s %q: %w", gvr.Resource, name, err)
		}
		var dst D
		if err := convert(&src, &dst); err != nil {
			return fmt.Errorf("converting %s %q: %w", gvr.Resource, name, err)
		}
		if err := emitter.emitObject(&dst); err != nil {
			return err
		}
	}
	return nil
}
