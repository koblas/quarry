// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "ID  Taken  Size  Source  Status\n", stdout)
	assert.Equal(t, "quarry: no snapshots in "+snapshotsShown+" yet; run quarry sync to take one\n", stderr)
	assert.NoDirExists(t, filepath.Join(storeDirUnder(home), "snapshots"))
}

func Test_run_snapshots_warns_when_the_store_cannot_be_read(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot tell which snapshot the store was built from: the file is not a DuckDB database\n", stderr)
	assert.Equal(t, ""+
		"ID                Taken                   Size  Source        Status\n"+
		"20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
		"20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
		"Total                                   3.5 MB\n", stdout)
}
