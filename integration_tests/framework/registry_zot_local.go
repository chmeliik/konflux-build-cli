package integration_tests_framework

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cliWrappers "github.com/konflux-ci/konflux-build-cli/pkg/cliwrappers"
	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

const (
	zotRegistryImage         = "ghcr.io/project-zot/zot-minimal:v2.1.11"
	zotRegistryContainerName = "zot-registry"
	zotRegistryDefaultPort   = "5000"
	zotRegistryUser          = "zotuser"
	zotRegistryPassword      = "zotpassword"

	zotRegistryStorageVolume  = true
	zotRegistryStorageHostDir = "/tmp/zot-registry-data"

	zotConfigDataDir    = "zotdata"
	zotConfigFileName   = "zot-config.json"
	zotRootKeyFileName  = "ca.key"
	zotRootCertFileName = "ca.crt"
	zotKeyFileName      = "server.key"
	zotCertFileName     = "server.crt"

	zotKeyPathInContainer  = "/etc/zot/certs/" + zotKeyFileName
	zotCertPathInContainer = "/etc/zot/certs/" + zotCertFileName
	zotDataPathInContainer = "/var/lib/registry"
)

var _ ImageRegistry = &ZotRegistry{}

type ZotRegistry struct {
	container *TestRunnerContainer
	logger    *logrus.Entry

	zotRegistryPort       string
	dataDirPath           string
	zotConfigPath         string
	zotHtpasswdPath       string
	rootKeyPath           string
	rootCertPath          string
	zotKeyPath            string
	zotCertPath           string
	dockerConfigJsonPath  string
	zotRegistryStorageDir string
}

func NewZotRegistry() ImageRegistry {
	zotConfigDataDirAbsolutePath, err := filepath.Abs(zotConfigDataDir)
	if err != nil {
		log.Fatal(err)
	}

	zotRegistryPort := os.Getenv("ZOT_REGISTRY_PORT")
	if zotRegistryPort == "" {
		zotRegistryPort = zotRegistryDefaultPort
	}

	return &ZotRegistry{
		container: NewTestRunnerContainer(zotRegistryContainerName, zotRegistryImage),
		logger:    l.Logger.WithField("logger", "zot"),

		zotRegistryPort:       zotRegistryPort,
		dataDirPath:           zotConfigDataDirAbsolutePath,
		zotConfigPath:         path.Join(zotConfigDataDirAbsolutePath, zotConfigFileName),
		zotHtpasswdPath:       path.Join(zotConfigDataDirAbsolutePath, "htpasswd"),
		rootKeyPath:           path.Join(zotConfigDataDirAbsolutePath, zotRootKeyFileName),
		rootCertPath:          path.Join(zotConfigDataDirAbsolutePath, zotRootCertFileName),
		zotKeyPath:            path.Join(zotConfigDataDirAbsolutePath, zotKeyFileName),
		zotCertPath:           path.Join(zotConfigDataDirAbsolutePath, zotCertFileName),
		dockerConfigJsonPath:  path.Join(zotConfigDataDirAbsolutePath, "config.json"),
		zotRegistryStorageDir: path.Join(zotRegistryStorageHostDir, strconv.FormatInt(time.Now().UnixMilli(), 10)),
	}
}

func (z *ZotRegistry) GetRegistryDomain() string {
	return "localhost:" + z.zotRegistryPort
}

func (z *ZotRegistry) GetTestNamespace() string {
	return z.GetRegistryDomain() + "/"
}

func (z *ZotRegistry) Start() error {
	z.container.ReplaceEntrypoint = false

	z.container.AddPort(z.zotRegistryPort, z.zotRegistryPort)

	z.container.AddVolumeWithOptions(z.zotConfigPath, "/etc/zot/config.json", "z")
	z.container.AddVolumeWithOptions(z.zotHtpasswdPath, "/etc/zot/htpasswd", "z")
	z.container.AddVolumeWithOptions(z.zotKeyPath, zotKeyPathInContainer, "z")
	z.container.AddVolumeWithOptions(z.zotCertPath, zotCertPathInContainer, "z")

	// Try to clean up the registry data.
	// It might fail due to permissions issue if the folder content was created from within a container,
	// but it doesn't matter because each new run uses different sub folder for storage.
	_ = os.RemoveAll(zotRegistryStorageHostDir)
	if zotRegistryStorageVolume {
		z.container.AddVolumeWithOptions(z.zotRegistryStorageDir, zotDataPathInContainer, "z")
		if err := EnsureDirectory(z.zotRegistryStorageDir); err != nil {
			return err
		}
	}

	isAlreadyRunning, err := z.container.ContainerExists(true)
	if err != nil {
		return err
	}
	if isAlreadyRunning {
		z.container.Delete()
	}

	if err := z.container.Start(); err != nil {
		return err
	}

	return z.WaitReady()
}

func (z *ZotRegistry) WaitReady() error {
	url := fmt.Sprintf("https://%s/v2/", z.GetRegistryDomain())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	username, password := z.GetCredentials()
	req.SetBasicAuth(username, password)

	client, err := z.createHttpClient()
	if err != nil {
		return err
	}

	const maxTries = 15
	for i := range maxTries {
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				z.logger.Info("Zot registry is ready")
				return nil
			}
		} else {
			z.logger.Infof("waiting Zot registry ready: %s", err.Error())
		}

		if i < maxTries {
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("failed to ping registry after %d retries", maxTries)
}

func (z *ZotRegistry) Stop() error {
	return z.container.Delete()
}

func (z *ZotRegistry) GetDockerConfigJsonContent() []byte {
	content, err := GenerateDockerAuthContent(z.GetRegistryDomain(), zotRegistryUser, zotRegistryPassword)
	if err != nil {
		z.logger.Fatalf("failed to create docker config json data: %s", err.Error())
	}
	return content
}

func (z *ZotRegistry) GetCaCertPath() string {
	return z.rootCertPath
}

func (z *ZotRegistry) IsLocal() bool {
	return true
}

func (z *ZotRegistry) GetCredentials() (string, string) {
	return zotRegistryUser, zotRegistryPassword
}

// CheckTagExistance quaries Zot API to check the tag existance.
// Args example: localhost:5000/image, tag
func (z *ZotRegistry) CheckTagExistance(imageName, tag string) (bool, error) {
	// Remove registry domain, e.g. localhost:5000/image -> image
	repoParts := strings.Split(imageName, "/")
	if len(repoParts) > 1 {
		repoParts = repoParts[1:]
	}
	imageName = strings.Join(repoParts, "/")

	url := fmt.Sprintf("https://%s/v2/%s/tags/list", z.GetRegistryDomain(), imageName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	username, password := z.GetCredentials()
	req.SetBasicAuth(username, password)

	client, err := z.createHttpClient()
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("received non-200 response status: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading response body: %v", err)
	}

	type TagListResponse struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	var tagListResponse TagListResponse
	if err := json.Unmarshal(body, &tagListResponse); err != nil {
		return false, fmt.Errorf("error unmarshaling response JSON: %v", err)
	}

	for _, t := range tagListResponse.Tags {
		if strings.EqualFold(t, tag) {
			return true, nil
		}
	}

	return false, nil
}

func (z *ZotRegistry) createHttpClient() (*http.Client, error) {
	caCert, err := os.ReadFile(z.rootCertPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, err
	}
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return client, nil
}

// Prepare ensures all needed files for Zot registry are in place.
func (z *ZotRegistry) Prepare() error {
	executor := cliWrappers.NewCliExecutor()

	os.Setenv("DOCKER_CONFIG", z.dataDirPath)

	if err := EnsureDirectory(zotConfigDataDir); err != nil {
		return err
	}

	if !FileExists(z.zotHtpasswdPath) {
		if err := z.createHtpasswdFile(executor); err != nil {
			return err
		}
	}

	// Check SSL cert chain
	if !(FileExists(z.rootKeyPath) && FileExists(z.rootCertPath) &&
		FileExists(z.zotKeyPath) && FileExists(z.zotCertPath)) {
		if err := z.generateCerts(executor); err != nil {
			return err
		}
	}

	// Create Zot config file
	zotConfigFilePath := path.Join(zotConfigDataDir, zotConfigFileName)
	if !FileExists(zotConfigFilePath) {
		if err := z.createZotConfig(zotConfigFilePath); err != nil {
			return err
		}
	} else {
		z.logger.Info("Using existing config file")
	}

	if !FileExists(z.dockerConfigJsonPath) {
		// Generate docker config json
		registryHosts := []string{z.GetRegistryDomain(), "localhost:" + z.zotRegistryPort}
		dockerConfigJson, err := GenerateDockerAuthContentWithAliases(registryHosts, zotRegistryUser, zotRegistryPassword)
		if err != nil {
			z.logger.Errorf("failed to generate dockerconfigjson: %s", err.Error())
			return err
		}
		if err := os.WriteFile(z.dockerConfigJsonPath, dockerConfigJson, 0644); err != nil {
			z.logger.Errorf("failed to save dockerconfigjson: %s", err.Error())
			return err
		}
	}

	if strings.ToLower(containerTool) == "podman" {
		// To make podman trust self signed cert, we need to copy the CA cert into
		// ~/.config/containers/certs.d/localhost:5000/ca-cert-file-name.crt
		if err := z.ensureZotCaCertInPodmanConfig(executor); err != nil {
			return err
		}
	}

	return nil
}

func (z *ZotRegistry) generateCerts(executor *cliWrappers.CliExecutor) error {
	opensslCreateCaKeyArgs := []string{
		"genrsa", "-out", z.rootKeyPath, "4096",
	}
	if stdout, stderr, _, err := executor.Execute("openssl", opensslCreateCaKeyArgs...); err != nil {
		z.logger.Errorf("failed to generate root CA key: %s\n%s", stdout, stderr)
		return err
	}

	opensslCreateCaCertArgs := []string{
		"req", "-x509", "-new",
		"-key", z.rootKeyPath,
		"-out", z.rootCertPath,
		"-days", "3650",
		"-subj", "/CN=localhost",
		"-addext",
		"basicConstraints=CA:TRUE",
	}
	if stdout, stderr, _, err := executor.Execute("openssl", opensslCreateCaCertArgs...); err != nil {
		z.logger.Errorf("failed to generate root CA cert: %s\n%s", stdout, stderr)
		return err
	}

	opensslCreateServerCertArgs := []string{
		"req", "-x509", "-newkey", "rsa:4096",
		"-keyout", z.zotKeyPath,
		"-out", z.zotCertPath,
		"-CA", z.rootCertPath,
		"-CAkey", z.rootKeyPath,
		"-days", "3650",
		"-nodes",
		"-subj", "/CN=localhost",
		"-addext",
		fmt.Sprintf("subjectAltName=DNS:localhost,IP:127.0.0.1,DNS:%s", zotRegistryContainerName),
	}
	if stdout, stderr, _, err := executor.Execute("openssl", opensslCreateServerCertArgs...); err != nil {
		z.logger.Errorf("failed to generate zot registry cert: %s\n%s", stdout, stderr)
		return err
	}
	return nil
}

func (z *ZotRegistry) createZotConfig(zotConfigFilePath string) error {
	config := fmt.Appendf(nil, `
{
 	"storage": {
        "rootDirectory":"/var/lib/registry"
	},
	"http": {
		"address": "0.0.0.0",
		"port": %s,
		"compat": ["docker2s2"],
		"tls": {
			"cert": "%s",
			"key": "%s"
		},
		"auth": {
			"htpasswd": {
				"path": "/etc/zot/htpasswd"
			}
		}
	},
	"log": {
		"level": "debug"
	}
}
`,
		z.zotRegistryPort,
		zotCertPathInContainer, zotKeyPathInContainer)

	if err := os.WriteFile(zotConfigFilePath, config, 0644); err != nil {
		z.logger.Errorf("failed to create zot config file")
		return err
	}
	return nil
}

func (z *ZotRegistry) createHtpasswdFile(executor *cliWrappers.CliExecutor) error {
	stdout, stderr, _, err := executor.Execute(
		"htpasswd", "-bBn", zotRegistryUser, zotRegistryPassword)
	if err != nil {
		z.logger.Errorf("failed to generate htpasswd file content: %s\n%s", stdout, stderr)
		return err
	}
	htpasswdContent := strings.TrimSpace(stdout)
	if err := os.WriteFile(z.zotHtpasswdPath, []byte(htpasswdContent), 0644); err != nil {
		z.logger.Errorf("failed to create htpasswd file: %s", err.Error())
		return err
	}
	return nil
}

// ensureZotCaCertInPodmanConfig puts the generated self-signed CA cert file into
// ~/.config/containers/certs.d/localhost:5000/ directory
// to make podman trust the Zot https endpoint with the self-signed certificate.
// Should be used with Podman only.
func (z *ZotRegistry) ensureZotCaCertInPodmanConfig(executor *cliWrappers.CliExecutor) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	zotRegistryPodmanCertsDir := path.Join(homeDir, ".config/containers/certs.d", z.GetRegistryDomain())
	if err := EnsureDirectory(zotRegistryPodmanCertsDir); err != nil {
		return err
	}

	zotRegistryPodmanCaCertPath := path.Join(zotRegistryPodmanCertsDir, zotRootCertFileName)

	if FileExists(zotRegistryPodmanCaCertPath) {
		// Check if the cert in Podman config is the same as teh cert in Zot config

		zotCaCertFileStat, err := os.Stat(z.rootCertPath)
		if err != nil {
			return fmt.Errorf("failed to stat Zot CA cert file: %w", err)
		}
		zotCaCertInPodmanConfFileStat, err := os.Stat(zotRegistryPodmanCaCertPath)
		if err != nil {
			return fmt.Errorf("failed to stat Zot CA cert file in Podman config dir: %w", err)
		}
		// Compare modification times
		if zotCaCertInPodmanConfFileStat.ModTime().After(zotCaCertFileStat.ModTime()) {
			z.logger.Info("Using existing Zot CA cert in Podman config directory")
			return nil
		}
	}

	// Copy the CA cert into Podman config directory.
	if stdout, stderr, _, err := executor.Execute("cp", z.rootCertPath, zotRegistryPodmanCaCertPath); err != nil {
		z.logger.Errorf("failed to copy root CA cert into podman config dir: %s\n%s", stdout, stderr)
		return err
	}
	return nil
}
