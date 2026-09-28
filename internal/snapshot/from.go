package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
)

// ImportFrom rebuilds the store from an earlier snapshot, named by ID or by
// .sqlite path, without touching Quicken or writing the snapshots directory.
// It refuses when the file's SHA-256 no longer matches its manifest, and
// returns a MismatchError when the current reference finds a missing table or column.
func (s *Server) ImportFrom(ctx context.Context, from string) (Outcome, error) {
	if s.reference == nil {
		return Outcome{}, errNoReference
	}
	snapshotPath, manifestPath, err := resolveFrom(s.home, s.snapshotDir, from)
	if err != nil {
		// unreachable: filepath.Abs fails only when os.Getwd does, which darwin never reports once the working directory exists at startup.
		return Outcome{}, err
	}

	recorded, err := readManifest(manifestPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, fromRefusal(s.home, snapshotPath, err))
	}
	// Hash before opening as SQLite: a changed file is reported as changed even when it no longer opens.
	size, sum, err := hashFile(snapshotPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, fromRefusal(s.home, snapshotPath, err))
	}
	if sum != recorded.Snapshot.SHA256 {
		return Outcome{}, changedSnapshotRefusal(s.home, snapshotPath)
	}
	accounts, actual, err := inspectContent(ctx, snapshotPath)
	if err != nil {
		return Outcome{}, FailureOutcome(ctx, fromRefusal(s.home, snapshotPath, err))
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
// value containing "/" or ending ".sqlite" is a path, anything else an ID in snapshotDir.
func resolveFrom(home, snapshotDir, value string) (snapshotPath, manifestPath string, err error) {
	snapshotPath = filepath.Join(snapshotDir, value+".sqlite")
	if strings.Contains(value, "/") || strings.HasSuffix(value, ".sqlite") {
		snapshotPath, err = filepath.Abs(homepath.Expand(home, value))
		if err != nil {
			// unreachable: filepath.Abs fails only when os.Getwd does, which darwin never reports once the working directory exists at startup.
			return "", "", fmt.Errorf("resolve snapshot path %s: %w", value, err)
		}
	}
	return snapshotPath, strings.TrimSuffix(snapshotPath, ".sqlite") + ".json", nil
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

// fromRefusal reports a --from snapshot that cannot be read, decoded or
// inspected, wrapping err so errors.Is still reaches the cause.
func fromRefusal(home, snapshotPath string, err error) error {
	reason := causeText(err)
	switch {
	case errors.Is(err, errNoAccountsTable):
		reason = "it has no accounts table"
	case errors.Is(err, errNoAccounts):
		reason = "it has no accounts"
	}
	return causedRefusal{
		msg: fmt.Sprintf("%s is not a quarry snapshot (%s); pass a snapshot taken by quarry sync with --from <snapshot>",
			homepath.Abbreviate(home, snapshotPath), reason),
		cause: err,
	}
}

// changedSnapshotRefusal reports a snapshot whose SHA-256 no longer matches its manifest.
func changedSnapshotRefusal(home, snapshotPath string) error {
	return RefusalError{msg: fmt.Sprintf(
		"%s has changed since quarry took it (its SHA-256 does not match its manifest); take a new snapshot with quarry sync",
		homepath.Abbreviate(home, snapshotPath))}
}
