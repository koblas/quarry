package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/sqlite"
)

// ImportFrom rebuilds the store from an earlier snapshot, named by ID or by
// .sqlite path, without touching Quicken or writing the snapshots directory.
// It refuses when from does not resolve to a usable snapshot file, when the
// file's SHA-256 no longer matches its manifest, and returns a
// MismatchError when the current reference finds a missing table or column.
func (s *Server) ImportFrom(ctx context.Context, from string) (Outcome, error) {
	if s.reference == nil {
		return Outcome{}, errNoReference
	}
	snapshotPath, manifestPath, isPath, err := resolveFrom(s.home, s.snapshotDir, from)
	if err != nil {
		// unreachable: on darwin os.Getwd succeeds after the working directory is removed; Linux exercises it via Test_import_from_refuses_a_relative_path_when_the_working_directory_no_longer_exists
		return Outcome{}, err
	}

	if refusal := fromPathRefusal(s.home, s.snapshotDir, snapshotPath, isPath); refusal != nil {
		return Outcome{}, FailureOutcome(ctx, refusal)
	}

	recorded, err := readManifest(manifestPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, manifestReadRefusal(s.home, snapshotPath, manifestPath, err))
	}
	// Hash before opening as SQLite: a changed file is reported as changed even when it no longer opens.
	size, sum, err := hashFile(snapshotPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, fromUnreadableRefusal(s.home, snapshotPath, err))
	}
	if sum != recorded.Snapshot.SHA256 {
		return Outcome{}, changedSnapshotRefusal(s.home, snapshotPath)
	}
	accounts, actual, err := inspectContent(ctx, snapshotPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, fromContentRefusal(s.home, snapshotPath, err))
	}

	manifest := Manifest{
		Snapshot: SnapshotInfo{
			Path:     snapshotPath,
			Manifest: manifestPath,
			Source:   recorded.Snapshot.Source,
			TakenAt:  recorded.Snapshot.TakenAt,
			Bytes:    size,
			SHA256:   sum,
			Accounts: accounts,
		},
		Schema: s.compareSchema(actual),
	}
	manifest.Warnings = schemaWarnings(recorded.Snapshot.Source, manifestPath, manifest.Schema)
	if !manifest.Schema.Verified {
		return Outcome{Manifest: manifest}, fromMismatchError(snapshotPath, recorded.Snapshot.Source, manifest.Schema)
	}
	return s.importVerified(ctx, manifest)
}

// resolveFrom maps a --from value to its snapshot and manifest paths: a
// value with "/" or ending ".sqlite" is a path (isPath true), else an ID in snapshotDir.
func resolveFrom(home, snapshotDir, value string) (snapshotPath, manifestPath string, isPath bool, err error) {
	snapshotPath = filepath.Join(snapshotDir, value+".sqlite")
	if strings.Contains(value, "/") || strings.HasSuffix(value, ".sqlite") {
		isPath = true
		snapshotPath, err = filepath.Abs(homepath.Expand(home, value))
		if err != nil {
			// unreachable: on darwin os.Getwd succeeds after the working directory is removed; Linux exercises it via Test_import_from_refuses_a_relative_path_when_the_working_directory_no_longer_exists
			return "", "", false, fmt.Errorf("resolve snapshot path %s: %w", value, err)
		}
	}
	return snapshotPath, strings.TrimSuffix(snapshotPath, ".sqlite") + ".json", isPath, nil
}

// fromPathRefusal reports F1/F1b/F2/F2b: whether snapshotPath names a
// usable file at all, before any manifest or content read is attempted.
func fromPathRefusal(home, snapshotDir, snapshotPath string, isPath bool) error {
	info, err := os.Stat(snapshotPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if isPath {
			return pathNotFoundRefusal(home, snapshotPath)
		}
		return idNotFoundRefusal(home, snapshotDir, snapshotID(snapshotPath))
	case err != nil:
		return fromUnreadableRefusal(home, snapshotPath, err)
	case info.IsDir() && strings.EqualFold(filepath.Ext(snapshotPath), ".quicken"):
		return quickenBundleRefusal(home, snapshotPath)
	case !info.Mode().IsRegular():
		return notASnapshotFileRefusal(home, snapshotPath, snapshotDir)
	}
	return nil
}

// pathNotFoundRefusal reports F1: a path-form --from value naming a file
// that does not exist.
func pathNotFoundRefusal(home, snapshotPath string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s does not exist; check the path passed to --from", homepath.Abbreviate(home, snapshotPath))}
}

// idNotFoundRefusal reports F1b: an ID-form --from value naming no snapshot in snapshotDir.
func idNotFoundRefusal(home, snapshotDir, id string) error {
	return RefusalError{msg: fmt.Sprintf(
		"no snapshot %s in %s; check the ID passed to --from", id, homepath.Abbreviate(home, snapshotDir))}
}

// notASnapshotFileRefusal reports F2: snapshotPath exists but is not a regular file.
func notASnapshotFileRefusal(home, snapshotPath, snapshotDir string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is not a snapshot file; pass a .sqlite snapshot from %s with --from <snapshot>",
		homepath.Abbreviate(home, snapshotPath), homepath.Abbreviate(home, snapshotDir))}
}

// quickenBundleRefusal reports F2b: snapshotPath is a Quicken bundle, not a snapshot.
func quickenBundleRefusal(home, snapshotPath string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is a Quicken file, not a snapshot; pass it with --quicken <path>, or pass a snapshot with --from <snapshot>",
		homepath.Abbreviate(home, snapshotPath))}
}

// readManifest reads and decodes the manifest at path.
func readManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return m, nil
}

// notSnapshotReason is F3's closed set of causes: only the classifiers
// below select one, never a computed error string.
type notSnapshotReason string

const (
	reasonNoManifest      notSnapshotReason = "no .json manifest next to it"
	reasonManifestNotJSON notSnapshotReason = "its manifest is not readable JSON"
	reasonNotSQLite       notSnapshotReason = "not a SQLite database"
	reasonNoAccountsTable notSnapshotReason = "it has no accounts table"
	reasonNoAccounts      notSnapshotReason = "it has no accounts"
)

// notSnapshotRefusal reports F3: snapshotPath is not a usable quarry
// snapshot, for reason.
func notSnapshotRefusal(home, snapshotPath string, err error, reason notSnapshotReason) error {
	return causedRefusal{
		msg: fmt.Sprintf("%s is not a quarry snapshot (%s); pass a snapshot taken by quarry sync with --from <snapshot>",
			homepath.Abbreviate(home, snapshotPath), reason),
		cause: err,
	}
}

// manifestReadRefusal classifies readManifest's failure: F3 when the
// manifest is missing or unparsable, else F4 naming the manifest itself.
func manifestReadRefusal(home, snapshotPath, manifestPath string, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return notSnapshotRefusal(home, snapshotPath, err, reasonNoManifest)
	case errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return notSnapshotRefusal(home, snapshotPath, err, reasonManifestNotJSON)
	default:
		return fromUnreadableRefusal(home, manifestPath, err)
	}
}

// fromUnreadableRefusal reports F4: an OS-level fault reading path, naming
// whichever file — snapshot or manifest — failed, with the OS reason verbatim.
func fromUnreadableRefusal(home, path string, err error) error {
	return causedRefusal{
		msg: fmt.Sprintf("cannot read %s: %s; check the file's permissions",
			homepath.Abbreviate(home, path), causeText(err)),
		cause: err,
	}
}

// fromContentRefusal classifies inspectContent's failure once the hash is
// verified: F3 for an open, integrity or accounts fault; else F4b.
func fromContentRefusal(home, snapshotPath string, err error) error {
	var integrityErr sqlite.IntegrityError
	switch {
	case sqlite.IsNotADB(err), errors.As(err, &integrityErr):
		return notSnapshotRefusal(home, snapshotPath, err, reasonNotSQLite)
	case errors.Is(err, errNoAccountsTable):
		return notSnapshotRefusal(home, snapshotPath, err, reasonNoAccountsTable)
	case errors.Is(err, errNoAccounts):
		return notSnapshotRefusal(home, snapshotPath, err, reasonNoAccounts)
	default:
		// unreachable: inspectContent's remaining query (its Schema read) has no fault seam of its own — every one of its internal error returns is itself marked unreachable in internal/platform/sqlite, and the one live cause (a ctx cancellation mid-read) is reclassified as sync-interrupted by ImportFrom's FailureOutcome before this classifier ever sees it.
		return causedRefusal{
			msg: fmt.Sprintf("cannot read %s: %s; take a new snapshot with quarry sync",
				homepath.Abbreviate(home, snapshotPath), causeText(err)),
			cause: err,
		}
	}
}

// changedSnapshotRefusal reports F5: a snapshot whose SHA-256 no longer matches its manifest.
func changedSnapshotRefusal(home, snapshotPath string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync",
		homepath.Abbreviate(home, snapshotPath))}
}
