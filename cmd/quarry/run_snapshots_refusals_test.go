// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const noSnapshotsLine = "quarry: no snapshots in " + snapshotsShown + " yet; run quarry sync to take one\n"

const cannotTellPrefix = "quarry: warning: cannot tell which snapshot the store was built from: "

func Test_run_snapshots_refuses_a_malformed_config_with_nothing_on_stdout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read "+configShown+": line 1: ")+"[^\n]+"+regexp.QuoteMeta(configFix)+"\n$", stderr)
}

func Test_run_snapshots_refuses_a_bad_config_value_with_nothing_on_stdout(t *testing.T) {
	cases := []struct {
		name    string
		content string
		line    string
	}{
		{
			name: "keep below one", content: "snapshots.keep = 0\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got 0",
		},
		{
			name: "quicken.path not a string", content: "quicken.path = 12\n",
			line: configShown + ": quicken.path must be a path in quotes, got 12",
		},
		{
			name: "quicken.path relative", content: "quicken.path = \"Home.quicken\"\n",
			line: configShown + ": quicken.path must be a full path or start with ~/, got \"Home.quicken\"",
		},
		{
			name: "reporting.currency another currency", content: "reporting.currency = \"EUR\"\n",
			line: configShown + ": reporting.currency must be CAD, USD or native, got \"EUR\"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+configShown+": is a directory"+configFix+"\n", stderr)
}

func Test_run_snapshots_never_looks_for_the_quicken_path_it_is_configured_with(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
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

func Test_run_snapshots_names_itself_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots again\n", stderr)
}

func Test_run_snapshots_reports_a_failed_stdout_write_and_prints_no_note(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"snapshots"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

func Test_run_snapshots_refuses_a_snapshots_folder_it_cannot_read(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, olderPair()...)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
}

func Test_run_snapshots_refuses_a_snapshot_it_cannot_stat(t *testing.T) {
	skipAsRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, olderPair()...)
	require.NoError(t, os.Chmod(dir, 0o400))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runSnapshots(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
}

func Test_run_snapshots_says_it_was_interrupted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeSnapshots(t, home, olderPair()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer

	exitCode := run(ctx, []string{"snapshots"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots interrupted\n", stderr.String())
}

func Test_run_snapshots_prints_the_no_snapshots_note_before_the_store_warning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runSnapshots(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "ID  Taken  Size  Source  Status\n", stdout)
	assert.Equal(t, noSnapshotsLine+cannotTellPrefix+"the file is not a DuckDB database\n", stderr)
}

func Test_run_snapshots_warns_when_the_store_has_no_import_history(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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

// LoadConfig is stubbed, so the refusal comes from the snapshots factory's own home lookup.
func Test_run_snapshots_factory_names_itself_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.LoadConfig = func(string) (config.Config, error) { return config.Config{}, nil }

	exitCode := runWith(context.Background(), []string{"snapshots"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots again\n", stderr.String())
}
