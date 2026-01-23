package common

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

const defaultMaxFileSize = 1024 * 1024 // 1MiB

func FetchFile(fileLocationUrl, filename string, sizeLimit int64) error {
	resp, err := http.Get(fileLocationUrl)
	if err != nil {
		return fmt.Errorf("File fetch HTTP request failure: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("File fetch HTTP request failure. Server response status: %d", resp.StatusCode)
	}

	effectiveSizeLimit := sizeLimit
	if sizeLimit == 0 {
		effectiveSizeLimit = defaultMaxFileSize
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, effectiveSizeLimit))
	if err != nil {
		return fmt.Errorf("Error on reading response data: %w", err)
	}

	if len(data) >= int(effectiveSizeLimit) {
		return fmt.Errorf("Remote file size exceeds maximum allowed size of %d bytes", effectiveSizeLimit)
	}

	if err = os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("Cannot write fetched file content to file %s: %w", filename, err)
	}
	return nil
}
