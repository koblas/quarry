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
	"strings"
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
	// orphans are the manifests whose snapshot file is gone and that no sync is writing.
	orphans []string
}

// List reads the snapshots folder, newest first, and marks the snapshot the store was
// built from. A missing folder lists nothing, an unreadable one or an ended ctx is a
// refusal; a store that cannot say marks nothing and sets StoreUnreadable.
func (s *Server) List(ctx context.Context) (Listing, error) {
	listing, err := s.listFolder()
	if err != nil {
		return Listing{}, err
	}
	if err := s.markStore(ctx, &listing); err != nil {
		return Listing{}, err
	}
	return listing, nil
}

// listFolder reads the snapshots folder into a Listing that says nothing of the store.
func (s *Server) listFolder() (Listing, error) {
	files, orphans, err := s.scanFolder()
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Dir: s.snapshotDir, Entries: make([]Entry, len(files)), orphans: orphans}
	for i, f := range files {
		entry := Entry{ID: f.id, Path: filepath.Join(s.snapshotDir, f.id+".sqlite"), Bytes: f.bytes}
		if manifest, err := readManifest(s.manifestPath(f.id)); err == nil {
			entry.Manifest = &manifest
			entry.TakenAt = recordedTakenAt(manifest.Snapshot.TakenAt)
		}
		listing.Entries[i] = entry
		listing.TotalBytes += f.bytes
	}
	if len(files) == 0 {
		listing.NoSnapshots = "no snapshots in " + homepath.Abbreviate(s.home, s.snapshotDir) + " yet; run quarry sync to take one"
	}
	return listing, nil
}

// manifestPath is where the manifest of the snapshot with id lives.
func (s *Server) manifestPath(id string) string { return filepath.Join(s.snapshotDir, id+".json") }

// markStore reads which snapshot the store was built from into listing and marks that entry.
func (s *Server) markStore(ctx context.Context, listing *Listing) error {
	recorded, reason, err := s.storeSnapshot(ctx)
	if err != nil {
		return err
	}
	if reason != "" {
		listing.StoreUnreadable = reason
		listing.StoreWarning = "cannot tell which snapshot the store was built from: " + reason
	}
	listing.StorePath = recorded
	markStoreSnapshot(listing.Entries, recorded)
	return nil
}

// snapshotFile is one snapshot's file as listed: its ID, the two parts the
// order reads from that ID, and its size on disk.
type snapshotFile struct {
	id, stamp, suffix string
	bytes             int64
}

// scanFolder reads the snapshots folder once: the regular snapshot files, newest first, and
// the orphan manifests' paths. An unreadable folder or unstattable snapshot is a refusal.
func (s *Server) scanFolder() ([]snapshotFile, []string, error) {
	dirEntries, err := os.ReadDir(s.snapshotDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, s.folderUnreadableRefusal(err)
	}
	names := make(map[string]bool, len(dirEntries))
	for _, dirEntry := range dirEntries {
		names[dirEntry.Name()] = true
	}
	var files []snapshotFile
	var orphans []string
	for _, dirEntry := range dirEntries {
		if match := manifestFilePattern.FindStringSubmatch(dirEntry.Name()); match != nil {
			// A manifest beside a partial snapshot is a sync in flight: it commits the manifest first.
			if dirEntry.Type().IsRegular() && !names[match[1]+".sqlite"] && !names["."+match[1]+".sqlite.partial"] {
				orphans = append(orphans, filepath.Join(s.snapshotDir, dirEntry.Name()))
			}
			continue
		}
		match := snapshotFilePattern.FindStringSubmatch(dirEntry.Name())
		if match == nil || !dirEntry.Type().IsRegular() {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			return nil, nil, s.folderUnreadableRefusal(err)
		}
		files = append(files, snapshotFile{id: ID(dirEntry.Name()), stamp: match[1], suffix: match[2], bytes: info.Size()})
	}
	slices.SortFunc(files, newestFirst)
	return files, orphans, nil
}

// newestFirst orders by ID timestamp, then _N suffix by value (none oldest, read as digits so it never
// has to fit an integer); equal values fall to the suffix text, highest first, so the order is total.
func newestFirst(a, b snapshotFile) int {
	digitsA, digitsB := strings.TrimLeft(a.suffix, "0"), strings.TrimLeft(b.suffix, "0")
	return cmp.Or(
		cmp.Compare(b.stamp, a.stamp),
		cmp.Compare(len(digitsB), len(digitsA)),
		cmp.Compare(digitsB, digitsA),
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
	// UnreadableReason is empty for Missing (no store) and OtherFormat (handled first here).
	if openErr.Fault == store.OpenFaultOtherFormat {
		if openErr.SnapshotPath == "" {
			return "", reasonOtherFormat, nil
		}
		return openErr.SnapshotPath, "", nil
	}
	return "", openErr.UnreadableReason(homepath.Abbreviate(s.home, openErr.Path)), nil
}

// markStoreSnapshot sets Store on the one entry that is the file recorded names, or on none.
// storeEntryIndex decides which.
func markStoreSnapshot(entries []Entry, recorded string) {
	if i := storeEntryIndex(entries, recorded); i >= 0 {
		entries[i].Store = true
	}
}

// storeEntryIndex is the entry that is the recorded file on disk, preferring the recorded ID, else
// the newest; failing that the entry whose ID is the recorded one's, letter case aside; else -1.
func storeEntryIndex(entries []Entry, recorded string) int {
	id := ID(recorded)
	sameFile := -1
	if want, err := os.Stat(recorded); err == nil {
		for i, entry := range entries {
			if info, err := os.Stat(entry.Path); err != nil || !os.SameFile(want, info) {
				continue
			}
			// A hard link under another ID must not outrank the file the store recorded.
			if strings.EqualFold(entry.ID, id) {
				return i
			}
			if sameFile < 0 {
				sameFile = i
			}
		}
	}
	if sameFile >= 0 {
		return sameFile
	}
	return slices.IndexFunc(entries, func(e Entry) bool { return strings.EqualFold(e.ID, id) })
}
