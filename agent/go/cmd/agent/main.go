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

package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	// Establish the agent's structured-logging seam. Packages (e.g.
	// config.Loader.Load) log through *slog.Logger and fall back to
	// slog.Default() when passed nil, so wiring it here once keeps that
	// default sane for the whole process.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	fmt.Println("Hello, World!")
}
