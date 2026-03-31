package common

import (
	_ "crypto/sha256"
	"fmt"

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

// NormalizeImageRefWithDigest converts an image reference to name@digest format.
// If the reference has both a tag and digest (e.g., registry/repo:tag@sha256:abc),
// it strips the tag and returns only name@digest (e.g., registry/repo@sha256:abc).
// This is necessary because buildah doesn't support the tag+digest format when
// used with the docker:// prefix. Returns the original reference if it doesn't
// have a digest or if parsing fails.
func NormalizeImageRefWithDigest(imageRef string) string {
	ref, err := reference.Parse(imageRef)
	if err != nil {
		return imageRef
	}

	// Check if the reference has a digest
	canonical, ok := ref.(reference.Canonical)
	if !ok {
		return imageRef
	}

	// Get the base named reference (without tag)
	named := canonical.(reference.Named)
	// TrimNamed removes any tag from the named reference
	baseName := reference.TrimNamed(named)

	// Create a new canonical reference with just name@digest
	normalized, err := reference.WithDigest(baseName, canonical.Digest())
	if err != nil {
		return imageRef
	}

	return normalized.String()
}

// GetImageDigest extracts the digest from an image reference.
// Returns the digest string (e.g., "sha256:abc123...") or empty string if no digest.
func GetImageDigest(imageRef string) string {
	ref, err := reference.Parse(imageRef)
	if err != nil {
		return ""
	}

	canonical, ok := ref.(reference.Canonical)
	if !ok {
		return ""
	}

	return canonical.Digest().String()
}

// ValidateImageHasTagOrDigest checks that an image reference has at least a tag or digest.
// Returns an error if the image has neither a tag nor a digest.
func ValidateImageHasTagOrDigest(imageRef string) error {
	ref, err := reference.Parse(imageRef)
	if err != nil {
		return err
	}

	// Check if the reference has a digest
	if _, ok := ref.(reference.Canonical); ok {
		return nil
	}

	// Check if the reference has a tag
	if _, ok := ref.(reference.Tagged); ok {
		return nil
	}

	return fmt.Errorf("image '%s' must have a tag or digest", imageRef)
}
