package integration_tests

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const (
	BuildImage = "quay.io/konflux-ci/buildah-task:latest@sha256:5c5eb4117983b324f932f144aa2c2df7ed508174928a423d8551c4e11f30fbd9"
	// Tests that need a real base image should try to use the same one when possible
	// (to reduce the time spent pulling base images)
	baseImage = "registry.access.redhat.com/ubi10/ubi-micro:10.1@sha256:2946fa1b951addbcd548ef59193dc0af9b3e9fedb0287b4ddb6e697b06581622"
)

type BuildParams struct {
	Context       string
	Containerfile string
	OutputRef     string
	Push          bool
	ExtraArgs     []string
}

// Public interface for parity with ApplyTags. Not used in these tests directly.
func RunBuild(buildParams BuildParams, imageRegistry ImageRegistry) error {
	opts := []ContainerOption{}
	// On macOS, containers run in a Linux VM; overlay storage driver
	// doesn't work reliably with host volume mounts through the VM
	if runtime.GOOS != "darwin" {
		containerStoragePath, err := createContainerStorageDir()
		if err != nil {
			return err
		}
		defer removeContainerStorageDir(containerStoragePath)
		opts = append(opts, WithVolumeWithOptions(containerStoragePath, "/var/lib/containers", "z"))
	}

	container, err := setupBuildContainer(buildParams, imageRegistry, opts...)
	defer container.DeleteIfExists()
	if err != nil {
		return err
	}

	return runBuild(container, buildParams)
}

// Creates a temporary directory for container storage in the repository root,
// under .test-containers-storage. Returns the full path to the created directory.
// This directory can be mounted at /var/lib/containers in the test runner container.
//
// Why put the directory in the repository root:
//   - The directory can't be in /tmp, because that is usually a tmpfs which doesn't
//     support all the operations that buildah needs to do with /var/lib/containers.
//   - The repository root is an obvious choice for a directory that likely isn't in /tmp,
//     is writable for the current user and doesn't pollute the user's home directory.
func createContainerStorageDir() (string, error) {
	repoRoot, err := filepath.Abs(FindRepoRoot())
	if err != nil {
		return "", err
	}

	storageBaseDir := path.Join(repoRoot, ".test-containers-storage")
	if err := EnsureDirectory(storageBaseDir); err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp(storageBaseDir, "storage-")
	if err != nil {
		return "", err
	}

	return tmpDir, nil
}

// Try to remove a directory created by createContainerStorageDir,
// and the parent .test-containers-storage directory if empty.
// The cleanup is best-effort and ignores errors.
func removeContainerStorageDir(containerStoragePath string) {
	// 1. 'chmod -R' to ensure write permissions (container storage often includes read-only files)
	_ = filepath.WalkDir(containerStoragePath, func(path string, d fs.DirEntry, err error) error {
		// Ignore errors, try to chmod everything if possible
		os.Chmod(path, 0777)
		return nil
	})
	// 2. 'rm -r'
	_ = os.RemoveAll(containerStoragePath)

	// Try to remove the parent .test-containers-storage directory. Will fail if it's not
	// empty (e.g. a different test process is running in parallel). This is fine. The last
	// test process that finishes should clean it up successfully.
	_ = os.Remove(filepath.Dir(containerStoragePath))
}

// Creates and starts a container for running builds.
// The caller is responsible for cleaning up the container.
// Returns a non-nil container even if an error occurs. The caller should always call
// DeleteIfExists() on the container for cleanup.
func setupBuildContainer(buildParams BuildParams, imageRegistry ImageRegistry, opts ...ContainerOption) (*TestRunnerContainer, error) {
	container := NewBuildCliRunnerContainer("kbc-build", BuildImage, opts...)
	container.AddVolumeWithOptions(buildParams.Context, "/workspace", "z")

	var err error
	if imageRegistry != nil {
		err = container.StartWithRegistryIntegration(imageRegistry)
	} else {
		err = container.Start()
	}

	return container, err
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

func TestBuild(t *testing.T) {
	SetupGomega(t)

	commonOpts := []ContainerOption{}
	// On macOS, containers run in a Linux VM; overlay storage driver
	// doesn't work reliably with host volume mounts through the VM
	if runtime.GOOS != "darwin" {
		containerStoragePath, err := createContainerStorageDir()
		Expect(err).ToNot(HaveOccurred())
		t.Cleanup(func() { removeContainerStorageDir(containerStoragePath) })
		commonOpts = append(commonOpts, WithVolumeWithOptions(containerStoragePath, "/var/lib/containers", "z"))
	}

	// Creates a build container and registers cleanup.
	setupBuildContainerWithCleanup := func(t *testing.T, buildParams BuildParams, imageRegistry ImageRegistry) *TestRunnerContainer {
		container, err := setupBuildContainer(buildParams, imageRegistry, commonOpts...)
		t.Cleanup(func() { container.DeleteIfExists() })
		Expect(err).ToNot(HaveOccurred())
		return container
	}

	t.Run("BuildOnly", func(t *testing.T) {
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
	})

	t.Run("BuildAndPush", func(t *testing.T) {
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
	})

	t.Run("WithExtraArgs", func(t *testing.T) {
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
	})

	t.Run("UsesRunInstruction", func(t *testing.T) {
		contextDir := setupTestContext(t)
		writeContainerfile(contextDir, fmt.Sprintf(`
FROM %s
RUN echo hi
`, baseImage))

		outputRef := "localhost/test-image:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:   contextDir,
			OutputRef: outputRef,
			Push:      false,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		err = container.ExecuteCommand("buildah", "images", outputRef)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Image %s should exist in local buildah storage", outputRef))
	})
}
