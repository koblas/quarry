package snapshot

import (
	"context"
	"errors"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/osreason"
)

// autoPrune deletes the snapshots beyond the newest s.autoKeep, never the one the store was
// built from, and records what it did in outcome.Pruned. It reads no store: the built snapshot
// is outcome's own. An ended ctx before a delete stops it with interruptedWhilePruning.
func (s *Server) autoPrune(ctx context.Context, outcome *Outcome) error {
	if s.autoKeep < 1 {
		return nil
	}
	pruned := &Pruned{Keep: s.autoKeep, Dir: s.snapshotDir}
	outcome.Pruned = pruned
	listing, err := s.listFolder()
	if err != nil {
		// listFolder refuses only with a causedRefusalError; the warning quotes the OS cause, not the refusal.
		reason := osreason.Reason(errors.Unwrap(err))
		outcome.pruneWarning = cannotListWarning(homepath.Abbreviate(s.home, s.snapshotDir), reason)
		outcome.pruneWarningAbsolute = cannotListWarning(s.snapshotDir, reason)
		return nil
	}
	markStoreSnapshot(listing.Entries, outcome.Manifest.Snapshot.Path)
	pruned.Snapshots = len(listing.Entries)
	var doomed []Entry
	doomed, pruned.StoreKept = selectPrune(listing.Entries, s.autoKeep)
	for i, entry := range doomed {
		if ctx.Err() != nil {
			pruned.NotDeleted = len(doomed) - i
			return interruptedWhilePruning(ctx)
		}
		s.deleteSnapshot(entry, pruned)
	}
	if ctx.Err() == nil {
		s.sweepOrphans(listing.orphans)
	}
	return nil
}

// interruptedWhilePruning is the refusal for a sync whose ctx ended with snapshots still to delete.
func interruptedWhilePruning(ctx context.Context) error {
	return causedRefusalError{
		msg:   "sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish",
		cause: ctx.Err(),
	}
}

// tryAgain ends every warning about a snapshot auto-prune could not delete.
const tryAgain = "; run quarry snapshots prune to try again"

// cannotListWarning is the warning for a snapshots folder shown as folder that could not be listed for reason.
func cannotListWarning(folder, reason string) string {
	return "cannot list " + folder + " to delete old snapshots: " + reason + tryAgain
}

// pruneWarnings returns o's auto-prune warnings: listWarning when the folder could not be
// listed, else one per snapshot that could not be deleted, in listing order.
func (o Outcome) pruneWarnings(listWarning string) []string {
	if listWarning != "" {
		return []string{listWarning}
	}
	if o.Pruned == nil {
		return nil
	}
	var warnings []string
	for _, failure := range o.Pruned.Failed {
		warnings = append(warnings, "cannot delete snapshot "+failure.Entry.ID+": "+failure.Reason+tryAgain)
	}
	return warnings
}

// WarningsAbsolute returns Warnings with a snapshots folder that could not be listed named by its
// absolute path, not ~-abbreviated; machine-readable output carries this form.
func (o Outcome) WarningsAbsolute() []string { return o.warnings(o.pruneWarningAbsolute) }
