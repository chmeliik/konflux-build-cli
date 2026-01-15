package integration_tests

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const BuildImage = "quay.io/konflux-ci/buildah-task:latest@sha256:5c5eb4117983b324f932f144aa2c2df7ed508174928a423d8551c4e11f30fbd9"

type BuildParams struct {
	Context       string
	Containerfile string
	OutputRef     string
	Push          bool
	ExtraArgs     []string
}

// Public interface for parity with ApplyTags. Not used in these tests directly.
func RunBuild(buildParams BuildParams, imageRegistry ImageRegistry) error {
	container, err := setupBuildContainer(buildParams, imageRegistry)
	if container != nil {
		defer container.Delete()
	}
	if err != nil {
		return err
	}
	return runBuild(container, buildParams)
}

// Creates and starts a container for running builds.
// The caller is responsible for cleaning up the container.
// May return a non-nil container even if an error occurs. In that case, the caller
// should clean up the container before failing.
func setupBuildContainer(buildParams BuildParams, imageRegistry ImageRegistry) (*TestRunnerContainer, error) {
	container := NewBuildCliRunnerContainer("kbc-build", BuildImage)
	container.AddVolumeWithOptions(buildParams.Context, "/workspace", "z")

	if imageRegistry != nil && imageRegistry.IsLocal() {
		container.AddVolumeWithOptions(imageRegistry.GetCaCertPath(), "/etc/pki/tls/certs/ca-custom-bundle.crt", "z")
	}

	err := container.Start()
	if err != nil {
		return nil, err
	}

	if imageRegistry != nil {
		login, password := imageRegistry.GetCredentials()
		err = container.InjectDockerAuth(imageRegistry.GetRegistryDomain(), login, password)
		if err != nil {
			return container, err
		}
	}

	return container, nil
}

// Executes the build command in the provided container.
func runBuild(container *TestRunnerContainer, buildParams BuildParams) error {
	var err error

	// Construct the build arguments
	args := []string{"image", "build"}
	args = append(args, "-t", buildParams.OutputRef)
	args = append(args, "-c", "/workspace")
	if buildParams.Containerfile != "" {
		args = append(args, "-f", buildParams.Containerfile)
	}
	if buildParams.Push {
		args = append(args, "--push")
	}
	// Add separator and extra args if provided
	if len(buildParams.ExtraArgs) > 0 {
		args = append(args, "--")
		args = append(args, buildParams.ExtraArgs...)
	}

	err = container.ExecuteBuildCli(args...)
	if err != nil {
		return err
	}

	return nil
}

// Registers the Gomega failure handler for the test.
func setupGomega(t *testing.T) {
	RegisterFailHandler(func(message string, callerSkip ...int) {
		fmt.Printf("Test Failure: %s\n", message)
		t.FailNow()
	})
}

// Creates a build container and registers cleanup.
func setupBuildContainerWithCleanup(t *testing.T, buildParams BuildParams, imageRegistry ImageRegistry) *TestRunnerContainer {
	container, err := setupBuildContainer(buildParams, imageRegistry)
	t.Cleanup(func() {
		if container != nil {
			container.Delete()
		}
	})
	Expect(err).ToNot(HaveOccurred())
	return container
}

// Creates a temporary directory for the test and registers cleanup.
func setupTestContext(t *testing.T) string {
	contextDir, err := CreateTempDir("build-test-context-")
	if err != nil {
		t.Fatalf("Failed to create test context: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(contextDir)
	})
	return contextDir
}

// Sets up the image registry and registers cleanup.
func setupImageRegistry(t *testing.T) ImageRegistry {
	imageRegistry := NewImageRegistry()
	err := imageRegistry.Prepare()
	Expect(err).ToNot(HaveOccurred())
	err = imageRegistry.Start()
	Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() {
		imageRegistry.Stop()
	})
	return imageRegistry
}

// Writes a Containerfile with the given content to the context directory.
func writeContainerfile(contextDir, content string) {
	err := os.WriteFile(path.Join(contextDir, "Containerfile"), []byte(content), 0644)
	Expect(err).ToNot(HaveOccurred())
}

func TestBuild_BuildOnly(t *testing.T) {
	setupGomega(t)

	contextDir := setupTestContext(t)
	writeContainerfile(contextDir, `
FROM scratch
LABEL test.label="build-test"
`)

	outputRef := "localhost/test-image:" + GenerateUniqueTag(t)

	buildParams := BuildParams{
		Context:   contextDir,
		OutputRef: outputRef,
		Push:      false,
	}

	container := setupBuildContainerWithCleanup(t, buildParams, nil)

	// Run build without pushing
	err := runBuild(container, buildParams)
	Expect(err).ToNot(HaveOccurred())

	// Verify the image exists in buildah's local storage
	err = container.ExecuteCommand("buildah", "images", outputRef)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Image %s should exist in local buildah storage", outputRef))
}

func TestBuild_BuildAndPush(t *testing.T) {
	setupGomega(t)

	imageRegistry := setupImageRegistry(t)

	contextDir := setupTestContext(t)
	writeContainerfile(contextDir, fmt.Sprintf(`
FROM scratch
LABEL test.label="build-and-push-test"
LABEL %s="1h"
`, QuayExpiresAfterLabelName))

	imageRepoUrl := imageRegistry.GetTestNamespace() + "build-test-image"
	outputRef := imageRepoUrl + ":" + GenerateUniqueTag(t)

	buildParams := BuildParams{
		Context:   contextDir,
		OutputRef: outputRef,
		Push:      true,
	}

	container := setupBuildContainerWithCleanup(t, buildParams, imageRegistry)

	err := runBuild(container, buildParams)
	Expect(err).ToNot(HaveOccurred())

	lastColon := strings.LastIndex(outputRef, ":")
	tag := outputRef[lastColon+1:]

	tagExists, err := imageRegistry.CheckTagExistance(imageRepoUrl, tag)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to check for %s tag existence", tag))
	Expect(tagExists).To(BeTrue(), fmt.Sprintf("Expected %s to exist in registry", outputRef))
}

func TestBuild_WithExtraArgs(t *testing.T) {
	setupGomega(t)

	contextDir := setupTestContext(t)

	writeContainerfile(contextDir, `
FROM scratch
LABEL test.label="extra-args-test"
`)

	outputRef := "localhost/test-image-extra-args:" + GenerateUniqueTag(t)

	buildParams := BuildParams{
		Context:   contextDir,
		OutputRef: outputRef,
		Push:      false,
		ExtraArgs: []string{"--logfile", "/tmp/kbc-build.log"},
	}

	container := setupBuildContainerWithCleanup(t, buildParams, nil)

	err := runBuild(container, buildParams)
	Expect(err).ToNot(HaveOccurred())

	// Verify the image exists in buildah's local storage
	err = container.ExecuteCommand("buildah", "images", outputRef)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Image %s should exist in local buildah storage", outputRef))

	// Verify that the logfile was created
	err = container.ExecuteCommand("test", "-f", "/tmp/kbc-build.log")
	Expect(err).ToNot(HaveOccurred(), "Expected /tmp/kbc-build.log to exist")
}
