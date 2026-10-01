// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	newestID = "20260930T141502Z"
	olderID  = "20260929T090011Z"
	oldestID = "20260927T143005Z"
)

// runSnapshotsJSON runs quarry snapshots --json, returning its exit code, stdout and stderr.
func runSnapshotsJSON(t *testing.T) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode := run(context.Background(), []string{"snapshots", "--json"}, &out, &errOut)
	return exitCode, out.String(), errOut.String()
}

// snapshotsDir is the snapshots folder under home.
func snapshotsDir(home string) string {
	return filepath.Join(storeDirUnder(home), "snapshots")
}

// jsonEntries decodes stdout's "snapshots" array, each entry as raw values so a null and a missing key differ.
func jsonEntries(t *testing.T, stdout string) []map[string]json.RawMessage {
	t.Helper()
	var doc struct {
		Snapshots []map[string]json.RawMessage `json:"snapshots"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc.Snapshots
}

// jsonStoreSnapshot decodes stdout's "store_snapshot" as raw text, so a null and a missing key differ.
func jsonStoreSnapshot(t *testing.T, stdout string) string {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return string(doc["store_snapshot"])
}

func Test_run_snapshots_json_prints_the_ruled_document_for_two_snapshots_and_a_store(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: olderID, bytes: middleBytes, taken: time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), source: homeQuicken, sha256: "aaaa", verified: false},
		snapshotFixture{id: newestID, bytes: newestBytes, taken: time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), source: homeQuicken, sha256: "bbbb", verified: true},
	)
	buildStoreFrom(t, home, filepath.Join(dir, newestID+".sqlite"))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"directory\": \""+dir+"\",\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": {\n"+
		"    \"id\": \""+newestID+"\",\n"+
		"    \"path\": \""+dir+"/"+newestID+".sqlite\"\n"+
		"  },\n"+
		"  \"snapshots\": [\n"+
		"    {\n"+
		"      \"id\": \""+newestID+"\",\n"+
		"      \"path\": \""+dir+"/"+newestID+".sqlite\",\n"+
		"      \"manifest\": \""+dir+"/"+newestID+".json\",\n"+
		"      \"taken_at\": \"2026-09-30T14:15:02Z\",\n"+
		"      \"bytes\": 3240000,\n"+
		"      \"source\": \""+homeQuicken+"\",\n"+
		"      \"sha256\": \"bbbb\",\n"+
		"      \"schema_verified\": true,\n"+
		"      \"store\": true\n"+
		"    },\n"+
		"    {\n"+
		"      \"id\": \""+olderID+"\",\n"+
		"      \"path\": \""+dir+"/"+olderID+".sqlite\",\n"+
		"      \"manifest\": \""+dir+"/"+olderID+".json\",\n"+
		"      \"taken_at\": \"2026-09-29T09:00:11Z\",\n"+
		"      \"bytes\": 2240000,\n"+
		"      \"source\": \""+homeQuicken+"\",\n"+
		"      \"sha256\": \"aaaa\",\n"+
		"      \"schema_verified\": false,\n"+
		"      \"store\": false\n"+
		"    }\n"+
		"  ],\n"+
		"  \"total_bytes\": 5480000,\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
}

func Test_run_snapshots_json_nulls_every_manifest_field_but_keeps_the_keys_for_a_snapshot_with_none(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"directory\": \""+dir+"\",\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": null,\n"+
		"  \"snapshots\": [\n"+
		"    {\n"+
		"      \"id\": \""+oldestID+"\",\n"+
		"      \"path\": \""+dir+"/"+oldestID+".sqlite\",\n"+
		"      \"manifest\": null,\n"+
		"      \"taken_at\": null,\n"+
		"      \"bytes\": 1240000,\n"+
		"      \"source\": null,\n"+
		"      \"sha256\": null,\n"+
		"      \"schema_verified\": null,\n"+
		"      \"store\": false\n"+
		"    }\n"+
		"  ],\n"+
		"  \"total_bytes\": 1240000,\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
}

func Test_run_snapshots_json_nulls_only_taken_at_when_the_manifest_holds_an_unparsable_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, sha256: "aaaa", verified: true})
	unparsable := `{"snapshot":{"source":"` + homeQuicken + `","taken_at":"last tuesday","sha256":"aaaa"},"schema":{"verified":true}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, oldestID+".json"), []byte(unparsable), 0o600))

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, "null", string(entries[0]["taken_at"]))
	assert.Equal(t, `"`+dir+"/"+oldestID+`.json"`, string(entries[0]["manifest"]))
	assert.Equal(t, `"`+homeQuicken+`"`, string(entries[0]["source"]))
	assert.Equal(t, `"aaaa"`, string(entries[0]["sha256"]))
	assert.Equal(t, "true", string(entries[0]["schema_verified"]))
}

func Test_run_snapshots_json_nulls_only_source_when_the_manifest_holds_an_empty_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: "", sha256: "aaaa", verified: true})

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, "null", string(entries[0]["source"]))
	assert.Equal(t, `"`+dir+"/"+oldestID+`.json"`, string(entries[0]["manifest"]))
	assert.Equal(t, `"2026-09-27T14:30:05Z"`, string(entries[0]["taken_at"]))
	assert.Equal(t, `"aaaa"`, string(entries[0]["sha256"]))
	assert.Equal(t, "true", string(entries[0]["schema_verified"]))
}

// writeManifestJSON writes a hand-built manifest beside the snapshot with id in dir, so its fields can differ from the file.
func writeManifestJSON(t *testing.T, dir, id, snapshotJSON string) {
	t.Helper()
	manifest := `{"snapshot":` + snapshotJSON + `,"schema":{"verified":true}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), []byte(manifest), 0o600))
}

func Test_run_snapshots_json_reports_the_size_on_disk_and_not_the_size_the_manifest_records(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T14:30:05Z","bytes":999,"sha256":"aaaa"}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, "1240000", string(entries[0]["bytes"]))
}

func Test_run_snapshots_json_prints_taken_at_in_UTC_whatever_offset_the_manifest_records(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T10:30:05-04:00","bytes":1240000,"sha256":"aaaa"}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, `"2026-09-27T14:30:05Z"`, string(entries[0]["taken_at"]))
}

func Test_run_snapshots_json_passes_an_empty_recorded_sha256_through_as_an_empty_string(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T14:30:05Z","bytes":1240000,"sha256":""}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, `""`, string(entries[0]["sha256"]))
}

func Test_run_snapshots_json_prints_an_empty_list_and_the_no_snapshots_line_when_none_are_taken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, noSnapshotsLine, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"directory\": \""+snapshotsDir(home)+"\",\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": null,\n"+
		"  \"snapshots\": [],\n"+
		"  \"total_bytes\": 0,\n"+
		"  \"warnings\": [\n"+
		"    \"no snapshots in "+snapshotsDir(home)+" yet; run quarry sync to take one\"\n"+
		"  ]\n"+
		"}\n", stdout)
	assert.NoDirExists(t, snapshotsDir(home))
}

func Test_run_snapshots_json_lists_config_then_no_snapshots_then_store_warnings_and_prefixes_them_on_stderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "snapshot.keep = 3\n")
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))
	configWarning := configShown + ": unknown key snapshot.keep; quarry ignores it"
	absoluteConfigWarning := configPath(home) + ": unknown key snapshot.keep; quarry ignores it"
	noSnapshots := "no snapshots in " + snapshotsShown + " yet; run quarry sync to take one"
	absoluteNoSnapshots := "no snapshots in " + snapshotsDir(home) + " yet; run quarry sync to take one"
	storeWarning := "cannot tell which snapshot the store was built from: the file is not a DuckDB database"

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"{\n"+
		"  \"directory\": \""+snapshotsDir(home)+"\",\n"+
		"  \"keep\": 12,\n"+
		"  \"store_snapshot\": null,\n"+
		"  \"snapshots\": [],\n"+
		"  \"total_bytes\": 0,\n"+
		"  \"warnings\": [\n"+
		"    \""+absoluteConfigWarning+"\",\n"+
		"    \""+absoluteNoSnapshots+"\",\n"+
		"    \""+storeWarning+"\"\n"+
		"  ]\n"+
		"}\n", stdout)
	assert.Equal(t, ""+
		"quarry: warning: "+configWarning+"\n"+
		"quarry: "+noSnapshots+"\n"+
		"quarry: warning: "+storeWarning+"\n", stderr)
}

func Test_run_snapshots_json_gives_a_null_store_snapshot_when_the_store_cannot_name_one(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, home, dir string)
	}{
		{name: "no store", setup: func(*testing.T, string, string) {}},
		{name: "store is not a DuckDB database", setup: func(t *testing.T, home, _ string) {
			t.Helper()
			require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))
		}},
		{name: "store has no import runs", setup: func(t *testing.T, home, dir string) {
			t.Helper()
			buildStoreFrom(t, home, filepath.Join(dir, oldestID+".sqlite"))
			editStore(t, home, "DELETE FROM import_runs")
		}},
		{name: "store of another format names no snapshot", setup: func(t *testing.T, home, _ string) {
			t.Helper()
			writeStoreFixture(t, home, "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);")
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, olderPair()[0])
			c.setup(t, home, dir)

			exitCode, stdout, stderr := runSnapshotsJSON(t)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, "null", jsonStoreSnapshot(t, stdout))
		})
	}
}

func Test_run_snapshots_json_names_the_recorded_path_and_marks_nothing_when_no_listed_snapshot_is_the_stores(t *testing.T) {
	cases := []struct {
		name     string
		recorded func(home, dir string) string
	}{
		{name: "built from a path outside the folder", recorded: func(home, _ string) string {
			return filepath.Join(home, "elsewhere", "20261001T000000Z.sqlite")
		}},
		{name: "snapshot deleted by hand", recorded: func(_, dir string) string {
			return filepath.Join(dir, newestID+".sqlite")
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			dir := writeSnapshots(t, home, olderPair()...)
			recorded := c.recorded(home, dir)
			buildStoreFrom(t, home, recorded)

			exitCode, stdout, stderr := runSnapshotsJSON(t)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.JSONEq(t, `{"id":"`+snapshotID(recorded)+`","path":"`+recorded+`"}`, jsonStoreSnapshot(t, stdout))
			entries := jsonEntries(t, stdout)
			require.Len(t, entries, len(olderPair()))
			for _, entry := range entries {
				assert.Equal(t, "false", string(entry["store"]))
			}
		})
	}
}

func Test_run_snapshots_json_reports_keep_from_the_config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[snapshots]\nkeep = 24\n")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Keep int `json:"keep"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, 24, doc.Keep)
}
