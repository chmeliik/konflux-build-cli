package cliwrappers_test

import (
	"errors"
	"slices"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

// expectArgAndValue ensures that the given args contain pair of --argName argValue
func expectArgAndValue(g *WithT, args []string, argName string, argValue string) {
	index := slices.Index(args, argName)
	g.Expect(index).ToNot(Equal(-1))
	g.Expect(index).To(BeNumerically("<", len(args)-2))
	g.Expect(args[index+1]).To(Equal(argValue))
}

func setupSkopeoCli() (*cliwrappers.SkopeoCli, *mockExecutor) {
	executor := &mockExecutor{}
	skopeoCli := &cliwrappers.SkopeoCli{Executor: executor}
	return skopeoCli, executor
}

func TestSkopeoCli_Copy(t *testing.T) {
	g := NewWithT(t)

	const sourceImage = "quay.io/org/namespace/base-image@sha256:4d6addf62a90e392ff6d3f470259eb5667eab5b9a8e03d20b41d0ab910f92170"
	const destinationImage = "registry.io:1234/namespace/target-image:tag"
	const multiArch = cliwrappers.SkopeoCopyArgMultiArchIndexOnly
	const retryTimes = 5

	t.Run("should copy tag with no options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return "", "", 0, nil
		}

		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      sourceImage,
			DestinationImage: destinationImage,
		}

		err := skopeoCli.Copy(copyArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(3))
		g.Expect(capturedArgs[0]).To(Equal("copy"))
		g.Expect(capturedArgs[1]).To(Equal("docker://" + sourceImage))
		g.Expect(capturedArgs[2]).To(Equal("docker://" + destinationImage))
	})

	t.Run("should copy tag with all supported options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return "", "", 0, nil
		}

		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      sourceImage,
			DestinationImage: destinationImage,
			MultiArch:        multiArch,
			RetryTimes:       retryTimes,
		}

		err := skopeoCli.Copy(copyArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(7))
		g.Expect(capturedArgs[0]).To(Equal("copy"))
		g.Expect(capturedArgs[len(capturedArgs)-2]).To(Equal("docker://" + sourceImage))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal("docker://" + destinationImage))
		expectArgAndValue(g, capturedArgs, "--multi-arch", string(multiArch))
		expectArgAndValue(g, capturedArgs, "--retry-times", strconv.Itoa(retryTimes))
	})

	t.Run("should copy tag with extra options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return "", "", 0, nil
		}

		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      sourceImage,
			DestinationImage: destinationImage,
			MultiArch:        multiArch,
			RetryTimes:       retryTimes,
			ExtraArgs:        []string{"--some-arg", "somevalue", "--someflag"},
		}

		err := skopeoCli.Copy(copyArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(10))
		g.Expect(capturedArgs[0]).To(Equal("copy"))
		g.Expect(capturedArgs[len(capturedArgs)-2]).To(Equal("docker://" + sourceImage))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal("docker://" + destinationImage))
		expectArgAndValue(g, capturedArgs, "--multi-arch", string(multiArch))
		expectArgAndValue(g, capturedArgs, "--retry-times", strconv.Itoa(retryTimes))
		expectArgAndValue(g, capturedArgs, "--some-arg", "somevalue")
		g.Expect(capturedArgs).To(ContainElement("--someflag"))
	})

	t.Run("should error if skopeo execution fails", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		isExecuteCalled := false
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			isExecuteCalled = true
			return "", "", 0, errors.New("failed to execute skopeo copy")
		}

		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      "base",
			DestinationImage: "target",
		}

		err := skopeoCli.Copy(copyArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(isExecuteCalled).To(BeTrue())
	})

	t.Run("should error if base image is empty", func(t *testing.T) {
		skopeoCli, _ := setupSkopeoCli()
		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      "",
			DestinationImage: "target",
		}
		err := skopeoCli.Copy(copyArgs)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("should error if target image is empty", func(t *testing.T) {
		skopeoCli, _ := setupSkopeoCli()
		copyArgs := &cliwrappers.SkopeoCopyArgs{
			SourceImage:      "base",
			DestinationImage: "",
		}
		err := skopeoCli.Copy(copyArgs)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestSkopeoCli_Inspect(t *testing.T) {
	g := NewWithT(t)

	const imageRef = "quay.io/org/namespace/base-image:tag"
	const retryTimes = 4
	const raw = true
	const noTags = true
	const format = "format"
	const output = `skopeo inspect output json`

	t.Run("should inspect image with no options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return output, "", 0, nil
		}

		inspectArgs := &cliwrappers.SkopeoInspectArgs{
			ImageRef: imageRef,
		}

		stdout, err := skopeoCli.Inspect(inspectArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(2))
		g.Expect(capturedArgs[0]).To(Equal("inspect"))
		g.Expect(capturedArgs[1]).To(Equal("docker://" + imageRef))
		g.Expect(stdout).To(Equal(output))
	})

	t.Run("should inspect image with all supported options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return output, "", 0, nil
		}

		inspectArgs := &cliwrappers.SkopeoInspectArgs{
			ImageRef:   imageRef,
			RetryTimes: retryTimes,
			Raw:        raw,
			NoTags:     noTags,
			Format:     format,
		}

		stdout, err := skopeoCli.Inspect(inspectArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(8))
		g.Expect(capturedArgs[0]).To(Equal("inspect"))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal("docker://" + imageRef))
		expectArgAndValue(g, capturedArgs, "--retry-times", strconv.Itoa(retryTimes))
		expectArgAndValue(g, capturedArgs, "--format", format)
		g.Expect(capturedArgs).To(ContainElement("--raw"))
		g.Expect(capturedArgs).To(ContainElement("--no-tags"))
		g.Expect(stdout).To(Equal(output))
	})

	t.Run("should inspect image with with extra options", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		var capturedArgs []string
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).To(Equal("skopeo"))
			capturedArgs = args
			return output, "", 0, nil
		}

		inspectArgs := &cliwrappers.SkopeoInspectArgs{
			ImageRef:   imageRef,
			RetryTimes: retryTimes,
			Raw:        raw,
			NoTags:     noTags,
			Format:     format,
			ExtraArgs:  []string{"--some-arg", "somevalue", "--someflag"},
		}

		stdout, err := skopeoCli.Inspect(inspectArgs)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(capturedArgs).To(HaveLen(11))
		g.Expect(capturedArgs[0]).To(Equal("inspect"))
		g.Expect(capturedArgs[len(capturedArgs)-1]).To(Equal("docker://" + imageRef))
		expectArgAndValue(g, capturedArgs, "--retry-times", strconv.Itoa(retryTimes))
		expectArgAndValue(g, capturedArgs, "--format", format)
		g.Expect(capturedArgs).To(ContainElement("--raw"))
		g.Expect(capturedArgs).To(ContainElement("--no-tags"))
		expectArgAndValue(g, capturedArgs, "--some-arg", "somevalue")
		g.Expect(capturedArgs).To(ContainElement("--someflag"))
		g.Expect(stdout).To(Equal(output))
	})

	t.Run("should error if skopeo execution fails", func(t *testing.T) {
		skopeoCli, executor := setupSkopeoCli()
		isExecuteCalled := false
		executor.executeFunc = func(command string, args ...string) (string, string, int, error) {
			isExecuteCalled = true
			return "", "", 0, errors.New("failed to execute skopeo inspect")
		}

		inspectArgs := &cliwrappers.SkopeoInspectArgs{
			ImageRef: imageRef,
		}

		_, err := skopeoCli.Inspect(inspectArgs)

		g.Expect(err).To(HaveOccurred())
		g.Expect(isExecuteCalled).To(BeTrue())
	})

	t.Run("should error if image reference is empty", func(t *testing.T) {
		skopeoCli, _ := setupSkopeoCli()
		inspectArgs := &cliwrappers.SkopeoInspectArgs{
			ImageRef: "",
		}
		_, err := skopeoCli.Inspect(inspectArgs)
		g.Expect(err).To(HaveOccurred())
	})
}
