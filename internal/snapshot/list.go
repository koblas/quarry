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
// store was built from it. Path and ManifestPath are names the directory listing returned.
type Entry struct {
	ID    string
	Path  string
	Bytes int64
	// ManifestPath is the manifest file chosen for this snapshot, readable or not; "" when it has none.
	ManifestPath string
	Manifest     *Manifest
	// TakenAt is the manifest's taken_at in UTC; zero when there is no manifest or it does not parse.
	TakenAt time.Time
	// Store marks the one entry output names as the store's snapshot.
	Store bool
	// storeFile is true for every entry that is the file the store recorded, whatever its name; none is ever deleted.
	storeFile bool
}

// Listing is what List found in the snapshots folder.
type Listing struct {
	// Dir is the snapshots folder.
	Dir string
	// Entries are the snapshots, newest first.
	Entries []Entry
	// StorePath is the snapshot path the store recorded, unresolved, even when that path cannot be
	// statted; "" when there is no store or the store itself cannot be read.
	StorePath string
	// TotalBytes sums every entry's size on disk.
	TotalBytes int64
	// NoSnapshots is the note that no snapshot exists yet; "" when one does.
	NoSnapshots string
	// NoSnapshotsAbsolute is NoSnapshots naming the folder by its absolute path; machine-readable output carries this form.
	NoSnapshotsAbsolute string
	// StoreUnreadable is why the store's snapshot cannot be told, a phrase naming any path it could not read; "" when it can, or there is no store.
	StoreUnreadable string
	// StoreWarning is StoreUnreadable as the warning a listing prints; "" when StoreUnreadable is.
	StoreWarning string
	// StoreWarningAbsolute is StoreWarning naming any path by its absolute path; machine-readable output carries this form.
	StoreWarningAbsolute string
	// orphans are the manifests whose snapshot file is gone and that no sync is writing.
	orphans []string
}

// List reads the snapshots folder, newest first, and marks the snapshot the store was
// built from. A missing folder lists nothing, an unreadable one or an ended ctx is a
// refusal; a store that cannot say, or whose recorded path cannot be statted, marks nothing and sets StoreUnreadable.
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
		entry := Entry{ID: f.id, Path: filepath.Join(s.snapshotDir, f.name), Bytes: f.bytes}
		if f.manifest != "" {
			entry.ManifestPath = filepath.Join(s.snapshotDir, f.manifest)
			if manifest, err := readManifest(entry.ManifestPath); err == nil {
				entry.Manifest = &manifest
				entry.TakenAt = recordedTakenAt(manifest.Snapshot.TakenAt)
			}
		}
		listing.Entries[i] = entry
		listing.TotalBytes += f.bytes
	}
	if len(files) == 0 {
		listing.NoSnapshots = noSnapshotsNote(homepath.Abbreviate(s.home, s.snapshotDir))
		listing.NoSnapshotsAbsolute = noSnapshotsNote(s.snapshotDir)
	}
	return listing, nil
}

// noSnapshotsNote is the note that no snapshot exists yet in a snapshots folder shown as folder.
func noSnapshotsNote(folder string) string {
	return "no snapshots in " + folder + " yet; run quarry sync to take one"
}

// markStore reads which snapshot the store was built from into listing and marks that entry. A recorded
// path that cannot be statted marks nothing and is reported as StoreUnreadable.
func (s *Server) markStore(ctx context.Context, listing *Listing) error {
	recorded, reason, err := s.storeSnapshot(ctx)
	if err != nil {
		return err
	}
	listing.StorePath = recorded
	absolute := reason
	if statErr := markStoreSnapshot(listing.Entries, recorded); statErr != nil {
		reason = "cannot read " + homepath.Abbreviate(s.home, recorded) + ": " + osreason.Reason(statErr)
		absolute = "cannot read " + recorded + ": " + osreason.Reason(statErr)
	}
	if reason != "" {
		listing.StoreUnreadable = reason
		listing.StoreWarning = cannotTellWarning + reason
		listing.StoreWarningAbsolute = cannotTellWarning + absolute
	}
	return nil
}

// cannotTellWarning opens the warning that the store's snapshot cannot be told.
const cannotTellWarning = "cannot tell which snapshot the store was built from: "

// snapshotFile is one snapshot's file as listed: its ID, the two parts the order reads from that ID, its
// on-disk name and its chosen manifest's name ("" when none), and its size on disk.
type snapshotFile struct {
	id, stamp, suffix string
	name, manifest    string
	bytes             int64
}

// scanFolder reads the snapshots folder once: the regular snapshot files chosen by selectFolder, newest
// first, and the orphan manifests' paths. An unreadable folder or unstattable snapshot is a refusal.
func (s *Server) scanFolder() ([]snapshotFile, []string, error) {
	dirEntries, err := os.ReadDir(s.snapshotDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, s.folderUnreadableRefusal(err)
	}
	selection := selectFolder(dirEntries)
	files := make([]snapshotFile, len(selection.snapshots))
	for i, chosen := range selection.snapshots {
		info, err := chosen.entry.Info()
		if err != nil {
			return nil, nil, s.folderUnreadableRefusal(err)
		}
		files[i] = snapshotFile{
			id: chosen.id, stamp: chosen.stamp, suffix: chosen.suffix,
			name: chosen.entry.Name(), manifest: chosen.manifest, bytes: info.Size(),
		}
	}
	slices.SortFunc(files, newestFirst)
	orphans := make([]string, len(selection.orphans))
	for i, name := range selection.orphans {
		orphans[i] = filepath.Join(s.snapshotDir, name)
	}
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

// markStoreSnapshot marks the entries that are the store's recorded snapshot (storeFile) and the one output names (Store);
// it marks nothing and returns the stat error when recorded fails to stat for any reason but not existing.
func markStoreSnapshot(entries []Entry, recorded string) error {
	want, err := os.Stat(recorded)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err //nolint:wrapcheck // callers read the *fs.PathError reason through osreason.Reason
	}
	sameFile := entriesAt(entries, want)
	for _, i := range sameFile {
		entries[i].storeFile = true
	}
	if i := storeEntryIndex(entries, sameFile, recorded); i >= 0 {
		entries[i].Store = true
	}
	return nil
}

// storeEntryIndex is the index of the entry output names as the store's snapshot, or -1. sameFile are the
// indexes, newest first, of the entries that are the file at recorded.
func storeEntryIndex(entries []Entry, sameFile []int, recorded string) int {
	// The entry that is the recorded file under the recorded name is the one the path resolves to. The symlink-free
	// name goes first: a hard link carrying the symlink's own name is not what the symlink points at.
	// A path that does not resolve leaves resolved empty, which names no entry: the error decides nothing.
	resolved, _ := filepath.EvalSymlinks(recorded)
	for _, name := range []string{snapshotName(resolved), snapshotName(recorded)} {
		if i := slices.IndexFunc(sameFile, func(i int) bool { return strings.EqualFold(entries[i].ID, name) }); i >= 0 {
			return sameFile[i]
		}
	}
	// The recorded file lies outside the folder and shares an entry's content by hard link: the newest such entry.
	if len(sameFile) > 0 {
		return sameFile[0]
	}
	// No entry is the recorded file, which is gone or a separate file elsewhere: the entry carrying its ID, if any.
	return slices.IndexFunc(entries, func(e Entry) bool { return strings.EqualFold(e.ID, snapshotName(recorded)) })
}

// snapshotName is path's base name without its extension; snapshot IDs contain no dot.
func snapshotName(path string) string {
	name := filepath.Base(path)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// entriesAt returns the indexes of the entries that are the file want describes, symlinks followed and
// letter case as the volume reads it; none when want is nil.
func entriesAt(entries []Entry, want fs.FileInfo) []int {
	var same []int
	for i, entry := range entries {
		if info, _ := os.Stat(entry.Path); os.SameFile(want, info) {
			same = append(same, i)
		}
	}
	return same
}
