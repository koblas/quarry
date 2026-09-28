package snapshot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/store"
)

// errNoImporter is SyncAndImport's error when the Server has no Importer configured.
var errNoImporter = errors.New("no importer configured")

// Outcome is what SyncAndImport returns: the snapshot's Manifest, and the
// store.Result of the import, when one ran. Store is nil when the schema
// check found a mismatch (import skipped) or Sync itself failed.
type Outcome struct {
	Manifest Manifest
	Store    *store.Result
}

// storeRefusalError is SyncAndImport's S3 frame: Error is the refusal text
// alone, while Unwrap preserves the importer's error for errors.As.
type storeRefusalError struct {
	msg   string
	cause error
}

// Error returns the refusal's message verbatim.
func (e storeRefusalError) Error() string { return e.msg }

// Unwrap returns the importer error the refusal wraps.
func (e storeRefusalError) Unwrap() error { return e.cause }

// snapshotID returns the id a refusal names for path: its basename with the
// .sqlite extension removed.
func snapshotID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".sqlite")
}

// SyncAndImport takes a snapshot of bundlePath, then imports it through the
// configured Importer, skipping the import (Store left nil) on a schema
// mismatch or any other Sync failure. An import failure never replaces the
// already-committed snapshot: it comes back as a store refusal wrapping the
// importer's error, so errors.As still reaches it.
func (s *Server) SyncAndImport(ctx context.Context, bundlePath string) (Outcome, error) {
	manifest, err := s.Sync(ctx, bundlePath)
	if err != nil {
		return Outcome{Manifest: manifest}, err
	}

	if s.importer == nil {
		return Outcome{Manifest: manifest}, errNoImporter
	}

	result, err := s.importer.Import(ctx, manifest.Snapshot.Path)
	if err != nil {
		return Outcome{Manifest: manifest}, s.storeBuildRefusal(manifest, err)
	}

	return Outcome{Manifest: manifest, Store: &result}, nil
}

// storeBuildRefusal reports a committed snapshot's import failure, wrapping
// err so errors.As still reaches it.
func (s *Server) storeBuildRefusal(manifest Manifest, err error) error {
	return storeRefusalError{
		msg: fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s",
			homepath.Abbreviate(s.home, filepath.Dir(s.storePath)), causeText(err), snapshotID(manifest.Snapshot.Path)),
		cause: err,
	}
}

// StdoutWriteRefusal reports that o's result could not be written to
// stdout. Before the store was built (o.Store == nil) it names the
// already-committed snapshot and its manifest; once the build was reached,
// it points at --from --json instead, since the manifest alone no longer
// carries the store result.
func (o Outcome) StdoutWriteRefusal(home string, err error) error {
	if o.Store == nil {
		return fmt.Errorf(
			"cannot write the result to stdout: %s; the snapshot is kept at %s and its .json manifest holds the full result",
			err, homepath.Abbreviate(home, o.Manifest.Snapshot.Path))
	}
	return fmt.Errorf(
		"cannot write the result to stdout: %s; run quarry sync --from %s --json to see it again",
		err, snapshotID(o.Manifest.Snapshot.Path))
}
