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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type errorWriter struct {
	err error
}

func (w errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

type observingWriter struct {
	writes chan<- string
}

func (w observingWriter) Write(data []byte) (int, error) {
	w.writes <- string(data)
	return len(data), nil
}

var _ = Describe("Command execution validation", func() {
	It("rejects a nil context", func() {
		var ctx context.Context
		_, err := NewRunner().Run(ctx, Command{Executable: "/bin/true"})
		Expect(err).To(MatchError(ContainSubstring("validating command execution")))
		Expect(err).To(MatchError(ContainSubstring("command context is nil")))
	})

	It("preserves cancellation as an error cause", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := NewRunner().Run(ctx, Command{Executable: "/bin/true"})

		Expect(err).To(MatchError(ContainSubstring("command context is not runnable")))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
	})

	It("returns a start error for a missing working directory", func() {
		_, err := NewRunner().Run(context.Background(), Command{
			Executable:       "/bin/true",
			WorkingDirectory: filepath.Join(GinkgoT().TempDir(), "missing"),
		})

		Expect(err).To(MatchError(ContainSubstring("executing command")))
		Expect(err).To(MatchError(ContainSubstring("starting command")))
		Expect(errors.Is(err, fs.ErrNotExist)).To(BeTrue())
	})
})

var _ = Describe("Command execution environment and process setup", func() {
	It("uses os/exec lookup and passes the command PATH to the child", func() {
		var stdout bytes.Buffer

		result, err := NewRunner().Run(context.Background(), NewCommand(
			"sh",
			WithArguments("-c", "printf %s \"$PATH\""),
			WithEnvironment(map[string]string{"PATH": "/command/path"}),
			WithStdout(&stdout),
		))

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(Result{ExitCode: SuccessExitCode}))
		Expect(stdout.String()).To(Equal("/command/path"))
	})

	It("merges inherited values with overrides without mutating caller data", func() {
		const inheritedName = "NODEWRIGHT_COMMAND_INHERITED"
		const overriddenName = "NODEWRIGHT_COMMAND_OVERRIDDEN"
		setEnvironment(inheritedName, "inherited")
		setEnvironment(overriddenName, "parent")

		environment := map[string]string{overriddenName: "command", "COMMAND_ONLY": "only"}
		arguments := []string{"-c", fmt.Sprintf("printf '%%s|%%s|%%s' \"$%s\" \"$%s\" \"$COMMAND_ONLY\"", inheritedName, overriddenName)}
		expectedEnvironment := maps.Clone(environment)
		expectedArguments := append([]string(nil), arguments...)
		var stdout bytes.Buffer

		result, err := NewRunner().Run(context.Background(), Command{
			Executable:  "/bin/sh",
			Arguments:   arguments,
			Environment: environment,
			Stdout:      &stdout,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(Result{ExitCode: 0}))
		Expect(stdout.String()).To(Equal("inherited|command|only"))
		Expect(environment).To(Equal(expectedEnvironment))
		Expect(arguments).To(Equal(expectedArguments))
	})

	It("uses the requested working directory", func() {
		workingDirectory := GinkgoT().TempDir()
		var stdout bytes.Buffer

		_, err := NewRunner().Run(context.Background(), Command{
			Executable:       "/bin/pwd",
			WorkingDirectory: workingDirectory,
			Stdout:           &stdout,
		})

		Expect(err).NotTo(HaveOccurred())
		resolvedWorkingDirectory, err := filepath.Abs(workingDirectory)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(stdout.String())).To(Equal(resolvedWorkingDirectory))
	})

	It("builds the environment after setting the working directory", func() {
		workingDirectory := GinkgoT().TempDir()
		var stdout bytes.Buffer

		_, err := NewRunner().Run(context.Background(), Command{
			Executable:       "/bin/sh",
			Arguments:        []string{"-c", "printf %s \"$PWD\""},
			WorkingDirectory: workingDirectory,
			Stdout:           &stdout,
		})

		Expect(err).NotTo(HaveOccurred())
		resolvedWorkingDirectory, err := filepath.Abs(workingDirectory)
		Expect(err).NotTo(HaveOccurred())
		Expect(stdout.String()).To(Equal(resolvedWorkingDirectory))
	})
})

var _ = Describe("Command execution streaming", func() {
	It("copies raw stdout and stderr", func() {
		var stdout, stderr bytes.Buffer

		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "printf 'stdout\\nraw'; printf 'stderr\\nraw' >&2"},
			Stdout:     &stdout,
			Stderr:     &stderr,
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(SuccessExitCode))
		Expect(stdout.String()).To(Equal("stdout\nraw"))
		Expect(stderr.String()).To(Equal("stderr\nraw"))
	})

	It("writes output before the process exits", func() {
		release := filepath.Join(GinkgoT().TempDir(), "release")
		writes := make(chan string, 2)
		done := make(chan error, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_, err := NewRunner().Run(ctx, Command{
				Executable: "/bin/sh",
				Arguments: []string{
					"-c",
					"printf first; while [ ! -e \"$1\" ]; do sleep 0.01; done; printf second",
					"sh",
					release,
				},
				Stdout: observingWriter{writes: writes},
			})
			done <- err
		}()

		Eventually(writes).Should(Receive(Equal("first")))
		Consistently(done, 100*time.Millisecond).ShouldNot(Receive())
		Expect(os.WriteFile(release, nil, 0o600)).To(Succeed())
		Eventually(done).Should(Receive(BeNil()))
		Eventually(writes).Should(Receive(Equal("second")))
	})

	It("discards streams when writers are nil", func() {
		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "printf output; printf error >&2"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.ExitCode).To(Equal(SuccessExitCode))
	})

	DescribeTable("returns writer errors from os/exec",
		func(failStdout, failStderr bool) {
			writerError := errors.New("writer failed")
			command := Command{
				Executable: "/bin/sh",
				Arguments:  []string{"-c", "printf stdout; printf stderr >&2"},
			}
			if failStdout {
				command.Stdout = errorWriter{err: writerError}
			}
			if failStderr {
				command.Stderr = errorWriter{err: writerError}
			}

			_, err := NewRunner().Run(context.Background(), command)

			Expect(errors.Is(err, writerError)).To(BeTrue())
		},
		Entry("stdout", true, false),
		Entry("stderr", false, true),
	)

	It("uses os/exec's exit-error precedence when output and execution both fail", func() {
		writerError := errors.New("writer failed")

		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "printf stdout; exit 23"},
			Stdout:     errorWriter{err: writerError},
		})

		Expect(result).To(Equal(Result{ExitCode: 23}))
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("Command execution results", func() {
	It("returns zero for successful execution", func() {
		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "exit 0"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(Result{ExitCode: 0}))
	})

	It("returns normal nonzero exit codes without a runner error", func() {
		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "exit 23"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(Result{ExitCode: 23}))
	})

	It("reports the Go exit code and terminating signal", func() {
		result, err := NewRunner().Run(context.Background(), Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "kill -TERM $$"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(Result{ExitCode: -1, Signal: syscall.SIGTERM}))
	})

	It("returns context cancellation as a runner error", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := NewRunner().Run(ctx, Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "sleep 10"},
		})

		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
	})

	It("bounds cancellation when a background child retains output pipes", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()

		_, err := NewRunner().Run(ctx, Command{
			Executable: "/bin/sh",
			Arguments:  []string{"-c", "sleep 10 & wait"},
			Stdout:     io.Discard,
			Stderr:     io.Discard,
		})

		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
		Expect(time.Since(started)).To(BeNumerically("<", processWaitDelay))
	})

	It("returns start failures as runner errors", func() {
		path := filepath.Join(GinkgoT().TempDir(), "invalid-binary")
		Expect(os.WriteFile(path, []byte("not an executable format"), 0o755)).To(Succeed())

		_, err := NewRunner().Run(context.Background(), Command{Executable: path})

		Expect(err).To(MatchError(ContainSubstring("starting command")))
	})

})

func setEnvironment(name, value string) {
	original, existed := os.LookupEnv(name)
	Expect(os.Setenv(name, value)).To(Succeed())
	DeferCleanup(func() {
		if existed {
			Expect(os.Setenv(name, original)).To(Succeed())
		} else {
			Expect(os.Unsetenv(name)).To(Succeed())
		}
	})
}
