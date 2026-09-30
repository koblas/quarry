package snapshot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/store"
)

// errImportNotWired is SyncAndImport's error when the Server has no Importer or StoreProbe configured.
var errImportNotWired = errors.New("no importer or store probe configured")

// Outcome is what SyncAndImport returns: the snapshot's Manifest and the
// import's store.Result, when one ran. Store is nil on a schema mismatch
// or Sync failure. StoreExisted (meaningful only when Store != nil &&
// !Store.Built) decides the V1 block's NOT REBUILT vs NOT BUILT line.
type Outcome struct {
	Manifest     Manifest
	Store        *store.Result
	StoreExisted bool

	// historyWarning is the warning for a built store whose previous import
	// history could not be carried forward; empty when it was carried.
	historyWarning string
}

// Warnings returns every warning o carries, without the "quarry: warning: "
// prefix: the manifest's own, then the one-sided-transfer warning when a
// built store kept one or more legs with no counterpart, then the warning
// that import history restarted. An unbuilt store adds none of them.
func (o Outcome) Warnings() []string {
	warnings := o.Manifest.Warnings
	if o.Store == nil || !o.Store.Built {
		return warnings
	}
	if n := len(o.Store.Validation.Transfers.OneSided); n > 0 {
		warnings = append(slices.Clip(warnings), oneSidedWarning(n))
	}
	if o.historyWarning != "" {
		warnings = append(slices.Clip(warnings), o.historyWarning)
	}
	return warnings
}

// historyRestartWarning renders the warning that the previous store's import
// history could not be carried forward, for a reason from UnreadableReason.
func historyRestartWarning(reason string) string {
	return "cannot carry import history forward from the previous store (" + reason + "); import_runs starts again with this sync"
}

// oneSidedWarning renders the warning for n one-sided transfers, singular
// at n == 1.
func oneSidedWarning(n int) string {
	if n == 1 {
		return "1 transfer has no matching transaction in another account; quarry keeps it as a one-sided transfer"
	}
	return humanize.Thousands(n) + " transfers have no matching transaction in another account; quarry keeps them as one-sided transfers"
}

// causedRefusalError is a refusal that keeps its cause: Error is the refusal
// text alone, while Unwrap preserves the cause for errors.Is and errors.As.
type causedRefusalError struct {
	msg   string
	cause error
}

// Error returns the refusal's message verbatim.
func (e causedRefusalError) Error() string { return e.msg }

// Unwrap returns the error the refusal wraps.
func (e causedRefusalError) Unwrap() error { return e.cause }

// ID returns the id a snapshot goes by for its .sqlite path: the base name
// with the extension removed.
func ID(path string) string {
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

	return s.importVerified(ctx, manifest)
}

// importVerified imports the snapshot manifest describes, once its hash
// and schema are verified, mapping a failure to its V1 or store refusal.
func (s *Server) importVerified(ctx context.Context, manifest Manifest) (Outcome, error) {
	if s.importer == nil || s.storeProbe == nil {
		return Outcome{Manifest: manifest}, errImportNotWired
	}

	result, err := s.importer.Import(ctx, store.SnapshotRef{
		Path: manifest.Snapshot.Path, SHA256: manifest.Snapshot.SHA256, SchemaFingerprint: manifest.Schema.Fingerprint,
		TakenAt: recordedTakenAt(manifest.Snapshot.TakenAt), Source: manifest.Snapshot.Source,
	})
	if err != nil {
		if errors.Is(err, store.ErrValidationFailed) {
			// Populated here even though Import's own Result contract leaves it empty on a failed build.
			result.Path = s.storeProbe.Path()
			return Outcome{Manifest: manifest, Store: &result, StoreExisted: s.storeProbe.Exists()},
				s.validationFailedRefusal(manifest, result.Validation, err)
		}
		return Outcome{Manifest: manifest}, s.importFailureRefusal(ctx, manifest, err)
	}

	outcome := Outcome{Manifest: manifest, Store: &result}
	if fault := result.HistoryFault; fault != nil {
		outcome.historyWarning = historyRestartWarning(fault.UnreadableReason(homepath.Abbreviate(s.home, s.storeProbe.Path())))
	}
	return outcome, nil
}

// recordedTakenAt parses a manifest's taken_at into UTC; an unparseable
// value gives the zero time, which the store records as NULL.
func recordedTakenAt(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// importFailureRefusal reports a committed snapshot's non-V1 import
// failure, wrapping err so errors.As still reaches it: I2 once ctx has
// ended, else S4, S1, S2 by the store sentinel err matches, else S3.
func (s *Server) importFailureRefusal(ctx context.Context, manifest Manifest, err error) error {
	id := ID(manifest.Snapshot.Path)
	probed := s.storeProbe.Path()
	storeDir := homepath.Abbreviate(s.home, filepath.Dir(probed))
	storePath := homepath.Abbreviate(s.home, probed)
	var msg string
	switch {
	case ctx.Err() != nil:
		msg = fmt.Sprintf("sync interrupted while building the store; %s was not changed; run quarry sync --from %s to rebuild it",
			storePath, id)
	case errors.Is(err, store.ErrUnmappable):
		msg = fmt.Sprintf("cannot import snapshot %s: %s; %s was not changed; run quarry sync --from %s once quarry supports it",
			id, causeText(err), storePath, id)
	case errors.Is(err, store.ErrStoreNotWritable):
		msg = fmt.Sprintf("cannot write to %s: permission denied; make the directory writable by your user", storeDir)
	case errors.Is(err, store.ErrDiskFull):
		msg = fmt.Sprintf("cannot write the store to %s: no space left on device; free disk space, then run quarry sync --from %s",
			storeDir, id)
	default:
		msg = fmt.Sprintf("cannot build the store in %s: %s; run quarry sync --from %s", storeDir, causeText(err), id)
	}
	return causedRefusalError{msg: msg, cause: err}
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
	return causedRefusalError{
		msg: fmt.Sprintf("validation failed: %s; %s was not changed; each difference is listed on stdout; "+
			"fix the account in Quicken and run quarry sync, or run quarry sync --from %s after updating quarry",
			strings.Join(clauses, " and "), homepath.Abbreviate(s.home, s.storeProbe.Path()), ID(manifest.Snapshot.Path)),
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
	return fmt.Sprintf("%s of %s %s %s Quicken's last reconciled balance", humanize.Thousands(n), humanize.Thousands(checked), noun, verb)
}

// splitMismatchClause renders n mismatched transactions, singular at n == 1.
func splitMismatchClause(n int) string {
	if n == 1 {
		return "1 transaction does not equal the sum of its splits"
	}
	return humanize.Thousands(n) + " transactions do not equal the sum of their splits"
}

// StdoutWriteRefusal reports that o's result could not be written to
// stdout. Before the store was built (o.Store == nil) it names the
// already-committed snapshot and its manifest; once the build was reached,
// it points at --from --json instead, since the manifest alone no longer
// carries the store result.
func (o Outcome) StdoutWriteRefusal(home string, err error) error {
	if o.Store == nil {
		return fmt.Errorf(
			"cannot write the result to stdout: %w; the snapshot is kept at %s and its .json manifest holds the full result",
			err, homepath.Abbreviate(home, o.Manifest.Snapshot.Path))
	}
	return fmt.Errorf(
		"cannot write the result to stdout: %w; run quarry sync --from %s --json to see it again",
		err, ID(o.Manifest.Snapshot.Path))
}
