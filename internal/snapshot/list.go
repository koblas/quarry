package snapshot

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/koblas/quarry/internal/platform/homepath"
	"github.com/koblas/quarry/internal/platform/osreason"
	"github.com/koblas/quarry/internal/store"
)

// reasonOtherFormat is why a store of another format that names no snapshot cannot say which one built it.
const reasonOtherFormat = "the store was built by another version of quarry"

// Entry is one snapshot in a Listing: its ID, its .sqlite path and size on
// disk, its manifest (nil when it has none or cannot read it) and whether the
// store was built from it.
type Entry struct {
	ID       string
	Path     string
	Bytes    int64
	Manifest *Manifest
	// TakenAt is the manifest's taken_at in UTC; zero when there is no manifest or it does not parse.
	TakenAt time.Time
	Store   bool
}

// Listing is what List found in the snapshots folder.
type Listing struct {
	// Dir is the snapshots folder.
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
	// StoreUnreadable is why the store's snapshot cannot be told, a bare phrase; "" when it can, or there is no store.
	StoreUnreadable string
	// StoreWarning is StoreUnreadable as the warning a listing prints; "" when StoreUnreadable is.
	StoreWarning string
}

// List reads the snapshots folder, newest first, and marks the snapshot the store was
// built from. A missing folder lists nothing, an unreadable one or an ended ctx is a
// refusal; a store that cannot say marks nothing and sets StoreUnreadable.
func (s *Server) List(ctx context.Context) (Listing, error) {
	files, err := s.snapshotFiles()
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Dir: s.snapshotDir, Entries: make([]Entry, len(files))}
	for i, f := range files {
		entry := Entry{ID: f.id, Path: filepath.Join(s.snapshotDir, f.id+".sqlite"), Bytes: f.bytes}
		if manifest, err := readManifest(filepath.Join(s.snapshotDir, f.id+".json")); err == nil {
			entry.Manifest = &manifest
			entry.TakenAt = recordedTakenAt(manifest.Snapshot.TakenAt)
		}
		listing.Entries[i] = entry
		listing.TotalBytes += f.bytes
	}
	if len(files) == 0 {
		listing.NoSnapshots = "no snapshots in " + homepath.Abbreviate(s.home, s.snapshotDir) + " yet; run quarry sync to take one"
	}

	recorded, reason, err := s.storeSnapshot(ctx)
	if err != nil {
		return Listing{}, err
	}
	if reason != "" {
		listing.StoreUnreadable = reason
		listing.StoreWarning = "cannot tell which snapshot the store was built from: " + reason
	}
	listing.StorePath = recorded
	markStoreSnapshot(listing.Entries, recorded)
	return listing, nil
}

// snapshotFile is one snapshot's file as listed: its ID, the two parts the
// order reads from that ID, and its size on disk.
type snapshotFile struct {
	id, stamp, suffix string
	bytes             int64
}

// snapshotFiles lists the regular snapshot files, newest first; a folder it cannot
// read, or a snapshot it cannot stat, is a refusal.
func (s *Server) snapshotFiles() ([]snapshotFile, error) {
	dirEntries, err := os.ReadDir(s.snapshotDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, s.folderUnreadableRefusal(err)
	}
	var files []snapshotFile
	for _, dirEntry := range dirEntries {
		match := snapshotFilePattern.FindStringSubmatch(dirEntry.Name())
		if match == nil || !dirEntry.Type().IsRegular() {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			return nil, s.folderUnreadableRefusal(err)
		}
		files = append(files, snapshotFile{id: snapshotID(dirEntry.Name()), stamp: match[1], suffix: match[2], bytes: info.Size()})
	}
	slices.SortFunc(files, newestFirst)
	return files, nil
}

// newestFirst orders by ID timestamp, then _N suffix (none oldest), as digits so it
// never has to fit an integer; taken_at plays no part.
func newestFirst(a, b snapshotFile) int {
	return cmp.Or(
		cmp.Compare(b.stamp, a.stamp),
		cmp.Compare(len(b.suffix), len(a.suffix)),
		cmp.Compare(b.suffix, a.suffix),
	)
}

// folderUnreadableRefusal reports that the snapshots folder, or a snapshot in it, cannot be read.
func (s *Server) folderUnreadableRefusal(err error) error {
	return causedRefusalError{
		msg:   "cannot read " + homepath.Abbreviate(s.home, s.snapshotDir) + ": " + osreason.Reason(err),
		cause: err,
	}
}

// storeSnapshot returns the snapshot path the store recorded, or the reason it cannot
// say; both empty when no store exists. ctx is checked before any fault is classified.
func (s *Server) storeSnapshot(ctx context.Context) (string, string, error) {
	if s.storeProbe == nil {
		return "", "", nil
	}
	recorded, err := s.storeProbe.BuiltFrom(ctx)
	if ctx.Err() != nil {
		return "", "", causedRefusalError{msg: "snapshots interrupted", cause: ctx.Err()}
	}
	if err == nil {
		return recorded, "", nil
	}
	openErr, ok := errors.AsType[*store.OpenError](err)
	if !ok {
		return "", "", fmt.Errorf("read the store's snapshot: %w", err)
	}
	// UnreadableReason is empty for Missing, which is no store and no warning, and for
	// OtherFormat, which is classified here first.
	if openErr.Fault == store.OpenFaultOtherFormat {
		if openErr.SnapshotPath == "" {
			return "", reasonOtherFormat, nil
		}
		return openErr.SnapshotPath, "", nil
	}
	return "", openErr.UnreadableReason(homepath.Abbreviate(s.home, openErr.Path)), nil
}

// markStoreSnapshot sets Store on the entry that is the file recorded names.
// Both sides resolve symlinks; a recorded path that no longer resolves marks nothing.
func markStoreSnapshot(entries []Entry, recorded string) {
	want := resolvedPath(recorded)
	if want == "" {
		return
	}
	for i := range entries {
		entries[i].Store = resolvedPath(entries[i].Path) == want
	}
}

// resolvedPath is path with symlinks resolved, or "" when it does not resolve.
func resolvedPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return resolved
}
