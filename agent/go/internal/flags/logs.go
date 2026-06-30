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

package flags

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultLogRetention = 5

type logCandidate struct {
	path      string
	timestamp time.Time
}

// CleanupOldLogs removes all but the newest keep regular files matching the
// literal filename shape. Files with equal timestamps are ordered by path so
// retention is deterministic.
func CleanupOldLogs(logFiles LogFiles, keep int) error {
	if keep < 0 {
		return fmt.Errorf("log retention must not be negative: %d", keep)
	}
	entries, err := os.ReadDir(logFiles.directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading log directory %q: %w", logFiles.directory, err)
	}

	files := make([]logCandidate, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, logFiles.prefix) || !strings.HasSuffix(name, logFiles.suffix) {
			continue
		}
		timestamp := strings.TrimSuffix(strings.TrimPrefix(name, logFiles.prefix), logFiles.suffix)
		parsedTimestamp, err := time.Parse(logTimeFormat, timestamp)
		if err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stating log file %q: %w", filepath.Join(logFiles.directory, name), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		files = append(files, logCandidate{
			path:      filepath.Join(logFiles.directory, name),
			timestamp: parsedTimestamp,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].timestamp.Equal(files[j].timestamp) {
			return files[i].path < files[j].path
		}
		return files[i].timestamp.After(files[j].timestamp)
	})
	for _, file := range files[min(keep, len(files)):] {
		if err := os.Remove(file.path); err != nil {
			return fmt.Errorf("removing old log %q: %w", file.path, err)
		}
	}
	return nil
}
