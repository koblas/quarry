// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_snapshots_prune_dry_run_lists_what_it_would_delete_and_deletes_nothing(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
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
