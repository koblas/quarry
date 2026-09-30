package snapshot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const interruptedPruneLine = "sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish"

// oldIDs returns n snapshot IDs from the year 2000, oldest first: older than any ID SyncAndImport mints.
func oldIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("2000%02d01T000000Z", i+1)
	}
	return ids
}

// futureIDs returns two snapshot IDs newer than any ID SyncAndImport mints, oldest first.
func futureIDs() []string { return []string{"20990101T000000Z", "20990201T000000Z"} }

// builtStore is the Importer result of a store that was built.
func builtStore() *fakeImporter { return &fakeImporter{result: store.Result{Built: true}} }

// syncBundle runs SyncAndImport over a fresh fixture bundle.
func syncBundle(t *testing.T, srv *snapshot.Server) (snapshot.Outcome, error) {
	t.Helper()
	return srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)
}

// assertSnapshotPairs asserts each id's .sqlite and .json in dir exist (want) or are gone (!want).
func assertSnapshotPairs(t *testing.T, dir string, want bool, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, ext := range []string{".sqlite", ".json"} {
			if want {
				assert.FileExists(t, filepath.Join(dir, id+ext))
			} else {
				assert.NoFileExists(t, filepath.Join(dir, id+ext))
			}
		}
	}
}

// presenceImporter records, during Import, whether each of paths still exists.
type presenceImporter struct {
	paths   []string
	present []bool
}

func (p *presenceImporter) Import(context.Context, store.SnapshotRef) (store.Result, error) {
	for _, path := range p.paths {
		_, err := os.Stat(path)
		p.present = append(p.present, err == nil)
	}
	return store.Result{Built: true}, nil
}

func Test_sync_and_import_deletes_the_snapshots_beyond_the_newest_n_once_the_store_is_built(t *testing.T) {
	t.Parallel()
	ids := oldIDs(3)
	cases := []struct {
		name     string
		fixtures int
		want     []string
	}{
		{"as many snapshots as the newest two", 1, []string{}},
		{"one beyond the newest two", 2, []string{ids[0]}},
		{"two beyond the newest two, newest first", 3, []string{ids[1], ids[0]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, ids[:c.fixtures]...)
			srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2))

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			require.NotNil(t, outcome.Pruned)
			assert.Equal(t, 2, outcome.Pruned.Keep)
			assert.Equal(t, 2, outcome.Pruned.Snapshots)
			assert.Equal(t, c.want, doomedIDs(outcome.Pruned.Deleted))
			assert.Empty(t, outcome.Pruned.Failed)
			assert.Zero(t, outcome.Pruned.NotDeleted)
			assertSnapshotPairs(t, dir, false, c.want...)
			assertSnapshotPairs(t, dir, true, ids[len(c.want):c.fixtures]...)
			assert.FileExists(t, outcome.Manifest.Snapshot.Path)
			assert.FileExists(t, outcome.Manifest.Snapshot.Manifest)
		})
	}
}

func Test_import_from_deletes_the_snapshots_beyond_the_newest_n_once_the_store_is_built(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2))
	taken := takeSnapshot(t, srv)
	ids := oldIDs(2)
	dir := prunable(t, home, ids...)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assertSnapshotPairs(t, dir, false, ids[0])
	assertSnapshotPairs(t, dir, true, ids[1])
	assert.FileExists(t, taken.Snapshot.Path)
}

func Test_import_from_never_deletes_the_snapshot_it_built_the_store_from(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1))
	taken := takeSnapshot(t, srv)
	ids := futureIDs()
	dir := prunable(t, home, ids...)
	builtID := snapshotIDFromPath(taken.Snapshot.Path)

	outcome, err := srv.ImportFrom(t.Context(), builtID)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	require.NotNil(t, outcome.Pruned.StoreKept)
	assert.Equal(t, builtID, outcome.Pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, builtID, ids[1])
}

// linkPair hard-links id's snapshot and manifest from one folder into another: a second path to the
// same bytes that no symlink resolution maps back to the first.
func linkPair(t *testing.T, from, to, id string) string {
	t.Helper()
	for _, ext := range []string{".sqlite", ".json"} {
		require.NoError(t, os.Link(filepath.Join(from, id+ext), filepath.Join(to, id+ext)))
	}
	return filepath.Join(to, id+".sqlite")
}

func Test_import_from_a_path_outside_the_folder_protects_the_same_id_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1))
	taken := takeSnapshot(t, srv)
	ids := futureIDs()
	dir := prunable(t, home, ids...)
	builtID := snapshotIDFromPath(taken.Snapshot.Path)
	outside := linkPair(t, dir, t.TempDir(), builtID)

	outcome, err := srv.ImportFrom(t.Context(), outside)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	require.NotNil(t, outcome.Pruned.StoreKept)
	assert.Equal(t, builtID, outcome.Pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, builtID)
}

func Test_import_from_a_path_outside_the_folder_protects_nothing_without_a_same_id_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1))
	elsewhere := t.TempDir()
	taken := takeSnapshot(t, newImportServer(t, elsewhere, builtStore()))
	ids := oldIDs(3)
	dir := prunable(t, home, ids...)

	outcome, err := srv.ImportFrom(t.Context(), taken.Snapshot.Path)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[1], ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Nil(t, outcome.Pruned.StoreKept)
	assertSnapshotPairs(t, dir, true, ids[2])
}

func Test_sync_and_import_does_not_prune_without_a_keep(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts []snapshot.Option
	}{
		{"no option", nil},
		{"a keep of zero", []snapshot.Option{snapshot.WithAutoPrune(0)}},
		{"a keep below zero", []snapshot.Option{snapshot.WithAutoPrune(-1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			ids := oldIDs(3)
			dir := prunable(t, home, ids...)
			rm := &fakeRemover{}
			srv := newImportServer(t, home, builtStore(), append(c.opts, snapshot.WithRemove(rm.remove))...)

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			assert.Nil(t, outcome.Pruned)
			assert.Empty(t, rm.calls)
			assertSnapshotPairs(t, dir, true, ids...)
		})
	}
}

func Test_sync_and_import_deletes_nothing_when_the_store_is_not_built(t *testing.T) {
	t.Parallel()
	unbuilt := store.Result{Validation: store.Validation{
		Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
	}}
	cases := []struct {
		name       string
		bundle     func(testing.TB, string) v9fixture.Bundle
		fake       *fakeImporter
		requireErr func(t *testing.T, err error)
	}{
		{"validation failed", v9fixture.OpenBundle, &fakeImporter{result: unbuilt, err: store.ErrValidationFailed}, requireErrorIs(store.ErrValidationFailed)},
		{"store refusal", v9fixture.OpenBundle, &fakeImporter{err: errImportBoom}, requireErrorIs(errImportBoom)},
		{"schema mismatch", v9fixture.MissingSchemaBundle, &fakeImporter{}, func(t *testing.T, err error) {
			t.Helper()
			var mismatch snapshot.MismatchError
			require.ErrorAs(t, err, &mismatch)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			ids := oldIDs(3)
			dir := prunable(t, home, ids...)
			writeManifest(t, dir, oldIDs(4)[3], manifestTaken("2000-04-01T00:00:00Z"))
			rm := &fakeRemover{}
			srv := newImportServer(t, home, c.fake, snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

			outcome, err := srv.SyncAndImport(t.Context(), c.bundle(t, t.TempDir()).Dir)

			c.requireErr(t, err)
			assert.Nil(t, outcome.Pruned)
			assert.Empty(t, rm.calls)
			assertSnapshotPairs(t, dir, true, ids...)
			assert.FileExists(t, filepath.Join(dir, oldIDs(4)[3]+".json"))
		})
	}
}

// requireErrorIs is a check that err is want.
func requireErrorIs(want error) func(t *testing.T, err error) {
	return func(t *testing.T, err error) {
		t.Helper()
		require.ErrorIs(t, err, want)
	}
}

func Test_sync_and_import_prunes_when_the_store_is_built_on_the_same_folder_and_remover(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, oldIDs(3)...)
	writeManifest(t, dir, oldIDs(4)[3], manifestTaken("2000-04-01T00:00:00Z"))
	rm := &fakeRemover{}
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.NotEmpty(t, rm.calls)
	require.NotNil(t, outcome.Pruned)
	assert.Len(t, outcome.Pruned.Deleted, 3)
	assert.NoFileExists(t, filepath.Join(dir, oldIDs(4)[3]+".json"))
}

func Test_sync_and_import_has_every_snapshot_on_disk_while_the_store_is_built(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(3)
	dir := prunable(t, home, ids...)
	importer := &presenceImporter{}
	for _, id := range ids {
		importer.paths = append(importer.paths, filepath.Join(dir, id+".sqlite"))
	}
	srv := newImportServer(t, home, importer, snapshot.WithAutoPrune(1))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []bool{true, true, true}, importer.present)
	require.NotNil(t, outcome.Pruned)
	assert.Len(t, outcome.Pruned.Deleted, 3)
}

func Test_sync_and_import_prunes_without_reading_the_store(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, oldIDs(3)...)
	storePath := filepath.Join(home, "quarry", "quarry.duckdb")
	control := &countingProbe{path: storePath}
	_, err := newPruneServer(home, control, &fakeRemover{}).PlanPrune(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, control.calls)
	probe := &countingProbe{path: storePath}
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1), snapshot.WithStoreProbe(probe))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Len(t, outcome.Pruned.Deleted, 3)
	assert.Zero(t, probe.calls)
}

func Test_sync_and_import_sweeps_orphan_manifests_once_the_store_is_built(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	dir := prunable(t, home, ids[0])
	writeManifest(t, dir, ids[1], manifestTaken("2000-02-01T00:00:00Z"))
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(12))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Empty(t, outcome.Pruned.Deleted)
	assert.NoFileExists(t, filepath.Join(dir, ids[1]+".json"))
	assertSnapshotPairs(t, dir, true, ids[0])
}

// afterBuild is what syncAfterBuild leaves: the sync's result, the removals it made, and the old snapshots' IDs and folder.
type afterBuild struct {
	outcome snapshot.Outcome
	err     error
	rm      *fakeRemover
	ids     []string
	dir     string
}

// syncAfterBuild syncs beside three old snapshots with a keep of 1, the ctx ending right after the build when interrupt is set.
func syncAfterBuild(t *testing.T, interrupt bool) afterBuild {
	t.Helper()
	home := t.TempDir()
	ids := oldIDs(3)
	dir := prunable(t, home, ids...)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	fake := &fakeImporter{result: store.Result{Built: true}, interrupt: interrupt, cancel: cancel}
	rm := &fakeRemover{}
	srv := newImportServer(t, home, fake, snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := srv.SyncAndImport(ctx, v9fixture.OpenBundle(t, t.TempDir()).Dir)
	return afterBuild{outcome: outcome, err: err, rm: rm, ids: ids, dir: dir}
}

func Test_sync_and_import_deletes_nothing_once_interrupted_after_the_build(t *testing.T) {
	t.Parallel()

	got := syncAfterBuild(t, true)

	require.EqualError(t, got.err, interruptedPruneLine)
	require.ErrorIs(t, got.err, context.Canceled)
	require.NotNil(t, got.outcome.Store)
	assert.True(t, got.outcome.Store.Built)
	require.NotNil(t, got.outcome.Pruned)
	assert.Equal(t, 3, got.outcome.Pruned.NotDeleted)
	assert.Empty(t, got.outcome.Pruned.Deleted)
	assert.Empty(t, got.rm.calls)
	assertSnapshotPairs(t, got.dir, true, got.ids...)
}

func Test_sync_and_import_deletes_the_old_snapshots_when_not_interrupted_after_the_build(t *testing.T) {
	t.Parallel()

	got := syncAfterBuild(t, false)

	require.NoError(t, got.err)
	assert.NotEmpty(t, got.rm.calls)
	require.NotNil(t, got.outcome.Pruned)
	assert.Len(t, got.outcome.Pruned.Deleted, 3)
	assertSnapshotPairs(t, got.dir, false, got.ids...)
}

func Test_sync_and_import_is_interrupted_while_deleting_old_snapshots(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(4)
	dir := prunable(t, home, ids[:3]...)
	writeManifest(t, dir, ids[3], manifestTaken("2000-04-01T00:00:00Z"))
	rm, ctx := cancelling(t, ids[2]+".sqlite")
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := srv.SyncAndImport(ctx, v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.EqualError(t, err, interruptedPruneLine)
	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[2]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Equal(t, 2, outcome.Pruned.NotDeleted)
	assertSnapshotPairs(t, dir, false, ids[2])
	assertSnapshotPairs(t, dir, true, ids[0], ids[1])
	assert.FileExists(t, filepath.Join(dir, ids[3]+".json"))
}

func Test_sync_and_import_completes_normally_when_interrupted_with_nothing_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	dir := prunable(t, home, ids[0])
	writeManifest(t, dir, ids[1], manifestTaken("2000-02-01T00:00:00Z"))
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	fake := &fakeImporter{result: store.Result{Built: true}, interrupt: true, cancel: cancel}
	srv := newImportServer(t, home, fake, snapshot.WithAutoPrune(12))

	outcome, err := srv.SyncAndImport(ctx, v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Empty(t, outcome.Pruned.Deleted)
	assert.Zero(t, outcome.Pruned.NotDeleted)
	assert.FileExists(t, filepath.Join(dir, ids[1]+".json"))
}

func Test_sync_and_import_completes_normally_when_the_signal_lands_on_the_last_snapshot_it_deletes(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	dir := prunable(t, home, ids[0])
	writeManifest(t, dir, ids[1], manifestTaken("2000-02-01T00:00:00Z"))
	rm, ctx := cancelling(t, ids[0]+".sqlite")
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := srv.SyncAndImport(ctx, v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Zero(t, outcome.Pruned.NotDeleted)
	assert.FileExists(t, filepath.Join(dir, ids[1]+".json"))
}
