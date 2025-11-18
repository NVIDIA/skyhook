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

package version

import "fmt"

// Version and GitSHA are injected at build time via -ldflags.
// They default to "dev" and "unknown" respectively for local builds.
var (
	Version = "dev"
	GitSHA  = "unknown"
)

// Summary returns a human-friendly representation of the build metadata.
// It shows both version and git SHA when available, or just the version if SHA is missing.
func Summary() string {
	if GitSHA == "" || GitSHA == "unknown" {
		return Version
	}
	return fmt.Sprintf("%s (%s)", Version, GitSHA)
}
