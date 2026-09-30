package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idLinkFuture is a snapshot ID newer than any SyncAndImport mints and older than futureIDs.
const idLinkFuture = "20980101T000000Z"

func Test_prune_deletes_no_snapshot_that_is_the_file_the_store_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
	recorded := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))

	pruned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).Prune(t.Context(), 1)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	require.NotNil(t, pruned.StoreKept)
	assert.Equal(t, idLinkBetween, pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, idMiddle)
	assert.FileExists(t, snapshotFile(dir, idLinkBetween))
}

func Test_prune_dry_run_lists_no_snapshot_that_is_the_file_the_store_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
	recorded := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))

	planned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).PlanPrune(t.Context(), 1)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(planned.WouldDelete))
	require.NotNil(t, planned.StoreKept)
	assert.Equal(t, idLinkBetween, planned.StoreKept.ID)
}

func Test_import_from_deletes_no_snapshot_that_is_the_file_it_built_the_store_from(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1))
	taken := takeSnapshot(t, srv)
	builtID := snapshotIDFromPath(taken.Snapshot.Path)
	ids := futureIDs()
	dir := prunable(t, home, ids...)
	hardLink(t, taken.Snapshot.Path, snapshotFile(dir, idLinkFuture))
	outside := t.TempDir()
	hardLink(t, taken.Snapshot.Manifest, filepath.Join(outside, "linked.json"))
	recorded := hardLink(t, taken.Snapshot.Path, filepath.Join(outside, "linked.sqlite"))

	outcome, err := srv.ImportFrom(t.Context(), recorded)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	require.NotNil(t, outcome.Pruned.StoreKept)
	assert.Equal(t, idLinkFuture, outcome.Pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, builtID)
	assert.FileExists(t, snapshotFile(dir, idLinkFuture))
}
