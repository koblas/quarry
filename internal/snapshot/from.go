package snapshot

import (
	"cmp"
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

// ImportFrom rebuilds the store from an earlier snapshot, named by ID or by .sqlite path, without touching
// Quicken or writing a new snapshot; a Server with WithAutoPrune then deletes old ones as SyncAndImport does.
// An ID resolves to the file quarry snapshots lists for it. It refuses when from does not resolve to a usable
// snapshot, and returns a MismatchError when the current reference finds a missing table or column.
func (s *Server) ImportFrom(ctx context.Context, from string) (Outcome, error) {
	if s.reference == nil {
		return Outcome{}, errNoReference
	}
	snapshotPath, manifestPath, err := locateFrom(s.home, s.snapshotDir, from)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, err)
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
	accounts, actual, err := inspectContent(ctx, snapshotPath, s.busyTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return Outcome{}, InterruptedRefusal()
		}
		return Outcome{}, fromContentRefusal(s.home, snapshotPath, err)
	}

	manifest := Manifest{
		Snapshot: Info{
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

// isPathForm reports whether a --from value names a file: it has a "/" or ends in .sqlite, in any letter case.
func isPathForm(value string) bool {
	return strings.Contains(value, "/") || sqliteExtension.MatchString(value)
}

// locateFrom maps a --from value to the snapshot file and manifest it names, or the refusal for one it
// cannot name. The manifest path is the on-disk name when one exists, else the lowercase name that reads as absent.
func locateFrom(home, snapshotDir, value string) (string, string, error) {
	if isPathForm(value) {
		return locateByPath(home, snapshotDir, value)
	}
	return locateByID(home, snapshotDir, value)
}

// locateByID lists snapshotDir once and takes the file and manifest selectFolder chose for id, so --from and
// quarry snapshots agree on which file is id. An unlistable folder is a refusal, never a lowercase guess.
func locateByID(home, snapshotDir, id string) (string, string, error) {
	dirEntries, err := os.ReadDir(snapshotDir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", idNotFoundRefusal(home, snapshotDir, id)
	}
	if err != nil {
		return "", "", folderUnreadableRefusal(home, snapshotDir, err)
	}
	chosen, ok := selectFolder(dirEntries).snapshot(id)
	if !ok {
		return "", "", idNotFoundRefusal(home, snapshotDir, id)
	}
	manifest := cmp.Or(chosen.manifest, id+".json")
	return filepath.Join(snapshotDir, chosen.entry.Name()), filepath.Join(snapshotDir, manifest), nil
}

// locateByPath resolves a path-form value, checks the file exists and is a snapshot candidate, then lists its
// parent folder for the manifest in any letter case.
func locateByPath(home, snapshotDir, value string) (string, string, error) {
	snapshotPath, err := filepath.Abs(homepath.Expand(home, value))
	if err != nil {
		// unreachable: on darwin os.Getwd succeeds after the working directory is removed; Linux exercises it via Test_import_from_refuses_a_relative_path_when_the_working_directory_no_longer_exists
		return "", "", fmt.Errorf("resolve snapshot path %s: %w", value, err)
	}
	if err := fromPathRefusal(home, snapshotDir, snapshotPath); err != nil {
		return "", "", err
	}
	folder, stem := filepath.Dir(snapshotPath), ID(snapshotPath)
	dirEntries, err := os.ReadDir(folder)
	if err != nil {
		return "", "", folderUnreadableRefusal(home, folder, err)
	}
	return snapshotPath, filepath.Join(folder, cmp.Or(selectManifest(dirEntries, stem), stem+".json")), nil
}

// fromPathRefusal reports whether snapshotPath names a usable file at all,
// before any manifest or content read is attempted.
func fromPathRefusal(home, snapshotDir, snapshotPath string) error {
	info, err := os.Stat(snapshotPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return pathNotFoundRefusal(home, snapshotPath)
	case err != nil:
		return fromUnreadableRefusal(home, snapshotPath, err)
	case info.IsDir() && strings.EqualFold(filepath.Ext(snapshotPath), ".quicken"):
		return quickenBundleRefusal(home, snapshotPath)
	case !info.Mode().IsRegular():
		return notASnapshotFileRefusal(home, snapshotPath, snapshotDir)
	}
	return nil
}

// pathNotFoundRefusal reports a path-form --from value naming a file that does not exist.
func pathNotFoundRefusal(home, snapshotPath string) error {
	return RefusalError{msg: homepath.Abbreviate(home, snapshotPath) + " does not exist; check the path passed to --from"}
}

// idNotFoundRefusal reports an ID-form --from value naming no snapshot in snapshotDir.
func idNotFoundRefusal(home, snapshotDir, id string) error {
	return RefusalError{msg: fmt.Sprintf(
		"no snapshot %s in %s; run quarry snapshots to list the ones kept", id, homepath.Abbreviate(home, snapshotDir))}
}

// notASnapshotFileRefusal reports that snapshotPath exists but is not a regular file.
func notASnapshotFileRefusal(home, snapshotPath, snapshotDir string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s is not a snapshot file; pass a .sqlite snapshot from %s with --from <snapshot>",
		homepath.Abbreviate(home, snapshotPath), homepath.Abbreviate(home, snapshotDir))}
}

// quickenBundleRefusal reports that snapshotPath is a Quicken bundle, not a snapshot.
func quickenBundleRefusal(home, snapshotPath string) error {
	return RefusalError{msg: homepath.Abbreviate(home, snapshotPath) + " is a Quicken file, not a snapshot; pass it with --quicken <path>, or pass a snapshot with --from <snapshot>"}
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

// notSnapshotReason is the closed set of causes a file can fail to be a
// usable snapshot for: only the classifiers below select one, never a computed error string.
type notSnapshotReason string

const (
	reasonNoManifest      notSnapshotReason = "no .json manifest next to it"
	reasonManifestNotJSON notSnapshotReason = "its manifest is not readable JSON"
	reasonNotSQLite       notSnapshotReason = "not a SQLite database"
	reasonNoAccountsTable notSnapshotReason = "it has no accounts table"
	reasonNoAccounts      notSnapshotReason = "it has no accounts"
)

// notSnapshotRefusal reports that snapshotPath is not a usable quarry snapshot, for reason.
func notSnapshotRefusal(home, snapshotPath string, err error, reason notSnapshotReason) error {
	return causedRefusalError{
		msg: fmt.Sprintf("%s is not a quarry snapshot (%s); pass a snapshot taken by quarry sync with --from <snapshot>",
			homepath.Abbreviate(home, snapshotPath), reason),
		cause: err,
	}
}

// manifestReadRefusal classifies readManifest's failure: not a usable
// snapshot when the manifest is missing or unparsable, else unreadable, naming the manifest itself.
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

// fromUnreadableRefusal reports an OS-level fault reading path, naming
// whichever file — snapshot or manifest — failed, with the OS reason verbatim.
func fromUnreadableRefusal(home, path string, err error) error {
	return causedRefusalError{
		msg: fmt.Sprintf("cannot read %s: %s; check the file's permissions",
			homepath.Abbreviate(home, path), causeText(err)),
		cause: err,
	}
}

// fromContentRefusal classifies inspectContent's failure once the hash is
// verified: not a usable snapshot for an open, integrity or accounts fault; else unreadable.
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
		return causedRefusalError{
			msg: fmt.Sprintf("cannot read %s: %s; take a new snapshot with quarry sync",
				homepath.Abbreviate(home, snapshotPath), causeText(err)),
			cause: err,
		}
	}
}

// changedSnapshotRefusal reports a snapshot whose SHA-256 no longer matches its manifest.
func changedSnapshotRefusal(home, snapshotPath string) error {
	return RefusalError{msg: homepath.Abbreviate(home, snapshotPath) + " has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync"}
}
