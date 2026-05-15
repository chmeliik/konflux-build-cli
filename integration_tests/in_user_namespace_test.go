//go:build linux

package integration_tests

import (
	"os/exec"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

func TestInUserNamespace(t *testing.T) {
	SetupGomega(t)

	t.Run("loopback up allows ping to localhost", func(t *testing.T) {
		SetupGomega(t)

		cmd := exec.Command(
			"unshare", "--map-root-user", "--net", "--",
			GetCliBinPath(), "internal", "in-user-namespace", "--loopback-up", "--",
			"ping", "-c1", "127.0.0.1",
		)
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "output: %s", output)
	})

	t.Run("without loopback up ping to localhost fails", func(t *testing.T) {
		SetupGomega(t)

		cmd := exec.Command(
			"unshare", "--map-root-user", "--net", "--",
			GetCliBinPath(), "internal", "in-user-namespace", "--",
			"ping", "-c1", "-W1", "127.0.0.1",
		)
		output, err := cmd.CombinedOutput()
		Expect(err).To(HaveOccurred(), "expected ping to fail, output: %s", output)
	})

	t.Run("disable RHSM host integration", func(t *testing.T) {
		SetupGomega(t)

		// Has the /usr/share/rhel/secrets directory
		testImage := "quay.io/konflux-ci/task-runner:1.7.0@sha256:1c0582a85dae0949f3ec94aca95695c5d7698b371f1b76c5a78822a6249073dc"

		t.Run("disabling works", func(t *testing.T) {
			SetupGomega(t)

			container := NewBuildCliRunnerContainer("kbc-in-user-namespace", testImage)
			Expect(container.Start()).To(Succeed())

			checkContainerHasRhelSecrets := func() {
				stdout, _, err := container.ExecuteCommandWithOutput("ls", "/usr/share/rhel/secrets")
				Expect(err).ToNot(HaveOccurred())
				lines := strings.Split(strings.TrimSpace(stdout), "\n")
				Expect(lines).To(ContainElements("etc-pki-entitlement", "redhat.repo", "rhsm"))
			}

			// Check that the test container really does have /usr/share/rhel/secrets and it's not empty
			checkContainerHasRhelSecrets()

			// Check that /usr/share/rhel/secrets is empty inside an
			// 'in-user-namespace --disable-rhsm-host-integration' process
			stdout, _, err := container.ExecuteCommandWithOutput(
				"unshare", "--map-root-user", "--mount", "--",
				KonfluxBuildCli, "internal", "in-user-namespace", "--disable-rhsm-host-integration", "--",
				"ls", "-l", "/usr/share/rhel/secrets",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(stdout)).To(Equal("total 0"))

			// Check that --disable-rhsm-host-integration didn't modify the container FS
			checkContainerHasRhelSecrets()
		})

		t.Run("disabling does nothing if already disabled", func(t *testing.T) {
			SetupGomega(t)

			container := NewBuildCliRunnerContainer("kbc-in-user-namespace", testImage)
			// Run as root so that we can delete /usr/share/rhel/secrets
			container.SetUser("root")
			Expect(container.Start()).To(Succeed())

			Expect(container.ExecuteCommand("rm", "-r", "/usr/share/rhel/secrets")).To(Succeed())

			stdout, _, err := container.ExecuteCommandWithOutput(
				"unshare", "--map-root-user", "--mount", "--",
				KonfluxBuildCli, "internal", "in-user-namespace", "--disable-rhsm-host-integration", "--",
				"bash", "-c", "[[ ! -e /usr/share/rhel/secrets ]] && echo does not exist",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(strings.TrimSpace(stdout)).To(Equal("does not exist"))
		})
	})

}
