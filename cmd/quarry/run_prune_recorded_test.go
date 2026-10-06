// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildStoreFromUnreadable builds the store under home from <home>/Backup/<id>.sqlite, then makes
// Backup unsearchable, so the recorded path stats with EACCES.
func buildStoreFromUnreadable(t *testing.T, home, id string) {
	t.Helper()
	backup := filepath.Join(home, "Backup")
	require.NoError(t, os.Mkdir(backup, 0o700))
	buildStoreFrom(t, home, filepath.Join(backup, id+".sqlite"))
	require.NoError(t, os.Chmod(backup, 0o000))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
}

func Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFromUnreadable(t, home, pruneMiddle)

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("cannot read ~/Backup/"+pruneMiddle+".sqlite: permission denied"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}
