package snapshot

import (
	"context"
	"errors"
	"io/fs"

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

// Prune deletes all but the newest keep snapshots, never the one the store was built
// from, and reports what it deleted and what it could not. It refuses a keep under 1
// (ErrKeepBelowOne), an ended ctx and an unreadable folder, and, only when a snapshot lies
// beyond the newest keep, a store that cannot say which snapshot built it; nothing is deleted then.
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
	for _, entry := range doomed {
		s.deleteSnapshot(entry, &pruned)
	}
	return pruned, nil
}

// deleteSnapshot removes entry's .sqlite, then its manifest, and records the outcome in
// pruned. A .sqlite already gone counts as neither deleted nor failed; any other fault
// leaves the manifest and is recorded as failed. A manifest that will not go is not reported.
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
	_ = s.remove(s.manifestPath(entry.ID))
}

// pruneInterrupted is the refusal for a prune whose ctx ended before it could delete.
func pruneInterrupted(ctx context.Context) error {
	return causedRefusalError{msg: "snapshots prune interrupted", cause: ctx.Err()}
}

// cannotTellRefusal is the refusal for a store that cannot say which snapshot built it.
func (s *Server) cannotTellRefusal(reason string) error {
	return RefusalError{msg: "cannot tell which snapshot the store at " + homepath.Abbreviate(s.home, s.storeProbe.Path()) +
		" was built from (" + reason + "), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again"}
}
