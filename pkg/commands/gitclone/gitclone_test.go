package gitclone

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	. "github.com/onsi/gomega"
)

func Test_GitClone_validateParams(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		params      Params
		expectError bool
		errContains string
	}{
		{
			name: "should pass with valid URL",
			params: Params{
				URL:              "https://git.test/user/repo.git",
				RetryMaxAttempts: 1,
			},
			expectError: false,
		},
		{
			name: "should pass with URL and revision",
			params: Params{
				URL:              "https://git.test/user/repo.git",
				Revision:         "main",
				RetryMaxAttempts: 1,
			},
			expectError: false,
		},
		{
			name: "should pass with all parameters",
			params: Params{
				URL:               "https://git.test/user/repo.git",
				Revision:          "v1.0.0",
				Depth:             10,
				ShortCommitLength: 8,
				OutputDir:         "/tmp",
				Subdirectory:      "source",
				RetryMaxAttempts:  10,
			},
			expectError: false,
		},
		{
			name:        "should fail with empty URL",
			params:      Params{},
			expectError: true,
			errContains: "url parameter is required",
		},
		{
			name: "should fail with empty URL string",
			params: Params{
				URL: "",
			},
			expectError: true,
			errContains: "url parameter is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &GitClone{
				Params: &tc.params,
			}

			err := c.validateParams()

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tc.errContains))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_GitClone_getCheckoutDir(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		params   Params
		expected string
	}{
		{
			name: "should return default path",
			params: Params{
				OutputDir:    ".",
				Subdirectory: "source",
			},
			expected: "source",
		},
		{
			name: "should combine output dir and subdirectory",
			params: Params{
				OutputDir:    "/tmp/workspace",
				Subdirectory: "source",
			},
			expected: "/tmp/workspace/source",
		},
		{
			name: "should handle custom subdirectory",
			params: Params{
				OutputDir:    "/workspace",
				Subdirectory: "my-repo",
			},
			expected: "/workspace/my-repo",
		},
		{
			name: "should handle empty subdirectory",
			params: Params{
				OutputDir:    "/workspace",
				Subdirectory: "",
			},
			expected: "/workspace",
		},
		{
			name: "should handle nested subdirectory",
			params: Params{
				OutputDir:    "/workspace",
				Subdirectory: "repos/source",
			},
			expected: "/workspace/repos/source",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &GitClone{
				Params: &tc.params,
			}

			result := c.getCheckoutDir()

			g.Expect(result).To(Equal(tc.expected))
		})
	}
}

func Test_GitClone_gatherCommitInfo(t *testing.T) {
	g := NewWithT(t)

	const fullSha = "abc123def456789012345678901234567890abcd"
	const shortSha = "abc123d"
	const timestamp = "1704067200"

	var _mockGitCli *mockGitCli
	var c *GitClone

	beforeEach := func() {
		_mockGitCli = &mockGitCli{}
		c = &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				URL:               "https://git.test/user/repo.git",
				OutputDir:         "/workspace",
				Subdirectory:      "source",
				ShortCommitLength: 7,
			},
		}
	}

	t.Run("should gather all commit info successfully", func(t *testing.T) {
		beforeEach()

		revParseCallCount := 0
		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			g.Expect(ref).To(Equal("HEAD"))

			revParseCallCount++
			if !short {
				return fullSha, nil
			}
			g.Expect(length).To(Equal(7))
			return shortSha, nil
		}

		isLogCalled := false
		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			isLogCalled = true
			g.Expect(format).To(Equal("%ct"))
			g.Expect(count).To(Equal(1))
			return timestamp, nil
		}

		err := c.gatherCommitInfo()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(revParseCallCount).To(Equal(2))
		g.Expect(isLogCalled).To(BeTrue())
		g.Expect(c.Results.Commit).To(Equal(fullSha))
		g.Expect(c.Results.ShortCommit).To(Equal(shortSha))
		g.Expect(c.Results.CommitTimestamp).To(Equal(timestamp))
		g.Expect(c.Results.URL).To(Equal("https://git.test/user/repo.git"))
		g.Expect(c.Results.ChainsGitURL).To(Equal("https://git.test/user/repo.git"))
		g.Expect(c.Results.ChainsGitCommit).To(Equal(fullSha))
	})

	t.Run("should fail if getting full SHA fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if !short {
				return "", errors.New("rev-parse failed")
			}
			return shortSha, nil
		}

		err := c.gatherCommitInfo()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get commit SHA"))
	})

	t.Run("should fail if getting short SHA fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				return "", errors.New("rev-parse short failed")
			}
			return fullSha, nil
		}

		err := c.gatherCommitInfo()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get short commit SHA"))
	})

	t.Run("should fail if getting timestamp fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				return shortSha, nil
			}
			return fullSha, nil
		}

		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			return "", errors.New("git log failed")
		}

		err := c.gatherCommitInfo()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get commit timestamp"))
	})

	t.Run("should use custom short commit length", func(t *testing.T) {
		beforeEach()
		c.Params.ShortCommitLength = 12

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				g.Expect(length).To(Equal(12))
				return "abc123def456", nil
			}
			return fullSha, nil
		}

		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			return timestamp, nil
		}

		err := c.gatherCommitInfo()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.Results.ShortCommit).To(Equal("abc123def456"))
	})
}

func Test_GitClone_performClone(t *testing.T) {
	g := NewWithT(t)

	var _mockGitCli *mockGitCli
	var c *GitClone
	var tmpDir string

	beforeEach := func() {
		tmpDir = t.TempDir()
		_mockGitCli = &mockGitCli{}
		c = &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				URL:              "https://git.test/user/repo.git",
				Depth:            1,
				RetryMaxAttempts: 10,
				OutputDir:        tmpDir,
				Subdirectory:     "source",
			},
		}
	}

	t.Run("should clone with basic parameters using init+fetch+checkout", func(t *testing.T) {
		beforeEach()

		isInitCalled := false
		_mockGitCli.InitFunc = func() error {
			isInitCalled = true
			return nil
		}

		isRemoteAddCalled := false
		_mockGitCli.RemoteAddFunc = func(name, url string) (string, error) {
			isRemoteAddCalled = true
			g.Expect(name).To(Equal("origin"))
			g.Expect(url).To(Equal("https://git.test/user/repo.git"))
			return "", nil
		}

		isFetchCalled := false
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			isFetchCalled = true
			g.Expect(opts.Remote).To(Equal("origin"))
			g.Expect(opts.Depth).To(Equal(1))
			return nil
		}

		isCheckoutCalled := false
		_mockGitCli.CheckoutFunc = func(ref string) error {
			isCheckoutCalled = true
			return nil
		}

		err := c.performClone()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isInitCalled).To(BeTrue())
		g.Expect(isRemoteAddCalled).To(BeTrue())
		g.Expect(isFetchCalled).To(BeTrue())
		g.Expect(isCheckoutCalled).To(BeTrue())
	})

	t.Run("should fetch with revision as refspec", func(t *testing.T) {
		beforeEach()
		c.Params.Revision = "develop"

		isFetchCalled := false
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			isFetchCalled = true
			g.Expect(opts.Refspec).To(Equal("develop"))
			return nil
		}

		err := c.performClone()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFetchCalled).To(BeTrue())
	})

	t.Run("should fetch with custom depth", func(t *testing.T) {
		beforeEach()
		c.Params.Depth = 50

		isFetchCalled := false
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			isFetchCalled = true
			g.Expect(opts.Depth).To(Equal(50))
			return nil
		}

		err := c.performClone()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFetchCalled).To(BeTrue())
	})

	t.Run("should fail if init fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.InitFunc = func() error {
			return errors.New("init failed")
		}

		err := c.performClone()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git init failed"))
	})

	t.Run("should pass maxAttempts to FetchWithRefspec", func(t *testing.T) {
		beforeEach()
		c.Params.RetryMaxAttempts = 5

		receivedMaxAttempts := 0
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			receivedMaxAttempts = opts.MaxAttempts
			return nil
		}

		err := c.performClone()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(receivedMaxAttempts).To(Equal(5))
	})

	t.Run("should fail if fetch fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			return errors.New("network error")
		}

		err := c.performClone()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git fetch failed"))
	})

	t.Run("should update submodules when enabled", func(t *testing.T) {
		beforeEach()
		c.Params.Submodules = true
		c.Params.SubmodulePaths = "lib,vendor"

		isSubmoduleUpdateCalled := false
		_mockGitCli.SubmoduleUpdateFunc = func(init bool, depth int, paths []string) error {
			isSubmoduleUpdateCalled = true
			g.Expect(init).To(BeTrue())
			g.Expect(depth).To(Equal(1))
			g.Expect(paths).To(Equal([]string{"lib", "vendor"}))
			return nil
		}

		err := c.performClone()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isSubmoduleUpdateCalled).To(BeTrue())
	})
}

func Test_GitClone_outputResults(t *testing.T) {
	g := NewWithT(t)

	var _mockResultsWriter *mockResultsWriter
	var c *GitClone

	beforeEach := func() {
		_mockResultsWriter = &mockResultsWriter{}
		c = &GitClone{
			ResultsWriter: _mockResultsWriter,
			Results: Results{
				Commit:          "abc123def456789012345678901234567890abcd",
				ShortCommit:     "abc123d",
				URL:             "https://git.test/user/repo.git",
				CommitTimestamp: "1704067200",
			},
		}
	}

	t.Run("should output results successfully", func(t *testing.T) {
		beforeEach()

		expectedJson := `{"commit":"abc123def456789012345678901234567890abcd"}`
		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			results, ok := result.(Results)
			g.Expect(ok).To(BeTrue())
			g.Expect(results.Commit).To(Equal("abc123def456789012345678901234567890abcd"))
			g.Expect(results.ShortCommit).To(Equal("abc123d"))
			g.Expect(results.URL).To(Equal("https://git.test/user/repo.git"))
			g.Expect(results.CommitTimestamp).To(Equal("1704067200"))
			return expectedJson, nil
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := c.outputResults()

		w.Close()
		var buf bytes.Buffer
		io.Copy(&buf, r)
		os.Stdout = oldStdout

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
		g.Expect(buf.String()).To(Equal(expectedJson + "\n"))
	})

	t.Run("should output results with merged SHA", func(t *testing.T) {
		beforeEach()
		c.Results.MergedSha = "def456abc789"

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			results, ok := result.(Results)
			g.Expect(ok).To(BeTrue())
			g.Expect(results.MergedSha).To(Equal("def456abc789"))
			return `{"commit":"abc123","mergedSha":"def456abc789"}`, nil
		}

		err := c.outputResults()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should fail if creating result json fails", func(t *testing.T) {
		beforeEach()

		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			return "", errors.New("failed to create json")
		}

		err := c.outputResults()

		g.Expect(err).To(HaveOccurred())
	})
}

func Test_GitClone_Run(t *testing.T) {
	g := NewWithT(t)

	const fullSha = "abc123def456789012345678901234567890abcd"
	const shortSha = "abc123d"
	const timestamp = "1704067200"

	var _mockGitCli *mockGitCli
	var _mockResultsWriter *mockResultsWriter
	var c *GitClone
	var tmpDir string

	beforeEach := func() {
		tmpDir = t.TempDir()
		_mockGitCli = &mockGitCli{}
		_mockResultsWriter = &mockResultsWriter{}
		c = &GitClone{
			CliWrappers:   CliWrappers{GitCli: _mockGitCli},
			ResultsWriter: _mockResultsWriter,
			Params: &Params{
				URL:               "https://git.test/user/repo.git",
				Depth:             1,
				ShortCommitLength: 7,
				OutputDir:         tmpDir,
				Subdirectory:      "source",
				RetryMaxAttempts:  10,
			},
		}
	}

	t.Run("should run successfully with basic parameters", func(t *testing.T) {
		beforeEach()

		isInitCalled := false
		_mockGitCli.InitFunc = func() error {
			isInitCalled = true
			return nil
		}

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				return shortSha, nil
			}
			return fullSha, nil
		}

		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			return timestamp, nil
		}

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			results, ok := result.(Results)
			g.Expect(ok).To(BeTrue())
			g.Expect(results.Commit).To(Equal(fullSha))
			g.Expect(results.ShortCommit).To(Equal(shortSha))
			g.Expect(results.CommitTimestamp).To(Equal(timestamp))
			return `{"commit":"abc123"}`, nil
		}

		err := c.Run()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isInitCalled).To(BeTrue())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should fail if URL is empty", func(t *testing.T) {
		beforeEach()
		c.Params.URL = ""

		err := c.Run()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("url parameter is required"))
	})

	t.Run("should fail if init fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.InitFunc = func() error {
			return errors.New("init failed")
		}

		err := c.Run()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("git init failed"))
	})

	t.Run("should fail if gathering commit info fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			return "", errors.New("rev-parse failed")
		}

		err := c.Run()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get commit SHA"))
	})

	t.Run("should fail if outputting results fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				return shortSha, nil
			}
			return fullSha, nil
		}

		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			return timestamp, nil
		}

		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			return "", errors.New("json marshal failed")
		}

		err := c.Run()

		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should run with revision parameter", func(t *testing.T) {
		beforeEach()
		c.Params.Revision = "v1.0.0"

		isFetchCalled := false
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			isFetchCalled = true
			g.Expect(opts.Refspec).To(Equal("v1.0.0"))
			return nil
		}

		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			if short {
				return shortSha, nil
			}
			return fullSha, nil
		}

		_mockGitCli.LogFunc = func(format string, count int) (string, error) {
			return timestamp, nil
		}

		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			return `{}`, nil
		}

		err := c.Run()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFetchCalled).To(BeTrue())
	})
}

func Test_normalizeGitURL(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "should strip trailing slash",
			input:    "https://git.test/user/repo/",
			expected: "https://git.test/user/repo",
		},
		{
			name:     "should strip .git suffix",
			input:    "https://git.test/user/repo.git",
			expected: "https://git.test/user/repo",
		},
		{
			name:     "should strip both trailing slash and .git",
			input:    "https://git.test/user/repo.git/",
			expected: "https://git.test/user/repo",
		},
		{
			name:     "should not modify clean URL",
			input:    "https://git.test/user/repo",
			expected: "https://git.test/user/repo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeGitURL(tc.input)
			g.Expect(result).To(Equal(tc.expected))
		})
	}
}

func Test_GitClone_mergeTargetBranch(t *testing.T) {
	g := NewWithT(t)

	const mergedSha = "merged123456789"

	var _mockGitCli *mockGitCli
	var c *GitClone

	beforeEach := func() {
		_mockGitCli = &mockGitCli{}
		c = &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				URL:              "https://git.test/user/repo.git",
				OutputDir:        "/workspace",
				Subdirectory:     "source",
				TargetBranch:     "main",
				Depth:            10,
				MergeSourceDepth: 0,
				RetryMaxAttempts:       3,
				MergeCommitAuthorName:  "Konflux CI Git Clone",
				MergeCommitAuthorEmail: "git-clone@konflux-ci.dev",
			},
			Results: Results{
				Commit: "abc123",
			},
		}
		_mockGitCli.RevParseFunc = func(ref string, short bool, length int) (string, error) {
			return mergedSha, nil
		}
	}

	t.Run("should merge from origin when no merge source URL", func(t *testing.T) {
		beforeEach()

		fetchedRemote := ""
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			fetchedRemote = opts.Remote
			g.Expect(opts.Refspec).To(Equal("main"))
			g.Expect(opts.Submodules).To(BeFalse())
			return nil
		}

		mergedRef := ""
		_mockGitCli.MergeFunc = func(ref, message string) (string, error) {
			mergedRef = ref
			return "", nil
		}

		err := c.mergeTargetBranch()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(fetchedRemote).To(Equal("origin"))
		g.Expect(mergedRef).To(Equal("origin/main"))
		g.Expect(c.Results.MergedSha).To(Equal(mergedSha))
	})

	t.Run("should use origin when merge source URL matches", func(t *testing.T) {
		beforeEach()
		c.Params.MergeSourceRepoURL = "https://git.test/user/repo.git"

		fetchedRemote := ""
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			fetchedRemote = opts.Remote
			return nil
		}

		_mockGitCli.MergeFunc = func(ref, message string) (string, error) {
			return "", nil
		}

		err := c.mergeTargetBranch()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(fetchedRemote).To(Equal("origin"))
	})

	t.Run("should add merge-source remote for different repo", func(t *testing.T) {
		beforeEach()
		c.Params.MergeSourceRepoURL = "https://github.com/other/repo"

		addedRemoteName := ""
		_mockGitCli.RemoteAddFunc = func(name, url string) (string, error) {
			addedRemoteName = name
			g.Expect(url).To(Equal("https://github.com/other/repo"))
			return "", nil
		}

		fetchedRemote := ""
		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			fetchedRemote = opts.Remote
			return nil
		}

		mergedRef := ""
		_mockGitCli.MergeFunc = func(ref, message string) (string, error) {
			mergedRef = ref
			return "", nil
		}

		err := c.mergeTargetBranch()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(addedRemoteName).To(Equal("merge-source"))
		g.Expect(fetchedRemote).To(Equal("merge-source"))
		g.Expect(mergedRef).To(Equal("merge-source/main"))
	})

	t.Run("should fail if remote add fails", func(t *testing.T) {
		beforeEach()
		c.Params.MergeSourceRepoURL = "https://github.com/other/repo"

		_mockGitCli.RemoteAddFunc = func(name, url string) (string, error) {
			return "", errors.New("remote add failed")
		}

		err := c.mergeTargetBranch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("remote add failed"))
	})

	t.Run("should fail if fetch fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.FetchWithRefspecFunc = func(opts cliwrappers.GitFetchOptions) error {
			return errors.New("fetch failed")
		}

		err := c.mergeTargetBranch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("fetch failed"))
	})

	t.Run("should fail if merge fails", func(t *testing.T) {
		beforeEach()

		_mockGitCli.MergeFunc = func(ref, message string) (string, error) {
			return "", errors.New("merge conflict")
		}

		err := c.mergeTargetBranch()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("merge conflict"))
	})

	t.Run("should set config with correct email and name", func(t *testing.T) {
		beforeEach()

		configValues := map[string]string{}
		_mockGitCli.ConfigLocalFunc = func(key, value string) error {
			configValues[key] = value
			return nil
		}
		_mockGitCli.MergeFunc = func(ref, message string) (string, error) {
			return "", nil
		}

		err := c.mergeTargetBranch()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configValues["user.email"]).To(Equal("git-clone@konflux-ci.dev"))
		g.Expect(configValues["user.name"]).To(Equal("Konflux CI Git Clone"))
	})
}

func Test_GitClone_cleanCheckoutDir(t *testing.T) {
	g := NewWithT(t)

	t.Run("should remove contents but preserve directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		checkoutDir := filepath.Join(tmpDir, "source")
		g.Expect(os.MkdirAll(checkoutDir, 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(checkoutDir, "file1.txt"), []byte("hello"), 0644)).To(Succeed())
		g.Expect(os.MkdirAll(filepath.Join(checkoutDir, "subdir"), 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(checkoutDir, "subdir", "file2.txt"), []byte("world"), 0644)).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "source"},
		}

		err := c.cleanCheckoutDir()

		g.Expect(err).ToNot(HaveOccurred())
		entries, err := os.ReadDir(checkoutDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(BeEmpty())
		// Directory itself still exists
		_, err = os.Stat(checkoutDir)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should succeed if directory does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "nonexistent"},
		}

		err := c.cleanCheckoutDir()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should fail if path is a file not a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "source")
		g.Expect(os.WriteFile(filePath, []byte("not a dir"), 0644)).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "source"},
		}

		err := c.cleanCheckoutDir()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not a directory"))
	})
}

func Test_GitClone_setupBasicAuth(t *testing.T) {
	g := NewWithT(t)

	t.Run("should skip when no basic auth directory", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{BasicAuthDirectory: ""},
		}

		err := c.setupBasicAuth()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should skip when auth directory does not exist", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{BasicAuthDirectory: "/nonexistent/path"},
		}

		err := c.setupBasicAuth()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should copy .git-credentials and rewrite .gitconfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		authDir := filepath.Join(tmpDir, "auth")
		internalDir := t.TempDir()
		g.Expect(os.MkdirAll(authDir, 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(authDir, ".git-credentials"), []byte("https://user:pass@github.com"), 0644)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(authDir, ".gitconfig"), []byte("[credential]\n  helper = store"), 0644)).To(Succeed())

		envVars := map[string]string{}
		_mockGitCli := &mockGitCli{
			SetEnvFunc: func(key, value string) { envVars[key] = value },
		}
		c := &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				URL:                "https://git.test/user/repo",
				BasicAuthDirectory: authDir,
			},
			internalDir: internalDir,
		}

		err := c.setupBasicAuth()

		g.Expect(err).ToNot(HaveOccurred())
		creds, err := os.ReadFile(filepath.Join(internalDir, ".git-credentials"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(creds)).To(Equal("https://user:pass@github.com"))
		config, err := os.ReadFile(filepath.Join(internalDir, ".gitconfig"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(config)).To(ContainSubstring("helper = store --file=" + filepath.Join(internalDir, ".git-credentials")))
		g.Expect(envVars[envGitConfigGlobal]).To(Equal(filepath.Join(internalDir, ".gitconfig")))
	})

	t.Run("should generate credentials from username/password", func(t *testing.T) {
		tmpDir := t.TempDir()
		authDir := filepath.Join(tmpDir, "auth")
		internalDir := t.TempDir()
		g.Expect(os.MkdirAll(authDir, 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(authDir, "username"), []byte("myuser"), 0644)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(authDir, "password"), []byte("mypass"), 0644)).To(Succeed())

		envVars := map[string]string{}
		_mockGitCli := &mockGitCli{
			SetEnvFunc: func(key, value string) { envVars[key] = value },
		}
		c := &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				URL:                "https://git.test/user/repo",
				BasicAuthDirectory: authDir,
			},
			internalDir: internalDir,
		}

		err := c.setupBasicAuth()

		g.Expect(err).ToNot(HaveOccurred())
		creds, err := os.ReadFile(filepath.Join(internalDir, ".git-credentials"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(creds)).To(Equal("https://myuser:mypass@git.test\n"))
		config, err := os.ReadFile(filepath.Join(internalDir, ".gitconfig"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(config)).To(ContainSubstring("helper = store --file=" + filepath.Join(internalDir, ".git-credentials")))
		g.Expect(envVars[envGitConfigGlobal]).To(Equal(filepath.Join(internalDir, ".gitconfig")))
	})

	t.Run("should fail with unknown auth format", func(t *testing.T) {
		tmpDir := t.TempDir()
		authDir := filepath.Join(tmpDir, "auth")
		g.Expect(os.MkdirAll(authDir, 0755)).To(Succeed())
		// Create an empty directory - neither format matches

		c := &GitClone{
			Params: &Params{
				URL:                "https://git.test/user/repo",
				BasicAuthDirectory: authDir,
			},
			internalDir: t.TempDir(),
		}

		err := c.setupBasicAuth()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("unknown basic-auth workspace format"))
	})
}

func Test_GitClone_setupSSH(t *testing.T) {
	g := NewWithT(t)

	t.Run("should skip when no ssh directory", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{SSHDirectory: ""},
		}

		err := c.setupSSH()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should skip when ssh directory does not exist", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{SSHDirectory: "/nonexistent/path"},
		}

		err := c.setupSSH()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should copy SSH files and set GIT_SSH_COMMAND", func(t *testing.T) {
		tmpDir := t.TempDir()
		sshDir := filepath.Join(tmpDir, "ssh-keys")
		internalDir := t.TempDir()
		g.Expect(os.MkdirAll(sshDir, 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("private-key"), 0644)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte("github.com ssh-rsa AAAA..."), 0644)).To(Succeed())

		envVars := map[string]string{}
		_mockGitCli := &mockGitCli{
			SetEnvFunc: func(key, value string) { envVars[key] = value },
		}
		c := &GitClone{
			CliWrappers: CliWrappers{GitCli: _mockGitCli},
			Params: &Params{
				SSHDirectory: sshDir,
			},
			internalDir: internalDir,
		}

		err := c.setupSSH()

		g.Expect(err).ToNot(HaveOccurred())
		key, err := os.ReadFile(filepath.Join(internalDir, ".ssh", "id_rsa"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(key)).To(Equal("private-key"))
		hosts, err := os.ReadFile(filepath.Join(internalDir, ".ssh", "known_hosts"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(hosts)).To(Equal("github.com ssh-rsa AAAA..."))

		sshCmd := envVars[envGitSSHCommand]
		g.Expect(sshCmd).To(ContainSubstring(fmt.Sprintf(`-i "%s"`, filepath.Join(internalDir, ".ssh", "id_rsa"))))
		g.Expect(sshCmd).To(ContainSubstring(fmt.Sprintf(`-o UserKnownHostsFile="%s"`, filepath.Join(internalDir, ".ssh", "known_hosts"))))
		g.Expect(sshCmd).To(ContainSubstring("-F /dev/null"))
	})

	t.Run("should skip subdirectories", func(t *testing.T) {
		tmpDir := t.TempDir()
		sshDir := filepath.Join(tmpDir, "ssh-keys")
		internalDir := t.TempDir()
		g.Expect(os.MkdirAll(sshDir, 0755)).To(Succeed())
		g.Expect(os.MkdirAll(filepath.Join(sshDir, "subdir"), 0755)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(sshDir, "id_rsa"), []byte("key"), 0644)).To(Succeed())

		c := &GitClone{
			CliWrappers: CliWrappers{GitCli: &mockGitCli{}},
			Params: &Params{
				SSHDirectory: sshDir,
			},
			internalDir: internalDir,
		}

		err := c.setupSSH()

		g.Expect(err).ToNot(HaveOccurred())
		_, err = os.Stat(filepath.Join(internalDir, ".ssh", "subdir"))
		g.Expect(os.IsNotExist(err)).To(BeTrue())
	})
}

func Test_GitClone_validateParams_subdirectory(t *testing.T) {
	g := NewWithT(t)

	t.Run("should reject absolute subdirectory", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", Subdirectory: "/etc/passwd", RetryMaxAttempts: 1},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("subdirectory must be a relative path"))
	})

	t.Run("should reject path traversal with ..", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", Subdirectory: "../../../etc/passwd", RetryMaxAttempts: 1},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("subdirectory must not contain path traversal"))
	})

	t.Run("should reject subdirectory escaping output dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		c := &GitClone{
			Params: &Params{
				URL:              "https://git.test/user/repo",
				OutputDir:        tmpDir,
				Subdirectory:     "source",
				RetryMaxAttempts: 1,
			},
		}

		err := c.validateParams()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should validate subdirectory even when OutputDir is empty", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{
				URL:              "https://git.test/user/repo",
				OutputDir:        "",
				Subdirectory:     "safe/nested/path",
				RetryMaxAttempts: 1,
			},
		}

		err := c.validateParams()

		g.Expect(err).ToNot(HaveOccurred())
	})
}

func Test_readFileWithLimit(t *testing.T) {
	g := NewWithT(t)

	t.Run("should read file within limit", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "small.txt")
		g.Expect(os.WriteFile(tmpFile, []byte("hello"), 0644)).To(Succeed())

		data, err := readFileWithLimit(tmpFile, 1024)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(data)).To(Equal("hello"))
	})

	t.Run("should reject file exceeding limit", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "large.txt")
		// Create a file larger than the limit
		largeData := make([]byte, 2048)
		g.Expect(os.WriteFile(tmpFile, largeData, 0644)).To(Succeed())

		_, err := readFileWithLimit(tmpFile, 1024)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("exceeds maximum allowed size"))
	})

	t.Run("should return error for nonexistent file", func(t *testing.T) {
		_, err := readFileWithLimit("/nonexistent/file", 1024)

		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should read file exactly at limit", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "exact.txt")
		exactData := make([]byte, 1024)
		g.Expect(os.WriteFile(tmpFile, exactData, 0644)).To(Succeed())

		data, err := readFileWithLimit(tmpFile, 1024)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(len(data)).To(Equal(1024))
	})
}

func Test_GitClone_verifyCheckoutDirContainment(t *testing.T) {
	g := NewWithT(t)

	t.Run("should pass with no subdirectory", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{OutputDir: t.TempDir(), Subdirectory: ""},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should pass with normal subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "source"},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should pass with existing real subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		g.Expect(os.MkdirAll(filepath.Join(tmpDir, "source"), 0755)).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "source"},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject symlinked subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := t.TempDir()
		g.Expect(os.Symlink(target, filepath.Join(tmpDir, "evil"))).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "evil"},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("symlink"))
	})

	t.Run("should reject symlink in intermediate path component", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := t.TempDir()
		// Create tmpDir/link -> target, then use subdirectory "link/nested"
		g.Expect(os.Symlink(target, filepath.Join(tmpDir, "link"))).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "link/nested"},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("symlink"))
	})

	t.Run("should pass with nested non-symlink subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		g.Expect(os.MkdirAll(filepath.Join(tmpDir, "a", "b"), 0755)).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "a/b"},
		}

		err := c.verifyCheckoutDirContainment()

		g.Expect(err).ToNot(HaveOccurred())
	})
}

func Test_GitClone_cleanCheckoutDir_symlink(t *testing.T) {
	g := NewWithT(t)

	t.Run("should reject symlinked checkout directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := t.TempDir()
		// Place a file in target so we can verify it's not deleted
		g.Expect(os.WriteFile(filepath.Join(target, "important.txt"), []byte("do not delete"), 0644)).To(Succeed())

		g.Expect(os.Symlink(target, filepath.Join(tmpDir, "source"))).To(Succeed())

		c := &GitClone{
			Params: &Params{OutputDir: tmpDir, Subdirectory: "source"},
		}

		err := c.cleanCheckoutDir()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("symlink"))

		// Verify target file was not deleted
		_, err = os.Stat(filepath.Join(target, "important.txt"))
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func Test_GitClone_validateParams_depth(t *testing.T) {
	g := NewWithT(t)

	t.Run("should fail with negative depth", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", Depth: -1},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("depth must be >= 0"))
	})

	t.Run("should pass with depth 0 (full history)", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", Depth: 0, RetryMaxAttempts: 1},
		}

		err := c.validateParams()

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should fail with negative merge-source-depth", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", MergeSourceDepth: -5, RetryMaxAttempts: 1},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("merge-source-depth must be >= 0"))
	})

	t.Run("should fail with retry-max-attempts exceeding upper bound", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", RetryMaxAttempts: 101},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("retry-max-attempts must be <= 100"))
	})

	t.Run("should fail with retry-max-attempts below lower bound", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", RetryMaxAttempts: 0},
		}

		err := c.validateParams()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("retry-max-attempts must be >= 1"))
	})

	t.Run("should pass with retry-max-attempts at upper bound", func(t *testing.T) {
		c := &GitClone{
			Params: &Params{URL: "https://git.test/user/repo", RetryMaxAttempts: 100},
		}

		err := c.validateParams()

		g.Expect(err).ToNot(HaveOccurred())
	})
}
