// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const snapshotsShown = "~/Library/Application Support/quarry/snapshots"

// Distinct sizes, none under 0.1 MB, whose rounded parts do not add up to the
// rounded whole: 1.2 + 2.2 = 3.4 MB, yet 3,480,000 bytes print as 3.5 MB.
const (
	oldestBytes = 1_240_000
	middleBytes = 2_240_000
	newestBytes = 3_240_000
)

const (
	homeQuicken   = "/Users/x/Documents/Home.quicken"
	familyQuicken = "/Users/x/Documents/Family.quicken"
)

// snapshotFixture is one snapshot to write into the folder. A zero taken
// writes the .sqlite alone, with no manifest.
type snapshotFixture struct {
	id       string
	bytes    int64
	taken    time.Time
	source   string
	sha256   string
	verified bool
}

// pinLocalZone makes the Taken column print in UTC-4, named EDT, wherever the test runs.
func pinLocalZone(t *testing.T) {
	t.Helper()
	saved := time.Local                          //nolint:gosmopolitan // the Taken column prints the local zone
	time.Local = time.FixedZone("EDT", -4*60*60) //nolint:gosmopolitan // the Taken column prints the local zone
	t.Cleanup(func() { time.Local = saved })     //nolint:gosmopolitan // restores the zone
}

// writeSnapshots writes fixtures into the snapshots folder under home: each
// .sqlite as a sparse file of the given size, each manifest through Manifest.Encode.
func writeSnapshots(t *testing.T, home string, fixtures ...snapshotFixture) string {
	t.Helper()
	dir := filepath.Join(storeDirUnder(home), "snapshots")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	for _, f := range fixtures {
		path := filepath.Join(dir, f.id+".sqlite")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		require.NoError(t, os.Truncate(path, f.bytes))
		if f.taken.IsZero() {
			continue
		}
		manifest := snapshot.Manifest{
			Snapshot: snapshot.Info{Source: f.source, TakenAt: f.taken.Format(time.RFC3339), Bytes: f.bytes, SHA256: f.sha256},
			Schema:   snapshot.SchemaInfo{Verified: f.verified},
		}
		data, err := manifest.Encode()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.id+".json"), data, 0o600))
	}
	return dir
}

// buildStoreFrom builds the store under home with one import run that names snapshotPath.
func buildStoreFrom(t *testing.T, home, snapshotPath string) {
	t.Helper()
	rows := spendRows(nil)
	rows.ImportRuns[0].Snapshot.Path = snapshotPath
	replaceStore(t, home, rows)
}

// runSnapshots runs quarry snapshots, returning its exit code, stdout and stderr.
func runSnapshots(t *testing.T) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode := run(context.Background(), []string{"snapshots"}, &out, &errOut)
	return exitCode, out.String(), errOut.String()
}

// olderPair is the two snapshots the folded tests list: 2.2 MB taken 2026-09-29, 1.2 MB taken 2026-09-27.
func olderPair() []snapshotFixture {
	return []snapshotFixture{
		{id: "20260927T143005Z", bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true},
		{id: "20260929T090011Z", bytes: middleBytes, taken: time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), source: homeQuicken, verified: true},
	}
}

func Test_run_snapshots_lists_newest_first_marks_the_stores_snapshot_and_totals_the_size(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: "20260927T143005Z", bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true},
		snapshotFixture{id: "20260930T141502Z", bytes: middleBytes, taken: time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC), source: homeQuicken, verified: true},
		snapshotFixture{id: "20260930T141502Z_2", bytes: newestBytes, taken: time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC), source: familyQuicken, verified: true},
	)
	buildStoreFrom(t, home, filepath.Join(dir, "20260930T141502Z.sqlite"))

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                  Taken                   Size  Source          Status\n"+
		"20260930T141502Z_2  2026-09-30 14:30 EDT  3.2 MB  Family.quicken\n"+
		"20260930T141502Z    2026-09-30 10:15 EDT  2.2 MB  Home.quicken    store\n"+
		"20260927T143005Z    2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                     6.7 MB\n", stdout)
}

func Test_run_snapshots_warns_about_an_unknown_config_key_and_still_lists(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	writeConfig(t, home, "snapshot.keep = 3\n")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}

func Test_run_snapshots_marks_a_schema_mismatch_and_a_missing_manifest(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home,
		snapshotFixture{id: "20260927T143005Z", bytes: oldestBytes},
		snapshotFixture{id: "20260929T090011Z", bytes: middleBytes, taken: time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), source: homeQuicken},
	)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken  schema differs\n"+
		"20260927T143005Z  unknown               1.2 MB  unknown       no manifest\n"+
		"Total                                   3.5 MB\n", stdout)
}

func Test_run_snapshots_with_none_taken_yet_says_how_to_take_one(t *testing.T) {
	home := newHome(t)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "ID  Taken  Size  Source  Status\n", stdout)
	assert.Equal(t, "quarry: no snapshots in "+snapshotsShown+" yet; run quarry sync to take one\n", stderr)
	assert.NoDirExists(t, filepath.Join(storeDirUnder(home), "snapshots"))
}

func Test_run_snapshots_warns_when_the_store_cannot_be_read(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	writeNonDuckDBStore(t, home)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot tell which snapshot the store was built from: the file is not a DuckDB database\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}

// recordedID is the snapshot the recorded-path cells record: the newer of olderPair's two.
const recordedID = "20260929T090011Z"

// cannotTellSentence is the sentence that the store's snapshot cannot be told, naming c's recorded
// path as shown; "" for a readable class.
func (c recordedClass) cannotTellSentence(shown string) string {
	if c.reason == "" {
		return ""
	}
	return "cannot tell which snapshot the store was built from: " + c.cannotReadPhrase(shown)
}

// stderrWarning is what snapshots prints to stderr for c: the warning line, or nothing.
func (c recordedClass) stderrWarning() string {
	if c.reason == "" {
		return ""
	}
	return "quarry: warning: " + c.cannotTellSentence("~/Backup/"+recordedID+".sqlite") + "\n"
}

// storeStatus is the Status cell of the recorded snapshot's row, with its leading gap.
func (c recordedClass) storeStatus() string {
	if c.marked {
		return "  store"
	}
	return ""
}

// jsonWarnings is the warnings array snapshots --json prints for c, the path in its absolute form.
func (c recordedClass) jsonWarnings(recorded string) []string {
	if c.reason == "" {
		return []string{}
	}
	return []string{c.cannotTellSentence(recorded)}
}

// snapshotsRecordedCell runs the snapshots command through run beside olderPair and a store built from
// recordedID recorded as c has it; it returns the recorded path and the run's result.
func snapshotsRecordedCell(t *testing.T, c recordedClass, run func(*testing.T) (int, string, string)) (string, int, string, string) {
	t.Helper()
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	recorded := buildStoreFromClass(t, home, recordedID, c)
	exitCode, stdout, stderr := run(t)
	return recorded, exitCode, stdout, stderr
}

func Test_run_snapshots_warns_and_marks_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	buildStoreFromUnreadable(t, home, recordedID)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, cannotTellPrefix+"cannot read ~/Backup/"+recordedID+".sqlite: permission denied\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}

func Test_run_snapshots_recorded_snapshot_cells(t *testing.T) {
	for _, c := range recordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			_, exitCode, stdout, stderr := snapshotsRecordedCell(t, c, runSnapshots)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stderrWarning(), stderr)
			assert.Equal(t, ""+
				"ID                Taken                   Size  Source        Status\n"+
				"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken"+c.storeStatus()+"\n"+
				"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
				"Total                                   3.5 MB\n", stdout)
		})
	}
}

func Test_run_snapshots_json_recorded_snapshot_cells(t *testing.T) {
	for _, c := range recordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			recorded, exitCode, stdout, stderr := snapshotsRecordedCell(t, c, runSnapshotsJSON)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stderrWarning(), stderr)
			var doc struct {
				StoreSnapshot struct {
					ID   string `json:"id"`
					Path string `json:"path"`
				} `json:"store_snapshot"`
				Snapshots []struct {
					ID    string `json:"id"`
					Store bool   `json:"store"`
				} `json:"snapshots"`
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			assert.Equal(t, recordedID, doc.StoreSnapshot.ID)
			assert.Equal(t, recorded, doc.StoreSnapshot.Path)
			assert.Equal(t, c.marked, doc.Snapshots[0].Store)
			assert.False(t, doc.Snapshots[1].Store)
			assert.Equal(t, c.jsonWarnings(recorded), doc.Warnings)
		})
	}
}

const noSnapshotsLine = "quarry: no snapshots in " + snapshotsShown + " yet; run quarry sync to take one\n"

const cannotTellPrefix = "quarry: warning: cannot tell which snapshot the store was built from: "

func Test_run_snapshots_refuses_a_malformed_config_with_nothing_on_stdout(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read "+configShown+": line 1: ")+"[^\n]+"+regexp.QuoteMeta(configFix)+"\n$", stderr)
}

func Test_run_snapshots_refuses_a_bad_config_value_with_nothing_on_stdout(t *testing.T) {
	cases := append([]badConfigValue{{
		name: "keep below one", content: "snapshots.keep = 0\n",
		line: configShown + ": snapshots.keep must be a whole number of 1 or more, got 0",
	}}, quickenAndReportingBadValues()...)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			writeSnapshots(t, home, olderPair()...)
			writeConfig(t, home, c.content)

			exitCode, stdout, stderr := runSnapshots(t)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: "+c.line+configFix+"\n", stderr)
		})
	}
}

func Test_run_snapshots_refuses_a_config_it_cannot_read(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+configShown+": is a directory"+configFix+"\n", stderr)
}

func Test_run_snapshots_never_looks_for_the_quicken_path_it_is_configured_with(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()[0])
	writeConfig(t, home, "quicken.path = \"~/Books/Missing.quicken\"\n")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
}

func Test_run_snapshots_prune_and_status_name_themselves_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	cases := []struct {
		name string
		args []string
		said string
	}{
		{name: "snapshots", args: []string{"snapshots"}, said: "quarry snapshots"},
		{name: "snapshots prune", args: []string{"snapshots", "prune"}, said: "quarry snapshots prune"},
		{name: "status", args: []string{"status"}, said: "quarry status"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", "")

			exitCode, stdout, stderr := runCapture(context.Background(), c.args)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run "+c.said+" again\n", stderr.String())
		})
	}
}

// LoadConfig is stubbed, so the refusal comes from the snapshots factory's own home lookup.
func Test_run_snapshots_and_prune_factories_name_themselves_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	cases := []struct {
		name string
		args []string
		said string
	}{
		{name: "snapshots", args: []string{"snapshots"}, said: "quarry snapshots"},
		{name: "snapshots prune", args: []string{"snapshots", "prune"}, said: "quarry snapshots prune"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", "")
			var stdout, stderr bytes.Buffer
			env := testEnv(&stdout, &stderr)
			env.LoadConfig = func(string) (config.Config, error) { return config.Config{}, nil }

			exitCode := runWith(context.Background(), c.args, env)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run "+c.said+" again\n", stderr.String())
		})
	}
}

func Test_run_snapshots_reports_a_failed_stdout_write_and_prints_no_note(t *testing.T) {
	newHome(t)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"snapshots"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

func Test_run_snapshots_refuses_a_snapshots_folder_it_cannot_read(t *testing.T) {
	cases := []struct {
		name string
		mode os.FileMode
	}{
		{name: "folder cannot be listed", mode: 0o000},
		{name: "snapshot cannot be stat-ed", mode: 0o400},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			skipAsRoot(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, olderPair()...)
			require.NoError(t, os.Chmod(dir, c.mode))
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

			exitCode, stdout, stderr := runSnapshots(t)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
		})
	}
}

func Test_run_snapshots_says_it_was_interrupted(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, olderPair()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, stdout, stderr := runCapture(ctx, []string{"snapshots"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots interrupted\n", stderr.String())
}

func Test_run_snapshots_prints_the_no_snapshots_note_before_the_store_warning(t *testing.T) {
	home := newHome(t)
	writeNonDuckDBStore(t, home)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "ID  Taken  Size  Source  Status\n", stdout)
	assert.Equal(t, noSnapshotsLine+cannotTellPrefix+"the file is not a DuckDB database\n", stderr)
}

func Test_run_snapshots_warns_when_the_store_has_no_import_history(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, olderPair()[0])
	buildStoreFrom(t, home, filepath.Join(dir, "20260927T143005Z.sqlite"))
	editStore(t, home, "DELETE FROM import_runs")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, cannotTellPrefix+"the store has no import history\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
}

func Test_run_snapshots_warns_when_a_store_of_another_format_names_no_snapshot(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home, olderPair()[0])
	writeStoreFixture(t, home, "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);")

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, cannotTellPrefix+"the store was built by another version of quarry\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
}

func Test_run_snapshots_shows_unknown_for_a_taken_at_or_source_the_manifest_does_not_hold(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: "20260927T143005Z", bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken},
		snapshotFixture{id: "20260929T090011Z", bytes: middleBytes, taken: time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC), source: "", verified: true},
	)
	unparsable := `{"snapshot":{"source":"` + homeQuicken + `","taken_at":"last tuesday"},"schema":{"verified":false}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.json"), []byte(unparsable), 0o600))

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  unknown\n"+
		"20260927T143005Z  unknown               1.2 MB  Home.quicken  schema differs\n"+
		"Total                                   3.5 MB\n", stdout)
}

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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T14:30:05Z","bytes":999,"sha256":"aaaa"}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, "1240000", string(entries[0]["bytes"]))
}

func Test_run_snapshots_json_prints_taken_at_in_UTC_whatever_offset_the_manifest_records(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T10:30:05-04:00","bytes":1240000,"sha256":"aaaa"}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, `"2026-09-27T14:30:05Z"`, string(entries[0]["taken_at"]))
}

func Test_run_snapshots_json_passes_an_empty_recorded_sha256_through_as_an_empty_string(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeManifestJSON(t, dir, oldestID, `{"source":"`+homeQuicken+`","taken_at":"2026-09-27T14:30:05Z","bytes":1240000,"sha256":""}`)

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	entries := jsonEntries(t, stdout)
	require.Len(t, entries, 1)
	assert.Equal(t, `""`, string(entries[0]["sha256"]))
}

func Test_run_snapshots_json_prints_an_empty_list_and_the_no_snapshots_line_when_none_are_taken(t *testing.T) {
	home := newHome(t)

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
	home := newHome(t)
	writeConfig(t, home, "snapshot.keep = 3\n")
	writeNonDuckDBStore(t, home)
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
			writeNonDuckDBStore(t, home)
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
			home := newHome(t)
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
			home := newHome(t)
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
	home := newHome(t)
	writeConfig(t, home, "[snapshots]\nkeep = 24\n")

	exitCode, stdout, stderr := runSnapshotsJSON(t)

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Keep int `json:"keep"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, 24, doc.Keep)
}

// renamedEntry reports name in place of the name of the real entry it wraps, keeping that entry's type and Info.
type renamedEntry struct {
	fs.DirEntry

	name string
}

func (e renamedEntry) Name() string { return e.name }

// readDirWithVariant is os.ReadDir plus one more regular entry named variant that wraps the real entry named like.
// A macOS volume cannot hold two letter cases of one name, so the second case exists only in this listing.
func readDirWithVariant(variant, like string) func(string) ([]fs.DirEntry, error) {
	return readDirWithVariants([2]string{variant, like})
}

// readDirWithVariants is readDirWithVariant for several [variant, like] pairs.
func readDirWithVariants(pairs ...[2]string) func(string) ([]fs.DirEntry, error) {
	return func(dir string) ([]fs.DirEntry, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		onDisk := entries
		for _, pair := range pairs {
			for _, entry := range onDisk {
				if entry.Name() == pair[1] {
					entries = append(entries, renamedEntry{DirEntry: entry, name: pair[0]})
				}
			}
		}
		return entries, nil
	}
}

// runSnapshotsWithReadDir runs the snapshots command args over a Server that lists its folders through readDir, plus opts.
func runSnapshotsWithReadDir(t *testing.T, home string, readDir func(string) ([]fs.DirEntry, error), opts []snapshot.Option, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(append([]snapshot.Option{
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithReadDir(readDir),
		}, opts...)...), nil
	}
	exitCode := runWith(context.Background(), append([]string{"snapshots"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

func Test_run_snapshots_lists_one_of_two_letter_cases_and_warns_naming_both(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	writeSnapshots(t, home,
		snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true})
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, nil)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   1.2 MB\n", stdout)
	assert.Equal(t, "quarry: warning: "+snapshotsShown+" holds both "+oldestID+".sqlite and "+oldestID+".SQLITE; "+
		"quarry lists, prunes and uses only "+oldestID+".sqlite; rename or remove the other\n", stderr)
}

func Test_run_snapshots_json_warns_about_a_stray_letter_case_with_the_absolute_folder(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, nil, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Snapshots []struct {
			ID string `json:"id"`
		} `json:"snapshots"`
		TotalBytes int64    `json:"total_bytes"`
		Warnings   []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	require.Len(t, doc.Snapshots, 1)
	assert.Equal(t, oldestID, doc.Snapshots[0].ID)
	assert.Equal(t, int64(oldestBytes), doc.TotalBytes)
	assert.Equal(t, []string{dir + " holds both " + oldestID + ".sqlite and " + oldestID + ".SQLITE; " +
		"quarry lists, prunes and uses only " + oldestID + ".sqlite; rename or remove the other"}, doc.Warnings)
}

// twoStrayCases is one ID holding a stray letter case of its snapshot and of its manifest.
func twoStrayCases(t *testing.T) (string, string, func(string) ([]fs.DirEntry, error)) {
	t.Helper()
	home := newHome(t)
	dir := writeSnapshots(t, home,
		snapshotFixture{id: oldestID, bytes: oldestBytes, taken: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC), source: homeQuicken, verified: true})
	readDir := readDirWithVariants(
		[2]string{oldestID + ".SQLITE", oldestID + ".sqlite"},
		[2]string{oldestID + ".JSON", oldestID + ".json"})
	return home, dir, readDir
}

const (
	strayManifestLine = " holds both " + oldestID + ".json and " + oldestID + ".JSON; " +
		"quarry lists, prunes and uses only " + oldestID + ".json; rename or remove the other"
	straySnapshotLine = " holds both " + oldestID + ".sqlite and " + oldestID + ".SQLITE; " +
		"quarry lists, prunes and uses only " + oldestID + ".sqlite; rename or remove the other"
)

func Test_run_snapshots_warns_on_stderr_about_each_stray_letter_case_of_one_id_snapshot_first(t *testing.T) {
	home, _, readDir := twoStrayCases(t)

	exitCode, _, stderr := runSnapshotsWithReadDir(t, home, readDir, nil)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, ""+
		"quarry: warning: "+snapshotsShown+straySnapshotLine+"\n"+
		"quarry: warning: "+snapshotsShown+strayManifestLine+"\n", stderr)
}

func Test_run_snapshots_json_warns_about_each_stray_letter_case_of_one_id_snapshot_first(t *testing.T) {
	home, dir, readDir := twoStrayCases(t)

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, nil, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, []string{dir + straySnapshotLine, dir + strayManifestLine}, doc.Warnings)
}

func Test_run_snapshots_json_puts_the_stray_letter_case_warning_between_the_config_and_store_warnings(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "snapshot.keep = 3\n")
	dir := writeSnapshots(t, home, snapshotFixture{id: oldestID, bytes: oldestBytes})
	writeNonDuckDBStore(t, home)
	readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")
	stray := " holds both " + oldestID + ".sqlite and " + oldestID + ".SQLITE; " +
		"quarry lists, prunes and uses only " + oldestID + ".sqlite; rename or remove the other"
	storeWarning := "cannot tell which snapshot the store was built from: the file is not a DuckDB database"

	exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, []snapshot.Option{
		snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
	}, "--json")

	require.Equal(t, 0, exitCode, stderr)
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	assert.Equal(t, []string{
		configPath(home) + ": unknown key snapshot.keep; quarry ignores it",
		dir + stray,
		storeWarning,
	}, doc.Warnings)
	assert.Equal(t, ""+
		"quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		"quarry: warning: "+snapshotsShown+stray+"\n"+
		"quarry: warning: "+storeWarning+"\n", stderr)
}

// recordingRemove records the base name of every file prune asks to remove and removes nothing.
func recordingRemove(removed *[]string) snapshot.Option {
	return snapshot.WithRemove(func(path string) error {
		*removed = append(*removed, filepath.Base(path))
		return nil
	})
}

func Test_run_snapshots_prune_is_silent_about_a_stray_letter_case(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantStdout  func(dir string) string
		wantRemoved []string
	}{
		{
			name: "text",
			args: []string{"prune", "--keep", "1"},
			wantStdout: func(string) string {
				return "Deleted 1 snapshot (1.2 MB), keeping the newest one:\n" +
					"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n"
			},
			wantRemoved: []string{oldestID + ".sqlite"},
		},
		{
			name: "json",
			args: []string{"prune", "--keep", "1", "--json"},
			wantStdout: func(dir string) string {
				return "{\n" +
					"  \"dry_run\": false,\n" +
					"  \"keep\": 1,\n" +
					pruneStoreSnapshotJSON(dir, recordedID) +
					"  \"deleted\": [\n" +
					pruneEntryJSON(dir, oldestID, oldestBytes) + "\n" +
					"  ],\n" +
					"  \"would_delete\": [],\n" +
					"  \"failed\": [],\n" +
					"  \"warnings\": []\n" +
					"}\n"
			},
			wantRemoved: []string{oldestID + ".sqlite"},
		},
		{
			name: "dry run json",
			args: []string{"prune", "--keep", "1", "--dry-run", "--json"},
			wantStdout: func(dir string) string {
				return "{\n" +
					"  \"dry_run\": true,\n" +
					"  \"keep\": 1,\n" +
					pruneStoreSnapshotJSON(dir, recordedID) +
					"  \"deleted\": [],\n" +
					"  \"would_delete\": [\n" +
					pruneEntryJSON(dir, oldestID, oldestBytes) + "\n" +
					"  ],\n" +
					"  \"failed\": [],\n" +
					"  \"warnings\": []\n" +
					"}\n"
			},
			wantRemoved: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, olderPair()...)
			buildStoreFrom(t, home, filepath.Join(dir, recordedID+".sqlite"))
			readDir := readDirWithVariant(oldestID+".SQLITE", oldestID+".sqlite")
			var removed []string

			exitCode, stdout, stderr := runSnapshotsWithReadDir(t, home, readDir, []snapshot.Option{
				snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
				recordingRemove(&removed),
			}, c.args...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, c.wantStdout(dir), stdout)
			assert.Equal(t, c.wantRemoved, removed)
		})
	}
}

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
