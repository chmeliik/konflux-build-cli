package integration_tests

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"path"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/konflux-ci/konflux-build-cli/integration_tests/framework"
)

const (
	// The certificate extension the CT log's SCT is embedded in.
	sctExtensionOID = "1.3.6.1.4.1.11129.2.4.2"
	// The Fulcio certificate extension carrying the OIDC issuer.
	fulcioIssuerExtensionOID = "1.3.6.1.4.1.57264.1.8"
)

func TestKeylessSigning(t *testing.T) {
	RequirePodman(t)
	SetupGomega(t)

	stack := NewSigstoreStack()
	Expect(stack.Start()).To(Succeed())
	defer stack.Stop()

	imageRegistry := NewImageRegistry()
	Expect(imageRegistry.Prepare()).To(Succeed())
	Expect(imageRegistry.Start()).To(Succeed())
	defer imageRegistry.Stop()

	const imageRepoName = "sign-test"
	imageRepoUrl := imageRegistry.GetTestNamespace() + imageRepoName
	Expect(CreateTestImage(TestImageConfig{
		ImageRef:       imageRepoUrl,
		RandomDataSize: 1024,
	})).To(Succeed())
	defer DeleteLocalImage(imageRepoUrl)

	imageDigest, err := PushImage(imageRepoUrl)
	Expect(err).ToNot(HaveOccurred())
	imageRef := imageRepoUrl + "@" + imageDigest

	token, err := stack.GetOIDCToken()
	Expect(err).ToNot(HaveOccurred())

	container := NewBuildCliRunnerContainer("cosign-test", TaskRunnerImageRef,
		WithEnv("TUF_URL", stack.TufURL()),
		WithEnv("REKOR_URL", stack.RekorURL()),
		WithEnv("SIGSTORE_FULCIO_URL", stack.FulcioURL()),
		WithEnv("SIGSTORE_OIDC_ISSUER", stack.OIDCIssuerURL()),
		WithEnv("SIGSTORE_ID_TOKEN", token),
	)
	defer container.DeleteIfExists()
	Expect(container.StartWithRegistryIntegration(imageRegistry)).To(Succeed())

	t.Run("Initialize", func(t *testing.T) {
		SetupGomega(t)

		stdout, stderr, err := runScript(container, `
cosign initialize --root "${TUF_URL}/root.json" --mirror "${TUF_URL}"
`)
		Expect(err).ToNot(HaveOccurred(), stderr)
		Expect(stdout + stderr).ToNot(ContainSubstring("Error:"))

		homeDir, err := container.GetHomeDir()
		Expect(err).ToNot(HaveOccurred())

		// The local mirror, not tuf-repo-cdn.sigstore.dev.
		remote, err := container.GetFileContent(path.Join(homeDir, ".sigstore/root/remote.json"))
		Expect(err).ToNot(HaveOccurred())
		Expect(remote).To(ContainSubstring(stack.TufURL()))

		// Proves cosign took the TUF path rather than the legacy per-target one.
		cached, _, err := container.ExecuteCommandWithOutput(
			"find", path.Join(homeDir, ".sigstore"), "-name", "*trusted_root.json")
		Expect(err).ToNot(HaveOccurred())
		Expect(cached).ToNot(BeEmpty(), "cosign cached no trusted_root.json")
	})

	t.Run("Sign", func(t *testing.T) {
		SetupGomega(t)

		stdout, stderr, err := runScript(container, `
cosign sign -y \
  --use-signing-config=false \
  --rekor-url="${REKOR_URL}" \
  --fulcio-url="${SIGSTORE_FULCIO_URL}" \
  --oidc-issuer="${SIGSTORE_OIDC_ISSUER}" \
  "`+imageRef+`"
`)
		Expect(err).ToNot(HaveOccurred(), stderr)
		Expect(stdout + stderr).ToNot(ContainSubstring("Error:"))
		Expect(stderr).To(ContainSubstring("Pushing signature to: " + imageRepoUrl))
	})

	t.Run("SignatureInRegistry", func(t *testing.T) {
		SetupGomega(t)

		referrersUrl := fmt.Sprintf("https://%s/v2/%s/referrers/%s",
			imageRegistry.GetRegistryDomain(), imageRepoName, imageDigest)
		request, err := http.NewRequest(http.MethodGet, referrersUrl, nil)
		Expect(err).ToNot(HaveOccurred())

		response, err := imageRegistry.DoRequest(request)
		Expect(err).ToNot(HaveOccurred())
		defer response.Body.Close()
		Expect(response.StatusCode).To(Equal(http.StatusOK))

		var index struct {
			Manifests []struct {
				ArtifactType string `json:"artifactType"`
			} `json:"manifests"`
		}
		Expect(json.NewDecoder(response.Body).Decode(&index)).To(Succeed())

		artifactTypes := []string{}
		for _, manifest := range index.Manifests {
			artifactTypes = append(artifactTypes, manifest.ArtifactType)
		}
		Expect(artifactTypes).To(ContainElement("application/vnd.dev.sigstore.bundle.v0.3+json"))
	})

	t.Run("RekorEntry", func(t *testing.T) {
		SetupGomega(t)

		// The stack is created from scratch for every run, so signing one image
		// leaves the transparency log with exactly one entry, at index 0.
		var log struct {
			TreeSize int `json:"treeSize"`
		}
		Expect(getJSON(stack.RekorURL()+"/api/v1/log", &log)).To(Succeed())
		Expect(log.TreeSize).To(Equal(1))

		entry := getRekorEntry(stack.RekorURL(), 0)
		Expect(entry.Kind).To(Equal("dsse"))
		Expect(entry.Spec.Signatures).To(HaveLen(1))
	})

	t.Run("SigningCertificate", func(t *testing.T) {
		SetupGomega(t)

		entry := getRekorEntry(stack.RekorURL(), 0)
		verifier, err := base64.StdEncoding.DecodeString(entry.Spec.Signatures[0].Verifier)
		Expect(err).ToNot(HaveOccurred())

		block, _ := pem.Decode(verifier)
		Expect(block).ToNot(BeNil(), "the logged verifier is not PEM:\n"+string(verifier))
		certificate, err := x509.ParseCertificate(block.Bytes)
		Expect(err).ToNot(HaveOccurred())

		Expect(certificate.EmailAddresses).To(ContainElement(SigstoreMockIdentity))
		Expect(certificate.Issuer.CommonName).To(Equal("sigstore-test-ca"))

		extensions := map[string]string{}
		for _, extension := range certificate.Extensions {
			extensions[extension.Id.String()] = string(extension.Value)
		}
		// The SCT proves Fulcio submitted the certificate to the CT log.
		Expect(extensions).To(HaveKey(sctExtensionOID))
		Expect(extensions).To(HaveKeyWithValue(
			fulcioIssuerExtensionOID, ContainSubstring(stack.OIDCIssuerURL())))
	})

	t.Run("Verify", func(t *testing.T) {
		SetupGomega(t)

		stdout, stderr, err := runScript(container, `
cosign verify \
  --rekor-url="${REKOR_URL}" \
  --certificate-identity=`+SigstoreMockIdentity+` \
  --certificate-oidc-issuer="${SIGSTORE_OIDC_ISSUER}" \
  "`+imageRef+`"
`)
		Expect(err).ToNot(HaveOccurred(), stderr)

		var payloads []struct {
			Critical struct {
				Image struct {
					Digest string `json:"docker-manifest-digest"`
				} `json:"image"`
			} `json:"critical"`
		}
		Expect(json.Unmarshal([]byte(stdout), &payloads)).To(Succeed(), stdout)
		Expect(payloads).To(HaveLen(1))
		Expect(payloads[0].Critical.Image.Digest).To(Equal(imageDigest))
	})

	t.Run("VerifyRejectsWrongIssuer", func(t *testing.T) {
		SetupGomega(t)

		_, stderr, err := runScript(container, `
cosign verify \
  --rekor-url="${REKOR_URL}" \
  --certificate-identity=`+SigstoreMockIdentity+` \
  --certificate-oidc-issuer=https://example.com/not-the-issuer \
  "`+imageRef+`"
`)
		Expect(err).To(HaveOccurred())
		Expect(stderr).To(ContainSubstring("failed to verify certificate identity"))
		Expect(stderr).To(ContainSubstring(
			`expected issuer value "https://example.com/not-the-issuer", got "` + stack.OIDCIssuerURL() + `"`))
	})
}

func runScript(container *TestRunnerContainer, script string) (string, string, error) {
	return container.ExecuteCommandWithOutput("bash", "-c", script)
}

type rekorEntryBody struct {
	Kind string `json:"kind"`
	Spec struct {
		Signatures []struct {
			Verifier string `json:"verifier"`
		} `json:"signatures"`
	} `json:"spec"`
}

// getRekorEntry fetches one transparency log entry and decodes its body, which
// Rekor returns base64-encoded.
func getRekorEntry(rekorURL string, logIndex int) rekorEntryBody {
	var entries map[string]struct {
		Body string `json:"body"`
	}
	Expect(getJSON(fmt.Sprintf("%s/api/v1/log/entries?logIndex=%d", rekorURL, logIndex), &entries)).To(Succeed())
	Expect(entries).To(HaveLen(1))

	var body rekorEntryBody
	for _, entry := range entries {
		decoded, err := base64.StdEncoding.DecodeString(entry.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(json.Unmarshal(decoded, &body)).To(Succeed(), string(decoded))
	}
	return body
}

func getJSON(url string, target any) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("GET %s returned %d: %s", url, response.StatusCode, body)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
