package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/testutil"
	. "github.com/onsi/gomega"
)

func Test_Build_validateParams(t *testing.T) {
	g := NewWithT(t)

	tempDir := t.TempDir()

	os.WriteFile(filepath.Join(tempDir, "notadir"), []byte("content"), 0644)

	tests := []struct {
		name         string
		params       BuildParams
		setupFunc    func() string // returns context directory
		errExpected  bool
		errSubstring string
	}{
		{
			name: "should allow valid parameters",
			params: BuildParams{
				OutputRef:     "quay.io/org/image:tag",
				Context:       tempDir,
				Containerfile: "",
			},
			errExpected: false,
		},
		{
			name: "should allow valid parameters with containerfile",
			params: BuildParams{
				OutputRef:     "registry.io/namespace/image:v1.0",
				Context:       tempDir,
				Containerfile: "Dockerfile",
			},
			errExpected: false,
		},
		{
			name: "should fail on invalid output-ref",
			params: BuildParams{
				OutputRef: "quay.io/org/imAge",
				Context:   tempDir,
			},
			errExpected:  true,
			errSubstring: "output-ref",
		},
		{
			name: "should fail on missing context directory",
			params: BuildParams{
				OutputRef: "quay.io/org/image:tag",
				Context:   filepath.Join(tempDir, "nonexistent"),
			},
			errExpected:  true,
			errSubstring: "does not exist",
		},
		{
			name: "should fail when context is a file not directory",
			params: BuildParams{
				OutputRef: "quay.io/org/image:tag",
				Context:   filepath.Join(tempDir, "notadir"),
			},
			errExpected:  true,
			errSubstring: "is not a directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Build{Params: &tc.params}

			if tc.setupFunc != nil {
				c.Params.Context = tc.setupFunc()
			}

			err := c.validateParams()

			if tc.errExpected {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(MatchRegexp(tc.errSubstring))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_Build_detectContainerfile(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		files            []string // files to create (paths relative to tempDir)
		containerfileArg string
		contextArg       string
		expectedPath     string
		expectError      bool
		errorContains    string
	}{
		{
			name:         "should auto-detect Containerfile in workdir",
			files:        []string{"Containerfile"},
			expectedPath: "Containerfile",
		},
		{
			name:         "should auto-detect Dockerfile in workdir",
			files:        []string{"Dockerfile"},
			expectedPath: "Dockerfile",
		},
		{
			name:         "should prefer Containerfile over Dockerfile when both exist",
			files:        []string{"Containerfile", "Dockerfile"},
			expectedPath: "Containerfile",
		},
		{
			name:         "should auto-detect Containerfile in context dir",
			files:        []string{"context/Containerfile"},
			contextArg:   "context",
			expectedPath: "context/Containerfile",
		},
		{
			name:         "should auto-detect Dockerfile in context dir",
			files:        []string{"context/Dockerfile"},
			contextArg:   "context",
			expectedPath: "context/Dockerfile",
		},
		{
			name:         "should prefer Containerfile over Dockerfile in context dir",
			files:        []string{"context/Containerfile", "context/Dockerfile"},
			contextArg:   "context",
			expectedPath: "context/Containerfile",
		},
		{
			name:             "should use explicit containerfile",
			files:            []string{"custom.dockerfile"},
			containerfileArg: "custom.dockerfile",
			expectedPath:     "custom.dockerfile",
		},
		{
			name:             "should fallback to context directory for explicit containerfile",
			files:            []string{"context/custom.dockerfile"},
			containerfileArg: "custom.dockerfile",
			contextArg:       "context",
			expectedPath:     "context/custom.dockerfile",
		},
		{
			name:             "should only fallback to context if the bare path doesn't exist",
			files:            []string{"custom.dockerfile", "context/custom.dockerfile"},
			containerfileArg: "custom.dockerfile",
			contextArg:       "context",
			expectedPath:     "custom.dockerfile",
		},
		{
			name:             "should fail when explicit containerfile not found",
			containerfileArg: "nonexistent.dockerfile",
			expectError:      true,
			errorContains:    "not found",
		},
		{
			name:          "should fail when no implicit containerfile found",
			expectError:   true,
			errorContains: "no Containerfile or Dockerfile found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()

			cwd, _ := os.Getwd()
			os.Chdir(tempDir)
			if cwd != "" {
				defer os.Chdir(cwd)
			}

			for _, filePath := range tc.files {
				dir := filepath.Dir(filePath)
				if dir != tempDir {
					os.MkdirAll(dir, 0755)
				}
				os.WriteFile(filePath, []byte("FROM scratch"), 0644)
			}

			if tc.contextArg == "" {
				tc.contextArg = "."
			}
			c := &Build{
				Params: &BuildParams{
					Context:       tc.contextArg,
					Containerfile: tc.containerfileArg,
				},
			}

			err := c.detectContainerfile()

			if tc.expectError {
				g.Expect(err).To(HaveOccurred())
				if tc.errorContains != "" {
					g.Expect(err.Error()).To(ContainSubstring(tc.errorContains))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(c.containerfilePath).To(Equal(tc.expectedPath))
			}
		})
	}
}

func Test_Build_setSecretArgs(t *testing.T) {
	g := NewWithT(t)

	t.Run("should append nothing when SecretDirs is nil", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: nil,
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(BeEmpty())
	})

	t.Run("should append nothing when SecretDirs is empty", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(BeEmpty())
	})

	t.Run("should append nothing for empty directory", func(t *testing.T) {
		tempDir := t.TempDir()
		emptyDir := filepath.Join(tempDir, "empty")
		if err := os.Mkdir(emptyDir, 0755); err != nil {
			t.Fatalf("Failed to create empty directory: %s", err)
		}

		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{emptyDir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(BeEmpty())
	})

	t.Run("should process single file in directory", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token": "secret-token",
		})

		secretDir := filepath.Join(tempDir, "secret1")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secretDir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secretDir, "token"), Id: "secret1/token"},
		}))
	})

	t.Run("should process multiple files in directory", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/password": "secret-pass",
			"secret1/token":    "secret-token",
		})

		secretDir := filepath.Join(tempDir, "secret1")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secretDir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secretDir, "password"), Id: "secret1/password"},
			{Src: filepath.Join(secretDir, "token"), Id: "secret1/token"},
		}))
	})

	t.Run("should process multiple directories", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token":    "token1",
			"secret2/password": "pass2",
		})

		secret1Dir := filepath.Join(tempDir, "secret1")
		secret2Dir := filepath.Join(tempDir, "secret2")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secret1Dir, secret2Dir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secret1Dir, "token"), Id: "secret1/token"},
			{Src: filepath.Join(secret2Dir, "password"), Id: "secret2/password"},
		}))
	})

	t.Run("should use custom name from name parameter", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token": "secret-token",
		})

		secretDir := filepath.Join(tempDir, "secret1")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{"src=" + secretDir + ",name=custom"},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secretDir, "token"), Id: "custom/token"},
		}))
	})

	t.Run("should skip subdirectories", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token":         "secret-token",
			"secret1/subdir/nested": "nested",
		})

		secretDir := filepath.Join(tempDir, "secret1")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secretDir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secretDir, "token"), Id: "secret1/token"},
		}))
	})

	t.Run("should allow same filename in different directories", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token": "token1",
			"secret2/token": "token2",
		})

		secret1Dir := filepath.Join(tempDir, "secret1")
		secret2Dir := filepath.Join(tempDir, "secret2")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secret1Dir, secret2Dir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: filepath.Join(secret1Dir, "token"), Id: "secret1/token"},
			{Src: filepath.Join(secret2Dir, "token"), Id: "secret2/token"},
		}))
	})

	t.Run("should error on duplicate secret IDs", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/token":       "token1",
			"other/secret1/token": "token2",
		})

		secret1Dir := filepath.Join(tempDir, "secret1")
		otherSecret1Dir := filepath.Join(tempDir, "other", "secret1")
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secret1Dir, otherSecret1Dir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("duplicate secret ID 'secret1/token'"))
	})

	t.Run("should error when directory does not exist", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{"/nonexistent/path"},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to read secret directory /nonexistent/path"))
	})

	t.Run("should not error when optional directory does not exist", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{"src=/nonexistent/path,optional=true"},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(BeEmpty())
	})

	t.Run("should error on invalid SecretDirs syntax", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{"src=/path,invalid=value"},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid attribute: invalid"))
	})

	t.Run("should error on invalid optional value", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{"src=/path,optional=maybe"},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid argument: optional=maybe"))
	})

	t.Run("should process symlink to file but skip symlink to directory", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secret1/..data/token": "secret-token",
			// secret1/token -> ..data/token
			// secret1/data -> ..data
		})

		secretDir := filepath.Join(tempDir, "secret1")
		tokenSymlink := filepath.Join(secretDir, "token")
		dataSymlink := filepath.Join(secretDir, "data")

		if err := os.Symlink("..data/token", tokenSymlink); err != nil {
			t.Fatalf("Failed to create symlink to file: %s", err)
		}
		if err := os.Symlink("..data", dataSymlink); err != nil {
			t.Fatalf("Failed to create symlink to directory: %s", err)
		}

		c := &Build{
			Params: &BuildParams{
				SecretDirs: []string{secretDir},
			},
		}

		err := c.setSecretArgs()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.buildahSecrets).To(Equal([]cliwrappers.BuildahSecret{
			{Src: tokenSymlink, Id: "secret1/token"},
		}))
	})
}

func Test_Build_parseContainerfile(t *testing.T) {
	g := NewWithT(t)

	t.Run("should successfully parse valid Containerfile", func(t *testing.T) {
		tempDir := t.TempDir()
		containerfilePath := filepath.Join(tempDir, "Containerfile")
		os.WriteFile(containerfilePath, []byte("FROM scratch\nRUN echo hello"), 0644)

		c := &Build{containerfilePath: containerfilePath, Params: &BuildParams{}}
		result, err := c.parseContainerfile()

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).ToNot(BeNil())
	})

	t.Run("should return error for non-existent file", func(t *testing.T) {
		c := &Build{containerfilePath: "/nonexistent/Containerfile", Params: &BuildParams{}}
		result, err := c.parseContainerfile()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(MatchRegexp("failed to parse /nonexistent/Containerfile:.* no such file or directory"))
		g.Expect(result).To(BeNil())
	})

	t.Run("should return error for invalid Containerfile syntax", func(t *testing.T) {
		tempDir := t.TempDir()
		containerfilePath := filepath.Join(tempDir, "Containerfile")
		os.WriteFile(containerfilePath, []byte("INVALID SYNTAX HERE"), 0644)

		c := &Build{containerfilePath: containerfilePath, Params: &BuildParams{}}
		result, err := c.parseContainerfile()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(MatchRegexp("failed to parse .*: unknown instruction: INVALID"))
		g.Expect(result).To(BeNil())
	})
}

func Test_Build_writeContainerfileJson(t *testing.T) {
	g := NewWithT(t)

	t.Run("should successfully write JSON to specified path", func(t *testing.T) {
		tempDir := t.TempDir()
		outputPath := filepath.Join(tempDir, "containerfile.json")

		containerfilePath := filepath.Join(tempDir, "Containerfile")
		os.WriteFile(containerfilePath, []byte("FROM scratch"), 0644)

		c := &Build{containerfilePath: containerfilePath, Params: &BuildParams{}}
		containerfile, err := c.parseContainerfile()
		g.Expect(err).ToNot(HaveOccurred())

		err = c.writeContainerfileJson(containerfile, outputPath)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(outputPath).To(BeAnExistingFile())

		content, err := os.ReadFile(outputPath)
		g.Expect(err).ToNot(HaveOccurred())

		// Full file content tested in integration tests
		g.Expect(string(content)).To(ContainSubstring(`"Stages":`))
	})

	t.Run("should return error when path is not writable", func(t *testing.T) {
		tempDir := t.TempDir()
		containerfilePath := filepath.Join(tempDir, "Containerfile")
		os.WriteFile(containerfilePath, []byte("FROM scratch"), 0644)

		c := &Build{containerfilePath: containerfilePath, Params: &BuildParams{}}
		containerfile, err := c.parseContainerfile()
		g.Expect(err).ToNot(HaveOccurred())

		unwritablePath := "/nonexistent/directory/containerfile.json"
		err = c.writeContainerfileJson(containerfile, unwritablePath)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to write Containerfile JSON"))
	})
}

func Test_Build_createBuildArgExpander(t *testing.T) {
	g := NewWithT(t)

	t.Run("should expand build args from CLI with KEY=value format", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				BuildArgs: []string{"NAME=foo", "VERSION=1.2.3"},
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("NAME")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("foo"))

		value, err = expander("VERSION")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("1.2.3"))
	})

	t.Run("should expand build args from CLI with KEY format (env lookup)", func(t *testing.T) {
		os.Setenv("TEST_ENV_VAR", "from-env")
		defer os.Unsetenv("TEST_ENV_VAR")

		c := &Build{
			Params: &BuildParams{
				BuildArgs: []string{"TEST_ENV_VAR"},
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("TEST_ENV_VAR")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("from-env"))
	})

	t.Run("should expand build args from file", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"build-args": "AUTHOR=John Doe\nVENDOR=konflux-ci.dev\n",
		})

		c := &Build{
			Params: &BuildParams{
				BuildArgsFile: filepath.Join(tempDir, "build-args"),
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("AUTHOR")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("John Doe"))

		value, err = expander("VENDOR")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("konflux-ci.dev"))
	})

	t.Run("should give CLI args precedence over file args", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"build-args": "NAME=file-value\nOTHER=from-file\n",
		})

		c := &Build{
			Params: &BuildParams{
				BuildArgs:     []string{"NAME=cli-value"},
				BuildArgsFile: filepath.Join(tempDir, "build-args"),
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("NAME")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("cli-value"))

		value, err = expander("OTHER")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("from-file"))
	})

	t.Run("should return error for undefined build arg", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("UNDEFINED")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not defined"))
		g.Expect(value).To(BeEmpty())
	})

	t.Run("should error when build args file not found", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				BuildArgsFile: "/nonexistent/build-args",
			},
		}

		expander, err := c.createBuildArgExpander()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to read build args file"))
		g.Expect(expander).To(BeNil())
	})

	t.Run("should error when build args file has invalid format", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"build-args": "INVALID LINE\n",
		})

		c := &Build{
			Params: &BuildParams{
				BuildArgsFile: filepath.Join(tempDir, "build-args"),
			},
		}

		expander, err := c.createBuildArgExpander()

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to read build args file"))
		g.Expect(expander).To(BeNil())
	})
}

func Test_Build_Run(t *testing.T) {
	g := NewWithT(t)

	var _mockBuildahCli *mockBuildahCli
	var _mockResultsWriter *mockResultsWriter
	var c *Build
	var tempDir string

	beforeEach := func() {
		tempDir = t.TempDir()
		contextDir := filepath.Join(tempDir, "context")
		os.Mkdir(contextDir, 0755)
		os.WriteFile(filepath.Join(contextDir, "Containerfile"), []byte("FROM scratch"), 0644)

		_mockBuildahCli = &mockBuildahCli{}
		_mockResultsWriter = &mockResultsWriter{}
		c = &Build{
			CliWrappers: BuildCliWrappers{BuildahCli: _mockBuildahCli},
			Params: &BuildParams{
				OutputRef:     "quay.io/org/image:tag",
				Context:       contextDir,
				Containerfile: "",
				Push:          true,
			},
			ResultsWriter: _mockResultsWriter,
		}
	}

	t.Run("should successfully build and push image", func(t *testing.T) {
		beforeEach()

		isBuildCalled := false
		_mockBuildahCli.BuildFunc = func(args *cliwrappers.BuildahBuildArgs) error {
			isBuildCalled = true
			g.Expect(args.OutputRef).To(Equal("quay.io/org/image:tag"))
			g.Expect(args.ContextDir).To(Equal(c.Params.Context))
			g.Expect(args.Containerfile).To(ContainSubstring("Containerfile"))
			return nil
		}

		isPushCalled := false
		_mockBuildahCli.PushFunc = func(args *cliwrappers.BuildahPushArgs) (string, error) {
			isPushCalled = true
			g.Expect(args.Image).To(Equal("quay.io/org/image:tag"))
			return "sha256:1234567890abcdef", nil
		}

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			buildResults, ok := result.(BuildResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(buildResults.ImageUrl).To(Equal("quay.io/org/image:tag"))
			g.Expect(buildResults.Digest).To(Equal("sha256:1234567890abcdef"))
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isBuildCalled).To(BeTrue())
		g.Expect(isPushCalled).To(BeTrue())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should successfully build without pushing", func(t *testing.T) {
		beforeEach()
		c.Params.Push = false

		isBuildCalled := false
		_mockBuildahCli.BuildFunc = func(args *cliwrappers.BuildahBuildArgs) error {
			isBuildCalled = true
			g.Expect(args.OutputRef).To(Equal("quay.io/org/image:tag"))
			return nil
		}

		isPushCalled := false
		_mockBuildahCli.PushFunc = func(args *cliwrappers.BuildahPushArgs) (string, error) {
			isPushCalled = true
			return "", nil
		}

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			buildResults, ok := result.(BuildResults)
			g.Expect(ok).To(BeTrue())
			g.Expect(buildResults.ImageUrl).To(Equal("quay.io/org/image:tag"))
			g.Expect(buildResults.Digest).To(BeEmpty())
			return "", nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isBuildCalled).To(BeTrue())
		g.Expect(isPushCalled).To(BeFalse())
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should pass buildahSecrets to buildah build", func(t *testing.T) {
		beforeEach()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"secrets/token": "secret-token",
		})
		secretDir := filepath.Join(tempDir, "secrets")
		c.Params.SecretDirs = []string{secretDir}

		isBuildCalled := false
		_mockBuildahCli.BuildFunc = func(args *cliwrappers.BuildahBuildArgs) error {
			isBuildCalled = true
			g.Expect(args.Secrets).To(Equal([]cliwrappers.BuildahSecret{
				{Src: filepath.Join(secretDir, "token"), Id: "secrets/token"},
			}))
			return nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isBuildCalled).To(BeTrue())
	})

	t.Run("should error if build fails", func(t *testing.T) {
		beforeEach()

		_mockBuildahCli.BuildFunc = func(args *cliwrappers.BuildahBuildArgs) error {
			return errors.New("buildah build failed")
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("buildah build failed"))
	})

	t.Run("should error if push fails", func(t *testing.T) {
		beforeEach()

		_mockBuildahCli.PushFunc = func(args *cliwrappers.BuildahPushArgs) (string, error) {
			return "", errors.New("buildah push failed")
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("buildah push failed"))
	})

	t.Run("should error if validation fails", func(t *testing.T) {
		beforeEach()
		c.Params.OutputRef = "invalid//image"

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if containerfile detection fails", func(t *testing.T) {
		beforeEach()
		// Remove the Containerfile
		os.Remove(filepath.Join(c.Params.Context, "Containerfile"))

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("no Containerfile or Dockerfile found"))
	})

	t.Run("should error if results json creation fails", func(t *testing.T) {
		beforeEach()

		isCreateResultJsonCalled := false
		_mockResultsWriter.CreateResultJsonFunc = func(result any) (string, error) {
			isCreateResultJsonCalled = true
			return "", errors.New("failed to create results json")
		}

		err := c.Run()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to create results json"))
		g.Expect(isCreateResultJsonCalled).To(BeTrue())
	})

	t.Run("should run buildah inside context directory with absolute paths", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"Containerfile":   "FROM scratch",
			"context/main.go": "package main",
			"secrets/token":   "secret-token",
		})

		originalCwd, _ := os.Getwd()
		os.Chdir(tempDir)
		defer os.Chdir(originalCwd)

		_mockBuildahCli := &mockBuildahCli{}
		_mockResultsWriter := &mockResultsWriter{}
		c := &Build{
			CliWrappers: BuildCliWrappers{BuildahCli: _mockBuildahCli},
			Params: &BuildParams{
				OutputRef:     "quay.io/org/image:tag",
				Containerfile: "Containerfile",
				Context:       "context",
				SecretDirs:    []string{"secrets"},
			},
			ResultsWriter: _mockResultsWriter,
		}

		expectedContextDir := filepath.Join(tempDir, "context")
		expectedContainerfile := filepath.Join(tempDir, "Containerfile")
		expectedSecretSrc := filepath.Join(tempDir, "secrets/token")

		_mockBuildahCli.BuildFunc = func(args *cliwrappers.BuildahBuildArgs) error {
			currentDir, err := os.Getwd()
			g.Expect(err).ToNot(HaveOccurred())

			// Check that the buildah build happens inside the contextDir
			g.Expect(currentDir).To(Equal(expectedContextDir))

			g.Expect(args.Containerfile).To(Equal(expectedContainerfile))
			g.Expect(args.ContextDir).To(Equal(expectedContextDir))
			g.Expect(args.Secrets).To(HaveLen(1))
			g.Expect(args.Secrets[0].Src).To(Equal(expectedSecretSrc))

			return nil
		}

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())

		// Check that the Run() function restored the cwd on exit
		restoredDir, _ := os.Getwd()
		g.Expect(restoredDir).To(Equal(tempDir))
	})
}
