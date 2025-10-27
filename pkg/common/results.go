package common

import (
	"fmt"
	"os"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

type ResultsWriterInterface interface {
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

	l.Logger.Infof("Wrote result into '%s': \n%s", path, result)

	return nil
}
