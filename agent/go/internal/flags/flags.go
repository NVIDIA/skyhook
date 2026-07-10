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
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/NVIDIA/nodewright/agent/internal/config"
	"github.com/NVIDIA/nodewright/agent/internal/stage"
	"github.com/NVIDIA/nodewright/agent/internal/step"
)

const flagFileMode = 0o600

// Reason explains why a step should run or be skipped.
type Reason string

const (
	ReasonFlagMissing         Reason = "flag-missing"
	ReasonAlwaysRun           Reason = "always-run"
	ReasonStageAlwaysRuns     Reason = "stage-always-runs"
	ReasonIdempotenceDisabled Reason = "idempotence-disabled"
	ReasonAlreadyCompleted    Reason = "already-completed"
)

// Decision is the result of checking a step's completion flag.
type Decision struct {
	Run    bool
	Reason Reason
}

// Store manages completion and control flags for one package.
//
// A flag is a small marker file the agent writes on the host after a step
// finishes successfully. On later runs the agent checks for the flag to decide
// whether the step already completed and can be skipped, which is how package
// execution stays idempotent across agent restarts and node reboots. Flag
// filenames embed a fingerprint of the step's execution-relevant inputs (path,
// arguments, return codes, environment, host execution), so changing any of
// those produces a new flag and the step runs again. Flags live beneath the
// package state directory on the mounted host root, grouped by package name
// and version.
type Store interface {
	Path(value step.Step) (string, error)
	Mark(value step.Step, message string) (string, error)
	Write(path string, data []byte) error
	Check(value step.Step, alwaysRun bool, currentStage stage.Stage) (Decision, error)
}

// NewStore returns the file-backed flag store. The implementation is
// unexported so downstream consumers depend on the Store interface.
func NewStore(layout Layout, cfg config.Config) (Store, error) {
	s, err := newFileStore(layout, cfg)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// fileStore keeps one package's flags under a host layout.
type fileStore struct {
	rootMount      string
	dir            string
	packageName    string
	packageVersion string
}

var _ Store = (*fileStore)(nil)

// newFileStore returns a store for the package described by cfg. Package
// identity is validated once here so later flag paths cannot escape the
// package's flag directory.
func newFileStore(layout Layout, cfg config.Config) (*fileStore, error) {
	if err := validatePackagePath(cfg); err != nil {
		return nil, fmt.Errorf("validating package path: %w", err)
	}
	return &fileStore{
		rootMount:      layout.rootMount,
		dir:            filepath.Join(layout.stateRoot, "flags"),
		packageName:    cfg.PackageName,
		packageVersion: cfg.PackageVersion,
	}, nil
}

// Path returns the completion-flag path for a step. The filename contains a
// stable SHA-256 fingerprint of the step path, arguments, and accepted return
// codes so any execution-relevant change gets a distinct flag.
func (s *fileStore) Path(value step.Step) (string, error) {
	if value == nil {
		return "", errors.New("building flag path: step must not be nil")
	}
	if !filepath.IsLocal(value.Path()) {
		return "", fmt.Errorf("building flag path: step path %q must be relative to the package root", value.Path())
	}

	marker, err := value.Fingerprint()
	if err != nil {
		return "", fmt.Errorf("building flag path: fingerprinting step %q: %w", value.Path(), err)
	}
	filename := filepath.Base(value.Path()) + "-" + marker + ".flag"
	return filepath.Join(s.rootMount, s.dir, s.packageName, s.packageVersion, filename), nil
}

// Mark records successful completion and returns the flag path written.
func (s *fileStore) Mark(value step.Step, message string) (string, error) {
	path, err := s.Path(value)
	if err != nil {
		return "", fmt.Errorf("marking step: resolving flag path: %w", err)
	}
	if err := s.Write(path, []byte(message)); err != nil {
		return "", fmt.Errorf("marking step %q: %w", value.Path(), err)
	}
	return path, nil
}

// Write creates or replaces a flag file owned by this store. In addition to
// per-step flags, callers can use it for control files such as START and
// check_results without duplicating directory and permission handling.
func (s *fileStore) Write(path string, data []byte) (retErr error) {
	relative, err := s.rootRelativePath(path)
	if err != nil {
		return fmt.Errorf("validating flag store path %q: %w", path, err)
	}
	root, err := os.OpenRoot(s.rootMount)
	if err != nil {
		return fmt.Errorf("opening mounted host root %q: %w", s.rootMount, err)
	}
	defer closeStoreRoot(root, &retErr)

	if err := ensureDirectoriesNoSymlinks(root, filepath.Dir(relative)); err != nil {
		return fmt.Errorf("preparing flag directory for %q: %w", path, err)
	}
	info, exists, err := lstatNoSymlinks(root, relative)
	if err != nil {
		return fmt.Errorf("validating flag target %q: %w", path, err)
	}
	if exists && !info.Mode().IsRegular() {
		return fmt.Errorf("flag path %q is not a regular file", path)
	}
	if err := writeFlagFile(root, relative, data, exists); err != nil {
		return fmt.Errorf("writing flag %q: %w", path, err)
	}
	return nil
}

// Check reports whether a step should run based on its completion flag and
// current execution policy.
func (s *fileStore) Check(value step.Step, alwaysRun bool, currentStage stage.Stage) (decision Decision, retErr error) {
	if value == nil {
		return Decision{}, errors.New("checking flag: step must not be nil")
	}
	if _, err := stage.ParseStage(string(currentStage)); err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: validating stage: %w", value.Path(), err)
	}
	if err := value.Idempotence().Validate(); err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: validating idempotence: %w", value.Path(), err)
	}
	path, err := s.Path(value)
	if err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: resolving store path: %w", value.Path(), err)
	}
	relative, err := s.rootRelativePath(path)
	if err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: validating store path: %w", value.Path(), err)
	}
	root, err := os.OpenRoot(s.rootMount)
	if err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: opening mounted host root %q: %w", value.Path(), s.rootMount, err)
	}
	defer closeStoreRoot(root, &retErr)

	info, exists, err := lstatNoSymlinks(root, relative)
	if err != nil {
		return Decision{}, fmt.Errorf("checking flag for step %q: inspecting store path: %w", value.Path(), err)
	}
	if !exists {
		return Decide(false, alwaysRun, currentStage, value.Idempotence()), nil
	}
	if !info.Mode().IsRegular() {
		return Decision{}, fmt.Errorf("checking flag for step %q: flag path %q is not a regular file", value.Path(), path)
	}
	return Decide(true, alwaysRun, currentStage, value.Idempotence()), nil
}

// Decide applies flag policy without filesystem access.
func Decide(flagExists, alwaysRun bool, currentStage stage.Stage, idempotence step.Idempotence) Decision {
	if !flagExists {
		return Decision{Run: true, Reason: ReasonFlagMissing}
	}
	if alwaysRun {
		return Decision{Run: true, Reason: ReasonAlwaysRun}
	}
	switch currentStage {
	case stage.Config, stage.Uninstall, stage.Upgrade:
		return Decision{Run: true, Reason: ReasonStageAlwaysRuns}
	}
	if idempotence == step.Disabled {
		return Decision{Run: true, Reason: ReasonIdempotenceDisabled}
	}
	return Decision{Run: false, Reason: ReasonAlreadyCompleted}
}

func (s *fileStore) rootRelativePath(path string) (string, error) {
	storePath := filepath.Join(s.rootMount, s.dir)
	relative, err := filepath.Rel(storePath, path)
	if err != nil {
		return "", fmt.Errorf("resolving path relative to flag store: %w", err)
	}
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("flag path %q must be contained within %q", path, storePath)
	}
	return filepath.Join(s.dir, relative), nil
}

func ensureDirectoriesNoSymlinks(root *os.Root, directory string) error {
	current := ""
	for _, component := range pathComponents(directory) {
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := root.Mkdir(current, directoryMode); err != nil && !errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("creating directory %q: %w", current, err)
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspecting directory %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symbolic link", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("path component %q is not a directory", current)
		}
	}
	return nil
}

func lstatNoSymlinks(root *os.Root, path string) (os.FileInfo, bool, error) {
	components := pathComponents(path)
	current := ""
	for i, component := range components {
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("inspecting path component %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("path component %q is a symbolic link", current)
		}
		if i < len(components)-1 && !info.IsDir() {
			return nil, false, fmt.Errorf("path component %q is not a directory", current)
		}
		if i == len(components)-1 {
			return info, true, nil
		}
	}
	return nil, false, nil
}

func pathComponents(path string) []string {
	clean := filepath.Clean(path)
	if clean == "." {
		return nil
	}
	return strings.Split(clean, string(filepath.Separator))
}

func writeFlagFile(root *os.Root, path string, data []byte, replace bool) (retErr error) {
	writePath := path
	if replace {
		writePath = filepath.Join(filepath.Dir(path), ".flag-"+rand.Text()+".tmp")
	}
	file, err := root.OpenFile(writePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, flagFileMode)
	if err != nil {
		return fmt.Errorf("creating flag %q without following symlinks: %w", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) && retErr == nil {
			retErr = fmt.Errorf("closing flag %q: %w", path, err)
		}
		if retErr != nil {
			if err := root.Remove(writePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				retErr = errors.Join(retErr, fmt.Errorf("removing incomplete flag %q: %w", writePath, err))
			}
		}
	}()

	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing flag %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing flag %q: %w", path, err)
	}
	if replace {
		if err := root.Rename(writePath, path); err != nil {
			return fmt.Errorf("atomically replacing flag %q: %w", path, err)
		}
	}
	return nil
}

func closeStoreRoot(root *os.Root, retErr *error) {
	if err := root.Close(); err != nil && *retErr == nil {
		*retErr = fmt.Errorf("closing mounted host root %q: %w", root.Name(), err)
	}
}
