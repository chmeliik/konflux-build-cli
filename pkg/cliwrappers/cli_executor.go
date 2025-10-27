package cliwrappers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type CliExecutorInterface interface {
	Execute(command string, args ...string) (stdout, stderr string, exitCode int, err error)
	ExecuteInDir(wordir, command string, args ...string) (stdout, stderr string, exitCode int, err error)
	ExecuteWithOutput(command string, args ...string) (stdout, stderr string, exitCode int, err error)
	ExecuteInDirWithOutput(wordir, command string, args ...string) (stdout, stderr string, exitCode int, err error)
}

var _ CliExecutorInterface = &CliExecutor{}

type CliExecutor struct{}

func NewCliExecutor() *CliExecutor {
	return &CliExecutor{}
}

// Execute runs specified command with given arguments.
// Returns stdout, stderr, exit code, error
func (e *CliExecutor) Execute(command string, args ...string) (string, string, int, error) {
	return e.ExecuteInDir("", command, args...)
}

// ExecuteInDir runs specified command in the given directory.
// Returns stdout, stderr, exit code, error
func (e *CliExecutor) ExecuteInDir(wordir, command string, args ...string) (string, string, int, error) {
	cmd := exec.Command(command, args...)
	if wordir != "" {
		cmd.Dir = wordir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()

	return stdoutBuf.String(), stderrBuf.String(), getExitCodeFromError(err), err
}

// ExecuteWithOutput runs specified command with args while printing stdout and stderr in real time.
// Returns stdout, stderr, exit code, error
func (e *CliExecutor) ExecuteWithOutput(command string, args ...string) (string, string, int, error) {
	return e.ExecuteInDirWithOutput("", command, args...)
}

// ExecuteInDirWithOutput runs specified command with args in given directory while printing stdout and stderr in real time.
// Returns stdout, stderr, exit code, error
func (e *CliExecutor) ExecuteInDirWithOutput(workdir, command string, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command(command, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to get stdout: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to get stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", -1, fmt.Errorf("failed to start command: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	readStream := func(linePrefix string, r io.Reader, buf *bytes.Buffer) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()
			l.Logger.Info(linePrefix + line)
			buf.WriteString(line + "\n")
		}
	}

	done := make(chan struct{}, 2)
	go func() {
		readStream(command+" [stdout] ", stdoutPipe, &stdoutBuf)
		done <- struct{}{}
	}()
	go func() {
		readStream(command+" [stderr] ", stderrPipe, &stderrBuf)
		done <- struct{}{}
	}()

	err = cmd.Wait()
	// Wait for both output streams to finish
	<-done
	<-done

	return stdoutBuf.String(), stderrBuf.String(), getExitCodeFromError(err), err
}

func getExitCodeFromError(cmdErr error) int {
	if cmdErr == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(cmdErr, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return -1
}

func CheckCliToolAvailable(cliTool string) (bool, error) {
	if _, err := exec.LookPath(cliTool); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to determine availability of '%s': %w", cliTool, err)
	}
	return true, nil
}
