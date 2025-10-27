package common

import (
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
