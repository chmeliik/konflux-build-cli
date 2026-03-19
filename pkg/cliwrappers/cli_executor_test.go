package cliwrappers_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

func captureLogOutput(fn func()) string {
	origOut := l.Logger.Out
	origFormatter := l.Logger.Formatter
	origLevel := l.Logger.Level

	var buf bytes.Buffer
	l.Logger.SetOutput(&buf)
	l.Logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	l.Logger.SetLevel(logrus.InfoLevel)

	defer func() {
		l.Logger.SetOutput(origOut)
		l.Logger.SetFormatter(origFormatter)
		l.Logger.SetLevel(origLevel)
	}()

	fn()

	return buf.String()
}

func TestNewCliExecutor(t *testing.T) {
	t.Run("should create new CLI executor instance", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		g.Expect(executor).ToNot(BeNil())
		g.Expect(executor).To(BeAssignableToTypeOf(&cliwrappers.CliExecutor{}))
	})
}

func TestCliExecutor_Execute(t *testing.T) {
	t.Run("should execute simple echo command successfully", func(t *testing.T) {
		g := NewWithT(t)

		const expectedOutput = "hello world"
		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.Execute(cliwrappers.Command("echo", expectedOutput))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal(expectedOutput))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command with multiple arguments", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.Execute(cliwrappers.Command("echo", "-n", "arg1", "arg2", "arg3"))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(Equal("arg1 arg2 arg3"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should capture both stdout and stderr", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.Execute(cliwrappers.Command("cmd", "/c", "echo stdout & echo stderr 1>&2"))
		} else {
			stdout, stderr, exitCode, err = executor.Execute(cliwrappers.Command("sh", "-c", "echo 'stdout'; echo 'stderr' >&2"))
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("stdout"))
		g.Expect(strings.TrimSpace(stderr)).To(Equal("stderr"))
	})

	t.Run("should handle command with non-zero exit code", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.Execute(cliwrappers.Command("cmd", "/c", "exit 50"))
		} else {
			stdout, stderr, exitCode, err = executor.Execute(cliwrappers.Command("sh", "-c", "exit 50"))
		}

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(50))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command not found", func(t *testing.T) {
		g := NewWithT(t)

		const nonExistentCommand = "this-command-does-not-exist"
		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.Execute(cliwrappers.Command(nonExistentCommand))

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should execute command in specified directory", func(t *testing.T) {
		g := NewWithT(t)

		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "testfile.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		g.Expect(err).ToNot(HaveOccurred())

		executor := cliwrappers.NewCliExecutor()

		var cmd cliwrappers.Cmd
		if runtime.GOOS == "windows" {
			cmd = cliwrappers.Command("cmd", "/c", "dir", "/b")
		} else {
			cmd = cliwrappers.Command("ls")
		}
		cmd.Dir = tempDir
		stdout, stderr, exitCode, err := executor.Execute(cmd)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("testfile.txt"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle invalid working directory", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()
		invalidDir := "/this/directory/does/not/exist"

		cmd := cliwrappers.Command("echo", "test")
		cmd.Dir = invalidDir
		stdout, stderr, exitCode, err := executor.Execute(cmd)

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
		g.Expect(err.Error()).To(ContainSubstring("no such file or directory"))
	})

	t.Run("should allow setting environment variables", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var cmd cliwrappers.Cmd
		if runtime.GOOS == "windows" {
			cmd = cliwrappers.Command("cmd", "/c", "echo %MY_TEST_VAR%")
		} else {
			cmd = cliwrappers.Command("sh", "-c", "echo $MY_TEST_VAR")
		}
		cmd.Env = append(os.Environ(), "MY_TEST_VAR=custom_value")
		stdout, stderr, exitCode, err := executor.Execute(cmd)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("custom_value"))
		g.Expect(stderr).To(BeEmpty())
	})
}

// Separate test suite for LogOutput: true because it's a separate code path
func TestCliExecutor_ExecuteWithLogOutput(t *testing.T) {
	t.Run("should execute command and return output", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		logOutput := captureLogOutput(func() {
			cmd := cliwrappers.Command("echo", "test output")
			cmd.LogOutput = true
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("test output"))
		g.Expect(stderr).To(BeEmpty())

		g.Expect(logOutput).To(ContainSubstring("echo [stdout] test output"))
	})

	t.Run("should handle commands that write to both stdout and stderr", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		logOutput := captureLogOutput(func() {
			var cmd cliwrappers.Cmd
			if runtime.GOOS == "windows" {
				cmd = cliwrappers.Command("cmd", "/c", "echo stdout output & echo stderr output 1>&2")
			} else {
				cmd = cliwrappers.Command("sh", "-c", "echo 'stdout output'; echo 'stderr output' >&2")
			}
			cmd.LogOutput = true
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("stdout output"))
		g.Expect(stderr).To(ContainSubstring("stderr output"))

		g.Expect(logOutput).To(
			Or(
				SatisfyAll(
					ContainSubstring("cmd [stdout] stdout output"),
					ContainSubstring("cmd [stderr] stderr output"),
				),
				SatisfyAll(
					ContainSubstring("sh [stdout] stdout output"),
					ContainSubstring("sh [stderr] stderr output"),
				),
			))
	})

	t.Run("should handle multiline output correctly", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		logOutput := captureLogOutput(func() {
			var cmd cliwrappers.Cmd
			if runtime.GOOS == "windows" {
				cmd = cliwrappers.Command("cmd", "/c", "echo line1 & echo line2 & echo line3")
			} else {
				cmd = cliwrappers.Command("sh", "-c", "echo 'line1'; echo 'line2'; echo 'line3'")
			}
			cmd.LogOutput = true
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("line1"))
		g.Expect(stdout).To(ContainSubstring("line2"))
		g.Expect(stdout).To(ContainSubstring("line3"))
		g.Expect(stderr).To(BeEmpty())

		g.Expect(logOutput).To(ContainSubstring("[stdout] line1"))
		g.Expect(logOutput).To(ContainSubstring("[stdout] line2"))
		g.Expect(logOutput).To(ContainSubstring("[stdout] line3"))
	})

	t.Run("should handle command with non-zero exit code", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		logOutput := captureLogOutput(func() {
			var cmd cliwrappers.Cmd
			if runtime.GOOS == "windows" {
				cmd = cliwrappers.Command("cmd", "/c", "echo output & exit 5")
			} else {
				cmd = cliwrappers.Command("sh", "-c", "echo 'output'; exit 5")
			}
			cmd.LogOutput = true
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(5))
		g.Expect(stdout).To(ContainSubstring("output"))
		g.Expect(stderr).To(BeEmpty())

		g.Expect(logOutput).To(ContainSubstring("[stdout] output"))
	})

	t.Run("should allow overwriting command name in logs", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		logOutput := captureLogOutput(func() {
			cmd := cliwrappers.Command("echo", "%s", "hello")
			cmd.LogOutput = true
			cmd.NameInLogs = "printf"
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		// %s shouldn't get substituted, the command is still echo, not printf
		g.Expect(strings.TrimSpace(stdout)).To(Equal("%s hello"))
		g.Expect(stderr).To(BeEmpty())

		g.Expect(logOutput).To(ContainSubstring("printf [stdout] %s hello"))
	})

	t.Run("should handle command not found", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		cmd := cliwrappers.Command("this-command-does-not-exist")
		cmd.LogOutput = true
		stdout, stderr, exitCode, err := executor.Execute(cmd)

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should capture full output even when a line exceeds the scanner buffer size", func(t *testing.T) {
		g := NewWithT(t)

		longLineFile := filepath.Join(t.TempDir(), "long_line.txt")
		// bufio.Scanner's default max token size is 64KB
		longLine := strings.Repeat("x", 128*1024)
		fileContent := fmt.Sprintf("before\n%s\nafter\n", longLine)

		err := os.WriteFile(longLineFile, []byte(fileContent), 0644)
		g.Expect(err).ToNot(HaveOccurred())

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		logOutput := captureLogOutput(func() {
			cmd := cliwrappers.Command("cat", longLineFile)
			cmd.LogOutput = true
			stdout, stderr, exitCode, err = executor.Execute(cmd)
		})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stderr).To(BeEmpty())
		g.Expect(stdout).To(ContainSubstring("before"))
		g.Expect(stdout).To(ContainSubstring("\n" + longLine + "\n"))
		g.Expect(stdout).To(ContainSubstring("after"))

		g.Expect(logOutput).To(ContainSubstring("before"))
		g.Expect(logOutput).To(ContainSubstring("stopped logging output: bufio.Scanner: token too long"))
		g.Expect(logOutput).ToNot(ContainSubstring(longLine))
	})
}

func TestCheckCliToolAvailable(t *testing.T) {
	t.Run("should return true for available CLI tool", func(t *testing.T) {
		g := NewWithT(t)

		var toolName string
		if runtime.GOOS == "windows" {
			toolName = "cmd"
		} else {
			toolName = "sh"
		}

		available, err := cliwrappers.CheckCliToolAvailable(toolName)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(available).To(BeTrue())
	})

	t.Run("should return false for unavailable CLI tool", func(t *testing.T) {
		g := NewWithT(t)

		nonExistentTool := "this-tool-definitely-does-not-exist"

		available, err := cliwrappers.CheckCliToolAvailable(nonExistentTool)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(available).To(BeFalse())
	})
}
