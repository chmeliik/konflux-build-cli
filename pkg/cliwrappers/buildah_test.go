package cliwrappers_test

import (
	"errors"
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func setupBuildahCli() (*cliwrappers.BuildahCli, *mockExecutor) {
	executor := &mockExecutor{}
	buildahCli := &cliwrappers.BuildahCli{Executor: executor}
	return buildahCli, executor
}

func TestBuildahCli_Build(t *testing.T) {
	g := NewWithT(t)

	const containerfile = "/path/to/Containerfile"
	const contextDir = "/path/to/context"
	const outputRef = "quay.io/org/image:tag"

	t.Run("should execute buildah correctly", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("buildah"))
			capturedArgs = args
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
		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
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

	t.Run("should error if containerfile is empty", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: "",
			ContextDir:    contextDir,
			OutputRef:     outputRef,
		}
		err := buildahCli.Build(buildArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("containerfile path is empty"))
	})

	t.Run("should error if context directory is empty", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    "",
			OutputRef:     outputRef,
		}
		err := buildahCli.Build(buildArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("context directory is empty"))
	})

	t.Run("should error if output-ref is empty", func(t *testing.T) {
		buildahCli, _ := setupBuildahCli()
		buildArgs := &cliwrappers.BuildahBuildArgs{
			Containerfile: containerfile,
			ContextDir:    contextDir,
			OutputRef:     "",
		}
		err := buildahCli.Build(buildArgs)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("output-ref is empty"))
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

	mockSuccessfulPush := func(captureArgs *[]string) func(command string, args ...string) (string, string, int, error) {
		return func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("buildah"))
			*captureArgs = args

			digestFile := findDigestFile(args)
			g.Expect(digestFile).ToNot(BeEmpty())

			os.WriteFile(digestFile, []byte(digest), 0644)

			return "", "", 0, nil
		}
	}

	t.Run("should push image", func(t *testing.T) {
		buildahCli, executor := setupBuildahCli()
		var capturedArgs []string
		executor.executeWithOutput = mockSuccessfulPush(&capturedArgs)

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
		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
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
		executor.executeWithOutput = mockSuccessfulPush(&capturedArgs)

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
		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			digestFile := findDigestFile(args)
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
		executor.executeWithOutput = mockSuccessfulPush(&capturedArgs)

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
