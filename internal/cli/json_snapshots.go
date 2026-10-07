package cli

import (
	"time"

	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/snapshot"
)

// snapshotsDocument is snapshots's --json stdout shape.
type snapshotsDocument struct {
	Directory     string                 `json:"directory"`
	Keep          int                    `json:"keep"`
	StoreSnapshot *storeSnapshotDocument `json:"store_snapshot"`
	Snapshots     []snapshotDocument     `json:"snapshots"`
	TotalBytes    int64                  `json:"total_bytes"`
	Warnings      []string               `json:"warnings"`
}

// storeSnapshotDocument names the snapshot the store recorded, listed or not.
type storeSnapshotDocument struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

// snapshotDocument is one entry of "snapshots"; every field the manifest
// supplies is null when it has none, and a taken_at or source it cannot supply is null alone.
type snapshotDocument struct {
	ID             string  `json:"id"`
	Path           string  `json:"path"`
	Manifest       *string `json:"manifest"`
	TakenAt        *string `json:"taken_at"`
	Bytes          int64   `json:"bytes"`
	Source         *string `json:"source"`
	SHA256         *string `json:"sha256"`
	SchemaVerified *bool   `json:"schema_verified"`
	Store          bool    `json:"store"`
}

// renderSnapshotsJSON renders l as snapshots's --json document with keep and
// warnings as given; snapshots is [] rather than null when l lists none.
func renderSnapshotsJSON(l snapshot.Listing, keep int, warnings []string) ([]byte, error) {
	entries := make([]snapshotDocument, len(l.Entries))
	for i, e := range l.Entries {
		entries[i] = newSnapshotDocument(e)
	}
	return marshalDocument(snapshotsDocument{
		Directory: l.Dir, Keep: keep, StoreSnapshot: newStoreSnapshotDocument(l.StorePath),
		Snapshots: entries, TotalBytes: l.TotalBytes, Warnings: warnings,
	})
}

// newStoreSnapshotDocument names the recorded snapshot path, or is nil when the store recorded none.
func newStoreSnapshotDocument(recorded string) *storeSnapshotDocument {
	if recorded == "" {
		return nil
	}
	return &storeSnapshotDocument{ID: snapshot.ID(recorded), Path: recorded}
}

// newSnapshotDocument converts e into its --json entry.
func newSnapshotDocument(e snapshot.Entry) snapshotDocument {
	doc := snapshotDocument{ID: e.ID, Path: e.Path, Bytes: e.Bytes, Store: e.Store}
	if e.Manifest == nil {
		return doc
	}
	doc.Manifest = &e.ManifestPath
	doc.Source = document.NullString(e.Manifest.Snapshot.Source)
	doc.SHA256 = &e.Manifest.Snapshot.SHA256
	doc.SchemaVerified = &e.Manifest.Schema.Verified
	if !e.TakenAt.IsZero() {
		takenAt := e.TakenAt.Format(time.RFC3339)
		doc.TakenAt = &takenAt
	}
	return doc
}

// snapshotsWarnings is the config warnings, then the no-snapshots note naming the folder by its
// absolute path, then the store warning, without the prefixes stderr gives them; never nil.
func snapshotsWarnings(config []string, l snapshot.Listing) []string {
	warnings := append([]string{}, config...)
	if l.NoSnapshots != "" {
		warnings = append(warnings, l.NoSnapshotsAbsolute)
	}
	if l.StoreWarningAbsolute != "" {
		warnings = append(warnings, l.StoreWarningAbsolute)
	}
	return warnings
}
