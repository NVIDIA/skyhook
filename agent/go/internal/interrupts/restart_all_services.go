// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interrupts

import (
	"context"
	"fmt"

	"github.com/NVIDIA/nodewright/agent/internal/command"
	"github.com/NVIDIA/nodewright/agent/internal/execution"
)

// RestartAllServices reloads all services.
type RestartAllServices struct{}

var _ Interrupt = RestartAllServices{}

func (RestartAllServices) Type() InterruptType { return RestartAllServicesType }

func (r RestartAllServices) Run(ctx context.Context, config execution.Config) (execution.Status, error) {
	if err := validateRun(ctx, config, r.Type()); err != nil {
		return execution.StatusFailed, err
	}

	runner := command.NewRunner(command.WithChroot(config.RootMount()))
	cmd := command.NewCommand(
		"service",
		command.WithArguments("procps", "force-reload"),
		command.WithWorkingDirectory(config.SkyhookDir()),
		command.WithStdout(config.Stdout()),
		command.WithStderr(config.Stderr()),
	)
	result, runErr := runner.Run(ctx, cmd)
	if runErr != nil {
		return execution.StatusFailed, fmt.Errorf("running interrupt %q command %q: %w", r.Type(), cmd.Executable, runErr)
	}
	if result.Signal != nil || result.ExitCode != command.SuccessExitCode {
		return execution.StatusFailed, nil
	}
	return execution.StatusSuccess, nil
}

func (r RestartAllServices) Serialize() ([]byte, error) {
	return marshalTypeOnly(r.Type())
}
