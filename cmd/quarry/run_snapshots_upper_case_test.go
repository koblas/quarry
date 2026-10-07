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
	home := newHome(t)
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

func Test_run_snapshots_json_gives_an_upper_case_manifest_its_on_disk_path(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: newestID, bytes: newestBytes, taken: time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), source: homeQuicken, verified: true},
	)
	require.NoError(t, os.Rename(filepath.Join(dir, newestID+".json"), filepath.Join(dir, newestID+".JSON")))
	require.Equal(t, []string{"20260930T141502Z.JSON", "20260930T141502Z.sqlite"}, dirNames(t, dir))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Contains(t, stdout, "\"manifest\": \""+dir+"/20260930T141502Z.JSON\",")
}

func Test_run_snapshots_lists_an_upper_case_sqlite_snapshot_like_a_lowercase_one(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true},
		snapshotFixture{id: newestID, bytes: newestBytes, taken: time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), source: homeQuicken, verified: true},
	)
	buildStoreFrom(t, home, renamedToUpperCase(t, dir, oldestID))
	require.Contains(t, dirNames(t, dir), "20260927T143005Z.SQLITE")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260930T141502Z  2026-09-30 10:15 EDT  3.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken  store\n"+
		"Total                                   4.5 MB\n", stdout)
}

func Test_run_snapshots_prune_deletes_an_upper_case_sqlite_snapshot_and_its_manifest(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want func(dir string) string
	}{
		{name: "text shows the ID", args: []string{"--keep", "2"}, want: func(string) string {
			return "" +
				"Deleted 1 snapshot (1.2 MB), keeping the newest 2:\n" +
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n"
		}},
		{name: "json gives the on-disk path", args: []string{"--keep", "2", "--json"}, want: func(dir string) string {
			return "" +
				"{\n" +
				"  \"dry_run\": false,\n" +
				"  \"keep\": 2,\n" +
				pruneStoreSnapshotJSON(dir, pruneNoon) +
				"  \"deleted\": [\n" +
				"    {\n" +
				"      \"id\": \"20260927T143005Z\",\n" +
				"      \"path\": \"" + dir + "/20260927T143005Z.SQLITE\",\n" +
				"      \"bytes\": 1240000\n" +
				"    }\n" +
				"  ],\n" +
				"  \"would_delete\": [],\n" +
				"  \"failed\": [],\n" +
				"  \"warnings\": []\n" +
				"}\n"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home,
				pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
				pruneFixture(pruneMiddle, keptBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
				pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
			)
			renamedToUpperCase(t, dir, pruneOldest)
			renamedToUpperCase(t, dir, pruneMiddle)
			buildStoreFrom(t, home, filepath.Join(dir, pruneNoon+".sqlite"))
			require.Equal(t, []string{
				"20260927T143005Z.SQLITE", "20260927T143005Z.json",
				"20260929T090011Z.SQLITE", "20260929T090011Z.json",
				"20260930T141502Z.json", "20260930T141502Z.sqlite",
			}, dirNames(t, dir))

			exitCode, stdout, stderr := runPrune(t, c.args...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, c.want(dir), stdout)
			assert.Equal(t, []string{
				"20260929T090011Z.SQLITE", "20260929T090011Z.json",
				"20260930T141502Z.json", "20260930T141502Z.sqlite",
			}, dirNames(t, dir))
		})
	}
}
