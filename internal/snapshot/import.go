package snapshot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/humanize"
	"github.com/koblas/quarry/internal/store"
)

// errImportNotWired is SyncAndImport's error when the Server has no Importer or StoreProbe configured.
var errImportNotWired = errors.New("no importer or store probe configured")

// Outcome is what SyncAndImport and ImportFrom return: the snapshot's Manifest
// and the import's store.Result, when one ran. Store is nil on a schema
// mismatch or Sync failure. StoreExisted (meaningful only when Store != nil &&
// !Store.Built) decides the V1 block's NOT REBUILT vs NOT BUILT line.
type Outcome struct {
	Manifest     Manifest
	Store        *store.Result
	StoreExisted bool
	// Pruned is what auto-prune did once the store was built; nil when none was built or the Server has no WithAutoPrune.
	Pruned *Pruned

	// historyWarning is the warning for a built store whose previous import
	// history could not be carried forward; empty when it was carried.
	historyWarning string
	// findingsWarning is the warning for a built store whose previous findings
	// could not be carried forward, or the combined history, findings and rates warning.
	findingsWarning string
	// ratesWarning is the warning for a built store whose previous exchange rates could not be carried forward.
	ratesWarning string
	// fetchWarning is the warning for a built store whose exchange-rate fetch fell short; empty when it did not.
	fetchWarning string
	// pruneWarning is the warning for a snapshots folder auto-prune could not list, and
	// pruneWarningAbsolute the same warning naming the folder by its absolute path.
	pruneWarning, pruneWarningAbsolute string
}

// Warnings returns every warning o carries, without the "quarry: warning: "
// prefix: the manifest's own, then, for a built store only, the import-history
// restart, findings restart, exchange-rates restart, rate-fetch and auto-prune warnings, in that order.
func (o Outcome) Warnings() []string { return o.warnings(o.pruneWarning) }

// warnings is Warnings with listWarning as the warning for a snapshots folder that could not be listed.
func (o Outcome) warnings(listWarning string) []string {
	warnings := o.Manifest.Warnings
	if o.Store == nil || !o.Store.Built {
		return warnings
	}
	if o.historyWarning != "" {
		warnings = append(slices.Clip(warnings), o.historyWarning)
	}
	if o.findingsWarning != "" {
		warnings = append(slices.Clip(warnings), o.findingsWarning)
	}
	if o.ratesWarning != "" {
		warnings = append(slices.Clip(warnings), o.ratesWarning)
	}
	if o.fetchWarning != "" {
		warnings = append(slices.Clip(warnings), o.fetchWarning)
	}
	return append(slices.Clip(warnings), o.pruneWarnings(listWarning)...)
}

// historyRestartWarning renders the warning that the previous store's import
// history could not be carried forward, for a reason from UnreadableReason.
func historyRestartWarning(reason string) string {
	return "cannot carry import history forward from the previous store (" + reason + "); import_runs starts again with this sync"
}

// findingsRestartWarning renders the warning that the previous store's findings
// could not be carried forward, for a reason from UnreadableReason.
func findingsRestartWarning(reason string) string {
	return "cannot carry findings forward from the previous store (" + reason + "); findings history starts again with this sync"
}

// ratesRestartWarning renders the warning that the previous store's exchange rates
// could not be carried forward, for a reason from UnreadableReason.
func ratesRestartWarning(reason string) string {
	return "cannot carry exchange rates forward from the previous store (" + reason + "); fetching them all again"
}

// combinedCarryWarning renders the warning for a previous store that could not be read at all,
// so none of its import history, findings or exchange rates were carried.
func combinedCarryWarning(reason string) string {
	return "cannot carry import history, findings or exchange rates forward from the previous store (" + reason + "); all three start again with this sync"
}

// fetchWarning renders the warning for a rate fetch that fell short of rates, "" when it did not.
// A partial fetch kept some rates; otherwise the store holds the rates it had, or none.
func fetchWarning(rates store.RatesSummary) string {
	if rates.FetchError == "" {
		return ""
	}
	last := rates.Last.Format(time.DateOnly)
	span := rates.First.Format(time.DateOnly) + " to " + last
	switch {
	case rates.Partial:
		return "could not fetch every exchange rate from the Bank of Canada: " + rates.FetchError +
			"; the store has rates from " + span + ", and later dates convert at the " + last +
			" rate; run quarry sync again to fetch the rest"
	case rates.First.IsZero():
		return "could not fetch exchange rates from the Bank of Canada: " + rates.FetchError +
			"; the store has no rates, so reports list amounts in each account's own currency; run quarry sync again to retry"
	default:
		return "could not fetch exchange rates from the Bank of Canada: " + rates.FetchError +
			"; the store has rates from " + span + ", and later dates convert at the " + last +
			" rate; run quarry sync again to retry"
	}
}

// carryWarnings maps a built store's carry faults to the history, findings and rates warnings they print.
// An unreadable store prints the combined warning alone (as the findings one); otherwise each fault prints its own.
func carryWarnings(result store.Result, display string) (string, string, string) {
	if result.StoreUnreadable && result.HistoryFault != nil {
		return "", combinedCarryWarning(result.HistoryFault.UnreadableReason(display)), ""
	}
	var history, findings, rates string
	if result.HistoryFault != nil {
		history = historyRestartWarning(result.HistoryFault.UnreadableReason(display))
	}
	if result.FindingsFault != nil {
		findings = findingsRestartWarning(result.FindingsFault.UnreadableReason(display))
	}
	if result.RatesFault != nil {
		rates = ratesRestartWarning(result.RatesFault.UnreadableReason(display))
	}
	return history, findings, rates
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

// SyncAndImport takes a snapshot of bundlePath, then imports it through the configured Importer,
// leaving Store nil on a Sync failure. An import failure never replaces the committed snapshot: it
// wraps the importer's error in a store refusal. A Server with WithAutoPrune then deletes old
// snapshots; interrupted with any left, it returns the built Outcome beside a refusal.
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

	result.Findings = finding.Classify(result.FindingStates, s.ignore).Counts
	outcome := Outcome{Manifest: manifest, Store: &result}
	outcome.historyWarning, outcome.findingsWarning, outcome.ratesWarning = carryWarnings(result, homepath.Abbreviate(s.home, s.storeProbe.Path()))
	outcome.fetchWarning = fetchWarning(result.Rates)
	err = s.autoPrune(ctx, &outcome)
	return outcome, err
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

// validationFailedRefusal reports V1: a build reached the validation gate and
// one or more checks failed.
func (s *Server) validationFailedRefusal(manifest Manifest, v store.Validation, cause error) error {
	var clauses []string
	if n := len(v.Balances.Mismatched); n > 0 {
		clauses = append(clauses, balanceMismatchClause(n, v.Balances.Checked))
	}
	if n := len(v.Splits.Mismatched); n > 0 {
		clauses = append(clauses, splitMismatchClause(n))
	}
	id := ID(manifest.Snapshot.Path)
	tail := "fix them in Quicken and run quarry sync, or run quarry sync --from " + id + " after updating quarry"
	if n := len(v.Shares.Mismatched); n > 0 {
		if len(clauses) == 0 {
			tail = shareOnlyTail(n, id)
		}
		clauses = append(clauses, shareMismatchClause(n, v.Shares.Checked))
	}
	return causedRefusalError{
		msg: fmt.Sprintf("validation failed: %s; %s was not changed; each difference is listed on stdout; %s",
			strings.Join(clauses, " and "), homepath.Abbreviate(s.home, s.storeProbe.Path()), tail),
		cause: cause,
	}
}

// shareOnlyTail is the stderr tail when only the share-count check failed: a
// share difference is a reading difference, so it points at --from, not Quicken.
func shareOnlyTail(mismatched int, id string) string {
	holdings := "the holding's"
	if mismatched > 1 {
		holdings = "those holdings'"
	}
	return fmt.Sprintf("quarry read %s transactions differently from Quicken, so run quarry sync --from %s after updating quarry", holdings, id)
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

// shareMismatchClause renders n mismatched of checked holdings: the noun
// agrees with checked, the verb and the plural "counts" with n.
func shareMismatchClause(n, checked int) string {
	noun := "holdings"
	if checked == 1 {
		noun = "holding"
	}
	verb, count := "do not match", "counts"
	if n == 1 {
		verb, count = "does not match", "count"
	}
	return fmt.Sprintf("%s of %s %s %s Quicken's share %s", humanize.Thousands(n), humanize.Thousands(checked), noun, verb, count)
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
