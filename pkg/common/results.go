package common

import (
	"encoding/json"
	"fmt"
	"os"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type ResultsWriterInterface interface {
	CreateResultJson(result any) (string, error)
	WriteResultString(result, path string) error
}

var _ ResultsWriterInterface = &ResultsWriter{}

type ResultsWriter struct{}

func NewResultsWriter() *ResultsWriter {
	return &ResultsWriter{}
}

// WriteResultString writes result data into file by given path
func (r *ResultsWriter) WriteResultString(result, path string) error {
	if path == "" {
		return nil
	}

	if err := os.WriteFile(path, []byte(result), 0644); err != nil {
		return fmt.Errorf("failed to write into result file '%s': %w", path, err)
	}

	l.Logger.Debugf("Wrote result into '%s':\n%s", path, result)

	return nil
}

// CreateResultJson converts a struct with results into JSON string.
// Mostly used by tasks to output results into stdout.
// Note, for Tekton results, the JSON must be escaped.
func (r *ResultsWriter) CreateResultJson(result any) (string, error) {
	resultJson, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(resultJson), nil
}
