package cliwrappers_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func setupBuildahCli() (*cliwrappers.BuildahCli, *mockExecutor) {
	executor := &mockExecutor{}
	buildahCli := &cliwrappers.BuildahCli{Executor: executor}
	return buildahCli, executor
}

func ensureRetryerDisabled(t *testing.T) {
	retryerDisabled := cliwrappers.DisableRetryer
	if !retryerDisabled {
		cliwrappers.DisableRetryer = true
		t.Cleanup(func() { cliwrappers.DisableRetryer = false })
	}
}

func TestBuildahCli_Build(t *testing.T) {
	g := NewWithT(t)

	const containerfile = "/path/to/Containerfile"
	const contextDir = "/path/to/context"
	const outputRef = "quay.io/org/image:tag"

	t.Run("should execute buildah correctly", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}

		err := buildahCli.Build(buildArgs)

		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs[0]).To(Equal("build"))
		expectArgAndValue(g, capturedArgs, "--file", containerfile)
		expectArgAndValue(g, capturedArgs, "--tag", outputRef)
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal(contextDir))
	})

	t.Run("should error if buildah execution fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("failed to execute buildah build")
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}

		err := buildahCli.Build(buildArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("failed to execute buildah build"))
	})

	t.Run("should error if args are invalid", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: "",
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}
		err := buildahCli.Build(buildArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("validating buildah args: containerfile path is empty"))
	})

	t.Run("should turn Secrets into --secret params", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			Secrets: []cliwrappers.BuildahSecret{
				{Src: "/some/file", Id: "mysecret_1"},
				{Src: "/other/file", Id: "mysecret_2"},
			},
		}

		err := buildahCli.Build(buildArgs)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs).To(ContainElement("--secret=src=/some/file,id=mysecret_1"))
		g.Expect(capturedArgs).To(ContainElement("--secret=src=/other/file,id=mysecret_2"))
	})

	t.Run("should turn Volumes into --volume params", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			Volumes: []cliwrappers.BuildahVolume{
				{HostDir: "/host/dir1", ContainerDir: "/container/dir1", Options: ""},
				{HostDir: "/host/dir2", ContainerDir: "/container/dir2", Options: "ro"},
			},
		}

		err := buildahCli.Build(buildArgs)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs).To(ContainElement("--volume=/host/dir1:/container/dir1"))
		g.Expect(capturedArgs).To(ContainElement("--volume=/host/dir2:/container/dir2:ro"))
	})

	t.Run("should turn BuildContextx into --build-context params", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			BuildContexts: []cliwrappers.BuildahBuildContext{
				{Name: "context1", Location: "context/dir/a"},
				{Name: "context2", Location: "/absolute/context/dir/b"},
			},
		}

		err := buildahCli.Build(buildArgs)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs).To(ContainElement("--build-context=context1=context/dir/a"))
		g.Expect(capturedArgs).To(ContainElement("--build-context=context2=/absolute/context/dir/b"))
	})

	t.Run("should turn BuildArgs(File) into --build-arg(-file) params", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			BuildArgs:     []string{"VERSION=1.0.0", "BUILD_DATE=2024-01-01"},
			BuildArgsFile: "/path/to/build-args-file",
		}

		err := buildahCli.Build(buildArgs)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs).To(ContainElement("--build-arg=VERSION=1.0.0"))
		g.Expect(capturedArgs).To(ContainElement("--build-arg=BUILD_DATE=2024-01-01"))
		g.Expect(capturedArgs).To(ContainElement("--build-arg-file=/path/to/build-args-file"))
	})

	t.Run("should turn Envs into --env params", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			Envs:          []string{"FOO=bar", "BAZ=qux"},
		}

		err := buildahCli.Build(buildArgs)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(capturedArgs).To(ContainElement("--env=FOO=bar"))
		g.Expect(capturedArgs).To(ContainElement("--env=BAZ=qux"))
	})

	t.Run("should append extra args before context directory", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			ExtraArgs:     []string{"--compat-volumes", "--force-rm"},
		}

		err := buildahCli.Build(buildArgs)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify the command structure
		g.Expect(capturedArgs[0]).To(Equal("build"))
		expectArgAndValue(g, capturedArgs, "--file", containerfile)
		expectArgAndValue(g, capturedArgs, "--tag", outputRef)
		// Extra args should be present
		g.Expect(capturedArgs).To(ContainElement("--compat-volumes"))
		g.Expect(capturedArgs).To(ContainElement("--force-rm"))
		// Context directory should be the last argument
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal(contextDir))
	})
}

func findDigestFile(args []string) string {
	for i, arg := range args {
		if arg == "--digestfile" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestBuildahCli_Push(t *testing.T) {
	g := NewWithT(t)

	const image = "quay.io/org/image:tag"
	const digest = "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	ensureRetryerDisabled(t)

	mockSuccessfulPush := func(captureArgs *[]string) func(cmd cliwrappers.Cmd) (string, string, int, error) {
		return func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			*captureArgs = cmd.Args

			digestFile := findDigestFile(cmd.Args)
			g.Expect(digestFile).ToNot(BeEmpty())

			os.WriteFile(digestFile, []byte(digest), 0644)

			return "", "", 0, nil
		}
	}

	t.Run("should push image", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = mockSuccessfulPush(&capturedArgs)

		pushArgs := &cliwrappers.BuildahPushArgs{
			Image: image,
		}

		returnedDigest, err := buildahCli.Push(pushArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs[0]).To(Equal("push"))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal(image))

		g.Expect(returnedDigest).To(Equal(digest))
	})

	t.Run("should error if buildah execution fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("failed to execute buildah push")
		}

		pushArgs := &cliwrappers.BuildahPushArgs{
			Image: image,
		}

		_, err := buildahCli.Push(pushArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("failed to execute buildah push"))
	})

	t.Run("should error if image is empty", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		pushArgs := &cliwrappers.BuildahPushArgs{
			Image: "",
		}
		_, err := buildahCli.Push(pushArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("image arg is empty"))
	})

	t.Run("should clean up digest file after push", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = mockSuccessfulPush(&capturedArgs)

		pushArgs := &cliwrappers.BuildahPushArgs{
			Image: image,
		}

		_, err := buildahCli.Push(pushArgs)

		g.Expect(err).ToNot(HaveOccurred())

		digestFile := findDigestFile(capturedArgs)
		g.Expect(digestFile).ToNot(BeEmpty())

		// Verify the digest file was cleaned up
		_, statErr := os.Stat(digestFile)
		g.Expect(os.IsNotExist(statErr)).To(BeTrue(), "digest file should be cleaned up")
	})

	t.Run("should handle digest with whitespace", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		digestWithWhitespace := "\n  " + digest + "  \n"
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			digestFile := findDigestFile(cmd.Args)
			os.WriteFile(digestFile, []byte(digestWithWhitespace), 0644)
			return "", "", 0, nil
		}

		pushArgs := &cliwrappers.BuildahPushArgs{
			Image: image,
		}

		returnedDigest, err := buildahCli.Push(pushArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(returnedDigest).To(Equal(digest), "digest should be trimmed")
	})

	t.Run("should include destination when provided", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		const destination = "docker://quay.io/other-org/other-image:tag"
		var capturedArgs []string
		executor.executeFunc = mockSuccessfulPush(&capturedArgs)

		pushArgs := &cliwrappers.BuildahPushArgs{
			Image:       image,
			Destination: destination,
		}

		_, err := buildahCli.Push(pushArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs[0]).To(Equal("push"))
		g.Expect(capturedArgs[len(capturedArgs)-2]).To(Equal(image))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal(destination))
	})
}

func TestBuildahCli_Pull(t *testing.T) {
	g := NewWithT(t)

	const image = "quay.io/org/image:tag"

	ensureRetryerDisabled(t)

	t.Run("should pull image", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return "", "", 0, nil
		}

		pullArgs := &cliwrappers.BuildahPullArgs{
			Image: image,
		}

		err := buildahCli.Pull(pullArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs[0]).To(Equal("pull"))
		g.Expect(capturedArgs[1]).To(Equal(image))
	})

	t.Run("should error if buildah execution fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("failed to execute buildah pull")
		}

		pullArgs := &cliwrappers.BuildahPullArgs{
			Image: image,
		}

		err := buildahCli.Pull(pullArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("failed to execute buildah pull"))
	})

	t.Run("should error if image is empty", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		pullArgs := &cliwrappers.BuildahPullArgs{
			Image: "",
		}
		err := buildahCli.Pull(pullArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("image arg is empty"))
	})

	t.Run("should set http proxy env vars", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedEnv []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			capturedEnv = cmd.Env
			return "", "", 0, nil
		}

		pullArgs := &cliwrappers.BuildahPullArgs{
			Image:     image,
			HttpProxy: "http://proxy.example.com:8080",
			NoProxy:   "localhost,127.0.0.1",
		}

		err := buildahCli.Pull(pullArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedEnv).To(ContainElements(
			"HTTP_PROXY=http://proxy.example.com:8080",
			"HTTPS_PROXY=http://proxy.example.com:8080",
			"NO_PROXY=localhost,127.0.0.1",
		))
	})

	t.Run("should not set env vars when no proxy is configured", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedEnv []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			capturedEnv = cmd.Env
			return "", "", 0, nil
		}

		pullArgs := &cliwrappers.BuildahPullArgs{Image: image}

		err := buildahCli.Pull(pullArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedEnv).To(BeNil())
	})
}

func TestBuildahCli_Inspect(t *testing.T) {
	g := NewWithT(t)

	const imageName = "localhost/image:tag"

	t.Run("should execute buildah inspect correctly", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			return `{"OCIv1": {}}`, "", 0, nil
		}

		inspectArgs := &cliwrappers.BuildahInspectArgs{
			Name: imageName,
			Type: "image",
		}

		jsonOutput, err := buildahCli.Inspect(inspectArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(jsonOutput).To(ContainSubstring("OCIv1"))
		g.Expect(capturedArgs[0]).To(Equal("inspect"))
		expectArgAndValue(g, capturedArgs, "--type", "image")
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal(imageName))
	})

	t.Run("should error when name is empty", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executorCalled := false
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			executorCalled = true
			return "", "", 0, nil
		}

		inspectArgs := &cliwrappers.BuildahInspectArgs{
			Name: "",
			Type: "image",
		}

		_, err := buildahCli.Inspect(inspectArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("name is empty"))
		g.Expect(executorCalled).To(BeFalse())
	})

	t.Run("should error when type is empty", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executorCalled := false
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			executorCalled = true
			return "", "", 0, nil
		}

		inspectArgs := &cliwrappers.BuildahInspectArgs{
			Name: imageName,
			Type: "",
		}

		_, err := buildahCli.Inspect(inspectArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("type is empty"))
		g.Expect(executorCalled).To(BeFalse())
	})

	t.Run("should error when buildah execution fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("buildah inspect failed")
		}

		inspectArgs := &cliwrappers.BuildahInspectArgs{
			Name: imageName,
			Type: "image",
		}

		_, err := buildahCli.Inspect(inspectArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("buildah inspect failed"))
	})
}

func TestBuildahCli_InspectImage(t *testing.T) {
	g := NewWithT(t)

	const imageName = "quay.io/org/image:tag"

	t.Run("should parse valid JSON successfully", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()

		sampleJSON := `{
			"OCIv1": {
				"created": "2024-01-01T00:00:00Z",
				"config": {
					"Env": ["PATH=/usr/bin", "HOME=/root"],
					"Labels": {"version": "1.0", "maintainer": "test"}
				}
			}
		}`

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return sampleJSON, "", 0, nil
		}

		imageInfo, err := buildahCli.InspectImage(imageName)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(imageInfo.OCIv1.Created).ToNot(BeNil())
		g.Expect(imageInfo.OCIv1.Created.Format(time.RFC3339)).To(Equal("2024-01-01T00:00:00Z"))
		g.Expect(imageInfo.OCIv1.Config.Env).To(Equal([]string{"PATH=/usr/bin", "HOME=/root"}))
		g.Expect(imageInfo.OCIv1.Config.Labels).To(Equal(map[string]string{"version": "1.0", "maintainer": "test"}))
	})

	t.Run("should error when Inspect fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return "", "", 1, errors.New("buildah inspect failed")
		}

		_, err := buildahCli.InspectImage(imageName)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("buildah inspect failed"))
	})

	t.Run("should error when JSON parsing fails", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()

		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			return `{invalid json}`, "", 0, nil
		}

		_, err := buildahCli.InspectImage(imageName)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("parsing inspect output"))
	})
}

func TestBuildahCli_Version(t *testing.T) {
	g := NewWithT(t)

	t.Run("should execute buildah version correctly", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeFunc = func(cmd cliwrappers.Cmd) (string, string, int, error) {
			g.Expect(cmd.Name).To(Equal("buildah"))
			capturedArgs = cmd.Args
			jsonOutput := `{
    "version": "1.42.2",
    "goVersion": "go1.24.10",
    "imageSpec": "1.1.1",
    "runtimeSpec": "1.2.1",
    "cniSpec": "1.1.0",
    "libcniVersion": "",
    "imageVersion": "5.38.0",
    "gitCommit": "",
    "built": "Wed Dec  3 15:03:30 2025",
    "osArch": "linux/amd64",
    "buildPlatform": "linux/amd64"
}`
			return jsonOutput, "", 0, nil
		}

		versionInfo, err := buildahCli.Version()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(Equal([]string{"version", "--json"}))

		g.Expect(versionInfo.Version).To(Equal("1.42.2"))
	})
}

func TestBuildahBuildArgs_MakePathsAbsolute(t *testing.T) {
	g := NewWithT(t)

	t.Run("should not modify absolute paths", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "/absolute/path/Containerfile",
			ContextDir:    "/absolute/path/context",
			Secrets: []cliwrappers.BuildahSecret{
				{Src: "/absolute/path/secret", Id: "secret1"},
			},
			Volumes: []cliwrappers.BuildahVolume{
				{HostDir: "/absolute/path/volume", ContainerDir: "/container/dir", Options: ""},
			},
			BuildContexts: []cliwrappers.BuildahBuildContext{
				{Name: "additional-context", Location: "/absolute/additional-context"},
			},
			BuildArgsFile: "/absolute/path/build-args-file",
		}

		err := args.MakePathsAbsolute("/base/dir")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(args.Containerfile).To(Equal("/absolute/path/Containerfile"))
		g.Expect(args.ContextDir).To(Equal("/absolute/path/context"))
		g.Expect(args.Secrets[0].Src).To(Equal("/absolute/path/secret"))
		g.Expect(args.Volumes[0].HostDir).To(Equal("/absolute/path/volume"))
		g.Expect(args.BuildContexts[0].Location).To(Equal("/absolute/additional-context"))
		g.Expect(args.BuildArgsFile).To(Equal("/absolute/path/build-args-file"))
	})

	t.Run("should make relative paths absolute", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "relative/Containerfile",
			ContextDir:    ".",
			Secrets: []cliwrappers.BuildahSecret{
				{Src: "relative/secret", Id: "secret1"},
			},
			Volumes: []cliwrappers.BuildahVolume{
				{HostDir: "relative/volume", ContainerDir: "/container/dir", Options: ""},
			},
			BuildContexts: []cliwrappers.BuildahBuildContext{
				{Name: "additional-context", Location: "relative/additional-context"},
			},
			BuildArgsFile: "relative/build-args-file",
		}

		err := args.MakePathsAbsolute("/base/dir")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(args.Containerfile).To(Equal("/base/dir/relative/Containerfile"))
		g.Expect(args.ContextDir).To(Equal("/base/dir"))
		g.Expect(args.Secrets[0].Src).To(Equal("/base/dir/relative/secret"))
		g.Expect(args.Volumes[0].HostDir).To(Equal("/base/dir/relative/volume"))
		g.Expect(args.BuildContexts[0].Location).To(Equal("/base/dir/relative/additional-context"))
		g.Expect(args.BuildArgsFile).To(Equal("/base/dir/relative/build-args-file"))
	})

	t.Run("should handle a mix of relative and absolute paths", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "/path/to/Containerfile",
			ContextDir:    "context",
			Secrets: []cliwrappers.BuildahSecret{
				{Src: "secret1/file", Id: "secret1"},
				{Src: "/absolute/secret2/file", Id: "secret2"},
			},
			Volumes: []cliwrappers.BuildahVolume{
				{HostDir: "volume1/dir", ContainerDir: "/container/dir1", Options: ""},
				{HostDir: "/absolute/volume2/dir", ContainerDir: "/container/dir2", Options: "ro"},
			},
			BuildContexts: []cliwrappers.BuildahBuildContext{
				{Name: "additional-context", Location: "/absolute/additional-context"},
				{Name: "additional-context", Location: "relative/additional-context"},
			},
		}

		err := args.MakePathsAbsolute("/base/dir")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(args.Containerfile).To(Equal("/path/to/Containerfile"))
		g.Expect(args.ContextDir).To(Equal("/base/dir/context"))
		g.Expect(args.Secrets[0].Src).To(Equal("/base/dir/secret1/file"))
		g.Expect(args.Secrets[1].Src).To(Equal("/absolute/secret2/file"))
		g.Expect(args.Volumes[0].HostDir).To(Equal("/base/dir/volume1/dir"))
		g.Expect(args.Volumes[1].HostDir).To(Equal("/absolute/volume2/dir"))
		g.Expect(args.BuildContexts[0].Location).To(Equal("/absolute/additional-context"))
		g.Expect(args.BuildContexts[1].Location).To(Equal("/base/dir/relative/additional-context"))
	})

	t.Run("should use current working directory when baseDir is relative", func(t *testing.T) {
		cwd, err := os.Getwd()
		g.Expect(err).ToNot(HaveOccurred())

		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "Containerfile",
			ContextDir:    "context",
		}

		err = args.MakePathsAbsolute(".")
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(args.Containerfile).To(Equal(filepath.Join(cwd, "Containerfile")))
		g.Expect(args.ContextDir).To(Equal(filepath.Join(cwd, "context")))
	})

	t.Run("should not modify empty BuildArgsFile", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "/absolute/path/Containerfile",
			ContextDir:    "/absolute/path/context",
			BuildArgsFile: "",
		}

		err := args.MakePathsAbsolute("/base/dir")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(args.BuildArgsFile).To(Equal(""))
	})
}

func TestBuildahBuildArgs_Validate(t *testing.T) {
	g := NewWithT(t)

	const containerfile = "/path/to/Containerfile"
	const contextDir = "/path/to/context"
	const outputRef = "quay.io/org/image:tag"

	t.Run("should succeed with valid args", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}

		err := args.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should error if containerfile is empty", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: "",
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}

		err := args.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("containerfile path is empty"))
	})

	t.Run("should error if context directory is empty", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    "",
			OutputRef:     outputRef,
		}

		err := args.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("context directory is empty"))
	})

	t.Run("should error if output-ref is empty", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     "",
		}

		err := args.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("output-ref is empty"))
	})

	t.Run("should error when volume HostDir contains ':'", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			Volumes:       []cliwrappers.BuildahVolume{{HostDir: "some:dir", ContainerDir: "/foo"}},
		}

		err := args.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("':' in volume mount source path: some:dir"))
	})

	t.Run("should error when volume ContainerDir contains ':'", func(t *testing.T) {
		args := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     outputRef,
			Volumes:       []cliwrappers.BuildahVolume{{HostDir: "/foo", ContainerDir: "other:dir"}},
		}

		err := args.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("':' in volume mount target path: other:dir"))
	})
}
