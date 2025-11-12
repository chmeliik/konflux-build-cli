package cliwrappers_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

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

		stdout, stderr, exitCode, err := executor.Execute("echo", expectedOutput)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal(expectedOutput))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command with multiple arguments", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.Execute("echo", "-n", "arg1", "arg2", "arg3")

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
			stdout, stderr, exitCode, err = executor.Execute("cmd", "/c", "echo stdout & echo stderr 1>&2")
		} else {
			stdout, stderr, exitCode, err = executor.Execute("sh", "-c", "echo 'stdout'; echo 'stderr' >&2")
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
			stdout, stderr, exitCode, err = executor.Execute("cmd", "/c", "exit 50")
		} else {
			stdout, stderr, exitCode, err = executor.Execute("sh", "-c", "exit 50")
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

		stdout, stderr, exitCode, err := executor.Execute(nonExistentCommand)

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})
}

func TestCliExecutor_ExecuteInDir(t *testing.T) {
	t.Run("should execute command in current directory when workdir is empty", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.ExecuteInDir("", "echo", "test")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("test"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should execute command in specified directory", func(t *testing.T) {
		g := NewWithT(t)

		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "testfile.txt")
		err := os.WriteFile(testFile, []byte("test content"), 0644)
		g.Expect(err).ToNot(HaveOccurred())

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.ExecuteInDir(tempDir, "cmd", "/c", "dir", "/b")
		} else {
			stdout, stderr, exitCode, err = executor.ExecuteInDir(tempDir, "ls")
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("testfile.txt"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle invalid working directory", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()
		invalidDir := "/this/directory/does/not/exist"

		stdout, stderr, exitCode, err := executor.ExecuteInDir(invalidDir, "echo", "test")

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
		g.Expect(err.Error()).To(ContainSubstring("no such file or directory"))
	})
}

func TestCliExecutor_ExecuteWithOutput(t *testing.T) {
	t.Run("should execute command and return output", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.ExecuteWithOutput("echo", "test output")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("test output"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command failure", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.ExecuteWithOutput("this-command-does-not-exist")

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle commands that write to both stdout and stderr", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("cmd", "/c", "echo stdout output & echo stderr output 1>&2")
		} else {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("sh", "-c", "echo 'stdout output'; echo 'stderr output' >&2")
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("stdout output"))
		g.Expect(stderr).To(ContainSubstring("stderr output"))
	})

	t.Run("should handle multiline output correctly", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("cmd", "/c", "echo line1 & echo line2 & echo line3")
		} else {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("sh", "-c", "echo 'line1'; echo 'line2'; echo 'line3'")
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("line1"))
		g.Expect(stdout).To(ContainSubstring("line2"))
		g.Expect(stdout).To(ContainSubstring("line3"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command with non-zero exit code", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		var err error
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("cmd", "/c", "echo output & exit 5")
		} else {
			stdout, stderr, exitCode, err = executor.ExecuteWithOutput("sh", "-c", "echo 'output'; exit 5")
		}

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(5))
		g.Expect(stdout).To(ContainSubstring("output"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle command not found", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.ExecuteWithOutput("this-command-does-not-exist")

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
	})
}

func TestCliExecutor_ExecuteInDirWithOutput(t *testing.T) {
	t.Run("should execute command in current directory when workdir is empty", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()

		stdout, stderr, exitCode, err := executor.ExecuteInDirWithOutput("", "echo", "test output")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(strings.TrimSpace(stdout)).To(Equal("test output"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should execute command in specified directory", func(t *testing.T) {
		g := NewWithT(t)

		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "output-test.txt")
		err := os.WriteFile(testFile, []byte("content"), 0644)
		g.Expect(err).ToNot(HaveOccurred())

		executor := cliwrappers.NewCliExecutor()

		var stdout, stderr string
		var exitCode int
		if runtime.GOOS == "windows" {
			stdout, stderr, exitCode, err = executor.ExecuteInDirWithOutput(tempDir, "cmd", "/c", "dir", "/b")
		} else {
			stdout, stderr, exitCode, err = executor.ExecuteInDirWithOutput(tempDir, "ls")
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(exitCode).To(Equal(0))
		g.Expect(stdout).To(ContainSubstring("output-test.txt"))
		g.Expect(stderr).To(BeEmpty())
	})

	t.Run("should handle invalid working directory", func(t *testing.T) {
		g := NewWithT(t)

		executor := cliwrappers.NewCliExecutor()
		invalidDir := "/this/directory/does/not/exist"

		stdout, stderr, exitCode, err := executor.ExecuteInDirWithOutput(invalidDir, "echo", "test")

		g.Expect(err).To(HaveOccurred())
		g.Expect(exitCode).To(Equal(-1))
		g.Expect(stdout).To(BeEmpty())
		g.Expect(stderr).To(BeEmpty())
		g.Expect(err.Error()).To(ContainSubstring("no such file or directory"))
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
