package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/osreason"
)

// ErrKeepBelowOne is Prune's refusal of a keep under 1: the store's snapshot is always kept.
var ErrKeepBelowOne = errors.New("keep must be 1 or more")

// Pruned is what Prune did: the keep it applied, the snapshots folder and how many
// snapshots it holds now, what was deleted and what could not be (a snapshot already
// gone is neither), and the store's entry when it lies beyond the newest keep and was kept.
type Pruned struct {
	Keep      int
	Dir       string
	Snapshots int
	Deleted   []Entry
	Failed    []PruneFailure
	StoreKept *Entry
	// DryRun is true when nothing was deleted because the run only planned; WouldDelete then lists the snapshots a real run deletes.
	DryRun      bool
	WouldDelete []Entry
	// StorePath is the snapshot path the store recorded, unresolved; "" when the store was not readable or names none.
	StorePath string
	// NotDeleted counts the selected snapshots never attempted because the ctx ended.
	NotDeleted int
}

// PruneFailure is a snapshot Prune could not delete and the OS reason why.
type PruneFailure struct {
	Entry  Entry
	Reason string
}

// selectPrune splits entries, newest first, into those beyond the newest keep to delete and the entry
// marked as the store's when it lies beyond them. No entry that is the store's recorded file is deleted.
func selectPrune(entries []Entry, keep int) ([]Entry, *Entry) {
	var doomed []Entry
	var storeKept *Entry
	for _, entry := range entries[min(keep, len(entries)):] {
		if entry.Store {
			storeKept = &entry
		}
		if entry.Store || entry.storeFile {
			continue
		}
		doomed = append(doomed, entry)
	}
	return doomed, storeKept
}

// prunePlan is the decision Prune and PlanPrune share: the facts so far, what lies beyond
// the newest keep, and the orphan manifests a real run sweeps.
type prunePlan struct {
	pruned  Pruned
	doomed  []Entry
	orphans []string
}

// planPrune decides what a prune would delete without touching a file. It refuses keep < 1, an ended ctx,
// an unreadable folder and, only beyond the newest keep, a store that cannot say which snapshot built it.
func (s *Server) planPrune(ctx context.Context, keep int) (prunePlan, error) {
	if keep < 1 {
		return prunePlan{}, ErrKeepBelowOne
	}
	if ctx.Err() != nil {
		return prunePlan{}, pruneInterrupted(ctx)
	}
	listing, err := s.listFolder()
	if err != nil {
		return prunePlan{}, err
	}
	beyond := len(listing.Entries) > keep
	if err := s.markStore(ctx, &listing); err != nil {
		if ctx.Err() != nil {
			return prunePlan{}, pruneInterrupted(ctx)
		}
		if beyond {
			return prunePlan{}, err
		}
	}
	plan := prunePlan{
		pruned:  Pruned{Keep: keep, Dir: listing.Dir, Snapshots: len(listing.Entries), StorePath: listing.StorePath},
		orphans: listing.orphans,
	}
	if !beyond {
		return plan, nil
	}
	if listing.StoreUnreadable != "" {
		return prunePlan{}, s.cannotTellRefusal(listing.StoreUnreadable)
	}
	plan.doomed, plan.pruned.StoreKept = selectPrune(listing.Entries, keep)
	return plan, nil
}

// PlanPrune is Prune without the deleting: WouldDelete lists what Prune would delete, in the
// same order, and no file is removed, orphan manifests included. Its refusals are Prune's.
func (s *Server) PlanPrune(ctx context.Context, keep int) (Pruned, error) {
	plan, err := s.planPrune(ctx, keep)
	if err != nil {
		return Pruned{}, err
	}
	plan.pruned.DryRun = true
	plan.pruned.WouldDelete = plan.doomed
	return plan.pruned, nil
}

// Prune deletes all but the newest keep snapshots, never one that is the file the store was built
// from, and reports what it deleted and what it could not. It refuses what PlanPrune refuses. An ended
// ctx stops it between snapshots and skips the orphan sweep.
func (s *Server) Prune(ctx context.Context, keep int) (Pruned, error) {
	plan, err := s.planPrune(ctx, keep)
	if err != nil {
		return Pruned{}, err
	}
	pruned := plan.pruned
	for i, entry := range plan.doomed {
		s.deleteSnapshot(entry, &pruned)
		// The first snapshot is always attempted: ctx was checked when the store was read.
		if left := len(plan.doomed) - i - 1; left > 0 && ctx.Err() != nil {
			pruned.NotDeleted = left
			return pruned, interruptedMidDelete(ctx, left)
		}
	}
	s.sweepOrphans(ctx, plan.orphans)
	return pruned, nil
}

// sweepOrphans removes each orphan manifest unless ctx has ended; one that will not go is not reported.
func (s *Server) sweepOrphans(ctx context.Context, orphans []string) {
	if ctx.Err() != nil {
		return
	}
	for _, path := range orphans {
		_ = s.remove(path)
	}
}

// deleteSnapshot removes entry's snapshot file, then its manifest, by the names the listing returned, and
// records the outcome in pruned. A snapshot already gone is neither deleted nor failed; any other fault leaves the manifest.
func (s *Server) deleteSnapshot(entry Entry, pruned *Pruned) {
	err := s.remove(entry.Path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		pruned.Failed = append(pruned.Failed, PruneFailure{Entry: entry, Reason: osreason.Reason(err)})
		return
	}
	pruned.Snapshots--
	if err == nil {
		pruned.Deleted = append(pruned.Deleted, entry)
	}
	// os.Remove would delete an empty directory or a symlink named like the manifest.
	if entry.ManifestPath == "" {
		return
	}
	if info, statErr := os.Lstat(entry.ManifestPath); statErr == nil && info.Mode().IsRegular() {
		_ = s.remove(entry.ManifestPath)
	}
}

// pruneInterrupted is the refusal for a prune whose ctx ended before it could delete.
func pruneInterrupted(ctx context.Context) error {
	return causedRefusalError{msg: "snapshots prune interrupted", cause: ctx.Err()}
}

// interruptedMidDelete is the refusal for a prune whose ctx ended with notDeleted snapshots unattempted.
func interruptedMidDelete(ctx context.Context, notDeleted int) error {
	noun, verb := "snapshots", "were"
	if notDeleted == 1 {
		noun, verb = "snapshot", "was"
	}
	return causedRefusalError{
		msg:   fmt.Sprintf("snapshots prune interrupted; %d %s %s not deleted", notDeleted, noun, verb),
		cause: ctx.Err(),
	}
}

// cannotTellRefusal is the refusal for a store that cannot say which snapshot built it.
func (s *Server) cannotTellRefusal(reason string) error {
	return RefusalError{msg: "cannot tell which snapshot the store at " + homepath.Abbreviate(s.home, s.storeProbe.Path()) +
		" was built from (" + reason + "), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again"}
}
