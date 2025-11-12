package common

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewResultsWriter(t *testing.T) {
	t.Run("should create ResultsWriter", func(t *testing.T) {
		g := NewWithT(t)

		writer := NewResultsWriter()

		g.Expect(writer).ToNot(BeNil())
	})
}

func TestResultsWriter_WriteResultString(t *testing.T) {
	t.Run("should skip writing result to file if path is empty", func(t *testing.T) {
		g := NewWithT(t)

		testContent := "test result content"

		writer := NewResultsWriter()
		err := writer.WriteResultString(testContent, "")

		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should write result to file successfully", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test_result.txt")
		testContent := "test result content"

		writer := NewResultsWriter()
		err := writer.WriteResultString(testContent, filePath)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify file was written
		content, err := os.ReadFile(filePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(content)).To(Equal(testContent))
	})

	t.Run("should return error when file cannot be written", func(t *testing.T) {
		g := NewWithT(t)

		invalidPath := "/invalid/path/that/does/not/exist/result.txt"
		testContent := "test content"

		writer := NewResultsWriter()
		err := writer.WriteResultString(testContent, invalidPath)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to write into result file"))
	})

	t.Run("should write file with correct permissions", func(t *testing.T) {
		g := NewWithT(t)

		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test_permissions.txt")
		testContent := "test content"

		writer := NewResultsWriter()
		err := writer.WriteResultString(testContent, filePath)

		g.Expect(err).ToNot(HaveOccurred())

		// Check file permissions
		fileInfo, err := os.Stat(filePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(0644)))
	})
}

func TestResultsWriter_CreateResultJson(t *testing.T) {
	t.Run("should create result json for results struct", func(t *testing.T) {
		g := NewWithT(t)

		const stringResult = "test string result"
		const intResult = 1234
		const boolResult = true
		arrResult := []string{"val1", "val2"}

		type TestResult struct {
			StringResult string   `json:"string-result"`
			IntResult    int      `json:"int-result"`
			BoolResult   bool     `json:"bool-result"`
			ArrayResult  []string `json:"array-result"`
		}
		testResult := TestResult{
			StringResult: stringResult,
			IntResult:    intResult,
			BoolResult:   boolResult,
			ArrayResult:  arrResult,
		}

		writer := NewResultsWriter()
		result, err := writer.CreateResultJson(testResult)

		g.Expect(err).ToNot(HaveOccurred())

		obtainedResult := TestResult{}
		err = json.Unmarshal([]byte(result), &obtainedResult)
		g.Expect(err).ShouldNot(HaveOccurred(), "failed to unmarshall json result")

		g.Expect(obtainedResult.StringResult).To(Equal(stringResult))
		g.Expect(obtainedResult.IntResult).To(Equal(intResult))
		g.Expect(obtainedResult.BoolResult).To(Equal(boolResult))
		g.Expect(obtainedResult.ArrayResult).To(Equal(arrResult))
	})

	t.Run("should create result json for results struct without json tags", func(t *testing.T) {
		g := NewWithT(t)

		const stringResult = "test string result"
		const intResult = 1234
		const boolResult = true
		arrResult := []string{"val1", "val2"}

		type TestResult struct {
			StringResult string
			IntResult    int
			BoolResult   bool
			ArrayResult  []string
		}
		testResult := TestResult{
			StringResult: stringResult,
			IntResult:    intResult,
			BoolResult:   boolResult,
			ArrayResult:  arrResult,
		}

		writer := NewResultsWriter()
		result, err := writer.CreateResultJson(testResult)

		g.Expect(err).ToNot(HaveOccurred())

		obtainedResult := TestResult{}
		err = json.Unmarshal([]byte(result), &obtainedResult)
		g.Expect(err).ShouldNot(HaveOccurred(), "failed to unmarshall json result")

		g.Expect(obtainedResult.StringResult).To(Equal(stringResult))
		g.Expect(obtainedResult.IntResult).To(Equal(intResult))
		g.Expect(obtainedResult.BoolResult).To(Equal(boolResult))
		g.Expect(obtainedResult.ArrayResult).To(Equal(arrResult))
	})

	t.Run("should create result json for single string value", func(t *testing.T) {
		g := NewWithT(t)

		stringResult := "test string result"

		writer := NewResultsWriter()
		result, err := writer.CreateResultJson(stringResult)

		g.Expect(err).ToNot(HaveOccurred())

		var obtainedResult string
		err = json.Unmarshal([]byte(result), &obtainedResult)
		g.Expect(err).ShouldNot(HaveOccurred(), "failed to unmarshall json result")
		g.Expect(obtainedResult).To(Equal("test string result"))
	})

	t.Run("should create result json for single int value", func(t *testing.T) {
		g := NewWithT(t)

		intResult := 12345

		writer := NewResultsWriter()
		result, err := writer.CreateResultJson(intResult)

		g.Expect(err).ToNot(HaveOccurred())

		var obtainedResult int
		err = json.Unmarshal([]byte(result), &obtainedResult)
		g.Expect(err).ShouldNot(HaveOccurred(), "failed to unmarshall json result")
		g.Expect(obtainedResult).To(Equal(12345))
	})

	t.Run("should error if failed to create json string", func(t *testing.T) {
		g := NewWithT(t)

		var nanFloat float64 = math.NaN()

		writer := NewResultsWriter()
		_, err := writer.CreateResultJson(nanFloat)
		g.Expect(err).To(HaveOccurred())
	})
}
