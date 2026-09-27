package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// errBundleMissingData signals that a candidate bundle has no readable
// regular file named "data".
var errBundleMissingData = errors.New("bundle has no data file")

// ResolveBundlePath expands and resolves path (a --quicken value) against
// home to an absolute bundle directory. It returns a RefusalError unless the
// result exists, does not end ".qdf"/".QDF", is a directory, and contains a
// readable regular file named "data".
func ResolveBundlePath(home, path string) (string, error) {
	abs, err := filepath.Abs(homepath.Expand(home, path))
	if err != nil {
		return "", fmt.Errorf("resolve bundle path %s: %w", path, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", RefusalError{msg: fmt.Sprintf(
				"%s does not exist; check the path passed to --quicken",
				homepath.Abbreviate(home, abs))}
		}
		return "", unreadableRefusal(home, abs, err)
	}

	if strings.EqualFold(filepath.Ext(abs), ".qdf") {
		return "", RefusalError{msg: fmt.Sprintf(
			"%s is a Quicken for Windows file; quarry reads only Quicken Classic for Mac .quicken files",
			homepath.Abbreviate(home, abs))}
	}

	if !info.IsDir() {
		return "", notABundleRefusal(home, abs)
	}

	if err := validateData(abs); err != nil {
		if errors.Is(err, errBundleMissingData) {
			return "", notABundleRefusal(home, abs)
		}
		return "", unreadableRefusal(home, filepath.Join(abs, "data"), err)
	}

	return abs, nil
}

// validateData confirms bundlePath contains a readable, regular file named
// "data" — the on-disk shape every .quicken bundle must have. It returns
// errBundleMissingData when data is absent or not a regular file, or the
// stat/open fault otherwise.
func validateData(bundlePath string) error {
	dataPath := filepath.Join(bundlePath, "data")

	info, err := os.Stat(dataPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errBundleMissingData
		}
		return err
	}
	if !info.Mode().IsRegular() {
		return errBundleMissingData
	}

	f, err := os.Open(dataPath)
	if err != nil {
		return err
	}
	_ = f.Close()
	return nil
}

// notABundleRefusal is R6: path is not a directory, or a directory with no
// regular file named "data".
func notABundleRefusal(home, path string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is not a Quicken for Mac file (expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		homepath.Abbreviate(home, path))}
}

// unreadableRefusal is R7/R7b: path could not be statted or opened for a
// reason other than not existing.
func unreadableRefusal(home, path string, cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read %s: %s; allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions",
		homepath.Abbreviate(home, path), osReason(cause))}
}

// osReason returns the innermost *fs.PathError's OS-level message, the exact
// text quarry's unreadable-file refusals quote.
func osReason(err error) string {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	return err.Error()
}
