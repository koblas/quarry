package snapshot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteFailureLine is the warning for a snapshot that could not be deleted, for reason.
func deleteFailureLine(id, reason string) string {
	return "cannot delete snapshot " + id + ": " + reason + "; run quarry snapshots prune to try again"
}

// failingRemover returns a fakeRemover that fails each named file with errno as os.Remove reports it.
func failingRemover(errno syscall.Errno, names ...string) *fakeRemover {
	rm := &fakeRemover{faults: map[string]error{}}
	for _, name := range names {
		rm.faults[name] = &fs.PathError{Op: "remove", Path: name, Err: errno}
	}
	return rm
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

func Test_outcome_lists_prune_warnings_after_the_history_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	result := store.Result{Built: true}
	result.HistoryFault = &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry", "quarry.duckdb")}
	result.StoreUnreadable = true
	srv := newImportServer(t, home, &fakeImporter{result: result},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []string{
		combinedCarryLine("the file is not a DuckDB database"),
		deleteFailureLine(ids[0], "permission denied"),
	}, outcome.Warnings())
}

func Test_outcome_lists_prune_warnings_after_the_findings_warning(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ids := oldIDs(2)
	prunable(t, home, ids...)
	fault := &store.OpenError{Fault: store.OpenFaultOther, Path: filepath.Join(home, "quarry", "quarry.duckdb"), Reason: "its findings table is incomplete"}
	srv := newImportServer(t, home, &fakeImporter{result: store.Result{Built: true, FindingsFault: fault}},
		snapshot.WithAutoPrune(1), snapshot.WithRemove(failingRemover(syscall.EACCES, ids[0]+".sqlite").remove))

	outcome, err := syncBundle(t, srv)

	require.NoError(t, err)
	assert.Equal(t, []string{
		findingsRestartLine("its findings table is incomplete"),
		deleteFailureLine(ids[0], "permission denied"),
	}, outcome.Warnings())
}

func Test_outcome_adds_no_prune_warning_for_a_store_that_was_not_built(t *testing.T) {
	t.Parallel()
	failed := []snapshot.PruneFailure{{Entry: snapshot.Entry{ID: idOldest}, Reason: "permission denied"}}
	built := snapshot.Outcome{Store: &store.Result{Built: true}, Pruned: &snapshot.Pruned{Failed: failed}}
	unbuilt := snapshot.Outcome{Store: &store.Result{}, Pruned: &snapshot.Pruned{Failed: failed}}

	assert.Equal(t, []string{deleteFailureLine(idOldest, "permission denied")}, built.Warnings())
	assert.Empty(t, unbuilt.Warnings())
}

// importFromUnlistableFolder rebuilds the store from a snapshot whose folder can be opened but not listed.
func importFromUnlistableFolder(t *testing.T, home string) (snapshot.Outcome, string) {
	t.Helper()
	skipUnderRoot(t)
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(2))
	taken := takeSnapshot(t, srv)
	dir := filepath.Join(home, "snapshots")
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(taken.Snapshot.Path))

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
	home := t.TempDir()
	outcome, dir := importFromUnlistableFolder(t, home)

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
