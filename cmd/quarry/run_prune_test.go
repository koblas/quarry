// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storeShown = "~/Library/Application Support/quarry/quarry.duckdb"

const (
	pruneOldest  = "20260927T143005Z"
	pruneMiddle  = "20260929T090011Z"
	pruneMorning = "20260930T090000Z"
	pruneNoon    = "20260930T141502Z"
	pruneNewest  = "20260930T141502Z_2"
)

// keptBytes is the size of the snapshots a test keeps: none under 0.1 MB, so the sparse file is real.
const keptBytes = 200_000

func pruneFixture(id string, bytes int64, taken time.Time) snapshotFixture {
	return snapshotFixture{id: id, bytes: bytes, taken: taken, source: homeQuicken, verified: true}
}

// fiveSnapshots is five snapshots, oldest first, the two oldest with the distinct sizes the Deleted block prints.
func fiveSnapshots() []snapshotFixture {
	return []snapshotFixture{
		pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
		pruneFixture(pruneMiddle, middleBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
		pruneFixture(pruneMorning, keptBytes, time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)),
		pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
		pruneFixture(pruneNewest, newestBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)),
	}
}

// runPrune runs quarry snapshots prune with args, returning its exit code, stdout and stderr.
func runPrune(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode := run(context.Background(), append([]string{"snapshots", "prune"}, args...), &out, &errOut)
	return exitCode, out.String(), errOut.String()
}

// requireSnapshotsGone asserts the snapshot file and manifest of each id are gone from dir.
func requireSnapshotsGone(t *testing.T, dir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		assert.NoFileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.NoFileExists(t, filepath.Join(dir, id+".json"))
	}
}

// requireSnapshotsKept asserts the snapshot file and manifest of each id are still in dir.
func requireSnapshotsKept(t *testing.T, dir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
}

func Test_run_snapshots_prune_deletes_all_but_the_newest_n(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keeps_the_stores_snapshot_when_it_is_older_than_the_newest_n(t *testing.T) {
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

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 3 and 20260801T120000Z, the store's snapshot:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
	requireSnapshotsKept(t, dir, storeSnapshot, pruneMiddle, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_when_the_store_cannot_be_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot tell which snapshot the store at "+storeShown+" was built from "+
		"(the file is not a DuckDB database), so no snapshot was deleted; "+
		"run quarry sync to rebuild the store, then prune again\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_keep_0_as_a_usage_error(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "0")

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; "+
		"Run 'quarry snapshots prune --help' for usage.\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

// refusingRemove removes as os.Remove does, except that the file named refused fails with EACCES.
func refusingRemove(refused string) func(string) error {
	return func(path string) error {
		if filepath.Base(path) == refused {
			return &fs.PathError{Op: "remove", Path: path, Err: syscall.EACCES}
		}
		return os.Remove(path)
	}
}

// Needs runWith: the one file that cannot be deleted is a Server option, not a folder mode.
func Test_run_snapshots_prune_reports_each_snapshot_it_could_not_delete_and_exits_1(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	five := fiveSnapshots()
	dir := writeSnapshots(t, home, five[0], five[1], five[4])
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
			snapshot.WithRemove(refusingRemove(pruneOldest+".sqlite")),
		), nil
	}

	exitCode := runWith(context.Background(), []string{"snapshots", "prune", "--keep", "1"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (2.2 MB), keeping the newest one:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n", stdout.String())
	assert.Equal(t, "quarry: cannot delete snapshot 20260927T143005Z: permission denied\n", stderr.String())
	requireSnapshotsGone(t, dir, pruneMiddle)
	requireSnapshotsKept(t, dir, pruneOldest, pruneNewest)
}
