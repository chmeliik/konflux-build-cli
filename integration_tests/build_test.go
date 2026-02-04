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
	"github.com/konflux-ci/konflux-build-cli/testutil"
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
	SecretDirs    []string
	WorkdirMount  string
	BuildArgs     []string
	BuildArgsFile string
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
	_, _, err := runBuildWithOutput(container, buildParams)
	return err
}

// Executes the build command and returns stdout, stderr, and error.
func runBuildWithOutput(container *TestRunnerContainer, buildParams BuildParams) (string, string, error) {
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
	// Add secret directories if provided
	if len(buildParams.SecretDirs) > 0 {
		args = append(args, "--secret-dirs")
		args = append(args, buildParams.SecretDirs...)
	}
	if buildParams.WorkdirMount != "" {
		args = append(args, "--workdir-mount", buildParams.WorkdirMount)
	}
	if len(buildParams.BuildArgs) > 0 {
		args = append(args, "--build-args")
		args = append(args, buildParams.BuildArgs...)
	}
	if buildParams.BuildArgsFile != "" {
		args = append(args, "--build-args-file", buildParams.BuildArgsFile)
	}
	// Add separator and extra args if provided
	if len(buildParams.ExtraArgs) > 0 {
		args = append(args, "--")
		args = append(args, buildParams.ExtraArgs...)
	}

	return container.ExecuteCommandWithOutput(KonfluxBuildCli, args...)
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

	setupBuildContainerWithCleanup := func(
		t *testing.T, buildParams BuildParams, imageRegistry ImageRegistry, opts ...ContainerOption,
	) *TestRunnerContainer {
		opts = append(commonOpts, opts...)
		container, err := setupBuildContainer(buildParams, imageRegistry, opts...)
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

	t.Run("WithSecretDirs", func(t *testing.T) {
		secretsBaseDir := t.TempDir()
		testutil.WriteFileTree(t, secretsBaseDir, map[string]string{
			"secret1/token":   "secret-token-value",
			"secret1/api-key": "secret-api-key-value",
			// secret2/password: symlink to ..data/password (similar to Kubernetes secret volumes)
			"secret2/..data/password": "secret-password-value",
		})
		err := os.Symlink("..data/password", filepath.Join(secretsBaseDir, "secret2/password"))
		Expect(err).ToNot(HaveOccurred())

		secretDirs := []string{
			// Should be accessible with IDs secret1_alias/*
			"src=/secrets/secret1,name=secret1_alias",
			// Should be accessible with IDs secret2/*
			"/secrets/secret2",
			// Should be ignored
			"src=/secrets/nonexistent,optional=true",
		}

		contextDir := setupTestContext(t)
		writeContainerfile(contextDir, fmt.Sprintf(`
FROM %s

# Test that secrets are accessible during build
RUN --mount=type=secret,id=secret1_alias/token \
    --mount=type=secret,id=secret1_alias/api-key \
    --mount=type=secret,id=secret2/password \
    echo "token=$(cat /run/secrets/secret1_alias/token)" && \
    echo "api-key=$(cat /run/secrets/secret1_alias/api-key)" && \
    echo "password=$(cat /run/secrets/secret2/password)"

LABEL test.label="secret-dirs-test"
	`, baseImage))

		outputRef := "localhost/test-image-secrets:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:    contextDir,
			OutputRef:  outputRef,
			Push:       false,
			SecretDirs: secretDirs,
		}

		// Setup container with extra volume for secrets
		container := setupBuildContainerWithCleanup(
			t, buildParams, nil, WithVolumeWithOptions(secretsBaseDir, "/secrets", "z"),
		)

		stdout, _, err := runBuildWithOutput(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		// Verify that the secret values appear in the build output
		Expect(stdout).To(ContainSubstring("token=secret-token-value"))
		Expect(stdout).To(ContainSubstring("api-key=secret-api-key-value"))
		Expect(stdout).To(ContainSubstring("password=secret-password-value"))

		// Verify the image exists in buildah's local storage
		err = container.ExecuteCommand("buildah", "images", outputRef)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Image %s should exist in local buildah storage", outputRef))
	})

	t.Run("WorkdirMount", func(t *testing.T) {
		contextDir := setupTestContext(t)

		outputRef := "localhost/test-image-workdir-mount:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:      contextDir,
			OutputRef:    outputRef,
			Push:         false,
			WorkdirMount: "/buildcontext",
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := container.ExecuteCommand("buildah", "pull", baseImage)
		Expect(err).ToNot(HaveOccurred())
		// Push the baseimage to <contextDir>/baseimage.tar (contextDir is mounted at /workspace in the container)
		err = container.ExecuteCommand(
			"buildah", "push", "--remove-signatures", baseImage, "oci-archive:/workspace/baseimage.tar",
		)
		Expect(err).ToNot(HaveOccurred())

		writeContainerfile(contextDir, fmt.Sprintf(`
FROM %s AS builder

# /buildcontext is mounted with --volume <contextDir>:/buildcontext
# baseimage.tar exists prior to starting the build
RUN cp /buildcontext/baseimage.tar /buildcontext/myimage.tar


# This form of FROM instructions uses the filesystem of the host, not the container,
# so /buildcontext does not exist. But during the build, the context directory is
# the working directory, so this works.
FROM oci-archive:./myimage.tar

# Need to reference builder here to force ordering, otherwise buildah would skip
# the builder stage entirely.
RUN --mount=type=bind,from=builder,src=.,target=/var/tmp \
    rm /buildcontext/myimage.tar
`, baseImage))

		err = runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		// Verify the image exists in buildah's local storage
		err = container.ExecuteCommand("buildah", "images", outputRef)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Image %s should exist in local buildah storage", outputRef))
	})

	t.Run("WithBuildArgs", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `
FROM scratch

ARG NAME
ARG VERSION
ARG AUTHOR
ARG VENDOR

LABEL name=$NAME
LABEL version=$VERSION
LABEL author=$AUTHOR
LABEL vendor=$VENDOR

LABEL test.label="build-args-test"
`)

		testutil.WriteFileTree(t, contextDir, map[string]string{
			"build-args-file": "AUTHOR=John Doe\nVENDOR=konflux-ci.dev",
		})

		outputRef := "localhost/test-image-build-args:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:       contextDir,
			OutputRef:     outputRef,
			Push:          false,
			BuildArgs:     []string{"NAME=foo", "VERSION=1.2.3"},
			BuildArgsFile: "/workspace/build-args-file",
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		stdout, _, err := container.ExecuteCommandWithOutput(
			"buildah", "inspect", "--format", `{{ range $k, $v := .OCIv1.Config.Labels }}{{ printf "%s=%s\n" $k $v }}{{ end }}`, outputRef,
		)
		Expect(err).ToNot(HaveOccurred())

		labelLines := strings.Split(strings.TrimSpace(stdout), "\n")
		Expect(labelLines).To(ContainElements(
			"name=foo",
			"version=1.2.3",
			"author=John Doe",
			"vendor=konflux-ci.dev",
		))
	})
}
