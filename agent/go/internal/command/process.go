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

package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// processWaitDelay bounds cleanup when a descendant keeps an inherited pipe
// open after the command exits or context cancellation begins.
const processWaitDelay = 2 * time.Second

func validateRun(ctx context.Context, command Command) error {
	if ctx == nil {
		return errors.New("validating command context: command context is nil")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("validating command context: command context is not runnable: %w", err)
	}
	if err := command.validate(); err != nil {
		return fmt.Errorf("validating command %q: %w", command.Executable, err)
	}
	return nil
}

func applyExecutablePermissions(
	ctx context.Context,
	command Command,
	file *os.File,
) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("checking command executable %q: %w", command.Executable, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("command executable %q is not a regular file: %w", command.Executable, fs.ErrPermission)
	}
	permissions := command.Permissions
	if command.RequiredPermissions != 0 {
		permissions = info.Mode().Perm() | command.RequiredPermissions
	}
	if info.Mode().Perm() == permissions {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("running command %q: %w", command.Executable, err)
	}
	if err := file.Chmod(permissions); err != nil {
		return fmt.Errorf("setting permissions on %q: %w", command.Executable, err)
	}
	return nil
}

func closeExecutable(command Command, file *os.File) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("closing command executable %q: %w", command.Executable, err)
	}
	return nil
}

func executeCommand(
	ctx context.Context,
	command Command,
	executable string,
	workingDirectory string,
	chroot string,
) (result Result, err error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("running command %q: %w", command.Executable, err)
	}

	process := exec.CommandContext(ctx, executable, command.Arguments...)
	process.Args[0] = command.Executable
	process.Dir = workingDirectory
	process.Env = process.Environ()
	for _, name := range sortedEnvironmentNames(command.Environment) {
		process.Env = append(process.Env, name+"="+command.Environment[name])
	}
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if chroot != "" {
		process.SysProcAttr.Chroot = chroot
	}
	process.Cancel = func() error {
		if process.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-process.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return fmt.Errorf("killing process group %d: %w", process.Process.Pid, err)
		}
		return nil
	}
	process.WaitDelay = processWaitDelay

	runErr := process.Run()
	if process.ProcessState != nil {
		result.ExitCode = ExitCode(process.ProcessState.ExitCode())
		if waitStatus, ok := process.ProcessState.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
			result.Signal = waitStatus.Signal()
		}
	}
	if runErr == nil {
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("running command %q: %w", command.Executable, err)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return result, nil
	}
	if process.Process == nil {
		return result, fmt.Errorf("starting command %q: %w", command.Executable, runErr)
	}
	return result, fmt.Errorf("waiting for command %q: %w", command.Executable, runErr)
}
