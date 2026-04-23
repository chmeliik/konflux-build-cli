package testutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
	"github.com/sirupsen/logrus"
)

// Writes files (specified as a map of {relative_path: file_content}) into the baseDir,
// creating subdirectories as needed.
func WriteFileTree(t *testing.T, baseDir string, files map[string]string) {
	for path, content := range files {
		fullPath := filepath.Join(baseDir, path)

		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %s", dir, err)
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create file %s: %s", path, err)
		}
	}
}

// Run the passed-in function while capturing log output, return the log output.
func CaptureLogOutput(fn func()) string {
	origOut := l.Logger.Out
	origFormatter := l.Logger.Formatter
	origLevel := l.Logger.Level

	var buf bytes.Buffer
	l.Logger.SetOutput(&buf)
	l.Logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	l.Logger.SetLevel(logrus.DebugLevel)

	defer func() {
		l.Logger.SetOutput(origOut)
		l.Logger.SetFormatter(origFormatter)
		l.Logger.SetLevel(origLevel)
	}()

	fn()

	return buf.String()
}
