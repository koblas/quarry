package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const interruptedPruneLine = "sync interrupted while deleting old snapshots; the store was rebuilt; run quarry snapshots prune to finish"

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

// syncBesideOldSnapshots syncs bundle through fake beside three old snapshots and an orphan manifest with a keep of 1.
func syncBesideOldSnapshots(t *testing.T, fake snapshot.Importer, bundle func(testing.TB, string) v9fixture.Bundle) afterBuild {
	t.Helper()
	home := t.TempDir()
	ids := oldIDs(3)
	dir := prunable(t, home, ids...)
	writeManifest(t, dir, oldIDs(4)[3], manifestTaken("2000-04-01T00:00:00Z"))
	rm := &fakeRemover{}
	srv := newImportServer(t, home, fake, snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := srv.SyncAndImport(t.Context(), bundle(t, t.TempDir()).Dir)
	return afterBuild{outcome: outcome, err: err, rm: rm, ids: ids, dir: dir}
}

// requireNothingDeleted asserts the sync pruned nothing and left every old snapshot and the orphan manifest.
func requireNothingDeleted(t *testing.T, got afterBuild) {
	t.Helper()
	assert.Nil(t, got.outcome.Pruned)
	assert.Empty(t, got.rm.calls)
	assertSnapshotPairs(t, got.dir, true, got.ids...)
	assert.FileExists(t, filepath.Join(got.dir, oldIDs(4)[3]+".json"))
}

func Test_sync_and_import_deletes_nothing_when_the_store_is_not_built(t *testing.T) {
	t.Parallel()
	unbuilt := store.Result{Validation: store.Validation{
		Balances: store.BalanceCheck{Checked: 1, Mismatched: []store.BalanceMismatch{{ID: "acct-1"}}},
	}}
	cases := []struct {
		name string
		fake *fakeImporter
		want error
	}{
		{"validation failed", &fakeImporter{result: unbuilt, err: store.ErrValidationFailed}, store.ErrValidationFailed},
		{"store refusal", &fakeImporter{err: errImportBoom}, errImportBoom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := syncBesideOldSnapshots(t, c.fake, v9fixture.OpenBundle)

			require.ErrorIs(t, got.err, c.want)
			requireNothingDeleted(t, got)
		})
	}
}

func Test_sync_and_import_deletes_nothing_on_a_schema_mismatch(t *testing.T) {
	t.Parallel()

	got := syncBesideOldSnapshots(t, &fakeImporter{}, v9fixture.MissingSchemaBundle)

	var mismatch snapshot.MismatchError
	require.ErrorAs(t, got.err, &mismatch)
	requireNothingDeleted(t, got)
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

func Test_sync_and_import_deletes_an_upper_case_sqlite_snapshot_then_its_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, oldIDs(3)...)
	for _, id := range oldIDs(3) {
		upperCased(t, dir, id)
	}
	rm := &fakeRemover{}
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2), snapshot.WithRemove(rm.remove))
	require.Equal(t, []string{
		"20000101T000000Z.SQLITE", "20000101T000000Z.json",
		"20000201T000000Z.SQLITE", "20000201T000000Z.json",
		"20000301T000000Z.SQLITE", "20000301T000000Z.json",
	}, dirNames(t, dir))

	_, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"20000201T000000Z.SQLITE", "20000201T000000Z.json",
		"20000101T000000Z.SQLITE", "20000101T000000Z.json",
	}, rm.calls)
	assert.Contains(t, dirNames(t, dir), "20000301T000000Z.SQLITE")
	assert.Contains(t, dirNames(t, dir), "20000301T000000Z.json")
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

func Test_sync_and_import_warns_about_each_snapshot_it_could_not_delete_and_succeeds(t *testing.T) {
	t.Parallel()
	ids := oldIDs(3)
	cases := []struct {
		name        string
		failing     []string
		wantDeleted []string
		wantFailed  []string
	}{
		{"one of two fails", []string{ids[1] + ".sqlite"}, []string{ids[0]}, []string{ids[1]}},
		{"both fail, newest first", []string{ids[0] + ".sqlite", ids[1] + ".sqlite"}, []string{}, []string{ids[1], ids[0]}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, ids...)
			srv := newImportServer(t, home, builtStore(),
				snapshot.WithAutoPrune(2), snapshot.WithRemove(failingRemover(syscall.EACCES, c.failing...).remove))

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			require.NotNil(t, outcome.Pruned)
			assert.Equal(t, c.wantDeleted, doomedIDs(outcome.Pruned.Deleted))
			require.Len(t, outcome.Pruned.Failed, len(c.wantFailed))
			var wantWarnings []string
			for i, id := range c.wantFailed {
				assert.Equal(t, id, outcome.Pruned.Failed[i].Entry.ID)
				assert.Equal(t, "permission denied", outcome.Pruned.Failed[i].Reason)
				wantWarnings = append(wantWarnings, deleteFailureLine(id, "permission denied"))
			}
			assert.Equal(t, wantWarnings, outcome.Warnings())
			assertSnapshotPairs(t, dir, true, c.wantFailed...)
		})
	}
}

func Test_outcome_lists_prune_warnings_after_the_carry_warnings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		result store.Result
		carry  []string
	}{
		{
			name:   "the history warning",
			result: store.Result{Built: true, StoreUnreadable: true, HistoryFault: &store.OpenError{Fault: store.OpenFaultNotDuckDB}},
			carry:  []string{combinedCarryLine("the file is not a DuckDB database")},
		},
		{
			name: "the findings warning",
			result: store.Result{Built: true, FindingsFault: &store.OpenError{
				Fault: store.OpenFaultOther, Reason: "its findings table is incomplete",
			}},
			carry: []string{findingsRestartLine("its findings table is incomplete")},
		},
		{
			name: "the history, findings and rates warnings",
			result: store.Result{
				Built:         true,
				HistoryFault:  &store.OpenError{Fault: store.OpenFaultOther, Reason: "its import_runs table is incomplete"},
				FindingsFault: &store.OpenError{Fault: store.OpenFaultOther, Reason: "its findings table repeats an id"},
				RatesFault:    &store.OpenError{Fault: store.OpenFaultOther, Reason: "its fx_rates table repeats a date"},
			},
			carry: []string{
				historyRestartLine("its import_runs table is incomplete"),
				findingsRestartLine("its findings table repeats an id"),
				ratesRestartLine("its fx_rates table repeats a date"),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			ids := oldIDs(2)
			prunable(t, home, ids...)
			srv := newImportServer(t, home, &fakeImporter{result: c.result},
				snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

			outcome, err := syncBundle(t, srv)

			require.NoError(t, err)
			want := append(slices.Clone(c.carry), deleteFailureLine(ids[0], "permission denied"))
			assert.Equal(t, want, outcome.Warnings())
			assert.Equal(t, want, outcome.WarningsAbsolute())
		})
	}
}

func Test_outcome_adds_no_prune_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	failed := []snapshot.PruneFailure{{Entry: snapshot.Entry{ID: idOldest}, Reason: "permission denied"}}
	built := snapshot.Outcome{Store: &store.Result{Built: true}, Pruned: &snapshot.Pruned{Failed: failed}}
	unbuilt := snapshot.Outcome{Store: &store.Result{}, Pruned: &snapshot.Pruned{Failed: failed}}

	assert.Equal(t, []string{deleteFailureLine(idOldest, "permission denied")}, built.Warnings())
	assert.Empty(t, unbuilt.Warnings())
}

// importFromUnlistableFolder rebuilds the store from a snapshot kept outside the snapshots folder,
// which can be opened but not listed.
func importFromUnlistableFolder(t *testing.T, home string) (snapshot.Outcome, string) {
	t.Helper()
	skipUnderRoot(t)
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2))
	taken := takeSnapshot(t, srv)
	dir := filepath.Join(home, "snapshots")
	kept := filepath.Join(home, "kept", filepath.Base(taken.Snapshot.Path))
	require.NoError(t, os.MkdirAll(filepath.Dir(kept), 0o700))
	for _, pair := range [][2]string{{taken.Snapshot.Path, kept}, {taken.Snapshot.Manifest, strings.TrimSuffix(kept, ".sqlite") + ".json"}} {
		require.NoError(t, os.Link(pair[0], pair[1]))
	}
	restrictMode(t, dir, 0o300)

	outcome, err := srv.ImportFrom(t.Context(), kept)

	require.NoError(t, err)
	return outcome, dir
}

func Test_import_from_warns_when_the_snapshots_folder_cannot_be_listed(t *testing.T) {
	t.Parallel()
	outcome, dir := importFromUnlistableFolder(t, t.TempDir())

	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, 2, outcome.Pruned.Keep)
	assert.Equal(t, dir, outcome.Pruned.Dir)
	assert.Empty(t, outcome.Pruned.Deleted)
	assert.Empty(t, outcome.Pruned.Failed)
	assert.Equal(t, []string{"cannot list ~/snapshots to delete old snapshots: permission denied; run quarry snapshots prune to try again"},
		outcome.Warnings())
}

func Test_outcome_names_the_unlistable_folder_by_its_absolute_path_in_the_absolute_warnings(t *testing.T) {
	t.Parallel()
	outcome, dir := importFromUnlistableFolder(t, t.TempDir())

	assert.Equal(t, []string{"cannot list " + dir + " to delete old snapshots: permission denied; run quarry snapshots prune to try again"},
		outcome.WarningsAbsolute())
}

func Test_outcome_absolute_warnings_match_the_warnings_when_no_folder_could_not_be_listed(t *testing.T) {
	t.Parallel()
	failed := []snapshot.PruneFailure{{Entry: snapshot.Entry{ID: idOldest}, Reason: "permission denied"}}
	outcome := snapshot.Outcome{Store: &store.Result{Built: true}, Pruned: &snapshot.Pruned{Failed: failed}}

	assert.Equal(t, outcome.Warnings(), outcome.WarningsAbsolute())
}

func Test_sync_and_import_neither_counts_nor_warns_about_a_snapshot_already_gone(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	dir := prunable(t, home, ids...)
	srv := newImportServer(t, home, builtStore(),
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.ENOENT, ids[1]+".sqlite").remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Empty(t, outcome.Pruned.Failed)
	assert.Empty(t, outcome.Warnings())
	assert.NoFileExists(t, filepath.Join(dir, ids[1]+".json"))
}

func Test_sync_and_import_says_nothing_when_a_manifest_or_orphan_cannot_be_removed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	dir := prunable(t, home, ids[0])
	writeManifest(t, dir, ids[1], manifestTaken("2000-02-01T00:00:00Z"))
	rm := failingRemover(syscall.EACCES, ids[0]+".json", ids[1]+".json")
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1), snapshot.WithRemove(rm.remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	assert.Empty(t, outcome.Pruned.Failed)
	assert.Empty(t, outcome.Warnings())
	assert.Contains(t, rm.calls, ids[1]+".json")
	assert.NoFileExists(t, filepath.Join(dir, ids[0]+".sqlite"))
}

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
	assert.Equal(t, 3, got.outcome.Pruned.Snapshots)
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
	assert.Equal(t, 1, got.outcome.Pruned.Snapshots)
	assert.Equal(t, []string{got.ids[1], got.ids[0]}, doomedIDs(got.outcome.Pruned.Deleted))
	assertSnapshotPairs(t, got.dir, false, got.ids[0], got.ids[1])
	assertSnapshotPairs(t, got.dir, true, got.ids[2])
	assert.NoFileExists(t, filepath.Join(got.dir, oldIDs(4)[3]+".json"))
	assert.Empty(t, got.outcome.Warnings())
	assert.Empty(t, got.outcome.WarningsAbsolute())
}
