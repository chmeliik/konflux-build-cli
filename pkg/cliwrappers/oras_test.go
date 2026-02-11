package cliwrappers_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
)

func setupOrasCli() (*cliwrappers.OrasCli, *mockExecutor) {
	executor := &mockExecutor{}
	orasCli := &cliwrappers.OrasCli{Executor: executor}
	cliwrappers.DisableRetryer = true
	return orasCli, executor
}

func TestOrasCli_Push(t *testing.T) {
	g := NewWithT(t)

	const artifactImage = "reg.io/org/app:sha256-1234567.containerfile"
	const imageDigest = "sha256:4d6addf62a90e392ff6d3f470259eb5667eab5b9a8e03d20b41d0ab910f92170"
	const fileName = "Containerfile"

	t.Run("successful push with minimum arguments", func(t *testing.T) {
		orasCli, executor := setupOrasCli()

		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).Should(Equal("oras"))
			g.Expect(args).Should(Equal([]string{"push", artifactImage, fileName}))

			stdout := "Digest: " + imageDigest
			return stdout, "push progress", 0, nil
		}

		pushArgs := &cliwrappers.OrasPushArgs{
			DestinationImage: artifactImage,
			FileName:         fileName,
		}

		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(stdout).Should(Equal("Digest: " + imageDigest))
		g.Expect(stderr).Should(Equal("push progress"))
	})

	t.Run("push with authentication", func(t *testing.T) {
		orasCli, executor := setupOrasCli()

		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).Should(Equal("oras"))
			expectedArgs := []string{"push", "--registry-config", "/path/to/registry-config", artifactImage, fileName}
			g.Expect(args).Should(Equal(expectedArgs))

			stdout := "Digest: " + imageDigest
			return stdout, "push progress", 0, nil
		}

		pushArgs := &cliwrappers.OrasPushArgs{
			DestinationImage: artifactImage,
			FileName:         fileName,
			RegistryConfig:   "/path/to/registry-config",
		}

		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(stdout).Should(Equal("Digest: " + imageDigest))
		g.Expect(stderr).Should(Equal("push progress"))
	})

	t.Run("push with specific artifact type", func(t *testing.T) {
		orasCli, executor := setupOrasCli()

		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).Should(Equal("oras"))
			expectedArgs := []string{"push", "--artifact-type", "application/vnd.custom.artifact", artifactImage, fileName}
			g.Expect(args).Should(Equal(expectedArgs))

			stdout := "Digest: " + imageDigest
			return stdout, "push progress", 0, nil
		}

		pushArgs := &cliwrappers.OrasPushArgs{
			DestinationImage: artifactImage,
			FileName:         fileName,
			ArtifactType:     "application/vnd.custom.artifact",
		}

		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(stdout).Should(Equal("Digest: " + imageDigest))
		g.Expect(stderr).Should(Equal("push progress"))
	})

	t.Run("push and output artifact info by go-template", func(t *testing.T) {
		orasCli, executor := setupOrasCli()

		executor.executeWithOutput = func(command string, args ...string) (string, string, int, error) {
			g.Expect(command).Should(Equal("oras"))
			expectedArgs := []string{"push", "--format", "go-template", "--template", "{{.reference}}", artifactImage, fileName}
			g.Expect(args).Should(Equal(expectedArgs))
			return imageDigest, "push progress", 0, nil
		}

		pushArgs := &cliwrappers.OrasPushArgs{
			DestinationImage: artifactImage,
			FileName:         fileName,
			Format:           "go-template",
			Template:         "{{.reference}}",
		}

		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(stdout).Should(Equal(imageDigest))
		g.Expect(stderr).Should(Equal("push progress"))
	})

	t.Run("should return error when missing destination image", func(t *testing.T) {
		pushArgs := &cliwrappers.OrasPushArgs{
			FileName: fileName,
		}

		orasCli, _ := setupOrasCli()
		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).Should(HaveOccurred())
		g.Expect(err.Error()).Should(ContainSubstring("destination image arg is empty"))
		g.Expect(stdout).Should(Equal(""))
		g.Expect(stderr).Should(Equal(""))
	})

	t.Run("should return error when missing input file", func(t *testing.T) {
		pushArgs := &cliwrappers.OrasPushArgs{
			DestinationImage: artifactImage,
		}

		orasCli, _ := setupOrasCli()
		stdout, stderr, err := orasCli.Push(pushArgs)

		g.Expect(err).Should(HaveOccurred())
		g.Expect(err.Error()).Should(ContainSubstring("file name arg is empty"))
		g.Expect(stdout).Should(Equal(""))
		g.Expect(stderr).Should(Equal(""))
	})
}
