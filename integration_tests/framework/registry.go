package integration_tests_framework

import (
	"encoding/base64"
	"encoding/json"
)

type ImageRegistry interface {
	// Returns true for locally hosted registries.
	IsLocal() bool
	// Ensures all resources are ready to use the registry.
	Prepare() error
	// Starts local registry.
	Start() error
	// Stops local registry.
	Stop() error
	// Returns usename and password / token to access test namespace, see GetTestNamespace.
	GetCredentials() (string, string)
	// Returns base registry url, e.g. registry.io:1234
	GetRegistryDomain() string
	// Returns first part of the image name to which user can push test data.
	// Example: quay.io/my-org/
	GetTestNamespace() string
	// Returns config.josn content to authorize clients.
	GetDockerConfigJsonContent() []byte
	// Returns path to the root CA certificate the registry is using (in case of self-signed certificate),
	// empty string otherwise.
	GetCaCertPath() string
	// Returns true if given image exists in the test namespace of the registry.
	CheckTagExistance(imageName, tag string) (bool, error)
}

func GenerateDockerAuthContent(registry, login, password string) ([]byte, error) {
	return GenerateDockerAuthContentWithAliases([]string{registry}, login, password)
}

func GenerateDockerAuthContentWithAliases(registries []string, login, password string) ([]byte, error) {
	type dockerConfigAuth struct {
		Auth string `json:"auth"`
	}
	type dockerConfigJson struct {
		Auths map[string]dockerConfigAuth `json:"auths"`
	}

	auth := dockerConfigAuth{Auth: base64.StdEncoding.EncodeToString([]byte(login + ":" + password))}
	auths := map[string]dockerConfigAuth{}
	for _, registry := range registries {
		auths[registry] = auth
	}
	dockerconfig := dockerConfigJson{Auths: auths}
	return json.Marshal(dockerconfig)
}
