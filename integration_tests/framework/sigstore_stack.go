package integration_tests_framework

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

const (
	sigstoreMysqlPort        = "3306"
	sigstoreTrillianPort     = "8090"
	sigstoreTrillianHTTPPort = "8091"
	sigstoreRedisPort        = "6379"
	sigstoreRekorPort        = "3000"
	sigstoreFulcioPort       = "5555"
	sigstoreFulcioGRPCPort   = "5554"
	// Rekor already binds the default metrics port (2112) on the host network.
	sigstoreFulcioMetricsPort = "2113"
	sigstoreDexPort           = "8888"

	sigstoreMysqlRootPassword = "zaphod"
	sigstoreMysqlDatabase     = "test"
	sigstoreMysqlUser         = "test"
	sigstoreMysqlPassword     = "zaphod"

	sigstoreRedisPassword = "test"

	// Where Dex sends the authorization code. Nothing listens there - the test
	// scripts the OAuth2 flow and reads the code out of the redirect itself.
	// It must not overlap with Dex's own routes, which live under the issuer
	// path (/auth), including Dex's internal connector callback /auth/callback.
	sigstoreOIDCRedirectURI = "http://localhost:5556/callback"

	trustedRootFileName   = "trusted-root.json"
	signingConfigFileName = "signing-config.json"

	// Where ContainerOptions mounts the cosign trust material.
	SigstoreTrustedRootInContainer   = "/etc/sigstore/" + trustedRootFileName
	SigstoreSigningConfigInContainer = "/etc/sigstore/" + signingConfigFileName

	// Services of this stack have no meaningful validity window, so claim one
	// that covers any certificate they can issue.
	sigstoreServiceStartTime = "2000-01-01T00:00:00Z"
	sigstoreServiceOperator  = "konflux-build-cli-tests"
)

type SigstoreStack struct {
	mysql       *TestRunnerContainer
	trillianSvr *TestRunnerContainer
	trillianSgn *TestRunnerContainer
	redis       *TestRunnerContainer
	rekor       *TestRunnerContainer
	fulcio      *TestRunnerContainer
	dex         *TestRunnerContainer

	executor  cliWrappers.CliExecutorInterface
	logger    *logrus.Entry
	configDir string

	fulcioRootCertPath string
	fulcioRootKeyPath  string
	rekorPubKeyPath    string
}

func NewSigstoreStack() *SigstoreStack {
	return &SigstoreStack{
		executor: cliWrappers.NewCliExecutor(),
		logger:   l.Logger.WithField("logger", "sigstore"),
	}
}

func (s *SigstoreStack) RekorURL() string {
	return "http://localhost:" + sigstoreRekorPort
}

func (s *SigstoreStack) FulcioURL() string {
	return "http://localhost:" + sigstoreFulcioPort
}

func (s *SigstoreStack) OIDCIssuerURL() string {
	return "http://localhost:" + sigstoreDexPort + "/auth"
}

func (s *SigstoreStack) FulcioRootCertPath() string {
	return s.fulcioRootCertPath
}

func (s *SigstoreStack) RekorPublicKeyPath() string {
	return s.rekorPubKeyPath
}

// ContainerOptions returns the options a test container needs in order for
// cosign to talk to this stack and trust it. The upstream way to distribute
// this material is a TUF mirror, but the sigstore TUF server only runs inside
// Kubernetes, so mount the generated files directly instead. Pass them to
// cosign with --trusted-root and --signing-config.
//
// Call this only after Start succeeded.
func (s *SigstoreStack) ContainerOptions() []ContainerOption {
	return []ContainerOption{
		WithVolumeWithOptions(
			filepath.Join(s.configDir, trustedRootFileName),
			SigstoreTrustedRootInContainer, "z",
		),
		WithVolumeWithOptions(
			filepath.Join(s.configDir, signingConfigFileName),
			SigstoreSigningConfigInContainer, "z",
		),
	}
}

func (s *SigstoreStack) Start() error {
	var err error

	s.configDir, err = CreateTempDir("sigstore-config-")
	if err != nil {
		return fmt.Errorf("failed to create sigstore config dir: %w", err)
	}

	if err := s.generateCerts(); err != nil {
		return fmt.Errorf("failed to generate certs: %w", err)
	}
	if err := s.startMySQL(); err != nil {
		return fmt.Errorf("failed to start MySQL: %w", err)
	}
	if err := s.initTrillianDB(); err != nil {
		return fmt.Errorf("failed to init Trillian DB: %w", err)
	}
	if err := s.startTrillian(); err != nil {
		return fmt.Errorf("failed to start Trillian: %w", err)
	}
	if err := s.startRedis(); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}
	if err := s.startRekor(); err != nil {
		return fmt.Errorf("failed to start Rekor: %w", err)
	}
	if err := s.startDex(); err != nil {
		return fmt.Errorf("failed to start Dex: %w", err)
	}
	if err := s.startFulcio(); err != nil {
		return fmt.Errorf("failed to start Fulcio: %w", err)
	}
	if err := s.fetchRekorPublicKey(); err != nil {
		return fmt.Errorf("failed to fetch Rekor public key: %w", err)
	}
	if err := s.generateCosignConfigs(); err != nil {
		return fmt.Errorf("failed to generate cosign configs: %w", err)
	}

	s.logger.Info("Sigstore stack is ready")
	return nil
}

func (s *SigstoreStack) Stop() {
	for _, c := range []*TestRunnerContainer{
		s.fulcio, s.dex, s.rekor, s.redis,
		s.trillianSgn, s.trillianSvr, s.mysql,
	} {
		if c != nil {
			c.DeleteIfExists()
		}
	}
	if s.configDir != "" {
		os.RemoveAll(s.configDir)
	}
}

func (s *SigstoreStack) generateCerts() error {
	s.fulcioRootKeyPath = filepath.Join(s.configDir, "fulcio-root.key")
	s.fulcioRootCertPath = filepath.Join(s.configDir, "fulcio-root.pem")

	if stdout, stderr, _, err := s.executor.Execute(cliWrappers.Command(
		"openssl", "ecparam", "-genkey", "-name", "prime256v1",
		"-noout", "-out", s.fulcioRootKeyPath,
	)); err != nil {
		s.logger.Errorf("failed to generate Fulcio root key: %s\n%s", stdout, stderr)
		return err
	}

	if stdout, stderr, _, err := s.executor.Execute(cliWrappers.Command(
		"openssl", "req", "-x509", "-new",
		"-key", s.fulcioRootKeyPath,
		"-out", s.fulcioRootCertPath,
		"-days", "3650",
		"-subj", "/CN=sigstore-test-ca",
		"-addext", "basicConstraints=critical,CA:TRUE",
		"-addext", "keyUsage=critical,keyCertSign",
	)); err != nil {
		s.logger.Errorf("failed to generate Fulcio root cert: %s\n%s", stdout, stderr)
		return err
	}

	// Fulcio runs as a non-root user that doesn't match the file owner inside the
	// container. The key is a throwaway test CA, so make it world-readable.
	if err := os.Chmod(s.fulcioRootKeyPath, 0644); err != nil {
		return fmt.Errorf("failed to chmod Fulcio root key: %w", err)
	}

	return nil
}

func (s *SigstoreStack) startMySQL() error {
	s.mysql = NewTestRunnerContainer("sigstore-mysql", SigstoreMysqlImage,
		WithNetwork("host"),
		WithEnv("MYSQL_ROOT_PASSWORD", sigstoreMysqlRootPassword),
		WithEnv("MYSQL_DATABASE", sigstoreMysqlDatabase),
		WithEnv("MYSQL_USER", sigstoreMysqlUser),
		WithEnv("MYSQL_PASSWORD", sigstoreMysqlPassword),
	)
	s.mysql.ReplaceEntrypoint = false

	if err := s.mysql.Start(); err != nil {
		return err
	}

	return s.waitForHealth("MySQL", func() error {
		_, _, _, err := s.executor.Execute(cliWrappers.Command(
			containerTool, "exec", "sigstore-mysql",
			"mysqladmin", "-h", "127.0.0.1",
			"--user="+sigstoreMysqlUser,
			"--password="+sigstoreMysqlPassword,
			"-s", "ping",
		))
		return err
	}, 30, 2*time.Second)
}

// mysqlURI returns the MySQL connection string without the database name.
func (s *SigstoreStack) mysqlURI() string {
	return fmt.Sprintf(
		"%s:%s@tcp(localhost:%s)/",
		sigstoreMysqlUser, sigstoreMysqlPassword, sigstoreMysqlPort,
	)
}

func (s *SigstoreStack) initTrillianDB() error {
	s.logger.Info("Initializing Trillian database")

	c := NewTestRunnerContainer("sigstore-createdb", SigstoreCreateDBImage,
		WithNetwork("host"),
		WithContainerArgs(
			"--db_name="+sigstoreMysqlDatabase,
			"--mysql_uri="+s.mysqlURI(),
		),
	)
	c.ReplaceEntrypoint = false

	if err := c.Start(); err != nil {
		return err
	}
	defer c.DeleteIfExists()

	return s.waitForContainerExit("sigstore-createdb", 60*time.Second)
}

func (s *SigstoreStack) startTrillian() error {
	mysqlURI := s.mysqlURI() + sigstoreMysqlDatabase

	s.trillianSvr = NewTestRunnerContainer("sigstore-trillian-server", SigstoreTrillianLogSvrImage,
		WithNetwork("host"),
		WithContainerArgs(
			"--storage_system=mysql",
			"--quota_system=noop",
			"--mysql_uri="+mysqlURI,
			"--rpc_endpoint=0.0.0.0:"+sigstoreTrillianPort,
			"--http_endpoint=0.0.0.0:"+sigstoreTrillianHTTPPort,
			"--alsologtostderr",
		),
	)
	s.trillianSvr.ReplaceEntrypoint = false

	if err := s.trillianSvr.Start(); err != nil {
		return err
	}

	if err := s.waitForHealth("Trillian log-server", func() error {
		return s.httpHealthCheck("http://localhost:" + sigstoreTrillianHTTPPort + "/healthz")
	}, 15, time.Second); err != nil {
		return err
	}

	s.trillianSgn = NewTestRunnerContainer("sigstore-trillian-signer", SigstoreTrillianLogSgnImage,
		WithNetwork("host"),
		WithContainerArgs(
			"--storage_system=mysql",
			"--quota_system=noop",
			"--mysql_uri="+mysqlURI,
			"--rpc_endpoint=0.0.0.0:8092",
			"--http_endpoint=0.0.0.0:8093",
			"--force_master",
			"--alsologtostderr",
		),
	)
	s.trillianSgn.ReplaceEntrypoint = false

	if err := s.trillianSgn.Start(); err != nil {
		return err
	}

	return nil
}

func (s *SigstoreStack) startRedis() error {
	s.redis = NewTestRunnerContainer("sigstore-redis", SigstoreRedisImage,
		WithNetwork("host"),
		WithContainerArgs(
			"--bind", "0.0.0.0",
			"--appendonly", "yes",
			"--requirepass", sigstoreRedisPassword,
		),
	)
	s.redis.ReplaceEntrypoint = false

	if err := s.redis.Start(); err != nil {
		return err
	}

	return s.waitForHealth("Redis", func() error {
		_, _, _, err := s.executor.Execute(cliWrappers.Command(
			containerTool, "exec", "sigstore-redis",
			"redis-cli", "-a", sigstoreRedisPassword, "ping",
		))
		return err
	}, 10, time.Second)
}

func (s *SigstoreStack) startRekor() error {
	s.rekor = NewTestRunnerContainer("sigstore-rekor", SigstoreRekorImage,
		WithNetwork("host"),
		WithContainerArgs(
			"serve",
			"--trillian_log_server.address=localhost",
			"--trillian_log_server.port="+sigstoreTrillianPort,
			"--rekor_server.address=0.0.0.0",
			"--port="+sigstoreRekorPort,
			"--rekor_server.signer=memory",
			// With no tlog_id, Rekor creates and initializes a Trillian tree itself.
			"--redis_server.address=localhost",
			"--redis_server.port="+sigstoreRedisPort,
			"--redis_server.password="+sigstoreRedisPassword,
			"--search_index.storage_provider=redis",
		),
	)
	s.rekor.ReplaceEntrypoint = false

	if err := s.rekor.Start(); err != nil {
		return err
	}

	return s.waitForHealth("Rekor", func() error {
		return s.httpHealthCheck("http://localhost:" + sigstoreRekorPort + "/ping")
	}, 15, 2*time.Second)
}

func (s *SigstoreStack) startDex() error {
	dexConfigPath := filepath.Join(s.configDir, "dex-config.yaml")
	dexConfig := fmt.Sprintf(`issuer: http://localhost:%s/auth
web:
  http: 0.0.0.0:%s
oauth2:
  responseTypes:
    - code
  skipApprovalScreen: true
idTokens:
  validFor: 5m
staticClients:
  - id: fulcio
    public: true
    name: fulcio
    redirectURIs:
      - %s
connectors:
  - type: mockCallback
    id: mock
    name: mock
storage:
  type: memory
`, sigstoreDexPort, sigstoreDexPort, sigstoreOIDCRedirectURI)

	if err := os.WriteFile(dexConfigPath, []byte(dexConfig), 0644); err != nil {
		return fmt.Errorf("failed to write Dex config: %w", err)
	}

	s.dex = NewTestRunnerContainer("sigstore-dex", SigstoreDexImage,
		WithNetwork("host"),
		WithVolumeWithOptions(dexConfigPath, "/etc/dex/config.yaml", "z"),
		// The image entrypoint is a wrapper script that execs its arguments.
		WithContainerArgs("dex", "serve", "/etc/dex/config.yaml"),
	)
	s.dex.ReplaceEntrypoint = false

	if err := s.dex.Start(); err != nil {
		return err
	}

	return s.waitForHealth("Dex", func() error {
		return s.httpHealthCheck("http://localhost:" + sigstoreDexPort + "/auth/healthz")
	}, 15, time.Second)
}

func (s *SigstoreStack) startFulcio() error {
	fulcioConfigPath := filepath.Join(s.configDir, "fulcio-config.yaml")
	fulcioConfig := fmt.Sprintf(`oidc-issuers:
  http://localhost:%s/auth:
    issuer-url: http://localhost:%s/auth
    client-id: fulcio
    type: email
`, sigstoreDexPort, sigstoreDexPort)

	if err := os.WriteFile(fulcioConfigPath, []byte(fulcioConfig), 0644); err != nil {
		return fmt.Errorf("failed to write Fulcio config: %w", err)
	}

	s.fulcio = NewTestRunnerContainer("sigstore-fulcio", SigstoreFulcioImage,
		WithNetwork("host"),
		WithVolumeWithOptions(fulcioConfigPath, "/etc/fulcio-config/config.yaml", "z"),
		WithVolumeWithOptions(s.fulcioRootCertPath, "/etc/fulcio/root.pem", "z"),
		WithVolumeWithOptions(s.fulcioRootKeyPath, "/etc/fulcio/root.key", "z"),
		WithContainerArgs(
			"serve",
			"--host=0.0.0.0",
			"--port="+sigstoreFulcioPort,
			"--grpc-port="+sigstoreFulcioGRPCPort,
			"--metrics-port="+sigstoreFulcioMetricsPort,
			"--ca=fileca",
			"--fileca-cert=/etc/fulcio/root.pem",
			"--fileca-key=/etc/fulcio/root.key",
			"--fileca-key-passwd=",
			"--ct-log-url=",
		),
	)
	s.fulcio.ReplaceEntrypoint = false

	if err := s.fulcio.Start(); err != nil {
		return err
	}

	return s.waitForHealth("Fulcio", func() error {
		return s.httpHealthCheck("http://localhost:" + sigstoreFulcioPort + "/healthz")
	}, 15, 2*time.Second)
}

func (s *SigstoreStack) fetchRekorPublicKey() error {
	s.rekorPubKeyPath = filepath.Join(s.configDir, "rekor.pub")

	resp, err := http.Get("http://localhost:" + sigstoreRekorPort + "/api/v1/log/publicKey")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch Rekor public key: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(s.rekorPubKeyPath, body, 0644)
}

// generateCosignConfigs builds the Sigstore trusted root and signing config
// that describe this stack, using the cosign binary from the task-runner
// image. Cosign needs both to sign and verify against a private deployment.
func (s *SigstoreStack) generateCosignConfigs() error {
	s.logger.Info("Generating cosign trusted root and signing config")

	const configDirInContainer = "/sigstore"

	runCosign := func(args ...string) error {
		runArgs := []string{
			"run", "--rm",
			"--volume", s.configDir + ":" + configDirInContainer + ":z",
			"--entrypoint", "cosign",
			TaskRunnerImageRef,
		}
		stdout, stderr, _, err := s.executor.Execute(
			cliWrappers.Command(containerTool, append(runArgs, args...)...),
		)
		if err != nil {
			return fmt.Errorf("cosign %s failed: %w\n%s\n%s", args[0], err, stdout, stderr)
		}
		return nil
	}

	inContainer := func(name string) string {
		return configDirInContainer + "/" + name
	}

	if err := runCosign(
		"trusted-root", "create",
		fmt.Sprintf("--fulcio=url=%s,certificate-chain=%s",
			s.FulcioURL(), inContainer(filepath.Base(s.fulcioRootCertPath))),
		fmt.Sprintf("--rekor=url=%s,public-key=%s,start-time=%s",
			s.RekorURL(), inContainer(filepath.Base(s.rekorPubKeyPath)), sigstoreServiceStartTime),
		"--out="+inContainer(trustedRootFileName),
	); err != nil {
		return err
	}

	return runCosign(
		"signing-config", "create",
		fmt.Sprintf("--fulcio=url=%s,api-version=1,start-time=%s,operator=%s",
			s.FulcioURL(), sigstoreServiceStartTime, sigstoreServiceOperator),
		fmt.Sprintf("--rekor=url=%s,api-version=1,start-time=%s,operator=%s",
			s.RekorURL(), sigstoreServiceStartTime, sigstoreServiceOperator),
		"--rekor-config=ANY",
		fmt.Sprintf("--oidc-provider=url=%s,api-version=1,start-time=%s,operator=%s",
			s.OIDCIssuerURL(), sigstoreServiceStartTime, sigstoreServiceOperator),
		"--out="+inContainer(signingConfigFileName),
	)
}

// followAuthRedirects walks the chain of redirects Dex issues for an
// authorization request (connector selection, mock connector callback,
// approval) and returns the authorization code Dex finally sends to the
// client redirect URI.
func (s *SigstoreStack) followAuthRedirects(authURL string) (string, error) {
	const maxRedirects = 10

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
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

		target, err := resp.Request.URL.Parse(location)
		if err != nil {
			return "", fmt.Errorf("failed to parse redirect %q: %w", location, err)
		}

		if strings.HasPrefix(target.String(), sigstoreOIDCRedirectURI) {
			code := target.Query().Get("code")
			if code == "" {
				return "", fmt.Errorf("no code in callback URL: %s", target)
			}
			return code, nil
		}
		next = target.String()
	}

	return "", fmt.Errorf("authorization flow did not complete within %d redirects", maxRedirects)
}

// GetOIDCToken obtains an OIDC token from the Dex mock provider by
// scripting the OAuth2 authorization code flow. Dex's mockCallback
// connector auto-approves without credentials.
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

func (s *SigstoreStack) waitForHealth(name string, check func() error, maxRetries int, interval time.Duration) error {
	for i := range maxRetries {
		if err := check(); err == nil {
			s.logger.Infof("%s is ready", name)
			return nil
		} else if i < maxRetries-1 {
			s.logger.Debugf("Waiting for %s: %v", name, err)
			time.Sleep(interval)
		}
	}
	return fmt.Errorf("%s failed to become ready after %d retries", name, maxRetries)
}

func (s *SigstoreStack) httpHealthCheck(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func (s *SigstoreStack) waitForContainerExit(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := s.isContainerRunning(name)
		if err != nil {
			return err
		}
		if !running {
			exitCode, err := s.getContainerExitCode(name)
			if err != nil {
				return err
			}
			if exitCode != "0" {
				stdout, stderr, _, _ := s.executor.Execute(cliWrappers.Command(containerTool, "logs", name))
				return fmt.Errorf(
					"container %s exited with code %s:\n%s\n%s", name, exitCode, stdout, stderr,
				)
			}
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("container %s did not exit within %s", name, timeout)
}

func (s *SigstoreStack) isContainerRunning(name string) (bool, error) {
	stdout, _, _, err := s.executor.Execute(cliWrappers.Command(
		containerTool, "inspect", "--format", "{{.State.Running}}", name,
	))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(stdout) == "true", nil
}

func (s *SigstoreStack) getContainerExitCode(name string) (string, error) {
	stdout, _, _, err := s.executor.Execute(cliWrappers.Command(
		containerTool, "inspect", "--format", "{{.State.ExitCode}}", name,
	))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}
