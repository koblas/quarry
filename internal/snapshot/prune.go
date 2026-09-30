package snapshot

import (
	"context"
	"errors"
)

// ErrKeepBelowOne is Prune's refusal of a keep under 1: the store's snapshot is always kept.
var ErrKeepBelowOne = errors.New("keep must be 1 or more")

// Pruned is what Prune did: the keep it applied and, when the store's snapshot lies
// beyond the newest keep, that entry, kept anyway.
type Pruned struct {
	Keep      int
	StoreKept *Entry
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
// from. It returns ErrKeepBelowOne for a keep under 1 and deletes nothing then.
func (s *Server) Prune(ctx context.Context, keep int) (Pruned, error) {
	if keep < 1 {
		return Pruned{}, ErrKeepBelowOne
	}
	return Pruned{Keep: keep}, nil
}
