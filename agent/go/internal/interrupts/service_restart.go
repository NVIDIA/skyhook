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

import "encoding/json"

const (
	systemctlCmd = "systemctl"
	restartCmd   = "restart"
)

// ServiceRestart restarts specific services.
type ServiceRestart struct {
	Services []string
}

var _ Interrupt = ServiceRestart{}

func (ServiceRestart) Type() string { return "service_restart" }

func (s ServiceRestart) InterruptCmd() [][]string {
	cmd := make([][]string, 0, 1+len(s.Services))
	cmd = append(cmd, []string{systemctlCmd, "daemon-reload"})
	for _, service := range s.Services {
		cmd = append(cmd, []string{systemctlCmd, restartCmd, service})
	}
	return cmd
}

func (s ServiceRestart) Serialize() ([]byte, error) {
	// Normalize nil to an empty slice so the wire shape is always
	// "services": [] rather than "services": null.
	services := s.Services
	if services == nil {
		services = []string{}
	}
	return json.Marshal(struct {
		Type     string   `json:"type"`
		Services []string `json:"services"`
	}{Type: s.Type(), Services: services})
}
