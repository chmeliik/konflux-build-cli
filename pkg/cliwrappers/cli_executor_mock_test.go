package cliwrappers_test

import (
	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

var _ cliwrappers.CliExecutorInterface = &mockExecutor{}

type mockExecutor struct {
	executeFunc            func(command string, args ...string) (string, string, int, error)
	executeInDirFunc       func(workdir, command string, args ...string) (string, string, int, error)
	executeWithOutput      func(command string, args ...string) (string, string, int, error)
	executeInDirWithOutput func(workdir, command string, args ...string) (string, string, int, error)
}

func (m *mockExecutor) Execute(command string, args ...string) (string, string, int, error) {
	if m.executeFunc != nil {
		return m.executeFunc(command, args...)
	}
	return "", "", 0, nil
}

func (m *mockExecutor) ExecuteInDir(workdir, command string, args ...string) (string, string, int, error) {
	if m.executeInDirFunc != nil {
		return m.executeInDirFunc(workdir, command, args...)
	}
	return "", "", 0, nil
}

func (m *mockExecutor) ExecuteWithOutput(command string, args ...string) (stdout, stderr string, exitCode int, err error) {
	if m.executeWithOutput != nil {
		return m.executeWithOutput(command, args...)
	}
	return "", "", 0, nil
}

func (m *mockExecutor) ExecuteInDirWithOutput(workdir, command string, args ...string) (stdout, stderr string, exitCode int, err error) {
	if m.executeInDirWithOutput != nil {
		return m.executeWithOutput(command, args...)
	}
	return "", "", 0, nil
}
