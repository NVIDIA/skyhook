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
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/nodewright/agent/internal/command"
	"github.com/NVIDIA/nodewright/agent/internal/execution"
)

const (
	systemctlCmd = "systemctl"
	restartCmd   = "restart"
)

// ServiceRestart runs systemctl daemon-reload inside the configured root mount,
// then restarts each configured service in order.
type ServiceRestart struct {
	Services []string
}

var _ Interrupt = ServiceRestart{}

func (ServiceRestart) Type() InterruptType { return ServiceRestartType }

func (s ServiceRestart) Run(ctx context.Context, config execution.Config) (execution.Status, error) {
	if err := validateRun(ctx, config, s.Type()); err != nil {
		return execution.StatusFailed, err
	}

	commands := make([]command.Command, 0, 1+len(s.Services))
	commands = append(commands, command.NewCommand(
		systemctlCmd,
		command.WithArguments("daemon-reload"),
		command.WithWorkingDirectory(config.SkyhookDir()),
		command.WithStdout(config.Stdout()),
		command.WithStderr(config.Stderr()),
	))
	for _, service := range s.Services {
		commands = append(commands, command.NewCommand(
			systemctlCmd,
			command.WithArguments(restartCmd, service),
			command.WithWorkingDirectory(config.SkyhookDir()),
			command.WithStdout(config.Stdout()),
			command.WithStderr(config.Stderr()),
		))
	}

	runner := command.NewRunner(command.WithChroot(config.RootMount()))
	for index, cmd := range commands {
		result, runErr := runner.Run(ctx, cmd)
		if runErr != nil {
			return execution.StatusFailed, fmt.Errorf(
				"running interrupt %q command %d %q: %w",
				s.Type(),
				index,
				cmd.Executable,
				runErr,
			)
		}
		if result.Signal != nil || result.ExitCode != command.SuccessExitCode {
			return execution.StatusFailed, nil
		}
	}
	return execution.StatusSuccess, nil
}

func (s ServiceRestart) Serialize() ([]byte, error) {
	// Normalize nil to an empty slice so the wire shape is always
	// "services": [] rather than "services": null.
	services := s.Services
	if services == nil {
		services = []string{}
	}
	return json.Marshal(struct {
		Type     InterruptType `json:"type"`
		Services []string      `json:"services"`
	}{Type: s.Type(), Services: services})
}
