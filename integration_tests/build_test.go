package integration_tests

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
	"github.com/konflux-ci/konflux-build-cli/testutil"
)

const (
	BuildImage = "quay.io/konflux-ci/buildah-task:latest@sha256:4c470b5a153c4acd14bf4f8731b5e36c61d7faafe09c2bf376bb81ce84aa5709"
	// Tests that need a real base image should try to use the same one when possible
	// (to reduce the time spent pulling base images)
	baseImage = "registry.access.redhat.com/ubi10/ubi-micro:10.1@sha256:2946fa1b951addbcd548ef59193dc0af9b3e9fedb0287b4ddb6e697b06581622"
)

type BuildParams struct {
	Context                 string
	Containerfile           string
	OutputRef               string
	Push                    bool
	SecretDirs              []string
	WorkdirMount            string
	BuildArgs               []string
	BuildArgsFile           string
	Envs                    []string
	Labels                  []string
	Annotations             []string
	AnnotationsFile         string
	ImageSource             string
	ImageRevision           string
	LegacyBuildTimestamp    string
	SourceDateEpoch         string
	RewriteTimestamp        bool
	QuayImageExpiresAfter   string
	AddLegacyLabels         bool
	ContainerfileJsonOutput string
	SkipInjections          bool
	ExtraArgs               []string
}

// Public interface for parity with ApplyTags. Not used in these tests directly.
func RunBuild(buildParams BuildParams, imageRegistry ImageRegistry) error {
	opts := []ContainerOption{}
	// On macOS, containers run in a Linux VM; overlay storage driver
	// doesn't work reliably with host volume mounts through the VM
	if runtime.GOOS != "darwin" {
		containerStoragePath, err := createContainerStorageDir()
		defer removeContainerStorageDir(containerStoragePath)
		if err != nil {
			return err
		}
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

	// Tests may run the BuildImage as a non-root user. Allow this user to write to the
	// container storage dir (by allowing everyone to write).
	if err := os.Chmod(tmpDir, 0777); err != nil {
		return tmpDir, err
	}

	return tmpDir, nil
}

// Try to remove a directory created by createContainerStorageDir,
// and the parent .test-containers-storage directory if empty.
// The cleanup is best-effort and ignores errors.
func removeContainerStorageDir(containerStoragePath string) {
	if containerStoragePath == "" {
		return
	}

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
	if len(buildParams.Envs) > 0 {
		args = append(args, "--envs")
		args = append(args, buildParams.Envs...)
	}
	if len(buildParams.Labels) > 0 {
		args = append(args, "--labels")
		args = append(args, buildParams.Labels...)
	}
	if len(buildParams.Annotations) > 0 {
		args = append(args, "--annotations")
		args = append(args, buildParams.Annotations...)
	}
	if buildParams.AnnotationsFile != "" {
		args = append(args, "--annotations-file", buildParams.AnnotationsFile)
	}
	if buildParams.ImageSource != "" {
		args = append(args, "--image-source", buildParams.ImageSource)
	}
	if buildParams.ImageRevision != "" {
		args = append(args, "--image-revision", buildParams.ImageRevision)
	}
	if buildParams.LegacyBuildTimestamp != "" {
		args = append(args, "--legacy-build-timestamp", buildParams.LegacyBuildTimestamp)
	}
	if buildParams.SourceDateEpoch != "" {
		args = append(args, "--source-date-epoch", buildParams.SourceDateEpoch)
	}
	if buildParams.RewriteTimestamp {
		args = append(args, "--rewrite-timestamp")
	}
	if buildParams.QuayImageExpiresAfter != "" {
		args = append(args, "--quay-image-expires-after", buildParams.QuayImageExpiresAfter)
	}
	if buildParams.AddLegacyLabels {
		args = append(args, "--add-legacy-labels")
	}
	if buildParams.ContainerfileJsonOutput != "" {
		args = append(args, "--containerfile-json-output", buildParams.ContainerfileJsonOutput)
	}
	if buildParams.SkipInjections {
		args = append(args, "--skip-injections")
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

type containerImageMeta struct {
	digest      string
	created     string
	labels      map[string]string
	annotations map[string]string
	envs        map[string]string
	layer_ids   []string
}

func getImageMeta(container *TestRunnerContainer, imageRef string) containerImageMeta {
	stdout, _, err := container.ExecuteCommandWithOutput("buildah", "inspect", imageRef)
	Expect(err).ToNot(HaveOccurred())

	var inspect struct {
		OCIv1 struct {
			Created string `json:"created"`
			Config  struct {
				Labels map[string]string
				Env    []string
			} `json:"config"`
			Rootfs struct {
				DiffIDs []string `json:"diff_ids"`
			} `json:"rootfs"`
		}
		ImageAnnotations map[string]string
		// This field has the same value as the output of
		// skopeo inspect --format '{{.Digest}}' containers-storage:<imageRef>
		FromImageDigest string
	}

	err = json.Unmarshal([]byte(stdout), &inspect)
	Expect(err).ToNot(HaveOccurred())

	envs := make(map[string]string, len(inspect.OCIv1.Config.Env))
	for _, env := range inspect.OCIv1.Config.Env {
		key, value, _ := strings.Cut(env, "=")
		envs[key] = value
	}

	return containerImageMeta{
		digest:      inspect.FromImageDigest,
		created:     inspect.OCIv1.Created,
		labels:      inspect.OCIv1.Config.Labels,
		annotations: inspect.ImageAnnotations,
		envs:        envs,
		layer_ids:   inspect.OCIv1.Rootfs.DiffIDs,
	}
}

func getContainerfileMeta(container *TestRunnerContainer, containerfileJsonPath string) containerImageMeta {
	containerfileJSON, err := container.GetFileContent(containerfileJsonPath)
	Expect(err).ToNot(HaveOccurred())

	// Unmarshal into a generic structure.
	// Can't unmarshal into the Dockerfile struct from dockerfile-json, because some of the fields
	// are of type instruction.Command (from buildkit), which is an interface type and can't be
	// unmarshalled without a custom UnmarshalJSON method.
	var containerfile struct {
		Stages []struct {
			Commands []struct {
				Name   string
				Labels []struct {
					Key   string
					Value string
				}
				Env []struct {
					Key   string
					Value string
				}
			}
		}
	}

	err = json.Unmarshal([]byte(containerfileJSON), &containerfile)
	Expect(err).ToNot(HaveOccurred())

	labels := make(map[string]string)
	envs := make(map[string]string)

	for _, cmd := range containerfile.Stages[0].Commands {
		for _, label := range cmd.Labels {
			labels[label.Key] = label.Value
		}
		for _, env := range cmd.Env {
			envs[env.Key] = env.Value
		}
	}

	return containerImageMeta{labels: labels, envs: envs}
}

func getLabelsFromLabelsJson(container *TestRunnerContainer, imageRef string) map[string]string {
	labelsJSON := getFileContentFromOutputImage(container, imageRef, "/usr/share/buildinfo/labels.json")

	var labels map[string]string
	err := json.Unmarshal([]byte(labelsJSON), &labels)
	Expect(err).ToNot(HaveOccurred())

	return labels
}

func formatAsKeyValuePairs(m map[string]string) []string {
	var pairs []string
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	return pairs
}

func getFileContentFromOutputImage(container *TestRunnerContainer, imageRef string, filePath string) string {
	return runWithMountedOutputImage(container, imageRef, `cat "$CONTAINER_ROOT"/`+filePath)
}

func statFileInOutputImage(container *TestRunnerContainer, imageRef string, filePath string) string {
	return runWithMountedOutputImage(container, imageRef, `stat "$CONTAINER_ROOT"/`+filePath)
}

func fileExistsInOutputImage(container *TestRunnerContainer, imageRef string, filePath string) bool {
	stdout := runWithMountedOutputImage(container, imageRef,
		`if [[ -e "$CONTAINER_ROOT"/`+filePath+" ]]; then echo exists; else echo does not exist; fi")

	return strings.TrimSpace(stdout) == "exists"
}

// Mount the filesystem of the 'imageRef' container, set CONTAINER_ROOT=<root of mount point>
// and execute the 'script'.
//
// This approach uses 'buildah unshare --mount' to make it possible to do assertions about the
// content of the built image.
//
// Other possible approaches that didn't work:
// - Use 'buildah run $containerID ...' (most tests are FROM scratch, there are no executables to run)
// - Use 'buildah mount' directly (failed with "Error: overlay: failed to make mount private ... operation not permitted")
func runWithMountedOutputImage(container *TestRunnerContainer, imageRef string, script string) string {
	err := container.ExecuteCommand("buildah", "from", "--name=testcontainer", imageRef)
	Expect(err).ToNot(HaveOccurred())
	defer container.ExecuteCommand("buildah", "rm", "testcontainer")

	stdout, _, err := container.ExecuteCommandWithOutput(
		"buildah", "unshare", "--mount", "CONTAINER_ROOT=testcontainer", "--", "bash", "-c", script,
	)
	Expect(err).ToNot(HaveOccurred())

	return stdout
}

// By default, Expect(m1).To(Equal(m2)) is very hard to process on failure,
// because the maps are printed in a random order and the error doesn't specify
// which keys differ.
// Format the maps as key value pairs, sort them and use ConsistOf instead of Equal,
// which shows the differing key value pairs.
func expectEqualMaps(m1 map[string]string, m2 map[string]string, description ...any) {
	m1pairs := formatAsKeyValuePairs(m1)
	slices.Sort(m1pairs)

	m2pairs := formatAsKeyValuePairs(m2)
	slices.Sort(m2pairs)

	Expect(m1pairs).To(ConsistOf(m2pairs), description...)
}

func TestBuild(t *testing.T) {
	SetupGomega(t)

	commonOpts := []ContainerOption{WithUser("taskuser")}
	// On macOS, containers run in a Linux VM; overlay storage driver
	// doesn't work reliably with host volume mounts through the VM
	if runtime.GOOS != "darwin" {
		containerStoragePath, err := createContainerStorageDir()
		t.Cleanup(func() { removeContainerStorageDir(containerStoragePath) })
		Expect(err).ToNot(HaveOccurred())
		commonOpts = append(commonOpts, WithVolumeWithOptions(containerStoragePath, "/home/taskuser/.local/share/containers", "z"))
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
ARG WITH_DEFAULT=default
ARG UNDEFINED

LABEL name=$NAME
LABEL version=$VERSION
LABEL author=$AUTHOR
LABEL vendor=$VENDOR
LABEL buildarg-with-default=$WITH_DEFAULT
LABEL undefined-buildarg=$UNDEFINED

LABEL test.label="build-args-test"
`)

		testutil.WriteFileTree(t, contextDir, map[string]string{
			"build-args-file": "AUTHOR=John Doe\nVENDOR=konflux-ci.dev",
		})

		outputRef := "localhost/test-image-build-args:" + GenerateUniqueTag(t)
		// Also verify that build args are handled properly for Containerfile parsing
		containerfileJsonPath := "/workspace/parsed-containerfile.json"

		buildParams := BuildParams{
			Context:                 contextDir,
			OutputRef:               outputRef,
			Push:                    false,
			BuildArgs:               []string{"NAME=foo", "VERSION=1.2.3"},
			BuildArgsFile:           "/workspace/build-args-file",
			ContainerfileJsonOutput: containerfileJsonPath,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		expectedLabels := []string{
			"name=foo",
			"version=1.2.3",
			"author=John Doe",
			"vendor=konflux-ci.dev",
			"buildarg-with-default=default",
			"undefined-buildarg=",
		}

		imageMeta := getImageMeta(container, outputRef)
		containerfileMeta := getContainerfileMeta(container, containerfileJsonPath)

		// Verify image labels
		imageLabels := formatAsKeyValuePairs(imageMeta.labels)
		Expect(imageLabels).To(ContainElements(expectedLabels))

		// Verify the parsed Containerfile has the same label values
		containerfileLabels := formatAsKeyValuePairs(containerfileMeta.labels)
		Expect(containerfileLabels).To(ContainElements(expectedLabels))

		// Verify that /usr/share/buildinfo/labels.json also has the same label values
		buildInfoLabels := formatAsKeyValuePairs(getLabelsFromLabelsJson(container, outputRef))
		Expect(buildInfoLabels).To(ContainElements(expectedLabels))
	})

	t.Run("PlatformBuildArgs", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `
FROM scratch

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG BUILDPLATFORM
ARG BUILDOS
ARG BUILDARCH
ARG BUILDVARIANT

LABEL TARGETPLATFORM=$TARGETPLATFORM
LABEL TARGETOS=$TARGETOS
LABEL TARGETARCH=$TARGETARCH
LABEL TARGETVARIANT=$TARGETVARIANT
LABEL BUILDPLATFORM=$BUILDPLATFORM
LABEL BUILDOS=$BUILDOS
LABEL BUILDARCH=$BUILDARCH
LABEL BUILDVARIANT=$BUILDVARIANT

LABEL test.label="platform-build-args-test"
`)

		outputRef := "localhost/test-image-platform-build-args:" + GenerateUniqueTag(t)
		containerfileJsonPath := "/workspace/parsed-containerfile.json"

		buildParams := BuildParams{
			Context:                 contextDir,
			OutputRef:               outputRef,
			Push:                    false,
			ContainerfileJsonOutput: containerfileJsonPath,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		// Verify platform values match between the parsed Containerfile, the actual image and buildinfo
		imageLabels := getImageMeta(container, outputRef).labels
		containerfileLabels := getContainerfileMeta(container, containerfileJsonPath).labels
		buildinfoLabels := getLabelsFromLabelsJson(container, outputRef)

		labelsToCheck := []string{
			"TARGETPLATFORM",
			"TARGETOS",
			"TARGETARCH",
			"TARGETVARIANT",
			"BUILDPLATFORM",
			"BUILDOS",
			"BUILDARCH",
			"BUILDVARIANT",
		}

		for _, label := range labelsToCheck {
			imageLabel := imageLabels[label]
			containerfileLabel := containerfileLabels[label]
			buildinfoLabel := buildinfoLabels[label]

			Expect(imageLabel).To(Equal(containerfileLabel),
				fmt.Sprintf("image label: %s=%s; containerfile label: %s=%s", label, imageLabel, label, containerfileLabel),
			)
			Expect(imageLabel).To(Equal(buildinfoLabel),
				fmt.Sprintf("image label: %s=%s; buildinfo label: %s=%s", label, imageLabel, label, buildinfoLabel),
			)
		}
	})

	t.Run("ContainerfileJsonOutput", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `FROM scratch`)

		outputRef := "localhost/test-containerfile-json-output:" + GenerateUniqueTag(t)
		containerfileJsonPath := "/workspace/parsed-containerfile.json"

		buildParams := BuildParams{
			Context:                 contextDir,
			OutputRef:               outputRef,
			Push:                    false,
			ContainerfileJsonOutput: containerfileJsonPath,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		containerfileJSON, err := container.GetFileContent(containerfileJsonPath)
		Expect(err).ToNot(HaveOccurred())

		Expect(containerfileJSON).To(Equal(`{
  "MetaArgs": null,
  "Stages": [
    {
      "Name": "",
      "OrigCmd": "FROM",
      "BaseName": "scratch",
      "Platform": "",
      "Comment": "",
      "SourceCode": "FROM scratch",
      "Location": [
        {
          "Start": {
            "Line": 1,
            "Character": 0
          },
          "End": {
            "Line": 1,
            "Character": 0
          }
        }
      ],
      "From": {
        "Scratch": true
      },
      "Commands": null
    }
  ]
}`))
	})

	t.Run("WithEnvs", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `
FROM scratch

LABEL foo=$FOO
LABEL bar=$BAR

LABEL test.label="envs-test"
`)

		outputRef := "localhost/test-image-envs:" + GenerateUniqueTag(t)
		// Also verify that envs are handled properly for Containerfile parsing
		containerfileJsonPath := "/workspace/parsed-containerfile.json"

		buildParams := BuildParams{
			Context:   contextDir,
			OutputRef: outputRef,
			Push:      false,
			Envs: []string{
				"FOO=foo-value",
				"BAR=bar-value",
				// Corner cases to verify that dockerfile-json and buildah handle them the same way
				// Should be an env var without a name (causes an error when starting the container)
				"=noname",
				// Should be an empty string
				"NOVALUE=",
				// Shouldn't be set at all
				"NOSUCHENV",
			},
			ContainerfileJsonOutput: containerfileJsonPath,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		expectedEnvs := []string{
			"FOO=foo-value",
			"BAR=bar-value",
			"=noname",
			"NOVALUE=",
		}
		expectedLabels := []string{
			"foo=foo-value",
			"bar=bar-value",
		}

		imageMeta := getImageMeta(container, outputRef)
		containerfileMeta := getContainerfileMeta(container, containerfileJsonPath)

		// Verify envs
		imageEnvs := formatAsKeyValuePairs(imageMeta.envs)
		Expect(imageEnvs).To(ContainElements(expectedEnvs))

		containerfileEnvs := formatAsKeyValuePairs(containerfileMeta.envs)
		Expect(containerfileEnvs).To(ContainElements(expectedEnvs))

		Expect(imageMeta.envs).ToNot(HaveKey("NOSUCHENV"))
		Expect(containerfileMeta.envs).ToNot(HaveKey("NOSUCHENV"))

		// Verify labels
		imageLabels := formatAsKeyValuePairs(imageMeta.labels)
		Expect(imageLabels).To(ContainElements(expectedLabels))

		containerfileLabels := formatAsKeyValuePairs(containerfileMeta.labels)
		Expect(containerfileLabels).To(ContainElements(expectedLabels))

		buildinfoLabels := formatAsKeyValuePairs(getLabelsFromLabelsJson(container, outputRef))
		Expect(buildinfoLabels).To(ContainElements(expectedLabels))
	})

	t.Run("WithLabelsAndAnnotations", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `FROM scratch`)

		outputRef := "localhost/test-image-labels-annotations:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:   contextDir,
			OutputRef: outputRef,
			Push:      false,
			Labels: []string{
				"custom.label1=value1",
				"custom.label2=value2",
			},
			Annotations: []string{
				"org.opencontainers.image.title=King Arthur",
				"org.opencontainers.image.description=Elected by farcical aquatic ceremony.",
			},
			ImageSource:           "https://github.com/konflux-ci/test",
			ImageRevision:         "abc123",
			LegacyBuildTimestamp:  "1767225600", // 2026-01-01
			QuayImageExpiresAfter: "2w",
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		expectedLabels := []string{
			"custom.label1=value1",
			"custom.label2=value2",
			// default labels (with user-supplied values)
			"org.opencontainers.image.source=https://github.com/konflux-ci/test",
			"org.opencontainers.image.revision=abc123",
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"quay.expires-after=2w",
		}

		expectedAnnotations := []string{
			"org.opencontainers.image.title=King Arthur",
			"org.opencontainers.image.description=Elected by farcical aquatic ceremony.",
			// default annotations (with user-supplied values)
			"org.opencontainers.image.source=https://github.com/konflux-ci/test",
			"org.opencontainers.image.revision=abc123",
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
		}

		imageMeta := getImageMeta(container, outputRef)

		imageLabels := formatAsKeyValuePairs(imageMeta.labels)
		Expect(imageLabels).To(ContainElements(expectedLabels))

		buildinfoLabels := formatAsKeyValuePairs(getLabelsFromLabelsJson(container, outputRef))
		Expect(buildinfoLabels).To(ContainElements(expectedLabels))

		imageAnnotations := formatAsKeyValuePairs(imageMeta.annotations)
		Expect(imageAnnotations).To(ContainElements(expectedAnnotations))
	})

	t.Run("AnnotationsFile", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `FROM scratch`)

		annotationsFileContent := `
# overrides default annotation
org.opencontainers.image.created=never

annotation.from.file=overriden-below
annotation.from.file=annotation-from-file

common.annotation=overriden-by-cli-annotation
`
		testutil.WriteFileTree(t, contextDir, map[string]string{
			"annotations.cfg": annotationsFileContent,
		})

		outputRef := "localhost/test-annotations-file:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:   contextDir,
			OutputRef: outputRef,
			Push:      false,
			Annotations: []string{
				"annotation.from.cli=annotation-from-CLI",
				"common.annotation=common-annotation-from-CLI",
			},
			AnnotationsFile: "/workspace/annotations.cfg",
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		imageMeta := getImageMeta(container, outputRef)

		imageAnnotations := formatAsKeyValuePairs(imageMeta.annotations)
		Expect(imageAnnotations).To(ContainElements(
			"org.opencontainers.image.created=never",
			"annotation.from.file=annotation-from-file",
			"annotation.from.cli=annotation-from-CLI",
			"common.annotation=common-annotation-from-CLI",
		))
	})

	t.Run("OverrideDefaultLabelsAndAnnotations", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `FROM scratch`)

		outputRef := "localhost/test-override-defaults:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:       contextDir,
			OutputRef:     outputRef,
			Push:          false,
			ImageSource:   "https://default.com",
			ImageRevision: "default",
			Labels: []string{
				// override source, but not revision
				"org.opencontainers.image.source=https://user-override.com",
			},
			Annotations: []string{
				// override revision, but not source
				"org.opencontainers.image.revision=override",
			},
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		imageMeta := getImageMeta(container, outputRef)

		imageLabels := formatAsKeyValuePairs(imageMeta.labels)
		Expect(imageLabels).To(ContainElements(
			"org.opencontainers.image.source=https://user-override.com",
			"org.opencontainers.image.revision=default",
		))
		buildinfoLabels := formatAsKeyValuePairs(getLabelsFromLabelsJson(container, outputRef))
		Expect(buildinfoLabels).To(ContainElements(
			"org.opencontainers.image.source=https://user-override.com",
			"org.opencontainers.image.revision=default",
		))

		imageAnnotations := formatAsKeyValuePairs(imageMeta.annotations)
		Expect(imageAnnotations).To(ContainElements(
			"org.opencontainers.image.source=https://default.com",
			"org.opencontainers.image.revision=override",
		))
	})

	t.Run("WithLegacyLabels", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `FROM scratch`)

		outputRef := "localhost/test-legacy-labels:" + GenerateUniqueTag(t)

		buildParams := BuildParams{
			Context:              contextDir,
			OutputRef:            outputRef,
			Push:                 false,
			ImageSource:          "https://github.com/konflux-ci/test",
			ImageRevision:        "abc123",
			LegacyBuildTimestamp: "1767225600", // 2026-01-01
			AddLegacyLabels:      true,
		}

		container := setupBuildContainerWithCleanup(t, buildParams, nil)

		err := runBuild(container, buildParams)
		Expect(err).ToNot(HaveOccurred())

		imageMeta := getImageMeta(container, outputRef)
		buildinfoLabels := getLabelsFromLabelsJson(container, outputRef)

		// Get the expected architecture from 'uname -m' in the container
		stdout, _, err := container.ExecuteCommandWithOutput("uname", "-m")
		Expect(err).ToNot(HaveOccurred())
		expectedArch := strings.TrimSpace(stdout)

		expectedLabels := []string{
			"org.opencontainers.image.source=https://github.com/konflux-ci/test",
			"vcs-url=https://github.com/konflux-ci/test",
			"org.opencontainers.image.revision=abc123",
			"vcs-ref=abc123",
			"vcs-type=git",
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			"build-date=2026-01-01T00:00:00Z",
		}

		imageLabels := formatAsKeyValuePairs(imageMeta.labels)
		buildinfoLabelPairs := formatAsKeyValuePairs(buildinfoLabels)

		Expect(imageMeta.labels).To(HaveKey("architecture"))
		Expect(imageMeta.labels["architecture"]).To(Equal(expectedArch),
			"architecture label should match uname -m output")
		Expect(buildinfoLabels).To(HaveKey("architecture"))
		Expect(buildinfoLabels["architecture"]).To(Equal(expectedArch),
			"architecture label should match uname -m output")

		Expect(imageLabels).To(ContainElements(expectedLabels))
		Expect(buildinfoLabelPairs).To(ContainElements(expectedLabels))

		// Annotations should not include legacy labels
		imageAnnotations := formatAsKeyValuePairs(imageMeta.annotations)
		Expect(imageMeta.annotations).ToNot(HaveKey("architecture"))
		Expect(imageMeta.annotations).ToNot(HaveKey("vcs-url"))
		Expect(imageMeta.annotations).ToNot(HaveKey("vcs-ref"))
		Expect(imageMeta.annotations).ToNot(HaveKey("vcs-type"))
		Expect(imageMeta.annotations).ToNot(HaveKey("build-date"))
		Expect(imageAnnotations).To(ContainElements(
			"org.opencontainers.image.source=https://github.com/konflux-ci/test",
			"org.opencontainers.image.revision=abc123",
			"org.opencontainers.image.created=2026-01-01T00:00:00Z",
		))
	})

	t.Run("SourceDateEpoch", func(t *testing.T) {
		contextDir := setupTestContext(t)

		writeContainerfile(contextDir, `
FROM scratch

ARG SOURCE_DATE_EPOCH

LABEL source-date-epoch=$SOURCE_DATE_EPOCH
`)

		sourceDateEpoch := "1767225600" // 2026-01-01

		checkSourceDateEpochEffects := func(imageMeta containerImageMeta) {
			imageAnnotations := formatAsKeyValuePairs(imageMeta.annotations)
			imageLabels := formatAsKeyValuePairs(imageMeta.labels)

			// Should set the 'created' attribute
			Expect(imageMeta.created).To(Equal("2026-01-01T00:00:00Z"))
			Expect(imageAnnotations).To(ContainElements(
				// Should set the org.opencontainers.image.created annotation
				"org.opencontainers.image.created=2026-01-01T00:00:00Z",
			))
			Expect(imageLabels).To(ContainElements(
				// Should set the org.opencontainers.image.created label
				"org.opencontainers.image.created=2026-01-01T00:00:00Z",
				// With --add-legacy-labels, should also set the build-date label
				"build-date=2026-01-01T00:00:00Z",
				// Should set the SOURCE_DATE_EPOCH build argument
				"source-date-epoch=1767225600",
			))
		}

		t.Run("FromCLI", func(t *testing.T) {
			outputRef := "localhost/test-source-date-epoch-from-cli:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:         contextDir,
				OutputRef:       outputRef,
				Push:            false,
				SourceDateEpoch: sourceDateEpoch,
				AddLegacyLabels: true,
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			imageMeta := getImageMeta(container, outputRef)
			checkSourceDateEpochEffects(imageMeta)
		})

		// Test the SOURCE_DATE_EPOCH environment variable as well, because unlike other env vars,
		// it doesn't have the KBC_ prefix and we still want to handle it.
		t.Run("FromEnv", func(t *testing.T) {
			outputRef := "localhost/test-source-date-epoch-from-cli:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:         contextDir,
				OutputRef:       outputRef,
				Push:            false,
				AddLegacyLabels: true,
			}

			container := setupBuildContainerWithCleanup(
				t, buildParams, nil, WithEnv("SOURCE_DATE_EPOCH", sourceDateEpoch),
			)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			imageMeta := getImageMeta(container, outputRef)
			checkSourceDateEpochEffects(imageMeta)
		})
	})

	t.Run("Reproducibility", func(t *testing.T) {
		buildImage := func() containerImageMeta {
			contextDir := setupTestContext(t)
			// The file is newly created for every test build, has a different timestamp every time
			// and would normally break reproducibility. Try setting RewriteTimestamp to false and
			// see that imageMeta.digest is different every time.
			testutil.WriteFileTree(t, contextDir, map[string]string{
				"hello.txt": "hello there\n",
			})

			writeContainerfile(contextDir, `
FROM scratch

COPY hello.txt /hello.txt
`)

			outputRef := "localhost/test-reproducibility:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
				// Thanks to the combination of --source-date-epoch and --rewrite-timestamp,
				// the timestamp of /hello.txt inside the built image will be clamped to 2026-01-01
				SourceDateEpoch:  "1767225600",
				RewriteTimestamp: true,
				// Add more labels to test that injecting labels.json doesn't break reproducibility.
				// E.g. if the order of labels in the file was random, it *would* break reproducibility.
				AddLegacyLabels: true,
				Labels:          []string{"label1=foo", "label2=bar"},
				ExtraArgs: []string{
					// Ensure the image config will be exactly the same regardles of the host OS/architecture
					"--platform", "linux/amd64",
					// We want to build the image twice and verify reproducibility, avoid caching
					"--no-cache",
				},
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			imageMeta := getImageMeta(container, outputRef)
			return imageMeta
		}

		imageMeta1 := buildImage()

		// Thanks to all time-related metadata being set to 2026-01-01, the build is fully reproducible
		// and the digest will stay the same every time.
		Expect(imageMeta1.digest).To(Equal("sha256:678998fc429d59022f0966551854f99bd1c553c01b74a093a420a927576f5d8a"))
		Expect(imageMeta1.created).To(Equal("2026-01-01T00:00:00Z"))

		// Ensure the hello.txt file created for the second test really does get a different timestamp
		time.Sleep(1 * time.Second)

		// Build from the same inputs (just with different timestamps) twice.
		// The built image should have the same digest both times.
		imageMeta2 := buildImage()
		Expect(imageMeta2.digest).To(Equal(imageMeta1.digest), "Digest unexpectedly changed after rebuild")
	})

	t.Run("InjectingBuildinfo", func(t *testing.T) {
		t.Run("LabelsJSON", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, fmt.Sprintf(`
# Real base image with its own labels, such as:
# - com.redhat.component
# - name
# - vendor
FROM %s

# Built-in buildarg
ARG BUILDOS
LABEL build.os=$BUILDOS

# User-provided arg
ARG FOO
LABEL buildarg.label=$FOO

# User-provided env
LABEL env.label=$BAR

# Static value
LABEL static.label=static-value

# Same as a regular buildah build, the number of layers in the resulting image
# should be [number of layers in the base image] + 1. Injecting the labels.json
# file must not affect that. To verify this, add more instructions that create
# layers, otherwise the injected COPY instruction could be the only one.
RUN echo "this instruction creates an intermediate layer" > /tmp/foo.txt
RUN echo "this instruction also creates an intermediate layer" > /tmp/bar.txt
`, baseImage))

			outputRef := "localhost/test-injecting-buildinfo-all-label-sources:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
				// Set a conspicuous source date epoch so that we can more easily verify
				// that our 'created' label overrides the one from the base image
				SourceDateEpoch: "882921600", // 1997-12-24
				BuildArgs:       []string{"FOO=foo"},
				Envs:            []string{"BAR=bar"},
				Labels:          []string{"cli.label=value-gets-overriden", "cli.label=label-from-CLI"},
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).To(SatisfyAll(
				// Base image labels
				HaveKeyWithValue("com.redhat.component", "ubi10-micro-container"),
				HaveKeyWithValue("name", "ubi10/ubi-micro"),
				HaveKeyWithValue("vendor", "Red Hat, Inc."),
				// Normally auto-injected, but not if --timestamp or --source-date-epoch is used
				// (see https://www.mankier.com/1/buildah-build#--identity-label).
				// So in this case, it comes from the base image.
				HaveKeyWithValue("io.buildah.version", "1.41.4"),
				// Containerfile labels
				HaveKeyWithValue("build.os", "linux"),
				HaveKeyWithValue("buildarg.label", "foo"),
				HaveKeyWithValue("env.label", "bar"),
				HaveKeyWithValue("static.label", "static-value"),
				// Auto-injected labels
				HaveKeyWithValue("org.opencontainers.image.created", "1997-12-24T00:00:00Z"),
				// CLI labels
				HaveKeyWithValue("cli.label", "label-from-CLI"),
			))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")

			Expect(imageMeta.layer_ids).To(HaveLen(2),
				"Expected the output image to have 2 layers: [number of base image layers] + 1",
			)

			stat := statFileInOutputImage(container, outputRef, "/usr/share/buildinfo/labels.json")
			Expect(stat).To(ContainSubstring("Access: (0644/-rw-r--r--)"),
				"The injected labels.json file should have mode 0644")
		})

		t.Run("LabelsFromEarlierStages", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, fmt.Sprintf(`
# Real base image used for an earlier stage
FROM %s AS stage1

LABEL stage1.label=value-gets-overriden
LABEL stage1.label=label-from-stage1

LABEL common.build.label=overriden-in-stage-2
LABEL common.label=overriden-in-final-stage


FROM stage1 AS stage2

LABEL stage2.label=value-gets-overriden
LABEL stage2.label=label-from-stage2

LABEL common.build.label=common-build-stage2


# Not a dependency for the final stage, buildah will skip this completely
FROM base.image.does.not/exist:latest AS unused-stage

LABEL unused.stage.label=label-from-unused-stage


FROM stage2

LABEL final.stage.label=value-gets-overriden
LABEL final.stage.label=label-from-final-stage

LABEL common.label=common-final-stage
`, baseImage))

			outputRef := "localhost/test-injecting-buildinfo-labels-from-earlier-stages:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).To(SatisfyAll(
				// Base image labels
				HaveKeyWithValue("com.redhat.component", "ubi10-micro-container"),
				HaveKeyWithValue("name", "ubi10/ubi-micro"),
				HaveKeyWithValue("vendor", "Red Hat, Inc."),
				// Labels from stages
				HaveKeyWithValue("stage1.label", "label-from-stage1"),
				HaveKeyWithValue("stage2.label", "label-from-stage2"),
				HaveKeyWithValue("final.stage.label", "label-from-final-stage"),
				HaveKeyWithValue("common.build.label", "common-build-stage2"),
				HaveKeyWithValue("common.label", "common-final-stage"),
				// Buildah version label. We don't know the version, just check that it exists.
				// The correct value is later verified by comparing labels.json to the actual labels.
				HaveKey("io.buildah.version"),
				// Should not have label from unused stage
				Not(HaveKey("unused.stage.label")),
			))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")
		})

		t.Run("LabelsFromScratch", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, `
FROM scratch

LABEL containerfile.label=label-from-containerfile
`)

			outputRef := "localhost/test-injecting-buildinfo-labels-from-scratch:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
				Labels:    []string{"cli.label=label-from-CLI"},
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).To(SatisfyAll(
				HaveKey("org.opencontainers.image.created"),
				HaveKeyWithValue("containerfile.label", "label-from-containerfile"),
				HaveKeyWithValue("cli.label", "label-from-CLI"),
			))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")
		})

		t.Run("AvoidsContainerignore", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, `FROM scratch`)

			testutil.WriteFileTree(t, contextDir, map[string]string{
				// Ignore everything, but has no effect on COPY --from=<different buildcontext>
				".containerignore": "*\n.*\n",
			})

			outputRef := "localhost/test-injecting-buildinfo-force-through-containerignore:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).To(HaveKey("io.buildah.version"))
			Expect(injectedLabels).To(HaveKey("org.opencontainers.image.created"))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")
		})

		t.Run("BuildahVersionPrecedence", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, `
FROM scratch

LABEL io.buildah.version=0.0.1
`)

			outputRef := "localhost/test-injecting-buildinfo-buildah-version-precedence:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
				Labels:    []string{"io.buildah.version=0.1.0"},
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).To(HaveKey("io.buildah.version"))
			// The auto-injected buildah version label cannot be overriden by anything
			Expect(injectedLabels["io.buildah.version"]).To(Not(HavePrefix("0")))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")
		})

		t.Run("NoBuildahVersion", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, `FROM scratch`)

			outputRef := "localhost/test-injecting-buildinfo-no-buildah-version:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:   contextDir,
				OutputRef: outputRef,
				Push:      false,
				// --source-date-epoch disables the io.buildah.version injection
				SourceDateEpoch: "1767225600",
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			injectedLabels := getLabelsFromLabelsJson(container, outputRef)
			Expect(injectedLabels).ToNot(HaveKey("io.buildah.version"))

			imageMeta := getImageMeta(container, outputRef)
			expectEqualMaps(injectedLabels, imageMeta.labels,
				"Expected labels.json (top) to match the actual image labels (bottom)")
		})

		t.Run("SkipInjections", func(t *testing.T) {
			contextDir := setupTestContext(t)

			writeContainerfile(contextDir, `FROM scratch`)

			outputRef := "localhost/test-injecting-buildinfo-skip-injections:" + GenerateUniqueTag(t)

			buildParams := BuildParams{
				Context:        contextDir,
				OutputRef:      outputRef,
				Push:           false,
				SkipInjections: true,
			}

			container := setupBuildContainerWithCleanup(t, buildParams, nil)

			err := runBuild(container, buildParams)
			Expect(err).ToNot(HaveOccurred())

			exists := fileExistsInOutputImage(container, outputRef, "/usr/share/buildinfo/labels.json")
			Expect(exists).To(BeFalse(), "Should not have injected labels.json")
		})

		t.Run("KnownIssues", func(t *testing.T) {
			t.Run("UnsupportedBaseImage", func(t *testing.T) {
				contextDir := setupTestContext(t)

				writeContainerfile(contextDir, `
# Note that this case is technically supportable, because the ./base_image exists
# before the build starts, so we *could* inspect it.
# But for any real-world use cases, this will not be the case. The ./base_image
# will be created during the build (like in the WorkdirMount testcase).
FROM oci:./base_image

LABEL containerfile.label=containerfile-label
`)

				outputRef := "localhost/test-injecting-buildinfo-unsupported-base-image:" + GenerateUniqueTag(t)

				buildParams := BuildParams{
					Context:   contextDir,
					OutputRef: outputRef,
					Push:      false,
					Labels:    []string{"cli.label=cli-label"},
				}

				container := setupBuildContainerWithCleanup(t, buildParams, nil)

				err := container.ExecuteCommand("buildah", "pull", baseImage)
				Expect(err).ToNot(HaveOccurred())
				// Push baseimage to <contextDir>/base_image (contextDir is mounted at /workspace)
				err = container.ExecuteCommand(
					"buildah", "push", "--remove-signatures", baseImage, "oci:/workspace/base_image",
				)
				Expect(err).ToNot(HaveOccurred())

				err = runBuild(container, buildParams)
				Expect(err).ToNot(HaveOccurred())

				injectedLabels := getLabelsFromLabelsJson(container, outputRef)
				// labels.json will still have labels from Containerfile etc.
				Expect(injectedLabels).To(SatisfyAll(
					HaveKeyWithValue("containerfile.label", "containerfile-label"),
					HaveKeyWithValue("cli.label", "cli-label"),
					HaveKey("io.buildah.version"),
				))
				// labels.json will NOT have any base image labels
				Expect(injectedLabels).To(SatisfyAll(
					Not(HaveKey("com.redhat.component")),
					Not(HaveKey("name")),
					Not(HaveKey("vendor")),
				))
				// but the actual image WILL have them
				imageMeta := getImageMeta(container, outputRef)
				Expect(imageMeta.labels).To(SatisfyAll(
					HaveKey("com.redhat.component"),
					HaveKey("name"),
					HaveKey("vendor"),
				))
			})

			t.Run("LabelsWithQuotes", func(t *testing.T) {
				contextDir := setupTestContext(t)

				writeContainerfile(contextDir, `
FROM scratch

LABEL with.single.quotes='label with single quotes'
LABEL with.double.quotes="label with double quotes"

ARG SINGLE_QUOTED_ARG='arg with single quotes'
ARG DOUBLE_QUOTED_ARG="arg with double quotes"

LABEL unquoted.arg.with.single.quotes=$SINGLE_QUOTED_ARG
LABEL unquoted.arg.with.double.quotes=$DOUBLE_QUOTED_ARG

LABEL quoted.arg.with.single.quotes="$SINGLE_QUOTED_ARG"
LABEL quoted.arg.with.double.quotes="$DOUBLE_QUOTED_ARG"

LABEL literal.argname.1='$SINGLE_QUOTED_ARG'
LABEL literal.argname.2='$DOUBLE_QUOTED_ARG'
`)

				outputRef := "localhost/test-injecting-buildinfo-labels-with-quotes:" + GenerateUniqueTag(t)

				buildParams := BuildParams{
					Context:   contextDir,
					OutputRef: outputRef,
					Push:      false,
				}

				container := setupBuildContainerWithCleanup(t, buildParams, nil)

				err := runBuild(container, buildParams)
				Expect(err).ToNot(HaveOccurred())

				imageMeta := getImageMeta(container, outputRef)
				injectedLabels := getLabelsFromLabelsJson(container, outputRef)

				// The actual labels follow shell expansion rules for quotes
				Expect(formatAsKeyValuePairs(imageMeta.labels)).To(ContainElements(
					`with.single.quotes=label with single quotes`,
					`with.double.quotes=label with double quotes`,
					`unquoted.arg.with.single.quotes=arg with single quotes`,
					`unquoted.arg.with.double.quotes=arg with double quotes`,
					`quoted.arg.with.single.quotes=arg with single quotes`,
					`quoted.arg.with.double.quotes=arg with double quotes`,
					`literal.argname.1=$SINGLE_QUOTED_ARG`,
					`literal.argname.2=$DOUBLE_QUOTED_ARG`,
				))

				// Quote handling is broken in dockerfile-json, so labels.json is equally broken
				Expect(formatAsKeyValuePairs(injectedLabels)).To(ContainElements(
					`with.single.quotes='label with single quotes'`,
					`with.double.quotes="label with double quotes"`,
					`unquoted.arg.with.single.quotes='arg with single quotes'`,
					`unquoted.arg.with.double.quotes="arg with double quotes"`,
					`quoted.arg.with.single.quotes="'arg with single quotes'"`,
					`quoted.arg.with.double.quotes=""arg with double quotes""`,
					`literal.argname.1=''arg with single quotes''`,
					`literal.argname.2='"arg with double quotes"'`,
				))
			})
		})
	})
}
