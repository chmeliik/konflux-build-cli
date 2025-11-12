package commands

import (
	"encoding/json"

	"github.com/konflux-ci/konflux-build-cli/pkg/common"
)

var _ common.ResultsWriterInterface = &mockResultsWriter{}

type mockResultsWriter struct {
	WriteResultStringFunc func(result, path string) error
	CreateResultJsonFunc  func(result any) (string, error)

	// Result file path => result data
	WrittenResults map[string]string
}

func (m *mockResultsWriter) CreateResultJson(result any) (string, error) {
	if m.CreateResultJsonFunc != nil {
		return m.CreateResultJsonFunc(result)
	}

	resultJson, err := json.Marshal(result)
	return string(resultJson), err
}

func (m *mockResultsWriter) WriteResultString(result, path string) error {
	if m.WriteResultStringFunc != nil {
		return m.WriteResultStringFunc(result, path)
	}

	if m.WrittenResults == nil {
		m.WrittenResults = make(map[string]string)
	}
	m.WrittenResults[path] = result
	return nil
}
