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

package schema

import (
	"fmt"
	"strconv"
	"strings"
)

// SchemaVersion identifies a supported config schema version.
type SchemaVersion string

const (
	// V1 is the v1 schema version.
	V1 SchemaVersion = "v1"
	// Latest is the virtual marker for the highest concrete version.
	Latest SchemaVersion = "latest"
)

// knownConcreteVersions lists every concrete (non-Latest) version.
// Highest derives from this slice, so adding a new version only
// requires appending here.
var knownConcreteVersions = []SchemaVersion{V1}

// String implements fmt.Stringer.
func (v SchemaVersion) String() string {
	return string(v)
}

// Highest returns the highest known concrete schema version.
func Highest() SchemaVersion {
	highest := knownConcreteVersions[0]
	for _, v := range knownConcreteVersions[1:] {
		if Compare(v, highest) > 0 {
			highest = v
		}
	}
	return highest
}

// LatestFrom returns the highest concrete schema version in values.
// Latest entries are ignored, since they are a virtual marker rather
// than a real version.
func LatestFrom(values []SchemaVersion) (SchemaVersion, error) {
	var (
		best    SchemaVersion
		hasBest bool
	)
	for _, value := range values {
		if value == Latest {
			continue
		}
		if !hasBest || Compare(value, best) > 0 {
			best = value
			hasBest = true
		}
	}
	if !hasBest {
		return "", fmt.Errorf("no concrete schema versions in input")
	}
	return best, nil
}

// Compare compares two schema versions.
// Returns -1 if a<b, 0 if a==b, and 1 if a>b. Latest sorts above any
// concrete version.
func Compare(a, b SchemaVersion) int {
	if a == b {
		return 0
	}
	if b == Latest {
		return -1
	}
	if a == Latest {
		return 1
	}

	aNum, _ := parseVersionNumber(a)
	bNum, _ := parseVersionNumber(b)
	if aNum < bNum {
		return -1
	}
	return 1
}

// ParseSchemaVersion parses a string value into a schema version.
func ParseSchemaVersion(value string) (SchemaVersion, error) {
	normalized := SchemaVersion(strings.ToLower(strings.TrimSpace(value)))
	if normalized == Latest {
		return normalized, nil
	}
	if _, err := parseVersionNumber(normalized); err != nil {
		return "", fmt.Errorf("invalid schema version %q: %w", value, err)
	}
	return normalized, nil
}

func parseVersionNumber(v SchemaVersion) (int, error) {
	text := strings.TrimPrefix(strings.ToLower(string(v)), "v")
	return strconv.Atoi(text)
}
