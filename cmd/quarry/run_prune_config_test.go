// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_snapshots_prune_with_nothing_beyond_the_default_cap_deletes_nothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_snapshots_keep_below_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}
