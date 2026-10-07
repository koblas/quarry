// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipOnCaseSensitiveVolume skips t unless the volume under dir resolves a file name in any letter case.
func skipOnCaseSensitiveVolume(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "caseprobe"), nil, 0o600))
	if _, err := os.Stat(filepath.Join(dir, "CASEPROBE")); err != nil {
		t.Skip("the volume is case-sensitive: a differently-cased name does not resolve")
	}
}

func Test_run_sync_from_a_lowercased_id_names_no_snapshot_and_prunes_nothing(t *testing.T) {
	home := newHome(t)
	older := oldSnapshots(1)
	id, dir := syncThenWrite(t, home, append(newerSnapshots(keptSnapshots), older...)...)
	lowered := strings.ToLower(id)

	exitCode, stdout, stderr := runSyncFrom(t, lowered)

	require.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: no snapshot "+lowered+" in "+snapshotsShown+"; run quarry snapshots to list the ones kept\n", stderr)
	assert.FileExists(t, filepath.Join(dir, older[0].id+".sqlite"))
}

func Test_run_snapshots_marks_the_store_snapshot_recorded_in_lowercase(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	skipOnCaseSensitiveVolume(t, home)
	dir := writeSnapshots(t, home, olderPair()...)
	buildStoreFrom(t, home, filepath.Join(dir, strings.ToLower(olderPair()[1].id)+".sqlite"))

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken  store\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}

func Test_run_snapshots_prune_keeps_the_store_snapshot_recorded_in_lowercase(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	skipOnCaseSensitiveVolume(t, home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, strings.ToLower(pruneOldest)+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 3 snapshots (2.6 MB), keeping the newest one and 20260927T143005Z, the store's snapshot:\n"+
		"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
		"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneNewest)
}
