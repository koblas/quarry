// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renamedToUpperCase renames id's .sqlite to .SQLITE and returns the new path.
// Case-insensitive volumes keep the old name's case unless the file is renamed.
func renamedToUpperCase(t *testing.T, dir, id string) string {
	t.Helper()
	upper := filepath.Join(dir, id+".SQLITE")
	require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
	return upper
}

// dirNames lists the names os.ReadDir returns for dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func Test_run_snapshots_json_lists_an_upper_case_sqlite_snapshot_with_its_on_disk_paths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, sha256: "aaaa", verified: true},
		snapshotFixture{id: newestID, bytes: newestBytes, taken: time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), source: homeQuicken, sha256: "bbbb", verified: true},
	)
	upper := renamedToUpperCase(t, dir, oldestID)
	buildStoreFrom(t, home, upper)
	require.Equal(t, []string{
		"20260927T143005Z.SQLITE", "20260927T143005Z.json",
		"20260930T141502Z.json", "20260930T141502Z.sqlite",
	}, dirNames(t, dir))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"directory\": \""+dir+"\",\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": {\n"+
		"    \"id\": \"20260927T143005Z\",\n"+
		"    \"path\": \""+dir+"/20260927T143005Z.SQLITE\"\n"+
		"  },\n"+
		"  \"snapshots\": [\n"+
		"    {\n"+
		"      \"id\": \"20260930T141502Z\",\n"+
		"      \"path\": \""+dir+"/20260930T141502Z.sqlite\",\n"+
		"      \"manifest\": \""+dir+"/20260930T141502Z.json\",\n"+
		"      \"taken_at\": \"2026-09-30T14:15:02Z\",\n"+
		"      \"bytes\": 3240000,\n"+
		"      \"source\": \""+homeQuicken+"\",\n"+
		"      \"sha256\": \"bbbb\",\n"+
		"      \"schema_verified\": true,\n"+
		"      \"store\": false\n"+
		"    },\n"+
		"    {\n"+
		"      \"id\": \"20260927T143005Z\",\n"+
		"      \"path\": \""+dir+"/20260927T143005Z.SQLITE\",\n"+
		"      \"manifest\": \""+dir+"/20260927T143005Z.json\",\n"+
		"      \"taken_at\": \"2026-09-27T14:30:05Z\",\n"+
		"      \"bytes\": 1240000,\n"+
		"      \"source\": \""+homeQuicken+"\",\n"+
		"      \"sha256\": \"aaaa\",\n"+
		"      \"schema_verified\": true,\n"+
		"      \"store\": true\n"+
		"    }\n"+
		"  ],\n"+
		"  \"total_bytes\": 4480000,\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
}
