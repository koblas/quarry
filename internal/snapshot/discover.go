package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverBundle finds the sole .quicken bundle directory (suffix matched
// case-insensitively, dotfiles ignored, symlinks followed) at the top level
// of home's Documents folder, then resolves it exactly as ResolveBundlePath
// would. It returns a RefusalError when Documents holds none, more than
// one, or cannot be read.
func DiscoverBundle(home string) (string, error) {
	documentsDir := filepath.Join(home, "Documents")

	entries, err := os.ReadDir(documentsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", noBundleFoundRefusal()
		}
		return "", documentsUnreadableRefusal(home, err)
	}

	// os.ReadDir already returns entries sorted by filename, so the
	// filtered names below need no further sort for R2's bytewise order.
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".quicken") {
			continue
		}
		info, err := os.Stat(filepath.Join(documentsDir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // a dangling symlink, not a candidate
			}
			return "", unreadableRefusal(home, filepath.Join(documentsDir, name), err)
		}
		if !info.IsDir() {
			continue
		}
		names = append(names, name)
	}

	switch len(names) {
	case 0:
		return "", noBundleFoundRefusal()
	case 1:
		return ResolveBundlePath(home, filepath.Join(documentsDir, names[0]))
	default:
		return "", multipleQuickenBundlesRefusal(names)
	}
}

// noBundleFoundRefusal is R1: no .quicken bundle found in ~/Documents.
func noBundleFoundRefusal() error {
	return RefusalError{msg: "no .quicken file found in ~/Documents; pass one with --quicken <path>"}
}

// multipleQuickenBundlesRefusal is R2: more than one .quicken bundle found;
// names must already be sorted bytewise.
func multipleQuickenBundlesRefusal(names []string) error {
	return RefusalError{msg: fmt.Sprintf(
		"found %d .quicken files in ~/Documents (%s); choose one with --quicken <path>",
		len(names), strings.Join(names, ", "))}
}

// documentsUnreadableRefusal is R3: ~/Documents could not be read for a
// reason other than not existing.
func documentsUnreadableRefusal(home string, cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read ~/Documents: %s; allow your terminal to access the Documents folder in "+
			"System Settings > Privacy & Security > Files and Folders, or pass --quicken <path>",
		causeText(cause))}
}
