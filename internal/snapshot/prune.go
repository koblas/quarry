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
	// NotDeleted counts the selected snapshots never attempted because the ctx ended.
	NotDeleted int
}

// PruneFailure is a snapshot Prune could not delete and the OS reason why.
type PruneFailure struct {
	Entry  Entry
	Reason string
}

// selectPrune splits entries, newest first, into those beyond the newest keep to
// delete and the store's entry when it lies beyond them, which is never deleted.
func selectPrune(entries []Entry, keep int) ([]Entry, *Entry) {
	var doomed []Entry
	var storeKept *Entry
	for _, entry := range entries[min(keep, len(entries)):] {
		if entry.Store {
			storeKept = &entry
			continue
		}
		doomed = append(doomed, entry)
	}
	return doomed, storeKept
}

// Prune deletes all but the newest keep snapshots, never the one the store was built from,
// and reports what it deleted and what it could not. It refuses keep < 1 (ErrKeepBelowOne),
// an ended ctx, an unreadable folder and, only past the newest keep, a store that cannot say
// which snapshot built it. An ended ctx stops it between snapshots; otherwise it sweeps orphan manifests.
func (s *Server) Prune(ctx context.Context, keep int) (Pruned, error) {
	if keep < 1 {
		return Pruned{}, ErrKeepBelowOne
	}
	if ctx.Err() != nil {
		return Pruned{}, pruneInterrupted(ctx)
	}
	listing, err := s.listFolder()
	if err != nil {
		return Pruned{}, err
	}
	pruned := Pruned{Keep: keep, Dir: listing.Dir, Snapshots: len(listing.Entries)}
	if len(listing.Entries) <= keep {
		s.sweepOrphans(listing.orphans)
		return pruned, nil
	}
	if err := s.markStore(ctx, &listing); err != nil {
		if ctx.Err() != nil {
			return Pruned{}, pruneInterrupted(ctx)
		}
		return Pruned{}, err
	}
	if listing.StoreUnreadable != "" {
		return Pruned{}, s.cannotTellRefusal(listing.StoreUnreadable)
	}
	doomed, storeKept := selectPrune(listing.Entries, keep)
	pruned.StoreKept = storeKept
	for i, entry := range doomed {
		s.deleteSnapshot(entry, &pruned)
		// The first snapshot is always attempted: ctx was checked when the store was read.
		if left := len(doomed) - i - 1; left > 0 && ctx.Err() != nil {
			pruned.NotDeleted = left
			return pruned, interruptedMidDelete(ctx, left)
		}
	}
	s.sweepOrphans(listing.orphans)
	return pruned, nil
}

// sweepOrphans removes each orphan manifest; one that will not go is not reported.
func (s *Server) sweepOrphans(orphans []string) {
	for _, path := range orphans {
		_ = s.remove(path)
	}
}

// deleteSnapshot removes entry's .sqlite, then its manifest, and records the outcome in pruned.
// A .sqlite already gone is neither deleted nor failed; any other fault leaves the manifest.
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
	manifest := s.manifestPath(entry.ID)
	if info, statErr := os.Lstat(manifest); statErr == nil && info.Mode().IsRegular() {
		_ = s.remove(manifest)
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
