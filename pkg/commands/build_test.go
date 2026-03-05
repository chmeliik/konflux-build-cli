package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/keilerkonzept/dockerfile-json/pkg/dockerfile"
	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/testutil"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
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
		{
			name: "should fail when when legacy-build-timestamp and source-date-epoch are used together",
			params: BuildParams{
				OutputRef:            "quay.io/org/image:tag",
				Context:              tempDir,
				LegacyBuildTimestamp: "1",
				SourceDateEpoch:      "1",
			},
			errExpected:  true,
			errSubstring: "are mutually exclusive",
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

	t.Run("should inject Envs", func(t *testing.T) {
		tempDir := t.TempDir()
		containerfilePath := filepath.Join(tempDir, "Containerfile")
		os.WriteFile(containerfilePath, []byte("FROM scratch\n"), 0644)

		os.Setenv("VAR_FROM_ENV", "from-env")
		defer os.Unsetenv("VAR_FROM_ENV")

		c := &Build{
			containerfilePath: containerfilePath,
			Params: &BuildParams{
				Envs: []string{"FOO=bar", "VAR_FROM_ENV"},
			},
		}

		result, err := c.parseContainerfile()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(result.Stages).To(HaveLen(1))

		expectedEnvs := []instructions.KeyValuePair{
			{Key: "FOO", Value: "bar"},
			{Key: "VAR_FROM_ENV", Value: "from-env"},
		}
		var actualEnvs []instructions.KeyValuePair

		for _, cmd := range result.Stages[0].Commands {
			if env, ok := cmd.Command.(*instructions.EnvCommand); ok {
				actualEnvs = append(actualEnvs, env.Env...)
			} else {
				t.Fatalf("Expected an ENV instruction, got %#v", cmd)
			}
		}

		g.Expect(actualEnvs).To(ConsistOf(expectedEnvs))
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

	t.Run("should provide built-in platform args by default", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		// Check that all built-in platform args are available
		platformArgs := []string{
			"TARGETPLATFORM", "TARGETOS", "TARGETARCH", "TARGETVARIANT",
			"BUILDPLATFORM", "BUILDOS", "BUILDARCH", "BUILDVARIANT",
		}

		for _, arg := range platformArgs {
			value, err := expander(arg)
			// TARGETVARIANT and BUILDVARIANT can be empty on non-ARM platforms
			if arg == "TARGETVARIANT" || arg == "BUILDVARIANT" {
				g.Expect(err).ToNot(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(value).ToNot(BeEmpty(), "arg %s should not be empty", arg)
			}
		}
	})

	t.Run("should allow file args to override built-in platform args", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"build-args": "TARGETOS=custom-os\nTARGETARCH=custom-arch\n",
		})

		c := &Build{
			Params: &BuildParams{
				BuildArgsFile: filepath.Join(tempDir, "build-args"),
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("TARGETOS")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("custom-os"))

		value, err = expander("TARGETARCH")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("custom-arch"))
	})

	t.Run("should allow CLI args to override built-in platform args", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				BuildArgs: []string{"TARGETOS=custom-os", "TARGETARCH=custom-arch"},
			},
		}

		expander, err := c.createBuildArgExpander()
		g.Expect(err).ToNot(HaveOccurred())

		value, err := expander("TARGETOS")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("custom-os"))

		value, err = expander("TARGETARCH")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(value).To(Equal("custom-arch"))
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
				OutputRef:      "quay.io/org/image:tag",
				Context:        contextDir,
				Containerfile:  "",
				Push:           true,
				SkipInjections: true,
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

	t.Run("should clean up temporary workdir on exit", func(t *testing.T) {
		beforeEach()

		testutil.WriteFileTree(t, tempDir, map[string]string{
			"tempWorkdir/file1.txt":             "hello",
			"tempWorkdir/file2.txt":             "hi",
			"tempWorkdir/buildinfo/labels.json": "{}",
		})
		c.tempWorkdir = filepath.Join(tempDir, "tempWorkdir")

		err := c.Run()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.tempWorkdir).ToNot(BeAnExistingFile(), "tempWorkdir should have been deleted")
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
				OutputRef:      "quay.io/org/image:tag",
				Containerfile:  "Containerfile",
				Context:        "context",
				SecretDirs:     []string{"secrets"},
				SkipInjections: true,
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

func Test_goArchToArchitectureLabel(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		goarch   string
		expected string
	}{
		{"amd64", "x86_64"},
		{"arm64", "aarch64"},
		{"ppc64le", "ppc64le"},
		{"s390x", "s390x"},
		{"unknown", "unknown"},
	}

	for _, tc := range tests {
		result := goArchToArchitectureLabel(tc.goarch)
		g.Expect(result).To(Equal(tc.expected), "goArchToUname(%s) should return %s", tc.goarch, tc.expected)
	}
}

func Test_Build_processLabelsAndAnnotations(t *testing.T) {
	g := NewWithT(t)

	t.Run("should add default labels and annotations with provided values", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				LegacyBuildTimestamp: "1767225600", // 2026-01-01
				ImageSource:          "https://github.com/org/repo",
				ImageRevision:        "abc123",
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.mergedLabels).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
		}))
		g.Expect(c.mergedAnnotations).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
		}))
	})

	t.Run("should always add creation time label and annotation", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.mergedLabels).To(ConsistOf(
			MatchRegexp(`^org.opencontainers.image.created=.+Z$`),
		))
		g.Expect(c.mergedAnnotations).To(Equal(c.mergedLabels))

		imageCreated := c.mergedLabels[0]

		_, rfc3339time, _ := strings.Cut(imageCreated, "=")
		timestamp, err := time.Parse(time.RFC3339, rfc3339time)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(timestamp).To(BeTemporally("~", time.Now(), time.Second))
	})

	t.Run("should prepend defaults to let user-provided values override them", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				LegacyBuildTimestamp: "1767225600", // 2026-01-01
				ImageSource:          "https://github.com/org/repo",
				ImageRevision:        "abc123",
				Labels: []string{
					"some-label=foo",
					"org.opencontainers.image.revision=main",
				},
				Annotations: []string{
					"some-annotation=bar",
					"org.opencontainers.image.source=https://github.com/other-org/other-repo",
					"org.opencontainers.image.created=1990-01-01T00:00:00Z",
				},
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.mergedLabels).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
			"some-label=foo",
			"org.opencontainers.image.revision=main",
		}))
		g.Expect(c.mergedAnnotations).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
			"some-annotation=bar",
			"org.opencontainers.image.source=https://github.com/other-org/other-repo",
			"org.opencontainers.image.created=1990-01-01T00:00:00Z",
		}))
	})

	t.Run("should add legacy labels when requested", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				LegacyBuildTimestamp: "1767225600", // 2026-01-01
				ImageSource:          "https://github.com/org/repo",
				ImageRevision:        "abc123",
				AddLegacyLabels:      true,
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		arch := goArchToArchitectureLabel(runtime.GOARCH)
		g.Expect(c.mergedLabels).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
			"build-date=2026-01-01T00:00:00Z",
			"architecture=" + arch,
			"vcs-url=https://github.com/org/repo",
			"vcs-ref=abc123",
			"vcs-type=git",
		}))
		// Should be added *only* as labels, not as annotations
		g.Expect(c.mergedAnnotations).To(Equal([]string{
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"org.opencontainers.image.source=https://github.com/org/repo",
			"org.opencontainers.image.revision=abc123",
		}))
	})

	t.Run("should use source-date-epoch value for timestamps", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				LegacyBuildTimestamp: "1767225600", // 2026-01-01
				AddLegacyLabels:      true,
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.mergedLabels).To(ContainElements(
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"build-date=2026-01-01T00:00:00Z",
		))
		g.Expect(c.mergedAnnotations).To(ContainElements(
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
		))
	})

	t.Run("should add quay.expires-after label when provided", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				QuayImageExpiresAfter: "2w",
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(c.mergedLabels).To(ContainElement("quay.expires-after=2w"))
		g.Expect(c.mergedAnnotations).ToNot(ContainElement("quay.expires-after=2w"))
	})

	t.Run("should return error for invalid legacy-build-timestamp", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				LegacyBuildTimestamp: "1767225600.5",
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("determining build timestamp: parsing legacy-build-timestamp:"))
	})

	t.Run("should return error for invalid source-date-epoch", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				SourceDateEpoch: "1767225600.5",
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("determining build timestamp: parsing source-date-epoch:"))
	})

	t.Run("should parse annotations from file", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"annotations.cfg": `
# comment, ignored
   # also a comment
annotation.from.file=annotation-from-file

with.hash.char=this comment # is not a comment

			leading.spaces=are-removed
			`,
		})

		c := &Build{
			Params: &BuildParams{
				SourceDateEpoch: "1767225600",
				AnnotationsFile: filepath.Join(tempDir, "annotations.cfg"),
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.mergedAnnotations).To(Equal([]string{
			// always added
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			// from file, sorted alphabetically
			"annotation.from.file=annotation-from-file",
			"leading.spaces=are-removed",
			"with.hash.char=this comment # is not a comment",
		}))
	})

	t.Run("should return error for nonexistent annotations file", func(t *testing.T) {
		c := &Build{
			Params: &BuildParams{
				AnnotationsFile: "/nonexistent/annotations.cfg",
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(MatchRegexp("parsing annotations file: .* /nonexistent/annotations.cfg"))
	})

	t.Run("should return error for invalid annotations file", func(t *testing.T) {
		tempDir := t.TempDir()
		testutil.WriteFileTree(t, tempDir, map[string]string{
			"annotations.cfg": "this line has no equals sign\n",
		})

		c := &Build{
			Params: &BuildParams{
				AnnotationsFile: filepath.Join(tempDir, "annotations.cfg"),
			},
		}

		err := c.processLabelsAndAnnotations()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(MatchRegexp("parsing annotations file: .*annotations.cfg:1: expected arg=value"))
	})
}

func Test_Build_splitTransport(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		input             string
		expectedTransport string
		expectedImageRef  string
	}{
		// No ref
		{"", "", ""},
		// Plain image refs (no transport)
		{"registry.io/image:tag", "", "registry.io/image:tag"},
		{"ubuntu:latest", "", "ubuntu:latest"},
		// Unknown transport (treated the same as no transport, no way to know this isn't a valid image:tag)
		{"made-up-transport:ubuntu", "", "made-up-transport:ubuntu"},
		// Known transports
		{"docker://registry.io/image:tag", "docker://", "registry.io/image:tag"},
		{"containers-storage:localhost/image:tag", "containers-storage:", "localhost/image:tag"},
		{"dir:/path/to/dir", "dir:", "/path/to/dir"},
		{"docker-archive:/path/to/archive.tar", "docker-archive:", "/path/to/archive.tar"},
		{"docker-daemon:image:tag", "docker-daemon:", "image:tag"},
		{"oci:/path/to/dir", "oci:", "/path/to/dir"},
		{"oci-archive:/path/to/archive.tar", "oci-archive:", "/path/to/archive.tar"},
		{"sif:/path/to/file.sif", "sif:", "/path/to/file.sif"},
	}

	for _, tc := range tests {
		testCase := fmt.Sprintf("splitTransport(%q)", tc.input)
		transport, imageRef := splitTransport(tc.input)

		g.Expect(transport).To(Equal(tc.expectedTransport), testCase)
		g.Expect(imageRef).To(Equal(tc.expectedImageRef), testCase)
	}
}

func Test_Build_shouldInspectImage(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		input          string
		expectedResult bool
	}{
		// No ref
		{"", false},
		// Plain image refs (no transport)
		{"registry.io/image:tag", true},
		{"ubuntu:latest", true},
		// Unknown transport (treated the same as no transport, no way to know this isn't a valid image:tag)
		{"made-up-transport:ubuntu", true},
		// Supported transports
		{"docker://registry.io/image:tag", true},
		{"containers-storage:localhost/image:tag", true},
		// Unsupported transports
		{"dir:/path/to/dir", false},
		{"docker-archive:/path/to/archive.tar", false},
		{"docker-daemon:image:tag", false},
		{"oci:/path/to/dir", false},
		{"oci-archive:/path/to/archive.tar", false},
		{"sif:/path/to/file.sif", false},
	}

	for _, tc := range tests {
		result := shouldInspectImage(tc.input)
		g.Expect(result).To(Equal(tc.expectedResult), fmt.Sprintf("shouldInspectImage(%q)", tc.input))
	}
}

func Test_Build_injectBuildinfo(t *testing.T) {
	g := NewWithT(t)

	tempDir := t.TempDir()
	containerfile := filepath.Join(tempDir, "Containerfile")
	g.Expect(os.WriteFile(containerfile, []byte("FROM scratch"), 0644)).To(Succeed())

	c := &Build{
		Params: &BuildParams{
			// Avoids the BuildahCli.Version() call
			SourceDateEpoch: "0",
		},
		containerfilePath: containerfile,
	}
	defer c.cleanup()

	var df *dockerfile.Dockerfile = nil
	var userLabels []string = nil

	g.Expect(c.injectBuildinfo(df, userLabels)).To(Succeed())

	// Containerfile is copied to tempWorkdir
	g.Expect(c.containerfileCopyPath).To(HavePrefix(c.tempWorkdir + "/"))
	g.Expect(filepath.Base(c.containerfileCopyPath)).To(MatchRegexp(`^Containerfile-`))
	// Original is unchanged
	originalContent, err := os.ReadFile(c.containerfilePath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(originalContent)).To(Equal("FROM scratch"))

	// COPY appended correctly even though containerfile lacks trailing newline
	copyContent, err := os.ReadFile(c.containerfileCopyPath)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(copyContent)).To(Equal(
		"FROM scratch\nCOPY --from=.konflux-buildinfo . /usr/share/buildinfo/\n",
	))

	// labels.json created in tempWorkdir/buildinfo with valid JSON
	labelsContent, err := os.ReadFile(filepath.Join(c.tempWorkdir, "buildinfo", "labels.json"))
	g.Expect(string(labelsContent)).To(Equal("{}\n"))

	// buildinfoBuildContext points to the buildinfo dir
	g.Expect(c.buildinfoBuildContext).NotTo(BeNil())
	g.Expect(c.buildinfoBuildContext.Name).To(Equal(".konflux-buildinfo"))
	g.Expect(c.buildinfoBuildContext.Location).To(Equal(filepath.Join(c.tempWorkdir, "buildinfo")))
}
