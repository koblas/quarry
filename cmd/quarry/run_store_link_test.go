// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkID is a valid snapshot ID older than every fixture, given to a link of the store's snapshot.
const linkID = "20200101T000000Z"

// storeIdentity returns, from stdout's snapshots --json document, the id of each entry marked
// "store": true and the id under "store_snapshot".
func storeIdentity(t *testing.T, stdout string) ([]string, string) {
	t.Helper()
	var doc struct {
		Snapshots []struct {
			ID    string `json:"id"`
			Store bool   `json:"store"`
		} `json:"snapshots"`
		StoreSnapshot struct {
			ID string `json:"id"`
		} `json:"store_snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	marked := []string{}
	for _, entry := range doc.Snapshots {
		if entry.Store {
			marked = append(marked, entry.ID)
		}
	}
	return marked, doc.StoreSnapshot.ID
}

func Test_run_snapshots_prune_dry_run_keeps_the_recorded_snapshot_and_counts_its_hard_link_as_deletable(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, filepath.Join(dir, pruneOldest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 4 snapshots (3.9 MB), keeping the newest one and 20260927T143005Z, the store's snapshot:\n"+
		"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
		"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20200101T000000Z  unknown               1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
	assert.FileExists(t, filepath.Join(dir, linkID+".sqlite"))
}

func Test_run_snapshots_json_marks_only_the_recorded_snapshot_when_another_is_a_hard_link_to_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	hardLink(t, snapshotFile(dir, pruneOldest), snapshotFile(dir, linkID))
	buildStoreFrom(t, home, filepath.Join(dir, pruneOldest+".sqlite"))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	marked, storeSnapshotID := storeIdentity(t, stdout)
	assert.Equal(t, []string{pruneOldest}, marked)
	assert.Equal(t, pruneOldest, storeSnapshotID)
}
