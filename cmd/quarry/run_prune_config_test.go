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

	"github.com/koblas/quarry/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_snapshots_prune_with_nothing_beyond_the_default_cap_deletes_nothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_snapshots_keep_below_one(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_malformed_config_with_nothing_deleted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "[snapshots\nkeep = 24\n")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Regexp(t, "^"+regexp.QuoteMeta("quarry: cannot read "+configShown+": line 1: ")+"[^\n]+"+regexp.QuoteMeta(configFix)+"\n$", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_bad_config_value_with_nothing_deleted(t *testing.T) {
	cases := []struct {
		name    string
		content string
		line    string
	}{
		{
			name: "keep of zero", content: "snapshots.keep = 0\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got 0",
		},
		{
			name: "keep as a string", content: "snapshots.keep = \"twelve\"\n",
			line: configShown + ": snapshots.keep must be a whole number of 1 or more, got \"twelve\"",
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
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			writeConfig(t, home, c.content)

			exitCode, stdout, stderr := runPrune(t)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: "+c.line+configFix+"\n", stderr)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_refuses_a_config_it_cannot_read(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+configShown+": is a directory"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_bad_config_even_when_keep_is_given(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keep_0_is_a_usage_error_beside_a_broken_config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t, "--keep", "0")

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; "+
		"Run 'quarry snapshots prune --help' for usage.\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_never_looks_for_the_quicken_path_it_is_configured_with(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "quicken.path = \"~/Books/Missing.quicken\"\n")

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_prints_config_warnings_before_its_own_stderr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshot.keep = 3\n")
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		cannotTellRefusal("the file is not a DuckDB database"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_prints_only_the_config_warning_on_stderr_after_a_successful_run(t *testing.T) {
	pinLocalZone(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshot.keep = 3\n")

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
}

func Test_run_snapshots_prune_prints_config_warnings_before_its_failed_delete_lines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshot.keep = 3\n")

	exitCode, _, stderr := runPruneRemoving(context.Background(), t, refusingRemove(pruneOldest+".sqlite"), "--keep", "3")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n"+
		"quarry: cannot delete snapshot "+pruneOldest+": permission denied\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMorning, pruneNoon, pruneNewest)
}

// LoadConfig is stubbed, so the refusal comes from the snapshots factory's own home lookup.
func Test_run_snapshots_prune_factory_names_itself_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.LoadConfig = func(string) (config.Config, error) { return config.Config{}, nil }

	exitCode := runWith(context.Background(), []string{"snapshots", "prune"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots prune again\n", stderr.String())
}
