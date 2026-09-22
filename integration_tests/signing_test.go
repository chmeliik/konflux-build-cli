package integration_tests

import (
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

func TestSigstoreStack(t *testing.T) {
	SetupGomega(t)

	stack := NewSigstoreStack()
	defer stack.Stop()
	err := stack.Start()
	Expect(err).ToNot(HaveOccurred())

	imageRegistry := NewImageRegistry()
	err = imageRegistry.Prepare()
	Expect(err).ToNot(HaveOccurred())
	err = imageRegistry.Start()
	Expect(err).ToNot(HaveOccurred())
	defer imageRegistry.Stop()

	imageRepoURL := imageRegistry.GetTestNamespace() + "sign-test"

	err = CreateTestImage(TestImageConfig{
		ImageRef:       imageRepoURL,
		RandomDataSize: 1024,
	})
	Expect(err).ToNot(HaveOccurred())

	imageDigest, err := PushImage(imageRepoURL)
	Expect(err).ToNot(HaveOccurred())
	Expect(imageDigest).ToNot(BeEmpty())

	imageRef := imageRepoURL + "@" + imageDigest

	container := NewBuildCliRunnerContainer("cosign-test", TaskRunnerImageRef, stack.ContainerOptions()...)
	defer container.DeleteIfExists()
	err = container.StartWithRegistryIntegration(imageRegistry)
	Expect(err).ToNot(HaveOccurred())

	t.Run("CosignAvailable", func(t *testing.T) {
		SetupGomega(t)

		stdout, _, err := container.ExecuteCommandWithOutput("cosign", "version")
		Expect(err).ToNot(HaveOccurred())
		Expect(stdout).To(ContainSubstring("cosign"))
	})

	t.Run("KeylessSign", func(t *testing.T) {
		SetupGomega(t)

		token, err := stack.GetOIDCToken()
		Expect(err).ToNot(HaveOccurred())
		Expect(token).ToNot(BeEmpty())

		err = container.CreateFileInContainer("/tmp/oidc-token", token)
		Expect(err).ToNot(HaveOccurred())

		_, _, err = container.ExecuteCommandWithOutput(
			"sh", "-c",
			"SIGSTORE_ID_TOKEN=$(cat /tmp/oidc-token) "+
				"cosign sign -y"+
				" --signing-config="+SigstoreSigningConfigInContainer+
				" --trusted-root="+SigstoreTrustedRootInContainer+
				" --allow-insecure-registry"+
				" "+imageRef,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("KeylessVerify", func(t *testing.T) {
		SetupGomega(t)

		_, _, err := container.ExecuteCommandWithOutput(
			"cosign", "verify",
			"--trusted-root="+SigstoreTrustedRootInContainer,
			"--certificate-identity-regexp=.*",
			"--certificate-oidc-issuer="+stack.OIDCIssuerURL(),
			// The test Fulcio has no certificate transparency log, so the
			// certificates it issues carry no SCT.
			"--insecure-ignore-sct",
			"--allow-insecure-registry",
			imageRef,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("AttachSBOM", func(t *testing.T) {
		SetupGomega(t)

		sbomContent := `{"spdxVersion": "SPDX-2.3", "dataLicense": "CC0-1.0", "SPDXID": "SPDXRef-DOCUMENT"}`
		err := container.CreateFileInContainer("/tmp/test.spdx.json", sbomContent)
		Expect(err).ToNot(HaveOccurred())

		_, _, err = container.ExecuteCommandWithOutput(
			"cosign", "attach", "sbom",
			"--sbom=/tmp/test.spdx.json",
			"--type=spdx",
			"--allow-insecure-registry",
			imageRef,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("AttestSBOM", func(t *testing.T) {
		SetupGomega(t)

		token, err := stack.GetOIDCToken()
		Expect(err).ToNot(HaveOccurred())

		err = container.CreateFileInContainer("/tmp/oidc-token", token)
		Expect(err).ToNot(HaveOccurred())

		_, _, err = container.ExecuteCommandWithOutput(
			"sh", "-c",
			"SIGSTORE_ID_TOKEN=$(cat /tmp/oidc-token) "+
				"cosign attest -y"+
				" --signing-config="+SigstoreSigningConfigInContainer+
				" --trusted-root="+SigstoreTrustedRootInContainer+
				" --type=spdxjson"+
				" --predicate=/tmp/test.spdx.json"+
				" --allow-insecure-registry"+
				" "+imageRef,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	t.Run("VerifyAttestation", func(t *testing.T) {
		SetupGomega(t)

		_, _, err := container.ExecuteCommandWithOutput(
			"cosign", "verify-attestation",
			"--trusted-root="+SigstoreTrustedRootInContainer,
			"--type=spdxjson",
			"--certificate-identity-regexp=.*",
			"--certificate-oidc-issuer="+stack.OIDCIssuerURL(),
			"--insecure-ignore-sct",
			"--allow-insecure-registry",
			imageRef,
		)
		Expect(err).ToNot(HaveOccurred())
	})
}
