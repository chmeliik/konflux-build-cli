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
			name: "nonexisting ../Dockerfile",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "../Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
			},
		},
		{
			name: "default search does not fall back to source root",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components/app",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(opts.SourceDir, "Containerfile"), dockerfileContent)
				writeFile(t, filepath.Join(opts.SourceDir, "Dockerfile"), dockerfileContent)
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
			name: "Containerfile takes precedence by default",
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
			name: "Dockerfile found by default if no Containerfile",
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
		{
			name: "default search in context dir",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "components/app",
				Dockerfile: "",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				path := createDir(t, opts.SourceDir, opts.ContextDir)
				writeFile(t, filepath.Join(path, "Containerfile"), dockerfileContent)
			},
			expectedDockerfile: "/components/app/Containerfile",
		},
		{
			name: "absolute context dir path",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: "delay to setup",
				Dockerfile: "./Dockerfile",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				opts.ContextDir = createDir(t, opts.SourceDir, "components")
				writeFile(t, filepath.Join(opts.ContextDir, "Dockerfile"), dockerfileContent)
			},
			expectedDockerfile: "/components/Dockerfile",
		},
		{
			name: "absolute Dockerfile path",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  "delay to setup",
				ContextDir: ".",
				Dockerfile: "delay to setup",
			},
			setup: func(t *testing.T, tc *TestCase) {
				opts := &tc.searchOpts
				opts.SourceDir = t.TempDir()
				opts.Dockerfile = filepath.Join(opts.SourceDir, "Dockerfile")
				writeFile(t, opts.Dockerfile, dockerfileContent)
			},
			expectedDockerfile: "/Dockerfile",
		},
		{
			name: "both source and context point to .",
			searchOpts: DockerfileSearchOpts{
				SourceDir:  ".",
				ContextDir: ".",
				Dockerfile: "dockerfiles/app",
			},
			setup: func(t *testing.T, tc *TestCase) {
				curDir, err := os.Getwd()
				if err != nil {
					t.Errorf("Error on getting current working directory: %v", err)
				}
				sourceDir := t.TempDir()
				os.Chdir(sourceDir)
				t.Cleanup(func() {
					os.Chdir(curDir)
				})
				path := createDir(t, sourceDir, "dockerfiles")
				writeFile(t, filepath.Join(path, "app"), dockerfileContent)
			},
			expectedDockerfile: "/dockerfiles/app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t, &tc)

			result, err := SearchDockerfile(tc.searchOpts)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			absResult, _ := filepath.Abs(result)
			absSourceDir, _ := filepath.Abs(tc.searchOpts.SourceDir)
			relativePath := strings.TrimPrefix(absResult, absSourceDir)
			if relativePath != tc.expectedDockerfile {
				t.Errorf("Expected getting Dockerfile %s, but got: '%s'", tc.expectedDockerfile, relativePath)
			}
		})
	}
}
