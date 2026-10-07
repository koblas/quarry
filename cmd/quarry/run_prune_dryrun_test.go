// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_snapshots_prune_dry_run_lists_what_it_would_delete_and_deletes_nothing(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_refuses_when_the_store_cannot_be_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1", "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the file is not a DuckDB database"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_uses_snapshots_keep(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_refuses_a_bad_config(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_within_the_cap(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_with_no_snapshots_folder(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: no snapshots in "+snapshotsShown+"\n", stdout)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_beside_an_unreadable_store(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
}

func Test_run_snapshots_prune_dry_run_names_the_stores_snapshot_when_it_lies_beyond_the_newest_n(t *testing.T) {
	pinLocalZone(t)
	const storeSnapshot = "20260801T120000Z"
	home := newHome(t)
	dir := writeSnapshots(t, home,
		pruneFixture(storeSnapshot, keptBytes, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)),
		pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
		pruneFixture(pruneMiddle, keptBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
		pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
		pruneFixture(pruneNewest, keptBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)),
	)
	buildStoreFrom(t, home, filepath.Join(dir, storeSnapshot+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 1 snapshot (1.2 MB), keeping the newest 3 and 20260801T120000Z, the store's snapshot:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, storeSnapshot, pruneOldest, pruneMiddle, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_reports_a_folder_it_cannot_read(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	var removed []string
	spy := func(path string) error {
		removed = append(removed, path)
		return nil
	}

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, spy, "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
	assert.Empty(t, removed)
}

func Test_run_snapshots_prune_dry_run_is_interrupted_before_the_store_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, stdout, stderr := runCapture(ctx, []string{"snapshots", "prune", "--keep", "1", "--dry-run"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots prune interrupted\n", stderr.String())
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_counts_the_cap_exactly(t *testing.T) {
	cases := []struct {
		name string
		keep string
		want string
	}{
		{"as many snapshots as the cap", "5", "Nothing to delete: 5 snapshots, within the newest 5\n"},
		{"one snapshot beyond the cap", "4", "Would delete 1 snapshot (1.2 MB), keeping the newest 4:\n  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

			exitCode, stdout, stderr := runPrune(t, "--keep", c.keep, "--dry-run")

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.want, stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}
