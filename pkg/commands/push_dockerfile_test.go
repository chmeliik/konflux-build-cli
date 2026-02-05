package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	"oras.land/oras-go/v2/registry/remote"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

const imageDigest = "sha256:e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b"

func TestValidateParams(t *testing.T) {
	t.Run("Capture invalid image name", func(t *testing.T) {
		for _, imageName := range []string{"", "localhost^5000/app"} {
			cmd := PushDockerfile{
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
			cmd := PushDockerfile{
				Params: &PushDockerfileParams{
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
			"^dockerfile",
			fmt.Sprintf("%s-%s", taggedDigest, taggedDigest),
			// exceeds the max length 57.
			fmt.Sprintf("%s-%s", taggedDigest, taggedDigest)[:58],
		}
		for _, testTagSuffix := range testCases {
			cmd := PushDockerfile{
				Params: &PushDockerfileParams{
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

func TestDockerfileImageTag(t *testing.T) {
	cmd := PushDockerfile{
		Params: &PushDockerfileParams{
			ImageDigest: imageDigest,
			TagSuffix:   ".containerfile",
		},
		imageName: "localhost:5000/cool/app",
	}
	expected := "sha256-e7afdb605d0685d214876ae9d13ae0cc15da3a766be86e919fecee4032b9783b.containerfile"
	imageTag := cmd.generateDockerfileImageTag()
	if imageTag != expected {
		t.Errorf("Expect tag %s, but got: %s", expected, imageTag)
	}
}

type mockOrasClient struct {
	pushFunc func(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error)
}

func (o *mockOrasClient) Push(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error) {
	return o.pushFunc(remoteRepo, tag, localFilePath, artifactType)
}

func TestRun(t *testing.T) {
	g := NewWithT(t)
	workDir := t.TempDir()

	os.Mkdir(filepath.Join(workDir, "source"), 0755)
	os.Mkdir(filepath.Join(workDir, "results"), 0755)

	os.WriteFile(filepath.Join(workDir, "source", "Containerfile"), []byte("FROM fedora"), 0644)

	originHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", workDir)

	curDir, _ := os.Getwd()
	defer func() {
		os.Chdir(curDir)
		os.Setenv("HOME", originHomeDir)
	}()

	// Mock docker config for selecting registry authentication.
	os.Mkdir(filepath.Join(workDir, ".docker"), 0755)
	// Base64-encoded from usernamed:passw0rd
	authConfig := `{"auths":{"localhost.reg.io":{"auth":"dXNlcm5hbWVkOnBhc3N3MHJk"}}}`
	os.WriteFile(filepath.Join(workDir, ".docker", "config.json"), []byte(authConfig), 0644)

	os.Chdir(workDir)

	t.Run("Successful push", func(t *testing.T) {
		artifactImageDigest := "sha256:a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c35b02788b272a5"

		orasClient := &mockOrasClient{}
		orasClient.pushFunc = func(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error) {
			return artifactImageDigest, nil
		}

		cmd := &PushDockerfile{
			Params: &PushDockerfileParams{
				ImageUrl:           "localhost.reg.io/app",
				ImageDigest:        imageDigest,
				Source:             "source",
				Dockerfile:         "Containerfile",
				Context:            ".",
				TagSuffix:          ".dockerfile",
				ArtifactType:       "application/vnd.konflux.dockerfile",
				ImageRefResultFile: filepath.Join(workDir, "results", "image-ref"),
			},
			ResultsWriter: &common.ResultsWriter{},
			OrasClient:    orasClient,
		}

		err := cmd.Run()
		g.Expect(err).ShouldNot(HaveOccurred())

		expectedImageRef := "localhost.reg.io/app@" + artifactImageDigest
		actualImageRef, _ := os.ReadFile(cmd.Params.ImageRefResultFile)
		g.Expect(string(actualImageRef)).Should(Equal(expectedImageRef))
	})

	t.Run("Do not push if Dockerfile is not found", func(t *testing.T) {
		cmd := &PushDockerfile{
			Params: &PushDockerfileParams{
				ImageUrl:    "localhost.reg.io/app",
				ImageDigest: imageDigest,
				Dockerfile:  "Dockerfile",
				Source:      "source",
				TagSuffix:   ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
		}

		err := cmd.Run()
		// How to capture the log message?
		g.Expect(err).ShouldNot(HaveOccurred())
	})

	t.Run("Registry authentication cannot be selected", func(t *testing.T) {
		cmd := &PushDockerfile{
			Params: &PushDockerfileParams{
				ImageUrl:    "other-registry.io/app",
				ImageDigest: imageDigest,
				Source:      "source",
				Dockerfile:  "Containerfile",
				Context:     ".",
				TagSuffix:   ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
		}

		err := cmd.Run()
		expectedErrMsg := "Registry authentication is not configured for other-registry.io/app"
		g.Expect(err).Should(MatchError(ContainSubstring(expectedErrMsg)))
	})

	t.Run("Oras push fails", func(t *testing.T) {
		orasClient := &mockOrasClient{}
		orasClient.pushFunc = func(remoteRepo *remote.Repository, tag, localFilePath, artifactType string) (string, error) {
			return "", fmt.Errorf("Mock oras push failed.")
		}

		cmd := &PushDockerfile{
			Params: &PushDockerfileParams{
				ImageUrl:    "localhost.reg.io/app",
				ImageDigest: imageDigest,
				Source:      "source",
				Dockerfile:  "Containerfile",
				Context:     ".",
				TagSuffix:   ".containerfile",
			},
			ResultsWriter: &common.ResultsWriter{},
			OrasClient:    orasClient,
		}

		err := cmd.Run()
		g.Expect(err).Should(MatchError(ContainSubstring("Mock oras push failed")))
	})
}
