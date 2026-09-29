package snapshot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
)

// DiscoverBundle finds the sole .quicken bundle across home's Documents
// folder and Quicken's own Documents folder, deduped by identity, then
// resolves it like ResolveBundlePath. It returns a RefusalError when the
// pool holds zero, two or more, or a location cannot be read.
func DiscoverBundle(home string) (string, error) {
	documentsDir := filepath.Join(home, "Documents")
	locations := []struct {
		dir     string
		missing func(error) bool
		refusal func(cause error) error
	}{
		{documentsDir, documentsMissing, documentsUnreadableRefusal},
		{quickenDocumentsDir(home), quickenDocumentsMissing, quickenDocumentsUnreadableRefusal},
	}

	var candidates []bundleCandidate
	for _, loc := range locations {
		found, err := scanForBundles(home, loc.dir, loc.missing, loc.refusal)
		if err != nil {
			return "", err
		}
		candidates = append(candidates, found...)
	}

	candidates = dedupeByIdentity(candidates)

	switch len(candidates) {
	case 0:
		return "", noBundleFoundRefusal()
	case 1:
		return ResolveBundlePath(home, candidates[0].path)
	default:
		return "", multipleQuickenBundlesRefusal(home, candidates)
	}
}

// quickenDocumentsDir is Quicken Classic for Mac's own Documents folder
// under home, distinct from the sibling Backups folder quarry never reads.
func quickenDocumentsDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "Quicken", "Documents")
}

// documentsMissing reports whether a ReadDir error on ~/Documents means the
// folder doesn't exist yet.
func documentsMissing(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// quickenDocumentsMissing also treats ENOTDIR — the folder or an ancestor
// being a plain file — as the location simply not existing.
func quickenDocumentsMissing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// bundleCandidate is a .quicken directory found during a scan, with the
// os.Stat result used for both the directory check and identity dedupe.
type bundleCandidate struct {
	path string
	info os.FileInfo
}

// scanForBundles lists dir's .quicken candidates, treating dir as absent
// when missing says so; any other failure becomes a RefusalError.
func scanForBundles(home, dir string, missing func(error) bool, refusal func(cause error) error) ([]bundleCandidate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if missing(err) {
			return nil, nil
		}
		return nil, refusal(err)
	}

	var candidates []bundleCandidate
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".quicken") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // a dangling symlink, not a candidate
			}
			return nil, unreadableRefusal(home, path, err)
		}
		if !info.IsDir() {
			continue
		}
		candidates = append(candidates, bundleCandidate{path: path, info: info})
	}
	return candidates, nil
}

// dedupeByIdentity keeps the first candidate per distinct os.SameFile
// identity, preserving scan order.
func dedupeByIdentity(candidates []bundleCandidate) []bundleCandidate {
	var kept []bundleCandidate
	for _, c := range candidates {
		duplicate := false
		for _, k := range kept {
			if os.SameFile(k.info, c.info) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			kept = append(kept, c)
		}
	}
	return kept
}

// noBundleFoundRefusal reports that no .quicken bundle was found in either
// location.
func noBundleFoundRefusal() error {
	return RefusalError{msg: "no .quicken file found in ~/Documents or " +
		"~/Library/Application Support/Quicken/Documents; pass one with --quicken <path>"}
}

// multipleQuickenBundlesRefusal reports every distinct bundle found, as full
// ~-abbreviated paths in bytewise order.
func multipleQuickenBundlesRefusal(home string, candidates []bundleCandidate) error {
	paths := make([]string, len(candidates))
	for i, c := range candidates {
		paths[i] = homepath.Abbreviate(home, c.path)
	}
	sort.Strings(paths)
	return RefusalError{msg: fmt.Sprintf(
		"found %s .quicken files (%s); choose one with --quicken <path>",
		humanize.Thousands(len(paths)), strings.Join(paths, ", "))}
}

// documentsUnreadableRefusal reports that ~/Documents could not be read.
func documentsUnreadableRefusal(cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read ~/Documents: %s; allow your terminal to access the Documents folder in "+
			"System Settings > Privacy & Security > Files and Folders, or pass --quicken <path>",
		causeText(cause))}
}

// quickenDocumentsUnreadableRefusal reports that Quicken's own Documents
// folder could not be read.
func quickenDocumentsUnreadableRefusal(cause error) error {
	return RefusalError{msg: fmt.Sprintf(
		"cannot read ~/Library/Application Support/Quicken/Documents: %s; "+
			"check the folder's permissions, or pass --quicken <path>",
		causeText(cause))}
}
