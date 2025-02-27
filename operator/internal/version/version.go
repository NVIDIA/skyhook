/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package version

import "golang.org/x/mod/semver"

var GIT_SHA string = ""
var VERSION string = ""

// isValid checks if the version is a valid semver version adds a 'v' prefix if missing
func IsValid(version string) bool {
	if version == "" {
		return false
	}
	if version[0] != 'v' {
		version = "v" + version
	}

	return semver.IsValid(version)
}

// Compare compares two versions and returns 0 if they are equal, 1 if version1 is greater than version2, -1 if version1 is less than version2
func Compare(version1, version2 string) int {
	if version1[0] != 'v' {
		version1 = "v" + version1
	}
	if version2[0] != 'v' {
		version2 = "v" + version2
	}
	return semver.Compare(version1, version2)
}

func MajorMinor(version string) string {
	if version == "" {
		return ""
	}
	if version[0] != 'v' {
		version = "v" + version
	}
	return semver.MajorMinor(version)
}
