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

func setupPushContainerfileContainer(imageRegistry ImageRegistry, opts ...ContainerOption) (*TestRunnerContainer, error) {
	container := NewBuildCliRunnerContainer("kbc-push-containerfile", TaskRunnerImage, opts...)

	var err error
	if imageRegistry != nil {
		err = container.StartWithRegistryIntegration(imageRegistry)
	} else {
		err = container.Start()
	}

	return container, err
}

// Creates a container and registers cleanup.
func setupPushContainerfileContainerWithCleanup(t *testing.T, imageRegistry ImageRegistry, opts ...ContainerOption) *TestRunnerContainer {
	container, err := setupPushContainerfileContainer(imageRegistry, opts...)
	t.Cleanup(func() { container.DeleteIfExists() })
	Expect(err).ShouldNot(HaveOccurred())
	return container
}

type PushContainerfileParams struct {
	source             string
	context            string
	containerfile      string
	digest             string
	tagSuffix          string
	artifactType       string
	resultPathImageRef string
}

func TestPushContainerfile(t *testing.T) {
	SetupGomega(t)
	g := NewWithT(t)

	commonOpts := []ContainerOption{}
	imageRegistry := setupImageRegistry(t)
	container := setupPushContainerfileContainerWithCleanup(t, imageRegistry, commonOpts...)

	dirs := []string{
		"source/containerfiles",
	}
	for _, dirname := range dirs {
		err := container.ExecuteCommand("mkdir", "-p", dirname)
		g.Expect(err).ShouldNot(HaveOccurred())
	}

	files := []string{
		"FROM fedora", "source/Containerfile",
		"FROM ubi9", "source/containerfiles/operator",
	}
	for i := 0; i < len(files); i += 2 {
		fileContent := files[i]
		fileName := files[i+1]
		script := fmt.Sprintf(`echo "%s" >%s`, fileContent, fileName)
		err := container.ExecuteCommand("bash", "-c", script)
		g.Expect(err).ShouldNot(HaveOccurred())
	}

	sourceContainerfileContentDigest := sha256Checksum("FROM fedora")
	sourceContainerfilesOperatorContentDigest := sha256Checksum("FROM ubi9")

	imageRepo := filepath.Join(imageRegistry.GetRegistryDomain(), "app")

	testCases := []struct {
		name                         string
		params                       PushContainerfileParams
		expectedTaggedDigest         string
		expectedContainerfileDigest  string
		expectedTitleAnnotationValue string
	}{
		{
			name: "Push and write result",
			params: PushContainerfileParams{
				source:             "source",
				digest:             "sha256:cfc8226f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a",
				containerfile:      "./Containerfile",
				resultPathImageRef: "/tmp/result-image-ref",
			},
			expectedTaggedDigest:         "sha256-cfc8226f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a",
			expectedContainerfileDigest:  sourceContainerfileContentDigest,
			expectedTitleAnnotationValue: "Containerfile",
		},
		{
			name: "Push with custom suffix",
			params: PushContainerfileParams{
				source:        "source",
				digest:        "sha256:f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226",
				containerfile: "./Containerfile",
				tagSuffix:     ".containerfile",
			},
			expectedTaggedDigest:         "sha256-f8268c70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226",
			expectedContainerfileDigest:  sourceContainerfileContentDigest,
			expectedTitleAnnotationValue: "Containerfile",
		},
		{
			name: "Push with custom artifact type",
			params: PushContainerfileParams{
				source:        "source",
				digest:        "sha256:48148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c708",
				containerfile: "./Containerfile",
				artifactType:  "application/vnd.my.org.containerfile",
			},
			expectedTaggedDigest:         "sha256-48148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c708",
			expectedContainerfileDigest:  sourceContainerfileContentDigest,
			expectedTitleAnnotationValue: "Containerfile",
		},
		{
			name: "Push custom containerfile from subdirectory",
			params: PushContainerfileParams{
				source:        "source",
				digest:        "sha256:70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c",
				containerfile: "./containerfiles/operator",
			},
			expectedTaggedDigest:         "sha256-70848148f19c35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c",
			expectedContainerfileDigest:  sourceContainerfilesOperatorContentDigest,
			expectedTitleAnnotationValue: "operator",
		},
		{
			name: "Push by using default ./Containerfile",
			params: PushContainerfileParams{
				source: "source",
				digest: "sha256:35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c",
			},
			expectedTaggedDigest:         "sha256-35b02788b272a5a7c0071906a9c6b654760e44a1fc8226f8268c70848148f19c",
			expectedContainerfileDigest:  sourceContainerfileContentDigest,
			expectedTitleAnnotationValue: "Containerfile",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := []string{
				"image", "push-containerfile",
				"--image-url", imageRepo,
				"--image-digest", tc.params.digest,
				"--source", "source",
			}
			if tc.params.containerfile != "" {
				cmd = append(cmd, "--containerfile", tc.params.containerfile)
			}
			if tc.params.resultPathImageRef != "" {
				cmd = append(cmd, "--result-path-image-ref", tc.params.resultPathImageRef)
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
				tagSuffix = ".containerfile"
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
			g.Expect(layerDescriptor.Digest, tc.expectedContainerfileDigest)

			expectedArtifactType := tc.params.artifactType
			if expectedArtifactType == "" {
				expectedArtifactType = "application/vnd.konflux.containerfile" // the default
			}

			g.Expect(manifest.ArtifactType).Should(Equal(expectedArtifactType))

			if tc.params.resultPathImageRef != "" {
				result, err := container.GetTaskResultValue(tc.params.resultPathImageRef)
				g.Expect(err).ShouldNot(HaveOccurred())
				digest := sha256Checksum(strings.TrimRight(manifestJson, "\r\n"))
				expectedArtifactImageRef := fmt.Sprintf("%s@sha256:%s", imageRepo, digest)
				g.Expect(result).Should(Equal(expectedArtifactImageRef))
			}
		})
	}
}
