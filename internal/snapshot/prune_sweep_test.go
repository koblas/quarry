package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idFifth = "20260925T080000Z"
	idSixth = "20260924T080000Z"
	// Names for the manifests and files Prune must leave alone.
	idDirManifest     = "20260101T000000Z"
	idLinkManifest    = "20260102T000000Z"
	idInFlightSyncing = "20260103T000000Z"
)

// cancelling returns a fakeRemover that ends the returned context once name has been removed.
func cancelling(t *testing.T, name string) (*fakeRemover, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	return &fakeRemover{cancelAfter: name, cancel: cancel}, ctx
}

func Test_prune_sweeps_only_orphan_manifests(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	writeManifest(t, dir, idFourth, manifestTaken("2026-09-26T08:00:00Z"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idDirManifest+".json"), 0o700))
	outside := filepath.Join(home, "outside.json")
	require.NoError(t, os.WriteFile(outside, []byte("{}"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, idLinkManifest+".json")))
	writeManifest(t, dir, idInFlightSyncing, manifestTaken("2026-01-03T00:00:00Z"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "."+idInFlightSyncing+".sqlite.partial"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.json"), []byte("{}"), 0o600))

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Empty(t, pruned.Failed)
	assert.NoFileExists(t, filepath.Join(dir, idFourth+".json"))
	for _, name := range []string{idNewest + ".json", idMiddle + ".json", idDirManifest + ".json", idLinkManifest + ".json", idInFlightSyncing + ".json", "notes.json"} {
		_, statErr := os.Lstat(filepath.Join(dir, name))
		require.NoError(t, statErr, name)
	}
	assert.FileExists(t, outside)
}

func Test_prune_sweeps_orphans_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))
	rm := &fakeRemover{}

	pruned, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, pruned.Deleted)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Equal(t, []string{idOldest + ".json"}, rm.calls)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_ignores_an_orphan_manifest_it_cannot_remove(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))

	pruned, err := newPruneServer(home, nil, failing(idOldest+".json", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, pruned.Failed)
	assert.Empty(t, pruned.Deleted)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_deletes_only_snapshot_files_inside_the_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest)
	target := filepath.Join(home, "outside.sqlite")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, idMiddle+".sqlite")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idOldest+".sqlite"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "."+idFourth+".sqlite.partial"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stray.sqlite"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 1)

	require.NoError(t, err)
	assert.Empty(t, pruned.Deleted)
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 1, pruned.Snapshots)
	assert.FileExists(t, target)
	for _, name := range []string{idMiddle + ".sqlite", idOldest + ".sqlite", "." + idFourth + ".sqlite.partial", "stray.sqlite", "sub"} {
		_, statErr := os.Lstat(filepath.Join(dir, name))
		require.NoError(t, statErr, name)
	}
}

func Test_prune_leaves_a_manifest_that_is_not_a_regular_file(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func(t *testing.T, path, outside string)
	}{
		{"an empty directory", func(t *testing.T, path, _ string) { t.Helper(); require.NoError(t, os.Mkdir(path, 0o700)) }},
		{"a symlink", func(t *testing.T, path, outside string) { t.Helper(); require.NoError(t, os.Symlink(outside, path)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle)
			writeSnapshot(t, dir, idOldest, 1000)
			outside := filepath.Join(home, "outside.json")
			require.NoError(t, os.WriteFile(outside, []byte("{}"), 0o600))
			c.build(t, filepath.Join(dir, idOldest+".json"), outside)

			pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
			assert.Empty(t, pruned.Failed)
			assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
			_, statErr := os.Lstat(filepath.Join(dir, idOldest+".json"))
			require.NoError(t, statErr)
			assert.FileExists(t, outside)
		})
	}
}

func Test_prune_stops_deleting_once_interrupted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth, idFifth)
	writeManifest(t, dir, idSixth, manifestTaken("2026-09-24T08:00:00Z"))
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.ErrorIs(t, err, context.Canceled)
	require.EqualError(t, err, "snapshots prune interrupted; 2 snapshots were not deleted")
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.NotDeleted)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
	for _, id := range []string{idFourth, idFifth} {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
	assert.FileExists(t, filepath.Join(dir, idSixth+".json"))
}

func Test_prune_says_one_snapshot_was_not_deleted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.EqualError(t, err, "snapshots prune interrupted; 1 snapshot was not deleted")
	assert.Equal(t, 1, pruned.NotDeleted)
}

func Test_prune_counts_only_unattempted_snapshots_after_a_failure_and_an_interrupt(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth, idFifth, idSixth)
	rm, ctx := cancelling(t, idFourth+".sqlite")
	rm.faults = failing(idOldest+".sqlite", syscall.EACCES).faults

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.EqualError(t, err, "snapshots prune interrupted; 2 snapshots were not deleted")
	assert.Equal(t, 2, pruned.NotDeleted)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, idOldest, pruned.Failed[0].Entry.ID)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
}

func Test_prune_finishes_without_sweeping_when_the_context_ends_after_the_last_delete(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	writeManifest(t, dir, idFourth, manifestTaken("2026-09-26T08:00:00Z"))
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Zero(t, pruned.NotDeleted)
	assert.FileExists(t, filepath.Join(dir, idFourth+".json"))
}
