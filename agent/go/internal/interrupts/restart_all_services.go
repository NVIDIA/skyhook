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

// RestartAllServices reloads all services.
type RestartAllServices struct{}

var _ Interrupt = RestartAllServices{}

func (RestartAllServices) Type() string { return "restart_all_services" }

func (RestartAllServices) InterruptCmd() [][]string {
	return [][]string{{"service", "procps", "force-reload"}}
}

func (r RestartAllServices) Serialize() ([]byte, error) {
	return marshalTypeOnly(r.Type())
}
