package snapshot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// variantEntry reports name and mode in place of those of the real entry it wraps, keeping that entry's Info.
type variantEntry struct {
	fs.DirEntry

	name string
	mode fs.FileMode
}

func (e variantEntry) Name() string      { return e.name }
func (e variantEntry) Type() fs.FileMode { return e.mode.Type() }
func (e variantEntry) IsDir() bool       { return e.mode.IsDir() }

// variant is an extra entry named name that wraps the real entry named like and has type mode.
// A case-insensitive volume cannot hold two letter cases of one name, so the second case exists only in the listing.
type variant struct {
	name, like string
	mode       fs.FileMode
}

// regularVariant is a regular-file variant named name, wrapping the real entry named like.
func regularVariant(name, like string) variant { return variant{name: name, like: like} }

// readDirWith is os.ReadDir plus one extra entry for each of variants, in the order given.
func readDirWith(t *testing.T, variants ...variant) func(string) ([]fs.DirEntry, error) {
	t.Helper()
	return func(dir string) ([]fs.DirEntry, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		onDisk := entries
		for _, v := range variants {
			i := indexOfEntry(onDisk, v.like)
			require.GreaterOrEqual(t, i, 0, "no real entry named %s to wrap", v.like)
			entries = append(entries, variantEntry{DirEntry: onDisk[i], name: v.name, mode: v.mode})
		}
		return entries, nil
	}
}

func indexOfEntry(entries []fs.DirEntry, name string) int {
	for i, e := range entries {
		if e.Name() == name {
			return i
		}
	}
	return -1
}

func Test_list_reads_the_snapshots_folder_through_the_read_dir_seam(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idOldest, 1500)
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t, regularVariant(idNewest+".sqlite", idOldest+".sqlite"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest, idOldest}, entryIDs(listing))
}

func Test_list_refuses_with_the_unreadable_folder_copy_when_the_read_dir_seam_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotsFolder(t, home)
	readDir := func(dir string) ([]fs.DirEntry, error) {
		return nil, &fs.PathError{Op: "open", Path: dir, Err: syscall.EACCES}
	}

	_, err := newListServer(home, nil, snapshot.WithReadDir(readDir)).List(t.Context())

	require.EqualError(t, err, "cannot read ~/snapshots: permission denied")
}

func Test_list_never_lists_or_counts_a_stray_letter_case_variant(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idOldest, 1500)
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t, regularVariant(idOldest+".SQLITE", idOldest+".sqlite"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	require.Len(t, listing.Entries, 1)
	assert.Equal(t, filepath.Join(dir, idOldest+".sqlite"), listing.Entries[0].Path)
	assert.EqualValues(t, 1500, listing.TotalBytes)
}

func Test_prune_never_deletes_a_stray_letter_case_variant(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}
	srv := newPruneServer(home, nil, rm, snapshot.WithReadDir(readDirWith(t, regularVariant(idOldest+".SQLITE", idOldest+".sqlite"))))

	pruned, err := srv.Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest + ".sqlite"}, rm.calls)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.Snapshots)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_plan_prune_never_would_delete_a_stray_letter_case_variant(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}
	srv := newPruneServer(home, nil, rm, snapshot.WithReadDir(readDirWith(t, regularVariant(idOldest+".SQLITE", idOldest+".sqlite"))))

	pruned, err := srv.PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.WouldDelete, 1)
	assert.Equal(t, filepath.Join(dir, idOldest+".sqlite"), pruned.WouldDelete[0].Path)
	assert.Equal(t, 3, pruned.Snapshots)
	assert.Empty(t, rm.calls)
}

func Test_sync_and_import_never_deletes_a_stray_letter_case_variant(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(3)
	dir := prunable(t, home, ids...)
	rm := &fakeRemover{}
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2), snapshot.WithRemove(rm.remove),
		snapshot.WithReadDir(readDirWith(t, regularVariant(ids[0]+".SQLITE", ids[0]+".sqlite"))))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[1] + ".sqlite", ids[1] + ".json", ids[0] + ".sqlite"}, rm.calls)
	assert.Equal(t, []string{ids[1], ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Empty(t, outcome.Pruned.Failed)
	assert.Empty(t, outcome.Warnings())
	assert.FileExists(t, filepath.Join(dir, ids[0]+".json"))
}
