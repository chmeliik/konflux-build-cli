package cliwrappers_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

// newTestGitCli creates a GitCli with a mock executor for testing.
func newTestGitCli(execFunc func(workdir, command string, args ...string) (string, string, int, error)) *cliwrappers.GitCli {
	return &cliwrappers.GitCli{
		Executor: &mockExecutor{executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
			if execFunc != nil {
				return execFunc(cmd.Dir, cmd.Name, cmd.Args...)
			}
			return "", "", 0, nil
		}},
		Workdir: "/test/workdir",
	}
}

func Test_parseGitVersion(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		input       string
		expected    [3]int
		expectError bool
		errContains string
	}{
		{
			name:     "should parse standard version",
			input:    "git version 2.43.0",
			expected: [3]int{2, 43, 0},
		},
		{
			name:     "should parse version with trailing newline",
			input:    "git version 2.25.1\n",
			expected: [3]int{2, 25, 1},
		},
		{
			name:     "should parse version with extra components (e.g. Apple Git)",
			input:    "git version 2.39.5.1.3",
			expected: [3]int{2, 39, 5},
		},
		{
			name:        "should fail on missing prefix",
			input:       "2.43.0",
			expectError: true,
			errContains: "failed to parse git version",
		},
		{
			name:        "should fail on too few components",
			input:       "git version 2.43",
			expectError: true,
			errContains: "failed to parse git version",
		},
		{
			name:        "should fail on non-numeric component",
			input:       "git version 2.abc.0",
			expectError: true,
			errContains: "failed to parse git version",
		},
		{
			name:        "should fail on empty input",
			input:       "",
			expectError: true,
			errContains: "failed to parse git version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			version, err := cliwrappers.ExportParseGitVersion(tc.input)
			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.errContains))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(version).To(Equal(tc.expected))
			}
		})
	}
}

func Test_isVersionAtLeast(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		version  [3]int
		minimum  [3]int
		expected bool
	}{
		{
			name:     "should return true when equal",
			version:  [3]int{2, 25, 0},
			minimum:  [3]int{2, 25, 0},
			expected: true,
		},
		{
			name:     "should return true when major is greater",
			version:  [3]int{3, 0, 0},
			minimum:  [3]int{2, 25, 0},
			expected: true,
		},
		{
			name:     "should return true when minor is greater",
			version:  [3]int{2, 43, 0},
			minimum:  [3]int{2, 25, 0},
			expected: true,
		},
		{
			name:     "should return true when patch is greater",
			version:  [3]int{2, 25, 1},
			minimum:  [3]int{2, 25, 0},
			expected: true,
		},
		{
			name:     "should return false when major is less",
			version:  [3]int{1, 30, 0},
			minimum:  [3]int{2, 25, 0},
			expected: false,
		},
		{
			name:     "should return false when minor is less",
			version:  [3]int{2, 24, 0},
			minimum:  [3]int{2, 25, 0},
			expected: false,
		},
		{
			name:     "should return false when patch is less",
			version:  [3]int{2, 25, 0},
			minimum:  [3]int{2, 25, 1},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := cliwrappers.ExportIsVersionAtLeast(tc.version, tc.minimum)
			g.Expect(result).To(Equal(tc.expected))
		})
	}
}

func Test_NewCli_versionCheck(t *testing.T) {
	g := NewWithT(t)

	t.Run("should fail when git version is below minimum", func(t *testing.T) {
		executor := &mockExecutor{
			executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				return "git version 2.24.0\n", "", 0, nil
			},
		}

		cli, err := cliwrappers.NewGitCli(executor, "/tmp")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("below minimum required"))
		g.Expect(cli).To(BeNil())
	})

	t.Run("should fail when git --version command fails", func(t *testing.T) {
		executor := &mockExecutor{
			executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				return "", "error", 1, errors.New("command failed")
			},
		}

		cli, err := cliwrappers.NewGitCli(executor, "/tmp")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get git version"))
		g.Expect(cli).To(BeNil())
	})

	t.Run("should succeed when git version meets minimum", func(t *testing.T) {
		executor := &mockExecutor{
			executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				return "git version 2.43.0\n", "", 0, nil
			},
		}

		cli, err := cliwrappers.NewGitCli(executor, "/tmp")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cli).ToNot(BeNil())
	})

	t.Run("should succeed when git version equals minimum", func(t *testing.T) {
		executor := &mockExecutor{
			executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				return "git version 2.25.0\n", "", 0, nil
			},
		}

		cli, err := cliwrappers.NewGitCli(executor, "/tmp")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cli).ToNot(BeNil())
	})
}

func Test_Init(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git init", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(workdir).To(Equal("/test/workdir"))
			g.Expect(command).To(Equal("git"))
			g.Expect(args).To(Equal([]string{"init"}))
			return "", "", 0, nil
		})

		err := cli.Init()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 1, errors.New("init failed")
		})

		err := cli.Init()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git init failed"))
	})
}

func Test_ConfigLocal(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git config --local", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"config", "--local", "user.email", "test@example.com"}))
			return "", "", 0, nil
		})

		err := cli.ConfigLocal("user.email", "test@example.com")

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject empty key", func(t *testing.T) {
		cli := newTestGitCli(nil)

		err := cli.ConfigLocal("", "value")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("config key must not be empty"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 1, errors.New("config failed")
		})

		err := cli.ConfigLocal("key", "value")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git config failed"))
	})
}

func Test_Checkout(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git checkout", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"checkout", "main"}))
			return "", "", 0, nil
		})

		err := cli.Checkout("main")

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject empty ref", func(t *testing.T) {
		cli := newTestGitCli(nil)

		err := cli.Checkout("")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("ref must not be empty"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 1, errors.New("checkout failed")
		})

		err := cli.Checkout("main")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git checkout failed"))
	})
}

func Test_Commit(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git commit with message", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"commit", "-m", "test commit"}))
			return "committed\n", "", 0, nil
		})

		result, err := cli.Commit("test commit")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("committed"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "nothing to commit", 1, errors.New("commit failed")
		})

		_, err := cli.Commit("msg")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git commit failed"))
	})
}

func Test_Merge(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git merge with correct args", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"merge", "-m", "merge msg", "--no-ff", "--allow-unrelated-histories", "origin/main"}))
			return "Merge made\n", "", 0, nil
		})

		result, err := cli.Merge("origin/main", "merge msg")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("Merge made"))
	})

	t.Run("should reject empty ref", func(t *testing.T) {
		cli := newTestGitCli(nil)

		_, err := cli.Merge("", "msg")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("ref must not be empty"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "conflict", 1, errors.New("merge failed")
		})

		_, err := cli.Merge("origin/main", "msg")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git merge failed"))
	})
}

func Test_RemoteAdd(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git remote add", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"remote", "add", "origin", "https://github.com/user/repo"}))
			return "", "", 0, nil
		})

		_, err := cli.RemoteAdd("origin", "https://github.com/user/repo")

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject empty name", func(t *testing.T) {
		cli := newTestGitCli(nil)

		_, err := cli.RemoteAdd("", "https://url")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("remote name must not be empty"))
	})

	t.Run("should reject empty url", func(t *testing.T) {
		cli := newTestGitCli(nil)

		_, err := cli.RemoteAdd("origin", "")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("remote url must not be empty"))
	})
}

func Test_RevParse(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git rev-parse for full SHA", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"rev-parse", "HEAD"}))
			return "abc123def456\n", "", 0, nil
		})

		result, err := cli.RevParse("HEAD", false, 0)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("abc123def456"))
	})

	t.Run("should run git rev-parse --short", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"rev-parse", "--short", "HEAD"}))
			return "abc123d\n", "", 0, nil
		})

		result, err := cli.RevParse("HEAD", true, 0)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("abc123d"))
	})

	t.Run("should run git rev-parse --short=N", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"rev-parse", "--short=12", "HEAD"}))
			return "abc123def456\n", "", 0, nil
		})

		result, err := cli.RevParse("HEAD", true, 12)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("abc123def456"))
	})

	t.Run("should reject empty ref", func(t *testing.T) {
		cli := newTestGitCli(nil)

		_, err := cli.RevParse("", false, 0)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("ref must not be empty"))
	})
}

func Test_Log(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run git log with format and count", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"log", "-1", "--pretty=%ct"}))
			return "1704067200\n", "", 0, nil
		})

		result, err := cli.Log("%ct", 1)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("1704067200"))
	})

	t.Run("should run git log without optional args", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"log"}))
			return "commit abc123\n", "", 0, nil
		})

		result, err := cli.Log("", 0)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal("commit abc123"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 128, errors.New("not a git repo")
		})

		_, err := cli.Log("", 0)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git log failed"))
	})
}

func Test_FetchTags(t *testing.T) {
	g := NewWithT(t)

	t.Run("should fetch and return tags", func(t *testing.T) {
		callCount := 0
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			callCount++
			if callCount == 1 {
				g.Expect(args).To(Equal([]string{"fetch", "--tags"}))
				return "", "", 0, nil
			}
			g.Expect(args).To(Equal([]string{"tag", "-l"}))
			return "v1.0.0\nv1.1.0\nv2.0.0\n", "", 0, nil
		})

		tags, err := cli.FetchTags()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(Equal([]string{"v1.0.0", "v1.1.0", "v2.0.0"}))
	})

	t.Run("should return empty list when no tags", func(t *testing.T) {
		callCount := 0
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			callCount++
			if callCount == 1 {
				return "", "", 0, nil
			}
			return "", "", 0, nil
		})

		tags, err := cli.FetchTags()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(tags).To(BeEmpty())
	})

	t.Run("should return error if fetch fails", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 1, errors.New("network error")
		})

		_, err := cli.FetchTags()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git fetch failed"))
	})
}

func Test_FetchWithRefspec(t *testing.T) {
	g := NewWithT(t)

	cliwrappers.DisableRetryer = true
	t.Cleanup(func() { cliwrappers.DisableRetryer = false })

	t.Run("should build correct args with all options", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			joined := strings.Join(args, " ")
			g.Expect(joined).To(ContainSubstring("fetch"))
			g.Expect(joined).To(ContainSubstring("--recurse-submodules=yes"))
			g.Expect(joined).To(ContainSubstring("--depth=5"))
			g.Expect(joined).To(ContainSubstring("origin"))
			g.Expect(joined).To(ContainSubstring("--update-head-ok"))
			g.Expect(joined).To(ContainSubstring("--force"))
			g.Expect(joined).To(ContainSubstring("refs/heads/main"))
			return "", "", 0, nil
		})

		err := cli.FetchWithRefspec(cliwrappers.GitFetchOptions{
			Remote:      "origin",
			Refspec:     "refs/heads/main",
			Depth:       5,
			Submodules:  true,
			MaxAttempts: 1,
		})

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should omit depth when zero", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			joined := strings.Join(args, " ")
			g.Expect(joined).ToNot(ContainSubstring("--depth"))
			return "", "", 0, nil
		})

		err := cli.FetchWithRefspec(cliwrappers.GitFetchOptions{
			Remote:      "origin",
			Depth:       0,
			MaxAttempts: 1,
		})

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject empty remote", func(t *testing.T) {
		cli := newTestGitCli(nil)

		err := cli.FetchWithRefspec(cliwrappers.GitFetchOptions{Remote: ""})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("remote must not be empty"))
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "fatal: error", 128, errors.New("fetch failed")
		})

		err := cli.FetchWithRefspec(cliwrappers.GitFetchOptions{
			Remote:      "origin",
			MaxAttempts: 1,
		})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git fetch failed"))
	})
}

func Test_SetSparseCheckout(t *testing.T) {
	g := NewWithT(t)

	t.Run("should configure sparse checkout", func(t *testing.T) {
		callCount := 0
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			callCount++
			if callCount == 1 {
				// ConfigLocal call
				g.Expect(args).To(Equal([]string{"config", "--local", "core.sparseCheckout", "true"}))
				return "", "", 0, nil
			}
			// sparse-checkout set call
			g.Expect(args).To(Equal([]string{"sparse-checkout", "set", "--", "src", "docs"}))
			return "", "", 0, nil
		})

		err := cli.SetSparseCheckout([]string{"src", "docs"})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(callCount).To(Equal(2))
	})

	t.Run("should reject empty directories", func(t *testing.T) {
		cli := newTestGitCli(nil)

		err := cli.SetSparseCheckout([]string{})

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("directories parameter empty"))
	})
}

func Test_SubmoduleUpdate(t *testing.T) {
	g := NewWithT(t)

	t.Run("should run with init and depth", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"submodule", "update", "--recursive", "--init", "--force", "--depth=5"}))
			return "", "", 0, nil
		})

		err := cli.SubmoduleUpdate(true, 5, nil)

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should run without init and with paths", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			g.Expect(args).To(Equal([]string{"submodule", "update", "--recursive", "--force", "--", "lib", "vendor"}))
			return "", "", 0, nil
		})

		err := cli.SubmoduleUpdate(false, 0, []string{"lib", "vendor"})

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should return error on failure", func(t *testing.T) {
		cli := newTestGitCli(func(workdir, command string, args ...string) (string, string, int, error) {
			return "", "error", 1, errors.New("submodule failed")
		})

		err := cli.SubmoduleUpdate(true, 0, nil)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git submodule failed"))
	})
}

func Test_SetEnv(t *testing.T) {
	g := NewWithT(t)

	t.Run("should pass extra env vars to commands via Cmd.Env", func(t *testing.T) {
		var capturedEnv []string
		cli := &cliwrappers.GitCli{
			Executor: &mockExecutor{executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				capturedEnv = cmd.Env
				return "", "", 0, nil
			}},
			Workdir: "/test/workdir",
		}

		cli.SetEnv("GIT_SSL_NO_VERIFY", "true")
		cli.SetEnv("GIT_CONFIG_GLOBAL", "/tmp/.gitconfig")

		_ = cli.Init()

		g.Expect(capturedEnv).ToNot(BeNil())
		g.Expect(capturedEnv).To(ContainElement("GIT_SSL_NO_VERIFY=true"))
		g.Expect(capturedEnv).To(ContainElement("GIT_CONFIG_GLOBAL=/tmp/.gitconfig"))
		g.Expect(len(capturedEnv)).To(BeNumerically(">=", len(os.Environ())))
	})

	t.Run("should leave Cmd.Env nil when no extra env is set", func(t *testing.T) {
		var capturedEnv []string
		envWasCaptured := false
		cli := &cliwrappers.GitCli{
			Executor: &mockExecutor{executeFunc: func(cmd cliwrappers.Cmd) (string, string, int, error) {
				capturedEnv = cmd.Env
				envWasCaptured = true
				return "", "", 0, nil
			}},
			Workdir: "/test/workdir",
		}

		_ = cli.Init()

		g.Expect(envWasCaptured).To(BeTrue())
		g.Expect(capturedEnv).To(BeNil())
	})
}
