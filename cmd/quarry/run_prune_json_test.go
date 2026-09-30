// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneEntryJSON is one deleted or would_delete entry as the indented document prints it.
func pruneEntryJSON(dir, id string, bytes int64) string {
	return "" +
		"    {\n" +
		"      \"id\": \"" + id + "\",\n" +
		"      \"path\": \"" + dir + "/" + id + ".sqlite\",\n" +
		"      \"bytes\": " + strconv.FormatInt(bytes, 10) + "\n" +
		"    }"
}

// pruneStoreSnapshotJSON is the store_snapshot object naming the snapshot with id in dir.
func pruneStoreSnapshotJSON(dir, id string) string {
	return "" +
		"  \"store_snapshot\": {\n" +
		"    \"id\": \"" + id + "\",\n" +
		"    \"path\": \"" + dir + "/" + id + ".sqlite\"\n" +
		"  },\n"
}

// pruneDocument decodes stdout's top-level keys as raw text, so a null and a missing key differ.
func pruneDocument(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc
}

func Test_run_snapshots_prune_json_prints_the_ruled_document_for_a_real_run(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 3,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneMiddle, middleBytes)+",\n"+
		pruneEntryJSON(dir, pruneOldest, oldestBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
}

func Test_run_snapshots_prune_json_fills_would_delete_and_leaves_deleted_and_failed_empty_for_a_dry_run(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--dry-run", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": true,\n"+
		"  \"keep\": 3,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [],\n"+
		"  \"would_delete\": [\n"+
		pruneEntryJSON(dir, pruneMiddle, middleBytes)+",\n"+
		pruneEntryJSON(dir, pruneOldest, oldestBytes)+"\n"+
		"  ],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_json_prints_empty_lists_and_the_store_snapshot_when_nothing_is_beyond_the_cap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNoon+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 12,\n"+
		pruneStoreSnapshotJSON(dir, pruneNoon)+
		"  \"deleted\": [],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
}

func Test_run_snapshots_prune_json_gives_a_null_store_snapshot_without_a_warning_when_the_unreadable_store_blocks_nothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": null,\n"+
		"  \"deleted\": [],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_json_gives_a_null_store_snapshot_when_there_is_no_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "null", string(pruneDocument(t, stdout)["store_snapshot"]))
}

func Test_run_snapshots_prune_json_prints_the_stores_snapshot_and_does_not_delete_it_when_it_lies_beyond_the_newest_n(t *testing.T) {
	const storeSnapshot = "20260801T120000Z"
	home := t.TempDir()
	t.Setenv("HOME", home)
	fixtures := append([]snapshotFixture{pruneFixture(storeSnapshot, keptBytes, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))}, fiveSnapshots()...)
	dir := writeSnapshots(t, home, fixtures...)
	buildStoreFrom(t, home, filepath.Join(dir, storeSnapshot+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

	require.Equal(t, 0, exitCode, stderr)
	doc := pruneDocument(t, stdout)
	assert.JSONEq(t, `{"id":"`+storeSnapshot+`","path":"`+dir+"/"+storeSnapshot+`.sqlite"}`, string(doc["store_snapshot"]))
	assert.JSONEq(t, `[
		{"id":"`+pruneMiddle+`","path":"`+dir+"/"+pruneMiddle+`.sqlite","bytes":2240000},
		{"id":"`+pruneOldest+`","path":"`+dir+"/"+pruneOldest+`.sqlite","bytes":1240000}
	]`, string(doc["deleted"]))
	requireSnapshotsKept(t, dir, storeSnapshot)
}

// Needs runPruneRemoving: the one file that cannot be deleted is a Server option, not a folder mode.
func Test_run_snapshots_prune_json_lists_the_failed_delete_beside_the_deleted_one_and_exits_1(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	five := fiveSnapshots()
	dir := writeSnapshots(t, home, five[0], five[1], five[4])
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, refusingRemove(pruneOldest+".sqlite"), "--keep", "1", "--json")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 1,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneMiddle, middleBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [\n"+
		"    {\n"+
		"      \"id\": \""+pruneOldest+"\",\n"+
		"      \"path\": \""+dir+"/"+pruneOldest+".sqlite\",\n"+
		"      \"reason\": \"permission denied\"\n"+
		"    }\n"+
		"  ],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	assert.Equal(t, "quarry: cannot delete snapshot "+pruneOldest+": permission denied\n", stderr)
	requireSnapshotsGone(t, dir, pruneMiddle)
	requireSnapshotsKept(t, dir, pruneOldest, pruneNewest)
}

func Test_run_snapshots_prune_json_prints_what_was_deleted_when_the_run_is_interrupted_and_exits_1(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remove := cancellingRemove(cancel, pruneMorning+".sqlite", "")

	exitCode, stdout, stderr := runPruneRemoving(ctx, t, remove, "--keep", "1", "--json")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 1,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneNoon, keptBytes)+",\n"+
		pruneEntryJSON(dir, pruneMorning, keptBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	assert.Equal(t, "quarry: snapshots prune interrupted; 2 snapshots were not deleted\n", stderr)
}

func Test_run_snapshots_prune_json_prints_no_document_when_the_run_refuses(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, home string)
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name: "store cannot say which snapshot built it",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
				require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))
			},
			args:       []string{"--keep", "1", "--json"},
			wantExit:   1,
			wantStderr: cannotTellRefusal("the file is not a DuckDB database"),
		},
		{
			name: "store cannot say which snapshot built it under --dry-run",
			setup: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
				require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))
			},
			args:       []string{"--keep", "1", "--dry-run", "--json"},
			wantExit:   1,
			wantStderr: cannotTellRefusal("the file is not a DuckDB database"),
		},
		{
			name: "config file is refused",
			setup: func(t *testing.T, home string) {
				t.Helper()
				writeConfig(t, home, "snapshots.keep = 0\n")
			},
			args:       []string{"--json"},
			wantExit:   1,
			wantStderr: "quarry: " + configShown + ": snapshots.keep must be a whole number of 1 or more, got 0" + configFix + "\n",
		},
		{
			name:       "usage error",
			setup:      func(*testing.T, string) {},
			args:       []string{"--keep", "0", "--json"},
			wantExit:   2,
			wantStderr: "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; Run 'quarry snapshots prune --help' for usage.\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			c.setup(t, home)

			exitCode, stdout, stderr := runPrune(t, c.args...)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, c.wantStderr, stderr)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_json_carries_a_config_warning_in_warnings_and_prefixed_on_stderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshot.keep = 3\n")
	warning := configShown + ": unknown key snapshot.keep; quarry ignores it"

	exitCode, stdout, stderr := runPrune(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.JSONEq(t, `["`+warning+`"]`, string(pruneDocument(t, stdout)["warnings"]))
	assert.Equal(t, "quarry: warning: "+warning+"\n", stderr)
}

func Test_run_snapshots_prune_json_reports_the_keep_from_the_config_when_keep_is_not_given(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t, "--dry-run", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "3", string(pruneDocument(t, stdout)["keep"]))
}
