// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotsKeepConfig is a config file setting snapshots.keep to keep.
func snapshotsKeepConfig(keep int) string {
	return "[snapshots]\nkeep = " + strconv.Itoa(keep) + "\n"
}

func Test_run_snapshots_prune_uses_snapshots_keep_when_keep_is_not_given(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keep_flag_overrides_snapshots_keep(t *testing.T) {
	cases := []struct {
		name       string
		configKeep int
		flagKeep   string
		stdout     string
		gone       []string
		kept       []string
	}{
		{
			name: "flag above the config", configKeep: 1, flagKeep: "3",
			stdout: "" +
				"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n" +
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n" +
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n",
			gone: []string{pruneMiddle, pruneOldest},
			kept: []string{pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name: "flag below the config", configKeep: 4, flagKeep: "1",
			stdout: "" +
				"Deleted 4 snapshots (3.9 MB), keeping the newest one:\n" +
				"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n" +
				"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n" +
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n" +
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n",
			gone: []string{pruneNoon, pruneMorning, pruneMiddle, pruneOldest},
			kept: []string{pruneNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			writeConfig(t, home, snapshotsKeepConfig(c.configKeep))

			exitCode, stdout, stderr := runPrune(t, "--keep", c.flagKeep)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stdout, stdout)
			requireSnapshotsGone(t, dir, c.gone...)
			requireSnapshotsKept(t, dir, c.kept...)
		})
	}
}

func Test_run_snapshots_prune_says_nothing_to_delete_when_snapshots_keep_equals_the_snapshot_count(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(5))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 5\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_deletes_exactly_the_oldest_when_snapshots_keep_is_one_below_the_count(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(4))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 4:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_accepts_snapshots_keep_of_one(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(1))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 4 snapshots (3.9 MB), keeping the newest one:\n"+
		"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
		"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMorning, pruneNoon, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneNewest)
}
