package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reshapingImporter is an Importer fake that replaces the snapshot it was asked to import: reshape gets its
// path and leaves whatever should stand there.
type reshapingImporter struct {
	reshape func(path string) error
}

func (r *reshapingImporter) Import(_ context.Context, snap store.SnapshotRef) (store.Result, error) {
	return store.Result{Built: true}, r.reshape(snap.Path)
}

// replaceWithSelfLink makes path a symlink to itself: stat fails with ELOOP, and the folder scan skips it as a link.
func replaceWithSelfLink(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Symlink(path, path)
}

func Test_sync_and_import_prunes_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	t.Parallel()
	fake := &reshapingImporter{reshape: replaceWithSelfLink}

	got := syncBesideOldSnapshots(t, fake, v9fixture.OpenBundle)

	require.NoError(t, got.err)
	require.NotNil(t, got.outcome.Pruned)
	assert.Equal(t, 1, got.outcome.Pruned.Keep)
	assert.Empty(t, got.outcome.Pruned.Deleted)
	assert.Empty(t, got.outcome.Pruned.Failed)
	assert.Empty(t, got.rm.calls)
	assertSnapshotPairs(t, got.dir, true, got.ids...)
	assert.FileExists(t, filepath.Join(got.dir, oldIDs(4)[3]+".json"))
	assert.Empty(t, got.outcome.Warnings())
	assert.Empty(t, got.outcome.WarningsAbsolute())
}

func Test_sync_and_import_prunes_beyond_the_newest_and_sweeps_orphans_when_the_recorded_snapshot_is_gone(t *testing.T) {
	t.Parallel()
	fake := &reshapingImporter{reshape: os.Remove}

	got := syncBesideOldSnapshots(t, fake, v9fixture.OpenBundle)

	require.NoError(t, got.err)
	require.NotNil(t, got.outcome.Pruned)
	assert.Equal(t, []string{got.ids[1], got.ids[0]}, doomedIDs(got.outcome.Pruned.Deleted))
	assertSnapshotPairs(t, got.dir, false, got.ids[0], got.ids[1])
	assertSnapshotPairs(t, got.dir, true, got.ids[2])
	assert.NoFileExists(t, filepath.Join(got.dir, oldIDs(4)[3]+".json"))
	assert.Empty(t, got.outcome.Warnings())
	assert.Empty(t, got.outcome.WarningsAbsolute())
}
