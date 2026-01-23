package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var dockerfileContent = []byte("FROM fedora")

type TestCase struct {
	name               string
	searchOpts         DockerfileSearchOpts
	expectedDockerfile string
	setup              func(*testing.T, *TestCase)
}

func createDir(t *testing.T, dirName ...string) string {
	path := filepath.Join(dirName...)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		t.Fatalf("Failed to create base directory: %v", err)
	}
	return path
}

func writeFile(t *testing.T, path string, content []byte) {
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("Failed to create escape target file: %v", err)
	}
}

func TestSearchDockerfileErrorOnEscapingFromSource(t *testing.T) {
	testCases := []TestCase{
		// Tests Dockerfile is escaped by source directory
		// Directory structure:
		// /tmp/workspace/source/ links to /tmp/escaped/
		// Search /tmp/workspace/source/Dockerfile
		{
			name: "escaped from source directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				workDir := t.TempDir()
				// source links to this target outside of the base path.
				escapedDir := createDir(t, workDir, "escaped")
				writeFile(t, filepath.Join(escapedDir, "Dockerfile"), dockerfileContent)
				// Link sourceDir to escapedDir
				linkName := filepath.Join(workDir, "source")
				if err := os.Symlink(escapedDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
				tc.searchOpts.SourceDir = linkName
			},
		},
		// Tests Dockerfile is escaped by context directory
		// Directory structure:
		// /tmp/workspace/source/dockerfiles(context)/ links to /tmp/workspace/escaped/
		// Search /tmp/workspace/source/dockerfiles/Dockerfile, where context is dockerfiles
		{
			name: "escaped from context directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "dockerfiles",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				workDir := t.TempDir()
				opts := &tc.searchOpts
				opts.SourceDir = createDir(t, workDir, "source")
				escapedDir := createDir(t, workDir, "escaped")
				writeFile(t, filepath.Join(escapedDir, "Dockerfile"), dockerfileContent)
				// Link contextDir to escapedDir
				linkName := filepath.Join(opts.SourceDir, opts.ContextDir)
				if err := os.Symlink(escapedDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
			},
		},
		// Tests Dockerfile is escaped from source by Dockerfile itself
		// Directory structure:
		// /tmp/workspace/source/dockerfiles/ links to /tmp/workspace/escaped/
		// Search /tmp/workspace/source/dockerfiles/Dockerfile, where dockerfile is dockerfiles/Dockerfile
		{
			name: "escaped from source by Dockerfile itself",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "dockerfiles/Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				workDir := t.TempDir()
				opts := &tc.searchOpts
				opts.SourceDir = createDir(t, workDir, "source")
				escapedDir := createDir(t, workDir, "escaped")
				writeFile(t, filepath.Join(escapedDir, "Dockerfile"), dockerfileContent)
				// Link directory dockerfiles/ to escapedDir
				linkName := filepath.Join(opts.SourceDir, filepath.Dir(opts.Dockerfile))
				if err := os.Symlink(escapedDir, linkName); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)

			result, err := SearchDockerfile(tc.searchOpts)

			if err == nil {
				t.Errorf("Expected error for Dockerfile escaped from base directory, but got result: %s", result)
			}
			if !strings.Contains(err.Error(), "Dockerfile is outside of the source directory") {
				t.Errorf("Expected error message about escaping from source directory, got: %v", err)
			}
		})
	}
}

func TestSearchDockerfileNotFound(t *testing.T) {
	testCases := []TestCase{
		{
			name: "source does not have Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				tc.searchOpts.SourceDir = t.TempDir()
			},
		},
		{
			name: "Dockerfile is specified with a different name",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Containerfile.operator",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
		},
		{
			name: "escaped from source by ../../../../../.../Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "../../../../Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				dockerfilePath := filepath.Join(opts.SourceDir, filepath.Base(opts.Dockerfile))
				writeFile(t, dockerfilePath, dockerfileContent)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)
			opts := tc.searchOpts
			result, err := SearchDockerfile(opts)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != "" {
				t.Errorf("Expected Dockerfile %s is not found and empty string is returned, but got: %s", opts.Dockerfile, result)
			}
		})
	}
}

func TestSearchDockerfile(t *testing.T) {
	testCases := []TestCase{
		{
			name: "found from source",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, opts.Dockerfile), dockerfileContent)
			},
			expectedDockerfile: "/Dockerfile",
		},
		{
			name: "found from context",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				path := createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(path, opts.Dockerfile), dockerfileContent)
			},
			expectedDockerfile: "/components/Dockerfile",
		},
		{
			name: "dockerfile includes directory",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "dockerfiles/app",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				path := createDir(t, opts.SourceDir, "dockerfiles")
				writeFile(t, filepath.Join(path, filepath.Base(opts.Dockerfile)), dockerfileContent)
			},
			expectedDockerfile: "/dockerfiles/app",
		},
		{
			name: "Dockerfile within context/ takes precedence",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components/app",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
				path := createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(path, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/components/app/Dockerfile",
		},
		{
			name: "Searched ./Container by default",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Containerfile"), dockerfileContent)
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/Containerfile",
		},
		{
			name: "Fallback to search ./Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/Dockerfile",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)

			result, err := SearchDockerfile(tc.searchOpts)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			expected := tc.expectedDockerfile
			relativePath := strings.TrimPrefix(result, tc.searchOpts.SourceDir)
			if relativePath != expected {
				t.Errorf("Expected getting Dockerfile %s, but got: '%s'", expected, relativePath)
			}
		})
	}
}
