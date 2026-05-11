package common

import (
	"fmt"
	"os"
	"path/filepath"
)

type DockerfileSearchOpts struct {
	// Source directory path containing application source code.
	SourceDir string
	// Build context directory within the source. It defaults to ".".
	ContextDir string
	// Dockerfile within the source. If not specified, it is searched in order
	// of ./Containerfile and ./Dockerfile. Containerfile takes precedence.
	Dockerfile string
}

// Like filepath.Join, but absolute path elements replace the preceding elements.
//
// Example:
// joinOrReplace("/abs1", "rel1", "/abs2", "rel2") => /abs2/rel2
func joinOrReplace(pathElem ...string) string {
	var actualPathElems []string
	for _, elem := range pathElem {
		if filepath.IsAbs(elem) {
			actualPathElems = actualPathElems[:0]
		}
		actualPathElems = append(actualPathElems, elem)
	}
	return filepath.Join(actualPathElems...)
}

// SearchDockerfile searches for a Dockerfile under the given source directory.
//
// Search for the Dockerfile under source/context/ first, then under source/.
// If Dockerfile is not specified, search ./Containerfile then ./Dockerfile.
//
// Note that the result path is not guaranteed to be a subpath of the source directory.
// If that is important, check with [RealPath.IsRelativeTo].
//
// Return an empty string if nothing is found.
func SearchDockerfile(opts DockerfileSearchOpts) (string, error) {
	if opts.SourceDir == "" {
		return "", fmt.Errorf("missing source directory")
	}
	contextDir := opts.ContextDir
	if contextDir == "" {
		contextDir = "."
	}

	var possibleDockerfiles []string
	if opts.Dockerfile != "" {
		// Look in the context dir first, then in the source dir.
		// This is the opposite order compared to buildah, kept for backwards compatibility.
		possibleDockerfiles = []string{
			joinOrReplace(opts.SourceDir, contextDir, opts.Dockerfile),
			joinOrReplace(opts.SourceDir, opts.Dockerfile),
		}
	} else {
		// Look for Containerfile/Dockerfile (in that order) in context dir, same as buildah
		possibleDockerfiles = []string{
			joinOrReplace(opts.SourceDir, contextDir, "Containerfile"),
			joinOrReplace(opts.SourceDir, contextDir, "Dockerfile"),
		}
	}

	for _, dockerfilePath := range possibleDockerfiles {
		if _, err := os.Stat(dockerfilePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("checking dockerfile existence: %w", err)
		}
		return dockerfilePath, nil
	}

	return "", nil
}
