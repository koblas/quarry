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

// configFileShown is the config file as user copy names it.
const configFileShown = "~/Library/Application Support/quarry/config.toml"

// notABundleText is the head of the not-a-bundle refusal; each origin appends its remedy.
const notABundleText = " is not a Quicken for Mac file (expected a .quicken bundle containing a data file); "

// BundleChoice is where a sync's bundle path came from: the --quicken value
// (blank when not given) and quicken.path from the config file (blank when
// unset).
type BundleChoice struct {
	Flag, Configured string
}

// ResolveBundle returns the bundle a plain sync snapshots: the flag path when
// given, else the configured path, else the one bundle DiscoverBundle finds.
// The configured path is not examined when a flag path is given. It returns a
// RefusalError whose copy names the origin of a path that fails.
func ResolveBundle(home string, choice BundleChoice) (string, error) {
	switch {
	case choice.Flag != "":
		return ResolveBundlePath(home, choice.Flag)
	case choice.Configured != "":
		return resolveBundlePath(home, choice.Configured, configuredRefusals)
	default:
		return DiscoverBundle(home)
	}
}

// ResolveBundlePath expands and resolves path (a --quicken value) against
// home to an absolute bundle directory. It returns a RefusalError unless the
// result exists, does not end ".qdf"/".QDF", is a directory, and contains a
// readable regular file named "data".
func ResolveBundlePath(home, path string) (string, error) {
	return resolveBundlePath(home, path, flagRefusals)
}

// originRefusals are the two refusals whose remedy depends on where the path
// came from; every other refusal is the same for any origin.
type originRefusals struct {
	notExist   func(home, abs string) error
	notABundle func(home, abs string) error
}

var flagRefusals = originRefusals{
	notExist: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + " does not exist; check the path passed to --quicken"}
	},
	notABundle: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + notABundleText + "pass the .quicken bundle with --quicken <path>"}
	},
}

var configuredRefusals = originRefusals{
	notExist: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + " does not exist; check quicken.path in " + configFileShown + ", or pass the file with --quicken <path>"}
	},
	notABundle: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + notABundleText + "set quicken.path in " + configFileShown + " to the .quicken bundle"}
	},
}

// discoveredRefusals are for the sole bundle discovery found: no flag was given and no path configured, so either remedy applies.
var discoveredRefusals = originRefusals{
	notExist: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + " does not exist; pass one with --quicken <path> or set quicken.path in " + configFileShown}
	},
	notABundle: func(home, abs string) error {
		return RefusalError{msg: homepath.Abbreviate(home, abs) + notABundleText + "pass the .quicken bundle with --quicken <path> or set quicken.path in " + configFileShown}
	},
}

// resolveBundlePath is ResolveBundlePath with the refusals of path's origin.
func resolveBundlePath(home, path string, origin originRefusals) (string, error) {
	abs, err := filepath.Abs(homepath.Expand(home, path))
	if err != nil {
		// unreachable: on darwin os.Getwd succeeds after the working directory is removed; Linux exercises it via Test_ResolveBundlePath_returns_an_error_when_the_working_directory_no_longer_exists
		return "", fmt.Errorf("resolve bundle path %s: %w", path, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", origin.notExist(home, abs)
		}
		return "", unreadableRefusal(home, abs, err)
	}

	if strings.EqualFold(filepath.Ext(abs), ".qdf") {
		return "", RefusalError{msg: homepath.Abbreviate(home, abs) + " is a Quicken for Windows file; quarry reads only Quicken Classic for Mac .quicken files"}
	}

	if !info.IsDir() {
		return "", origin.notABundle(home, abs)
	}

	if err := validateData(abs); err != nil {
		switch {
		case errors.Is(err, errBundleMissingData):
			return "", origin.notABundle(home, abs)
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
		return fmt.Errorf("stat data file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errBundleMissingData
	}

	f, err := os.Open(dataPath)
	if err != nil {
		return fmt.Errorf("open data file: %w", err)
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

// notOpenInQuickenRefusal reports that path's data file header reports WAL
// format with no live -wal file, so Quicken does not currently have it
// open.
func notOpenInQuickenRefusal(home, path string) error {
	return RefusalError{msg: homepath.Abbreviate(home, path) + " is not open in Quicken (its database has no write-ahead log); open it in Quicken, then run quarry sync again"}
}

// unreadableRefusal reports that path could not be statted or opened for a
// reason other than not existing.
func unreadableRefusal(home, path string, cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read %s: %s; allow your terminal to access the folder in System Settings > Privacy & Security, or check the file's permissions",
		homepath.Abbreviate(home, path), causeText(cause))}
}
