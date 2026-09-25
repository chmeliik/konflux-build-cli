package integration_tests_framework

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.yaml.in/yaml/v3"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

const (
	// Ports published to the host by sigstore-stack.yaml. Keep in sync with it.
	sigstoreRekorPort  = "3000"
	sigstoreDexPort    = "8888"
	sigstoreFulcioPort = "5555"
	sigstoreTufPort    = "8080"

	// The client redirect URL set in the Dex config in sigstore-stack.yaml.
	// Nothing listens there, tests script the OAuth2 flow and read the auth code themselves.
	sigstoreOIDCRedirectURI = "http://localhost:5556/callback"

	// The identity Dex's mockCallback connector returns for every authorization request.
	// https://github.com/dexidp/dex/blob/98e0af563213b163ccaed6bd1b8314e2fa22c641/connector/mock/connectortest.go#L15
	SigstoreMockIdentity = "kilgore@kilgore.trout"
)

// SigstoreStack runs Fulcio, Rekor, a TUF server, an OIDC provider and supporting services
// as a single podman pod, so tests can exercise the keyless signing flow without Kubernetes.
//
// Podman only: depends on the pod mechanism to deploy the whole stack in a common network
// while exposing only the necessary ports to the host.
type SigstoreStack struct {
	logger   *logrus.Entry
	executor cliWrappers.CliExecutorInterface

	// Directory holding the generated keys and other config.
	configDir string

	fulcioRootKeyPath  string
	fulcioRootCertPath string
	rekorKeyPath       string
	rekorPubKeyPath    string
	ctfeKeyPath        string
	ctfePubKeyPath     string
}

func NewSigstoreStack() *SigstoreStack {
	return &SigstoreStack{
		logger:   l.Logger.WithField("logger", "sigstore"),
		executor: cliWrappers.NewCliExecutor(),
	}
}

func (s *SigstoreStack) RekorURL() string  { return "http://localhost:" + sigstoreRekorPort }
func (s *SigstoreStack) FulcioURL() string { return "http://localhost:" + sigstoreFulcioPort }
func (s *SigstoreStack) TufURL() string    { return "http://localhost:" + sigstoreTufPort }

// OIDCIssuerURL is the Dex issuer URL configured in sigstore-stack.yaml.
// It matches the issuer URL configured for Fulcio and must be used as the
// --oidc-issuer value with cosign in order for keyless signing to work.
func (s *SigstoreStack) OIDCIssuerURL() string {
	return "http://localhost:" + sigstoreDexPort + "/auth"
}

func (s *SigstoreStack) Start() error {
	configDir, err := CreateTempDir("sigstore-stack-")
	if err != nil {
		return err
	}
	s.configDir = configDir

	if err := s.generateKeyMaterial(); err != nil {
		return fmt.Errorf("failed to generate key material: %w", err)
	}

	keysPath := filepath.Join(s.configDir, "sigstore-keys.yaml")
	if err := s.writeKeyConfigMaps(keysPath); err != nil {
		return fmt.Errorf("failed to write key ConfigMaps: %w", err)
	}

	manifestPath := filepath.Join(FindRepoRoot(), "integration_tests", "framework", "sigstore-stack.yaml")
	s.logger.Infof("Starting Sigstore stack from %s", manifestPath)
	cmd := cliWrappers.Command(
		"podman", "kube", "play", "--replace", "--configmap", keysPath, manifestPath,
	)
	cmd.LogOutput = true
	stdout, stderr, _, err := s.executor.Execute(cmd)
	if err != nil {
		s.logger.Errorf("failed to play the Sigstore pod:\n%s\n%s", stdout, stderr)
		return err
	}

	if err := s.waitReady(); err != nil {
		logs, logsErr := s.getTrimmedLogs()
		if logsErr != nil {
			s.logger.Errorf("Failed to collect Sigstore pod logs: %s", logsErr)
		} else {
			s.logger.Errorf("Sigstore pod logs:\n%s", logs)
		}
		return err
	}

	s.logger.Info("Sigstore stack is ready")
	return nil
}

func (s *SigstoreStack) Stop() {
	if stdout, stderr, _, err := s.executor.Execute(cliWrappers.Command(
		"podman", "pod", "rm", "--force", "--time=0", "sigstore",
	)); err != nil {
		s.logger.Error("failed to remove the Sigstore pod")
		s.logger.Errorf("[stdout]:\n%s", stdout)
		s.logger.Errorf("[stderr]:\n%s", stderr)
	}

	if s.configDir != "" {
		os.RemoveAll(s.configDir)
		s.configDir = ""
	}
}

// generateKeyMaterial creates the Fulcio CA, the Rekor signing key and the CTFE signing key,
// plus the public keys the TUF server publishes.
func (s *SigstoreStack) generateKeyMaterial() error {
	s.fulcioRootKeyPath = filepath.Join(s.configDir, "fulcio-root.key")
	s.fulcioRootCertPath = filepath.Join(s.configDir, "fulcio-root.pem")
	s.rekorKeyPath = filepath.Join(s.configDir, "rekor.key")
	s.rekorPubKeyPath = filepath.Join(s.configDir, "rekor.pub")
	s.ctfeKeyPath = filepath.Join(s.configDir, "ctfe.key")
	s.ctfePubKeyPath = filepath.Join(s.configDir, "ctfe.pub")

	for _, keyPath := range []string{s.fulcioRootKeyPath, s.rekorKeyPath, s.ctfeKeyPath} {
		// Generate all private keys as ECDSA P-256. Required for TesseraCT, works for the others.
		err := s.openssl(
			"ecparam", "-genkey", "-name", "prime256v1", "-noout", "-out", keyPath,
		)
		if err != nil {
			return fmt.Errorf("generating %s: %w", filepath.Base(keyPath), err)
		}
	}

	for _, key := range []struct{ priv, pub string }{
		{s.rekorKeyPath, s.rekorPubKeyPath},
		{s.ctfeKeyPath, s.ctfePubKeyPath},
	} {
		if err := s.openssl("ec", "-in", key.priv, "-pubout", "-out", key.pub); err != nil {
			return fmt.Errorf("generating %s: %w", filepath.Base(key.pub), err)
		}
	}

	err := s.openssl(
		"req", "-x509", "-new",
		"-key", s.fulcioRootKeyPath,
		"-out", s.fulcioRootCertPath,
		"-days", "3650",
		"-subj", "/CN=sigstore-test-ca",
		"-addext", "basicConstraints=critical,CA:TRUE",
		"-addext", "keyUsage=critical,keyCertSign",
	)
	if err != nil {
		return fmt.Errorf("generating %s: %w", s.fulcioRootCertPath, err)
	}

	return nil
}

func (s *SigstoreStack) openssl(args ...string) error {
	stdout, stderr, _, err := s.executor.Execute(cliWrappers.Command("openssl", args...))
	if err != nil {
		s.logger.Error("openssl failed")
		s.logger.Errorf("[stdout]:\n%s", stdout)
		s.logger.Errorf("[stderr]:\n%s", stderr)
		return err
	}
	return nil
}

// writeKeyConfigMaps renders the generated key material as ConfigMaps
// for `podman kube play --configmap`. The pod manifest references them by name.
func (s *SigstoreStack) writeKeyConfigMaps(path string) error {
	configMapSpecs := []struct {
		name  string
		files map[string]string // key in the ConfigMap => host path
	}{
		{"sigstore-fulcio-keys", map[string]string{
			"root.pem": s.fulcioRootCertPath,
			"root.key": s.fulcioRootKeyPath,
		}},
		{"sigstore-rekor-keys", map[string]string{
			"rekor.key": s.rekorKeyPath,
		}},
		{"sigstore-ctfe-keys", map[string]string{
			"private": s.ctfeKeyPath,
			"fulcio":  s.fulcioRootCertPath,
		}},
		{"sigstore-tuf-targets", map[string]string{
			"fulcio_v1.crt.pem": s.fulcioRootCertPath,
			"ctfe.pub":          s.ctfePubKeyPath,
			"rekor.pub":         s.rekorPubKeyPath,
		}},
	}

	var fileContent bytes.Buffer
	for _, cm := range configMapSpecs {
		data := map[string]string{}
		for key, hostPath := range cm.files {
			content, err := os.ReadFile(hostPath)
			if err != nil {
				return err
			}
			data[key] = string(content)
		}

		cmContent, err := yaml.Marshal(map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]string{"name": cm.name},
			"data":       data,
		})
		if err != nil {
			return fmt.Errorf("marshaling %s to yaml: %w", cm.name, err)
		}

		fileContent.WriteString("---\n")
		fileContent.Write(cmContent)
	}

	return os.WriteFile(path, fileContent.Bytes(), 0644)
}

// waitReady polls the host-facing services until all of them are ready.
func (s *SigstoreStack) waitReady() error {
	services := []struct {
		name string
		url  string
	}{
		{"TUF", s.TufURL() + "/root.json"},
		{"Dex", s.OIDCIssuerURL() + "/healthz"},
		{"Fulcio", s.FulcioURL() + "/healthz"},
		{"Rekor", s.RekorURL() + "/ping"},
	}

	const maxAttempts = 60

	waitForHealth := func(name, url string) bool {
		for i := range maxAttempts - 1 {
			if i > 0 {
				time.Sleep(time.Second)
			}

			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				} else {
					s.logger.Debugf("%s not yet ready: GET %s returned %d", name, url, resp.StatusCode)
				}
			} else {
				s.logger.Debugf("%s not yet ready: %s", name, err)
			}
		}
		return false
	}

	for _, svc := range services {
		if !waitForHealth(svc.name, svc.url) {
			return fmt.Errorf("%s still not ready after %d attempts", svc.name, maxAttempts)
		}
		s.logger.Infof("%s is ready", svc.name)
	}
	return nil
}

// getTrimmedLogs gets logs for the Sigstore pod.
// For Rekor and the two Trillian serivces, which crashloop until their dependencies are ready,
// limits the logs to the last two restarts.
func (s *SigstoreStack) getTrimmedLogs() (string, error) {
	cmd := exec.Command("podman", "pod", "logs", "--names", "sigstore")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Errorf("'podman pod logs' output:\n%s", output)
		return "", err
	}
	outputLines := slices.Collect(strings.Lines(string(output)))
	outputLines = trimRestarts(outputLines, "sigstore-rekor", "starting rekor-server")
	outputLines = trimRestarts(outputLines, "sigstore-trillian-server", "Log Server Starting")
	outputLines = trimRestarts(outputLines, "sigstore-trillian-signer", "Log Signer Starting")
	return strings.Join(outputLines, ""), nil
}

// trimRestarts drops all logs for serviceName before the second-to-last occurrence of startString.
func trimRestarts(logLines []string, serviceName string, startString string) []string {
	firstField := func(s string) string {
		for field := range strings.FieldsSeq(s) {
			return field
		}
		return ""
	}

	var lastMatchIndices []int
	for i, line := range slices.Backward(logLines) {
		if firstField(line) == serviceName && strings.Contains(line, startString) {
			lastMatchIndices = append(lastMatchIndices, i)
		}
		if len(lastMatchIndices) == 2 {
			break
		}
	}

	if len(lastMatchIndices) < 2 {
		return logLines
	}

	var filtered []string
	secondLastIndex := lastMatchIndices[len(lastMatchIndices)-1]
	for i, line := range logLines {
		if i >= secondLastIndex || firstField(line) != serviceName {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// GetOIDCToken obtains an OIDC token from Dex by scripting the OAuth2 authorization flow.
// Dex's mockCallback connector auto-approves without credentials.
// The identity represented by the token is always SigstoreMockIdentity.
func (s *SigstoreStack) GetOIDCToken() (string, error) {
	authURL := fmt.Sprintf(
		"http://localhost:%s/auth/auth?client_id=fulcio&response_type=code&redirect_uri=%s&scope=%s&nonce=test",
		sigstoreDexPort,
		url.QueryEscape(sigstoreOIDCRedirectURI),
		url.QueryEscape("openid email"),
	)

	code, err := s.followAuthRedirects(authURL)
	if err != nil {
		return "", err
	}

	tokenResp, err := http.PostForm(
		"http://localhost:"+sigstoreDexPort+"/auth/token",
		url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {code},
			"client_id":    {"fulcio"},
			"redirect_uri": {sigstoreOIDCRedirectURI},
		},
	)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(tokenResp.Body)
		return "", fmt.Errorf("token endpoint returned %d: %s", tokenResp.StatusCode, body)
	}

	var tokenData struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenData.IDToken == "" {
		return "", fmt.Errorf("no id_token in token response")
	}

	return tokenData.IDToken, nil
}

// followAuthRedirects walks the chain of redirects Dex issues for an authorization request
// and returns the authorization code extracted from the final redirect.
func (s *SigstoreStack) followAuthRedirects(authURL string) (string, error) {
	const maxRedirects = 10

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not follow redirects, we need to handle them ourselves.
			return http.ErrUseLastResponse
		},
	}

	next := authURL
	for range maxRedirects {
		resp, err := client.Get(next)
		if err != nil {
			return "", fmt.Errorf("request to %s failed: %w", next, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		location := resp.Header.Get("Location")
		if location == "" {
			return "", fmt.Errorf("%s returned %d without a redirect: %s", next, resp.StatusCode, body)
		}

		// Parse location and, if it's relative, make it absolute
		locationURL, err := resp.Request.URL.Parse(location)
		if err != nil {
			return "", fmt.Errorf("failed to parse redirect %q: %w", location, err)
		}

		if strings.HasPrefix(locationURL.String(), sigstoreOIDCRedirectURI) {
			code := locationURL.Query().Get("code")
			if code == "" {
				return "", fmt.Errorf("no code in callback URL: %s", locationURL)
			}
			return code, nil
		}
		next = locationURL.String()
	}

	return "", fmt.Errorf("authorization flow did not complete within %d redirects", maxRedirects)
}
