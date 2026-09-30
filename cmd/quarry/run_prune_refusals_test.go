// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannotTellRefusal is the prune refusal for a store that cannot say which snapshot built it, with reason as its cause.
func cannotTellRefusal(reason string) string {
	return "quarry: cannot tell which snapshot the store at " + storeShown + " was built from (" + reason + "), " +
		"so no snapshot was deleted; run quarry sync to rebuild the store, then prune again\n"
}

// thirteenSnapshots is one snapshot more than the default keep, oldest first, ids 20260901T090000Z to 20260913T090000Z.
func thirteenSnapshots() []snapshotFixture {
	fixtures := make([]snapshotFixture, 13)
	for i := range fixtures {
		fixtures[i] = snapshotFixture{id: fmt.Sprintf("202609%02dT090000Z", i+1), bytes: keptBytes}
	}
	fixtures[0].bytes = oldestBytes
	return fixtures
}

// runPruneRemoving runs quarry snapshots prune with args under ctx, its Server removing files through remove.
func runPruneRemoving(ctx context.Context, t *testing.T, remove func(string) error, args ...string) (int, string, string) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
			snapshot.WithRemove(remove),
		), nil
	}

	exitCode := runWith(ctx, append([]string{"snapshots", "prune"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

// cancellingRemove removes as os.Remove does, refuses the file named refused with EACCES,
// and cancels once it has dealt with the file named trigger.
func cancellingRemove(cancel context.CancelFunc, trigger, refused string) func(string) error {
	inner := refusingRemove(refused)
	return func(path string) error {
		err := inner(path)
		if filepath.Base(path) == trigger {
			cancel()
		}
		return err
	}
}

func Test_run_snapshots_prune_without_keep_deletes_beyond_the_newest_12(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, thirteenSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, "20260913T090000Z.sqlite"))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 12:\n"+
		"  20260901T090000Z  unknown  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, "20260901T090000Z")
	assert.FileExists(t, filepath.Join(dir, "20260902T090000Z.sqlite"))
}

func Test_run_snapshots_prune_names_itself_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots prune again\n", stderr)
}

func Test_run_snapshots_prune_protects_nothing_when_there_is_no_store(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
}

func Test_run_snapshots_prune_says_nothing_to_delete_beside_an_unreadable_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_says_nothing_to_delete_within_the_newest_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()[0])

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 1 snapshot, within the newest one\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest)
}

func Test_run_snapshots_prune_names_the_stores_snapshot_when_it_is_the_only_one_beyond_the_newest_n(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, thirteenSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, "20260901T090000Z.sqlite"))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 13 snapshots, within the newest 12 and 20260901T090000Z, the store's snapshot\n", stdout)
	assert.FileExists(t, filepath.Join(dir, "20260901T090000Z.sqlite"))
}

func Test_run_snapshots_prune_says_nothing_to_delete_with_no_snapshots_folder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: no snapshots in "+snapshotsShown+"\n", stdout)
}

func Test_run_snapshots_prune_says_nothing_to_delete_when_the_only_candidate_is_already_gone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	five := fiveSnapshots()
	dir := writeSnapshots(t, home, five[0], five[4])
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	vanishing := func(path string) error {
		err := os.Remove(path)
		if filepath.Base(path) == pruneOldest+".sqlite" {
			return &fs.PathError{Op: "remove", Path: path, Err: syscall.ENOENT}
		}
		return err
	}

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, vanishing, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 1 snapshot, within the newest one\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
}

func Test_run_snapshots_prune_prints_one_line_per_failure_and_nothing_on_stdout_when_none_succeeded(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMiddle+": permission denied\n"+
		"quarry: cannot delete snapshot "+pruneOldest+": permission denied\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_says_it_was_interrupted_before_any_delete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer

	exitCode := run(ctx, []string{"snapshots", "prune", "--keep", "1"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots prune interrupted\n", stderr.String())
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_prints_what_it_deleted_before_the_interrupt_line(t *testing.T) {
	cases := []struct {
		name       string
		keep       string
		wantStderr string
		wantGone   []string
		wantKept   []string
	}{
		{
			name:       "one snapshot never attempted",
			keep:       "3",
			wantStderr: "quarry: snapshots prune interrupted; 1 snapshot was not deleted\n",
			wantGone:   []string{pruneMiddle},
			wantKept:   []string{pruneOldest, pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name:       "several snapshots never attempted",
			keep:       "2",
			wantStderr: "quarry: snapshots prune interrupted; 2 snapshots were not deleted\n",
			wantGone:   []string{pruneMorning},
			wantKept:   []string{pruneOldest, pruneMiddle, pruneNoon, pruneNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			trigger := c.wantGone[0] + ".sqlite"

			exitCode, stdout, stderr := runPruneRemoving(ctx, t, cancellingRemove(cancel, trigger, ""), "--keep", c.keep)

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.wantStderr, stderr)
			assert.Contains(t, stdout, "Deleted 1 snapshot")
			requireSnapshotsGone(t, dir, c.wantGone...)
			requireSnapshotsKept(t, dir, c.wantKept...)
		})
	}
}

func Test_run_snapshots_prune_prints_the_failure_line_before_the_interrupt_line(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remove := cancellingRemove(cancel, pruneMorning+".sqlite", pruneMorning+".sqlite")

	exitCode, stdout, stderr := runPruneRemoving(ctx, t, remove, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (0.2 MB), keeping the newest one:\n"+
		"  "+pruneNoon+"  2026-09-30 10:15 EDT  0.2 MB\n", stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMorning+": permission denied\n"+
		"quarry: snapshots prune interrupted; 2 snapshots were not deleted\n", stderr)
}

func Test_run_snapshots_prune_refuses_a_store_with_no_import_history(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	editStore(t, home, "DELETE FROM import_runs")

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the store has no import history"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_store_of_another_format_naming_no_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeStoreFixture(t, home, "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);")

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the store was built by another version of quarry"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_snapshots_folder_it_cannot_read(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
}

func Test_run_snapshots_prune_reports_a_failed_stdout_write(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, thirteenSnapshots()...)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"snapshots", "prune"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}
