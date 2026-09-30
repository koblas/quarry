package snapshot

import "context"

// Entry is one snapshot in a Listing: its ID, its .sqlite path and size on
// disk, its manifest (nil when it has none) and whether the store was built from it.
type Entry struct {
	ID       string
	Path     string
	Bytes    int64
	Manifest *Manifest
	Store    bool
}

// Listing is what List found in the snapshots folder.
type Listing struct {
	// Dir is the absolute snapshots folder.
	Dir string
	// Entries are the snapshots, newest first.
	Entries []Entry
	// StorePath is the snapshot path the store recorded, unresolved; "" when
	// there is no store or it cannot be read.
	StorePath string
	// TotalBytes sums every entry's size on disk.
	TotalBytes int64
	// NoSnapshots is the note that no snapshot exists yet; "" when one does.
	NoSnapshots string
	// StoreWarning says the store's snapshot could not be told; "" when it could.
	StoreWarning string
}

// List reads the snapshots folder, newest first.
func (s *Server) List(_ context.Context) (Listing, error) {
	return Listing{}, nil
}
