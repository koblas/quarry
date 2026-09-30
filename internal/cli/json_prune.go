package cli

import "github.com/koblas/quarry/internal/snapshot"

// prunedDocument is snapshots prune's --json stdout shape, one for real and dry runs.
type prunedDocument struct {
	DryRun        bool                   `json:"dry_run"`
	Keep          int                    `json:"keep"`
	StoreSnapshot *storeSnapshotDocument `json:"store_snapshot"`
	Deleted       []prunedEntryDocument  `json:"deleted"`
	WouldDelete   []prunedEntryDocument  `json:"would_delete"`
	Failed        []pruneFailureDocument `json:"failed"`
	Warnings      []string               `json:"warnings"`
}

// prunedEntryDocument is one entry of "deleted" or "would_delete"; bytes is the .sqlite's size.
type prunedEntryDocument struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// pruneFailureDocument is one entry of "failed".
type pruneFailureDocument struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// renderPrunedJSON renders p as prune's --json document with warnings as given; every list is [] rather than null.
func renderPrunedJSON(p snapshot.Pruned, warnings []string) ([]byte, error) {
	failed := make([]pruneFailureDocument, len(p.Failed))
	for i, f := range p.Failed {
		failed[i] = pruneFailureDocument{ID: f.Entry.ID, Path: f.Entry.Path, Reason: f.Reason}
	}
	return marshalDocument(prunedDocument{
		DryRun: p.DryRun, Keep: p.Keep, StoreSnapshot: newStoreSnapshotDocument(p.StorePath),
		Deleted: newPrunedEntryDocuments(p.Deleted), WouldDelete: newPrunedEntryDocuments(p.WouldDelete),
		Failed: failed, Warnings: append([]string{}, warnings...),
	})
}

// newPrunedEntryDocuments converts entries into the --json entry shape.
func newPrunedEntryDocuments(entries []snapshot.Entry) []prunedEntryDocument {
	out := make([]prunedEntryDocument, len(entries))
	for i, e := range entries {
		out[i] = prunedEntryDocument{ID: e.ID, Path: e.Path, Bytes: e.Bytes}
	}
	return out
}
