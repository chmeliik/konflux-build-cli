package integration_tests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const TaskRunnerImage = "quay.io/konflux-ci/task-runner:1.3.0"

func sha256Checksum(input string) string {
	hash := sha256.New()
	hash.Write([]byte(input))
	hashBytes := hash.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

func setupPushDockerfileContainer(imageRegistry ImageRegistry, opts ...ContainerOption) (*TestRunnerContainer, error) {
	container := NewBuildCliRunnerContainer("kbc-push-dockerfile", TaskRunnerImage, opts...)

	var err error
	if imageRegistry != nil {
		err = container.StartWithRegistryIntegration(imageRegistry)
	} else {
		err = container.Start()
	}

	return container, err
}

// Creates a container and registers cleanup.
func setupPushDockerfileContainerWithCleanup(t *testing.T, imageRegistry ImageRegistry, opts ...ContainerOption) *TestRunnerContainer {
	container, err := setupPushDockerfileContainer(imageRegistry, opts...)
	t.Cleanup(func() { container.DeleteIfExists() })
	Expect(err).ShouldNot(HaveOccurred())
	return container
}

type PushDockerfileParams struct {
	source             string
	context            string
	dockerfile         string
	digest             string
	tagSuffix          string
	artifactType       string
	imageRefResultFile string
}

func TestPushDockerfile(t *testing.T) {
	SetupGomega(t)
	g := NewWithT(t)

	commonOpts := []ContainerOption{}
	imageRegistry := setupImageRegistry(t)
	container := setupPushDockerfileContainerWithCleanup(t, imageRegistry, commonOpts...)

	dirs := []string{
		"source/containerfiles",
	}
	for _, dirname := range dirs {
		err := container.ExecuteCommand("mkdir", "-p", dirname)
		g.Expect(err).ShouldNot(HaveOccurred())
	}

	files := []string{
		"FROM fedora", "source/Dockerfile",
		"FROM ubi9", "source/containerfiles/operator",
	}
	for i := 0; i < len(files); i += 2 {
		fileContent := files[i]
		fileName := files[i+1]
		script := fmt.Sprintf(`echo "%s" >%s`, fileContent, fileName)
		err := container.ExecuteCommand("bash", "-c", script)
		g.Expect(err).ShouldNot(HaveOccurred())
	}

	sourceDockerfileContentDigest := sha256Checksum("FROM fedora")
	sourceContainerfilesOperatorContentDigest := sha256Checksum("FROM ubi9")

	imageRepo := filepath.Join(imageRegistry.GetRegistryDomain(), "app")

	testCases := []struct {
		name                         string
		params                       PushDockerfileParams
		expectedTaggedDigest         string
		expectedDockerfileDigest     string
		expectedTitleAnnotationValue string
	}{
		{
			name: "Push and write result",
			params: PushDockerfileParams{
				source:             "source",
				digest:             "sha256:cfc8226f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a",
				dockerfile:         "./Dockerfile",
				imageRefResultFile: "/tmp/result-image-ref",
			},
			expectedTaggedDigest:         "sha256-cfc8226f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a",
			expectedDockerfileDigest:     sourceDockerfileContentDigest,
			expectedTitleAnnotationValue: "Dockerfile",
		},
		{
			name: "Push with custom suffix",
			params: PushDockerfileParams{
				source:     "source",
				digest:     "sha256:f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226",
				dockerfile: "./Dockerfile",
				tagSuffix:  ".containerfile",
			},
			expectedTaggedDigest:         "sha256-f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226",
			expectedDockerfileDigest:     sourceDockerfileContentDigest,
			expectedTitleAnnotationValue: "Dockerfile",
		},
		{
			name: "Push with custom artifact type",
			params: PushDockerfileParams{
				source:       "source",
				digest:       "sha256:48148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c708",
				dockerfile:   "./Dockerfile",
				artifactType: "application/vnd.my.org.containerfile",
			},
			expectedTaggedDigest:         "sha256-48148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c708",
			expectedDockerfileDigest:     sourceDockerfileContentDigest,
			expectedTitleAnnotationValue: "Dockerfile",
		},
		{
			name: "Push custom Dockerfile from subdirectory",
			params: PushDockerfileParams{
				source:     "source",
				digest:     "sha256:70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c",
				dockerfile: "./containerfiles/operator",
			},
			expectedTaggedDigest:         "sha256-70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c",
			expectedDockerfileDigest:     sourceContainerfilesOperatorContentDigest,
			expectedTitleAnnotationValue: "operator",
		},
		{
			name: "Push by using default ./Dockerfile",
			params: PushDockerfileParams{
				source: "source",
				digest: "sha256:35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c",
			},
			expectedTaggedDigest:         "sha256-35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c",
			expectedDockerfileDigest:     sourceDockerfileContentDigest,
			expectedTitleAnnotationValue: "Dockerfile",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := []string{
				"image", "push-dockerfile",
				"--image-url", imageRepo,
				"--image-digest", tc.params.digest,
				"--source", "source",
			}
			if tc.params.dockerfile != "" {
				cmd = append(cmd, "--dockerfile", tc.params.dockerfile)
			}
			if tc.params.imageRefResultFile != "" {
				cmd = append(cmd, "--image-ref-result-file", tc.params.imageRefResultFile)
			}
			if tc.params.tagSuffix != "" {
				cmd = append(cmd, "--tag-suffix", tc.params.tagSuffix)
			}
			if tc.params.artifactType != "" {
				cmd = append(cmd, "--artifact-type", tc.params.artifactType)
			}
			if tc.params.context != "" {
				cmd = append(cmd, "--context", tc.params.context)
			}

			err := container.ExecuteBuildCli(cmd...)
			g.Expect(err).ShouldNot(HaveOccurred())

			tagSuffix := tc.params.tagSuffix
			if tagSuffix == "" {
				tagSuffix = ".dockerfile"
			}

			expectedTag := fmt.Sprintf("%s%s", tc.expectedTaggedDigest, tagSuffix)
			artifactImageRef := imageRepo + ":" + expectedTag

			cmdArgs := []string{"inspect", "--raw", "docker://" + artifactImageRef}
			manifestJson, _, err := container.ExecuteCommandWithOutput("skopeo", cmdArgs...)
			g.Expect(err).ShouldNot(HaveOccurred())

			var manifest v1.Manifest
			err = json.Unmarshal([]byte(manifestJson), &manifest)
			g.Expect(err).ShouldNot(HaveOccurred())

			layerDescriptor := manifest.Layers[0]
			layerAnnotations := layerDescriptor.Annotations
			if title, exists := layerAnnotations["org.opencontainers.image.title"]; exists {
				g.Expect(title).Should(Equal(tc.expectedTitleAnnotationValue))
			}
			g.Expect(layerDescriptor.Digest, tc.expectedDockerfileDigest)

			expectedArtifactType := tc.params.artifactType
			if expectedArtifactType == "" {
				expectedArtifactType = "application/vnd.konflux.dockerfile" // the default
			}

			g.Expect(manifest.ArtifactType).Should(Equal(expectedArtifactType))

			if tc.params.imageRefResultFile != "" {
				result, err := container.GetTaskResultValue(tc.params.imageRefResultFile)
				g.Expect(err).ShouldNot(HaveOccurred())
				digest := sha256Checksum(strings.TrimRight(manifestJson, "\r\n"))
				expectedArtifactImageRef := fmt.Sprintf("%s@sha256:%s", imageRepo, digest)
				g.Expect(result).Should(Equal(expectedArtifactImageRef))
			}
		})
	}
}
