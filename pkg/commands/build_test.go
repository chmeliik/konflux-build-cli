package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
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
				g.Expect(err.Error()).To(ContainSubstring(tc.errSubstring))
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
}
