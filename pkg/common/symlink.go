package common

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	l "github.com/konflux-ci/konflux-build-cli/pkg/logger"
)

// CheckSymlinks walks the given directory tree and returns an error if any
// symlink points to a target outside the directory.
func CheckSymlinks(dir string) error {
	l.Logger.Debugf("Checking for symlinks pointing outside the directory %s", dir)

	// Resolve the directory to handle symlinks in the path itself (e.g., on macOS /tmp -> /private/tmp)
	absBaseDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of directory: %w", err)
	}
	absBaseDir, err = filepath.EvalSymlinks(absBaseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks in directory path: %w", err)
	}

	var invalidSymlinks []string

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type()&os.ModeSymlink != 0 {
			// This is a symlink
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				l.Logger.Errorf("Broken symlink found: %s", path)
				invalidSymlinks = append(invalidSymlinks, path)
				return nil
			}

			absTarget, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("failed to get absolute path of symlink target: %w", err)
			}

			// Check if target is inside the base directory
			if !strings.HasPrefix(absTarget, absBaseDir+string(os.PathSeparator)) && absTarget != absBaseDir {
				l.Logger.Errorf("Symlink points outside directory: %s -> %s", path, target)
				invalidSymlinks = append(invalidSymlinks, path)
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(invalidSymlinks) > 0 {
		return fmt.Errorf("found %d symlink(s) pointing outside the directory", len(invalidSymlinks))
	}

	l.Logger.Debug("Symlink check passed")
	return nil
}
