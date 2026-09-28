package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/store"
)

// errNoImporter is SyncAndImport's error when the Server has no Importer configured.
var errNoImporter = errors.New("no importer configured")

// Outcome is what SyncAndImport returns: the snapshot's Manifest and the
// import's store.Result, when one ran. Store is nil on a schema mismatch
// or Sync failure. StoreExisted (meaningful only when Store != nil &&
// !Store.Built) decides the V1 block's NOT REBUILT vs NOT BUILT line.
type Outcome struct {
	Manifest     Manifest
	Store        *store.Result
	StoreExisted bool
}

// Warnings returns every warning o carries, without the "quarry: warning: "
// prefix: the manifest's own, then the one-sided-transfer warning when a
// built store kept one or more legs with no counterpart. An unbuilt store
// kept nothing, so it adds no warning.
func (o Outcome) Warnings() []string {
	warnings := o.Manifest.Warnings
	if o.Store == nil || !o.Store.Built {
		return warnings
	}
	if n := len(o.Store.Validation.Transfers.OneSided); n > 0 {
		warnings = append(slices.Clip(warnings), oneSidedWarning(n))
	}
	return warnings
}

// oneSidedWarning renders the warning for n one-sided transfers, singular
// at n == 1.
func oneSidedWarning(n int) string {
	if n == 1 {
		return "1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer"
	}
	return fmt.Sprintf("%d transfers have no matching transaction in another account; quarry keeps them as one-sided transfers", n)
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
// importer's error, so errors.As still reaches it. A failed check (V1) sets
// Store to the unbuilt result, so a later stdout write against it still
// gets the O1b refusal.
func (s *Server) SyncAndImport(ctx context.Context, bundlePath string) (Outcome, error) {
	manifest, err := s.Sync(ctx, bundlePath)
	if err != nil {
		return Outcome{Manifest: manifest}, err
	}

	if s.importer == nil {
		return Outcome{Manifest: manifest}, errNoImporter
	}

	result, err := s.importer.Import(ctx, store.SnapshotRef{
		Path: manifest.Snapshot.Path, SHA256: manifest.Snapshot.SHA256, SchemaFingerprint: manifest.Schema.Fingerprint,
	})
	if err != nil {
		if errors.Is(err, store.ErrValidationFailed) {
			// Populated here even though Import's own Result contract leaves it empty on a failed build.
			result.Path = s.storePath
			return Outcome{Manifest: manifest, Store: &result, StoreExisted: s.previousStoreExists()},
				s.validationFailedRefusal(manifest, result.Validation, err)
		}
		return Outcome{Manifest: manifest}, s.storeBuildRefusal(manifest, err)
	}

	return Outcome{Manifest: manifest, Store: &result}, nil
}

// previousStoreExists reports whether a store was already at s.storePath;
// a stat error other than "not found" is treated as existed.
func (s *Server) previousStoreExists() bool {
	_, err := os.Stat(s.storePath)
	return !errors.Is(err, fs.ErrNotExist)
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

// validationFailedRefusal reports V1: a build reached the balance or
// split-sum gate and one or more checks failed.
func (s *Server) validationFailedRefusal(manifest Manifest, v store.Validation, cause error) error {
	var clauses []string
	if n := len(v.Balances.Mismatched); n > 0 {
		clauses = append(clauses, balanceMismatchClause(n, v.Balances.Checked))
	}
	if n := len(v.Splits.Mismatched); n > 0 {
		clauses = append(clauses, splitMismatchClause(n))
	}
	return storeRefusalError{
		msg: fmt.Sprintf("validation failed: %s; %s was not changed; each difference is listed on stdout; "+
			"fix the account in Quicken and run quarry sync, or run quarry sync --from %s after updating quarry",
			strings.Join(clauses, " and "), homepath.Abbreviate(s.home, s.storePath), snapshotID(manifest.Snapshot.Path)),
		cause: cause,
	}
}

// balanceMismatchClause renders n mismatched of checked accounts: the noun
// agrees with checked, the verb with n.
func balanceMismatchClause(n, checked int) string {
	noun := "accounts"
	if checked == 1 {
		noun = "account"
	}
	verb := "do not match"
	if n == 1 {
		verb = "does not match"
	}
	return fmt.Sprintf("%d of %d %s %s Quicken's last reconciled balance", n, checked, noun, verb)
}

// splitMismatchClause renders n mismatched transactions, singular at n == 1.
func splitMismatchClause(n int) string {
	if n == 1 {
		return "1 transaction does not equal the sum of its splits"
	}
	return fmt.Sprintf("%d transactions do not equal the sum of their splits", n)
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
