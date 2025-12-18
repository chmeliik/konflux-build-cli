package common

import (
	_ "crypto/sha256"

	"github.com/containers/image/v5/docker/reference"
	go_digest "github.com/opencontainers/go-digest"
)

// GetImageName trims tag and/or digest from given image reference using containers/image library.
func GetImageName(imageURL string) string {
	ref, err := reference.Parse(imageURL)
	named, ok := ref.(reference.Named)
	if err != nil || !ok {
		// If parsing fails or the reference doesn't include a name,
		// return empty string for backwards compatibility.
		return ""
	}
	return named.Name()
}

// IsImageNameValid validates image name using containers/image library.
func IsImageNameValid(imageName string) bool {
	return imageName != "" && GetImageName(imageName) == imageName
}

func IsImageTagValid(tagName string) bool {
	// Create a minimal named reference to test tag validation against
	namedRef, _ := reference.ParseNamed("registry.io/test")
	// Try to create a tagged reference - if it succeeds, the tag is valid
	_, err := reference.WithTag(namedRef, tagName)
	return err == nil
}

func IsImageDigestValid(digest string) bool {
	// Use the go-digest library (which is used by containers/image) to parse and validate.
	_, err := go_digest.Parse(digest)
	return err == nil
}
