package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	"github.com/konflux-ci/konflux-build-cli/pkg/common"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const imageDigest = "sha256:e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b"

func TestValidateParams(t *testing.T) {
	t.Run("Capture invalid image name", func(t *testing.T) {
		for _, imageName := range []string{"", "localhost^5000/app"} {
			cmd := PushContainerfile{
				imageName: imageName,
			}
			err := cmd.validateParams()
			if err == nil {
				t.Errorf("Expected getting error for invalid image name, but no error is return.")
				return
			}
			if !regexp.MustCompile("^image name .+ is invalid").MatchString(err.Error()) {
				t.Errorf("Error is not about invalid image name, got: %s", err.Error())
			}
		}
	})

	t.Run("Capture invalid digest", func(t *testing.T) {
		for _, digest := range []string{"", "some-digest"} {
			cmd := PushContainerfile{
				Params: &PushContainerfileParams{
					ImageDigest: digest,
				},
				imageName: "localhost:5000/cool/app",
			}
			err := cmd.validateParams()
			if err == nil {
				t.Errorf("Expected getting error for invalid image name, but no error is return.")
				return
			}
			if !regexp.MustCompile("^image digest .+ is invalid").MatchString(err.Error()) {
				t.Errorf("Error is not about invalid digest, got: %s", err.Error())
			}
		}
	})

	t.Run("Capture invalid tag suffix", func(t *testing.T) {
		taggedDigest := "sha256-e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b"
		testCases := []string{
			"",
			"^containerfile",
			fmt.Sprintf("%s-%s", taggedDigest, taggedDigest),
			// exceeds the max length 57.
			fmt.Sprintf("%s-%s", taggedDigest, taggedDigest)[:58],
		}
		for _, testTagSuffix := range testCases {
			cmd := PushContainerfile{
				Params: &PushContainerfileParams{
					ImageDigest: imageDigest,
					TagSuffix:   testTagSuffix,
				},
				imageName: "localhost:5000/cool/app",
			}
			err := cmd.validateParams()
			if err == nil {
				t.Errorf("Expected getting error for invalid tag suffix, but no error is return.")
				return
			}
			if !regexp.MustCompile("^Tag suffix includes invalid char.+").MatchString(err.Error()) {
				t.Errorf("Error is not about invalid tag suffix, got: %s", err.Error())
			}
		}
	})
}

func TestGenerateContainerfileImageTag(t *testing.T) {
	cmd := PushContainerfile{
		Params: &PushContainerfileParams{
			ImageDigest: imageDigest,
			TagSuffix:   ".containerfile",
		},
		imageName: "localhost:5000/cool/app",
	}
	expected := "sha256-e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b.containerfile"
	imageTag := cmd.generateContainerfileImageTag()
	if imageTag != expected {
		t.Errorf("Expect tag %s, but got: %s", expected, imageTag)
	}
}

func TestRun(t *testing.T) {
	g := NewWithT(t)
	workDir := t.TempDir()

	os.Mkdir(filepath.Join(workDir, "source"), 0755)
	os.Mkdir(filepath.Join(workDir, "results"), 0755)
	os.WriteFile(filepath.Join(workDir, "source", "Containerfile"), []byte("FROM fedora"), 0644)

	originalHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", workDir)

	curDir, _ := os.Getwd()
	defer func() {
		os.Chdir(curDir)
		os.Setenv("HOME", originalHomeDir)
	}()

	// Mock docker config for selecting registry authentication.
	os.Mkdir(filepath.Join(workDir, ".docker"), 0755)
	const authConfig = `{"auths":{"localhost.reg.io":{"auth":"token"}}}`
	os.WriteFile(filepath.Join(workDir, ".docker", "config.json"), []byte(authConfig), 0644)

	os.Chdir(workDir)

	t.Run("Successful push", func(t *testing.T) {
		artifactImageDigest := "sha256:a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c35b02788b272a5"

		sourcePathCases := []struct {
			name string
			path string
		}{
			{name: "use relative path", path: "source"},
			{name: "use absolute path", path: filepath.Join(workDir, "source")},
		}

		orasCli := &mockOrasCli{}
		orasCli.PushFunc = func(args *cliwrappers.OrasPushArgs) (string, string, error) {
			expectedImage := "localhost.reg.io/app:sha256-e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b.containerfile"
			g.Expect(args.DestinationImage).Should(Equal(expectedImage))
			g.Expect(args.FileName).Should(Equal("Containerfile"))
			g.Expect(args.Template).Should(Equal("{{.reference}}"))
			g.Expect(args.Format).Should(Equal("go-template"))
			g.Expect(args.ArtifactType).Should(Equal("application/vnd.konflux.containerfile"))
			g.Expect(args.RegistryConfig).ShouldNot(Equal(""))
			authContent, err := os.ReadFile(args.RegistryConfig)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(string(authContent)).Should(Equal(authConfig))
			return "localhost.reg.io/app@" + artifactImageDigest, "", nil
		}

		for _, tc := range sourcePathCases {
			t.Run(tc.name, func(t *testing.T) {
				cmd := &PushContainerfile{
					Params: &PushContainerfileParams{
						ImageUrl:           "localhost.reg.io/app",
						ImageDigest:        imageDigest,
						Source:             tc.path,
						Containerfile:      "Containerfile",
						Context:            ".",
						TagSuffix:          ".containerfile",
						ArtifactType:       "application/vnd.konflux.containerfile",
						ResultPathImageRef: filepath.Join(workDir, "results", "image-ref"),
					},
					ResultsWriter: &common.ResultsWriter{},
					CliWrappers:   OrasCliWrappers{OrasCli: orasCli},
				}

				err := cmd.Run()
				g.Expect(err).ShouldNot(HaveOccurred())

				expectedImageRef := "localhost.reg.io/app@" + artifactImageDigest
				actualImageRef, _ := os.ReadFile(cmd.Params.ResultPathImageRef)
				g.Expect(string(actualImageRef)).Should(Equal(expectedImageRef))
			})
		}

	})

	t.Run("Do not push if specified Containerfile is not found", func(t *testing.T) {
		logFilename := filepath.Join(t.TempDir(), "logfile")
		logFile, _ := os.OpenFile(logFilename, os.O_CREATE|os.O_WRONLY, 0644)

		originLogOutput := l.Logger.Out
		originLoglevel := l.Logger.Level

		defer func() {
			l.Logger.SetOutput(originLogOutput)
			l.Logger.SetLevel(originLoglevel)
		}()

		l.Logger.SetLevel(logrus.DebugLevel)
		l.Logger.SetOutput(logFile)

		cmd := &PushContainerfile{
			Params: &PushContainerfileParams{
				ImageUrl:      "localhost.reg.io/app",
				ImageDigest:   imageDigest,
				Containerfile: "Dockerfile",
				Source:        "source",
				TagSuffix:     ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
		}

		err := cmd.Run()
		g.Expect(err).ShouldNot(HaveOccurred())

		logFile.Close()
		logContent, _ := os.ReadFile(logFilename)
		expectedMsg := "Containerfile 'Dockerfile' is not found from source 'source' and context ''. Abort push."
		g.Expect(string(logContent)).Should(ContainSubstring(expectedMsg))
	})

	t.Run("Registry authentication cannot be selected", func(t *testing.T) {
		cmd := &PushContainerfile{
			Params: &PushContainerfileParams{
				ImageUrl:      "other-registry.io/app",
				ImageDigest:   imageDigest,
				Source:        "source",
				Containerfile: "Containerfile",
				Context:       ".",
				TagSuffix:     ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
		}

		err := cmd.Run()
		expectedErrMsg := "Registry authentication is not configured for other-registry.io/app"
		g.Expect(err).Should(MatchError(ContainSubstring(expectedErrMsg)))
	})

	t.Run("Oras push fails", func(t *testing.T) {
		orasCli := &mockOrasCli{}
		orasCli.PushFunc = func(args *cliwrappers.OrasPushArgs) (string, string, error) {
			return "", "", fmt.Errorf("Mock oras push failed")
		}

		cmd := &PushContainerfile{
			Params: &PushContainerfileParams{
				ImageUrl:      "localhost.reg.io/app",
				ImageDigest:   imageDigest,
				Source:        "source",
				Containerfile: "Containerfile",
				Context:       ".",
				TagSuffix:     ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
			CliWrappers:   OrasCliWrappers{OrasCli: orasCli},
		}

		err := cmd.Run()
		g.Expect(err).Should(MatchError(ContainSubstring("Mock oras push failed")))
	})
}
