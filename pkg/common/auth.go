package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	registryDockerIO      = "docker.io"
	registryIndexDockerIO = "https://index.docker.io/v1/"
)

type RegistryAuth struct {
	Registry string
	Token    string
}

type RegistryAuths struct {
	Auths map[string]AuthEntry `json:"auths"`
}

type AuthEntry struct {
	Auth string `json:"auth"`
}

// SelectRegistryAuth selects registry authentication credential from an authentication file.
//
// The format of authentication file, like ~/.docker/config.json, is not well defined. Some clients
// allow the specification of repository specific tokens, e.g. buildah and kubernetes, while others
// only allow registry specific tokens, e.g. oras. SelectRegistryAuth serves as an adapter to allow
// repository specific tokens for clients that do not support it.
//
// Arguments:
//   - imageRef: Image reference like registry.io/namespace/image:tag. It can be an image repository,
//     or a full reference with either tag, digest or both.
//   - authFilePath: Path to authentication file.
//
// Returns an object of RegistryAuth and an error.
func SelectRegistryAuth(imageRef string, authFilePath string) (*RegistryAuth, error) {
	imageRepo := GetImageName(imageRef)
	if imageRepo == "" {
		return nil, fmt.Errorf("Invalid image reference '%s'", imageRef)
	}

	registryAuths, err := readAuthFile(authFilePath)
	if err != nil {
		return nil, err
	}

	token := findAuth(registryAuths, imageRepo)
	if token == "" {
		return nil, fmt.Errorf("Registry authentication is not configured for %s.", imageRepo)
	}

	return &RegistryAuth{
		Registry: strings.Split(imageRepo, "/")[0],
		Token:    token,
	}, nil
}

// SelectRegistryAuthFromDefaultAuthFile selects authentication credential from default
// authentication file ~/.docker/config.json. Refer to SelectRegistryAuth for more details.
func SelectRegistryAuthFromDefaultAuthFile(imageRef string) (*RegistryAuth, error) {
	authFile := GetDefaultAuthFile()
	return SelectRegistryAuth(imageRef, authFile)
}

// findAuth finds out authentication credential string by image repository.
// Argument registryAuths contains loaded authentication credentials loaded from authfile.
// If nothing is found, returns an empty string.
func findAuth(registryAuths *RegistryAuths, imageRepo string) string {
	authKey := imageRepo
	for {
		if authEntry, exists := registryAuths.Auths[authKey]; exists {
			return authEntry.Auth
		}
		index := strings.LastIndex(authKey, "/")
		if index < 0 {
			break
		}
		authKey = authKey[:index]
	}
	// When log into dockerhub, oras-login writes https://index.docker.io/v1/ as registry into authfile.
	registry := strings.Split(imageRepo, "/")[0]
	if registry == registryDockerIO {
		if authEntry, exists := registryAuths.Auths[registryIndexDockerIO]; exists {
			return authEntry.Auth
		}
	}
	return ""
}

func GetDefaultAuthFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".docker", "config.json")
}

func readAuthFile(authFilePath string) (*RegistryAuths, error) {
	data, err := os.ReadFile(authFilePath)
	if err != nil {
		return nil, err
	}

	var registryAuths RegistryAuths
	err = json.Unmarshal(data, &registryAuths)
	if err != nil {
		return nil, err
	}

	return &registryAuths, nil
}
