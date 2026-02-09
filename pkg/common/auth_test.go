package common

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	dockerIOToken      = "docker.io token"
	indexDockerIOToken = "index.docker.io token"
	quayIOKonfluxToken = "quay.io-konflux token"
	quayIOToken        = "quay.io token"
	regIOToken         = "reg.io token"
	regIOFooBarToken   = "reg.io-foo-bar token"
)

func generateDigest() string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	chars := []rune("0123456789abcdef")
	charsLen := len(chars)
	digest := make([]rune, 64)
	for i := range digest {
		digest[i] = chars[rng.Intn(charsLen)]
	}
	return string(digest)
}

func createAuthFile(auths map[string]interface{}) (string, error) {
	tmpDir := os.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	data, err := json.Marshal(auths)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return "", err
	}

	return configPath, nil
}

func TestSelectAuth(t *testing.T) {
	auths := map[string]interface{}{
		"auths": map[string]interface{}{
			"docker.io":                   map[string]string{"auth": dockerIOToken},
			"https://index.docker.io/v1/": map[string]string{"auth": indexDockerIOToken},
			"quay.io/konflux-ci/foo":      map[string]string{"auth": quayIOKonfluxToken},
			"quay.io":                     map[string]string{"auth": quayIOToken},
			"reg.io":                      map[string]string{"auth": regIOToken},
			"reg.io/foo/bar":              map[string]string{"auth": regIOFooBarToken},
		},
	}

	authFile, err := createAuthFile(auths)
	if err != nil {
		t.Fatalf("Failed to create auth file: %v", err)
	}
	defer os.Remove(authFile)

	testCases := []struct {
		imageRef      string
		expectedToken string
	}{
		{"docker.io/library/debian:latest", dockerIOToken},
		{"quay.io", quayIOToken},
		{"quay.io/foo", quayIOToken},
		{"quay.io/foo:0.1", quayIOToken},
		{"quay.io/foo:0.1@sha256:" + generateDigest(), quayIOToken},
		{"quay.io/konflux-ci", quayIOToken},
		{"quay.io/konflux-ci/foo", quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo:0.3", quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo@sha256:" + generateDigest(), quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo:0.3@sha256:" + generateDigest(), quayIOKonfluxToken},
		{"quay.io/konflux-ci/foo/bar", quayIOKonfluxToken},
		{"reg.io", regIOToken},
		{"reg.io/foo", regIOToken},
		{"reg.io/foo/bar", regIOFooBarToken},
		{"new-reg.io/cool-app", "err"},
		{"arbitrary-input", "err"},
	}

	for _, tc := range testCases {
		t.Run(tc.imageRef, func(t *testing.T) {
			registryAuth, err := SelectRegistryAuth(tc.imageRef, authFile)

			if tc.expectedToken == "err" {
				if err == nil {
					t.Errorf("selectRegistryAuth does not return error")
				}
				if !strings.Contains(err.Error(), "Registry authentication is not configured") {
					t.Errorf("selectRegistryAuth does not return error representing token is not found.")
				}
				return
			}

			if registryAuth.Token != tc.expectedToken {
				t.Errorf("Expected token %q, got %q", tc.expectedToken, registryAuth.Token)
			}
		})
	}
}

func TestFallbackSelectionForDockerIO(t *testing.T) {
	auths := map[string]interface{}{
		"auths": map[string]interface{}{
			"https://index.docker.io/v1/": map[string]string{"auth": indexDockerIOToken},
			"quay.io/konflux-ci/foo":      map[string]string{"auth": quayIOKonfluxToken},
			"quay.io":                     map[string]string{"auth": quayIOToken},
		},
	}

	authFile, err := createAuthFile(auths)
	if err != nil {
		t.Fatalf("Failed to create auth file: %v", err)
	}
	defer os.Remove(authFile)

	registryAuth, err := SelectRegistryAuth("docker.io/library/postgres", authFile)

	if err != nil {
		t.Error("Token is not got from auth file.")
		return
	}

	if registryAuth.Registry != registryDockerIO {
		t.Errorf("Token is not selected for registry %s", registryDockerIO)
		return
	}

	if registryAuth.Token != indexDockerIOToken {
		t.Errorf("Token is not selected from registry %s from auth file.", registryIndexDockerIO)
		return
	}
}
