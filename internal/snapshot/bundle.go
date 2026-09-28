package snapshot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// errBundleMissingData signals that a candidate bundle has no readable
// regular file named "data".
var errBundleMissingData = errors.New("bundle has no data file")

// errNotOpenInQuicken signals a WAL-formatted header with no live -wal sibling: written in WAL mode once, but nothing has it open now.
var errNotOpenInQuicken = errors.New("database has no write-ahead log")

// walFormatByte is the SQLite header's file-format-version value (offsets 18-19) that journal_mode=WAL leaves behind.
const walFormatByte = 2

// ResolveBundlePath expands and resolves path (a --quicken value) against
// home to an absolute bundle directory. It returns a RefusalError unless the
// result exists, does not end ".qdf"/".QDF", is a directory, and contains a
// readable regular file named "data".
func ResolveBundlePath(home, path string) (string, error) {
	abs, err := filepath.Abs(homepath.Expand(home, path))
	if err != nil {
		// unreachable: on darwin os.Getwd succeeds after the working directory is removed; Linux exercises it via Test_ResolveBundlePath_returns_an_error_when_the_working_directory_no_longer_exists
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
		switch {
		case errors.Is(err, errBundleMissingData):
			return "", notABundleRefusal(home, abs)
		case errors.Is(err, errNotOpenInQuicken):
			return "", notOpenInQuickenRefusal(home, abs)
		default:
			return "", unreadableRefusal(home, filepath.Join(abs, "data"), err)
		}
	}

	return abs, nil
}

// validateData confirms bundlePath has a readable regular file named
// "data" that is not a closed WAL-formatted database (errNotOpenInQuicken).
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
	defer func() { _ = f.Close() }()

	// Read the header through this same open, before any SQLite connection
	// touches data: opening it a second time would risk racing a real
	// Quicken write between the two opens.
	// A short read leaves n < len(header), which just fails the check below.
	var header [20]byte
	n, _ := io.ReadFull(f, header[:])
	if n == len(header) && header[18] == walFormatByte && header[19] == walFormatByte {
		if _, err := os.Stat(dataPath + "-wal"); errors.Is(err, fs.ErrNotExist) {
			return errNotOpenInQuicken
		}
	}
	return nil
}

// notABundleRefusal reports that path is not a directory, or a directory
// with no regular file named "data".
func notABundleRefusal(home, path string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is not a Quicken for Mac file (expected a .quicken bundle containing a data file); pass the .quicken bundle with --quicken <path>",
		homepath.Abbreviate(home, path))}
}

// notOpenInQuickenRefusal reports that path's data file header reports WAL
// format with no live -wal file, so Quicken does not currently have it
// open.
func notOpenInQuickenRefusal(home, path string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is not open in Quicken (its database has no write-ahead log); open it in Quicken, then run quarry sync again",
		homepath.Abbreviate(home, path))}
}

// unreadableRefusal reports that path could not be statted or opened for a
// reason other than not existing.
func unreadableRefusal(home, path string, cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read %s: %s; allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions",
		homepath.Abbreviate(home, path), causeText(cause))}
}
