// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/config"
	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const storeShown = "~/Library/Application Support/quarry/quarry.duckdb"

const (
	pruneOldest  = "20260927T143005Z"
	pruneMiddle  = "20260929T090011Z"
	pruneMorning = "20260930T090000Z"
	pruneNoon    = "20260930T141502Z"
	pruneNewest  = "20260930T141502Z_2"
)

// keptBytes is the size of the snapshots a test keeps: none under 0.1 MB, so the sparse file is real.
const keptBytes = 200_000

func pruneFixture(id string, bytes int64, taken time.Time) snapshotFixture {
	return snapshotFixture{id: id, bytes: bytes, taken: taken, source: homeQuicken, verified: true}
}

// fiveSnapshots is five snapshots, oldest first, the two oldest with the distinct sizes the Deleted block prints.
func fiveSnapshots() []snapshotFixture {
	return []snapshotFixture{
		pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
		pruneFixture(pruneMiddle, middleBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
		pruneFixture(pruneMorning, keptBytes, time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)),
		pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
		pruneFixture(pruneNewest, newestBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)),
	}
}

// runPrune runs quarry snapshots prune with args, returning its exit code, stdout and stderr.
func runPrune(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode := run(context.Background(), append([]string{"snapshots", "prune"}, args...), &out, &errOut)
	return exitCode, out.String(), errOut.String()
}

// requireSnapshotsGone asserts the snapshot file and manifest of each id are gone from dir.
func requireSnapshotsGone(t *testing.T, dir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		assert.NoFileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.NoFileExists(t, filepath.Join(dir, id+".json"))
	}
}

// requireSnapshotsKept asserts the snapshot file and manifest of each id are still in dir.
func requireSnapshotsKept(t *testing.T, dir string, ids ...string) {
	t.Helper()
	for _, id := range ids {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
}

func Test_run_snapshots_prune_deletes_all_but_the_newest_n(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keeps_the_stores_snapshot_when_it_is_older_than_the_newest_n(t *testing.T) {
	pinLocalZone(t)
	const storeSnapshot = "20260801T120000Z"
	home := newHome(t)
	dir := writeSnapshots(t, home,
		pruneFixture(storeSnapshot, keptBytes, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)),
		pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
		pruneFixture(pruneMiddle, keptBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
		pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
		pruneFixture(pruneNewest, keptBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)),
	)
	buildStoreFrom(t, home, filepath.Join(dir, storeSnapshot+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 3 and 20260801T120000Z, the store's snapshot:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
	requireSnapshotsKept(t, dir, storeSnapshot, pruneMiddle, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_when_the_store_cannot_be_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot tell which snapshot the store at "+storeShown+" was built from "+
		"(the file is not a DuckDB database), so no snapshot was deleted; "+
		"run quarry sync to rebuild the store, then prune again\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_keep_0_as_a_usage_error(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "0")

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; "+
		"Run 'quarry snapshots prune --help' for usage.\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

// refusingRemove removes as os.Remove does, except that the file named refused fails with EACCES.
func refusingRemove(refused string) func(string) error {
	return func(path string) error {
		if filepath.Base(path) == refused {
			return &fs.PathError{Op: "remove", Path: path, Err: syscall.EACCES}
		}
		return os.Remove(path)
	}
}

// Needs runWith: the one file that cannot be deleted is a Server option, not a folder mode.
func Test_run_snapshots_prune_reports_each_snapshot_it_could_not_delete_and_exits_1(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	five := fiveSnapshots()
	dir := writeSnapshots(t, home, five[0], five[1], five[4])
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
			snapshot.WithRemove(refusingRemove(pruneOldest+".sqlite")),
		), nil
	}

	exitCode := runWith(context.Background(), []string{"snapshots", "prune", "--keep", "1"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (2.2 MB), keeping the newest one:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n", stdout.String())
	assert.Equal(t, "quarry: cannot delete snapshot 20260927T143005Z: permission denied\n", stderr.String())
	requireSnapshotsGone(t, dir, pruneMiddle)
	requireSnapshotsKept(t, dir, pruneOldest, pruneNewest)
}

func Test_run_snapshots_prune_with_nothing_beyond_the_default_cap_deletes_nothing(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_snapshots_keep_below_one(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_malformed_config_with_nothing_deleted(t *testing.T) {
	home := newHome(t)
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
			home := newHome(t)
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
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(filepath.Join(storeDirUnder(home), "config.toml"), 0o700))

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+configShown+": is a directory"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_bad_config_even_when_keep_is_given(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keep_0_is_a_usage_error_beside_a_broken_config(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "quicken.path = \"~/Books/Missing.quicken\"\n")

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_prints_config_warnings_before_its_own_stderr(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	env := testEnv(&stdout, &stderr)
	env.LoadConfig = func(string) (config.Config, error) { return config.Config{}, nil }

	exitCode := runWith(context.Background(), []string{"snapshots", "prune"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots prune again\n", stderr.String())
}

// snapshotsKeepConfig is a config file setting snapshots.keep to keep.
func snapshotsKeepConfig(keep int) string {
	return "[snapshots]\nkeep = " + strconv.Itoa(keep) + "\n"
}

func Test_run_snapshots_prune_uses_snapshots_keep_when_keep_is_not_given(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_keep_flag_overrides_snapshots_keep(t *testing.T) {
	cases := []struct {
		name       string
		configKeep int
		flagKeep   string
		stdout     string
		gone       []string
		kept       []string
	}{
		{
			name: "flag above the config", configKeep: 1, flagKeep: "3",
			stdout: "" +
				"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n" +
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n" +
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n",
			gone: []string{pruneMiddle, pruneOldest},
			kept: []string{pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name: "flag below the config", configKeep: 4, flagKeep: "1",
			stdout: "" +
				"Deleted 4 snapshots (3.9 MB), keeping the newest one:\n" +
				"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n" +
				"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n" +
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n" +
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n",
			gone: []string{pruneNoon, pruneMorning, pruneMiddle, pruneOldest},
			kept: []string{pruneNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			writeConfig(t, home, snapshotsKeepConfig(c.configKeep))

			exitCode, stdout, stderr := runPrune(t, "--keep", c.flagKeep)

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.stdout, stdout)
			requireSnapshotsGone(t, dir, c.gone...)
			requireSnapshotsKept(t, dir, c.kept...)
		})
	}
}

func Test_run_snapshots_prune_says_nothing_to_delete_when_snapshots_keep_equals_the_snapshot_count(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(5))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 5\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_deletes_exactly_the_oldest_when_snapshots_keep_is_one_below_the_count(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(4))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 4:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
	requireSnapshotsKept(t, dir, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_accepts_snapshots_keep_of_one(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(1))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 4 snapshots (3.9 MB), keeping the newest one:\n"+
		"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
		"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMorning, pruneNoon, pruneMiddle, pruneOldest)
	requireSnapshotsKept(t, dir, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_lists_what_it_would_delete_and_deletes_nothing(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_refuses_when_the_store_cannot_be_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--keep", "1", "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the file is not a DuckDB database"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_uses_snapshots_keep(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_refuses_a_bad_config(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshots.keep = 0\n")

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: "+configShown+": snapshots.keep must be a whole number of 1 or more, got 0"+configFix+"\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_within_the_cap(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_with_no_snapshots_folder(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: no snapshots in "+snapshotsShown+"\n", stdout)
}

func Test_run_snapshots_prune_dry_run_says_nothing_to_delete_beside_an_unreadable_store(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t, "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
}

func Test_run_snapshots_prune_dry_run_names_the_stores_snapshot_when_it_lies_beyond_the_newest_n(t *testing.T) {
	pinLocalZone(t)
	const storeSnapshot = "20260801T120000Z"
	home := newHome(t)
	dir := writeSnapshots(t, home,
		pruneFixture(storeSnapshot, keptBytes, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)),
		pruneFixture(pruneOldest, oldestBytes, time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)),
		pruneFixture(pruneMiddle, keptBytes, time.Date(2026, 9, 29, 9, 0, 11, 0, time.UTC)),
		pruneFixture(pruneNoon, keptBytes, time.Date(2026, 9, 30, 14, 15, 2, 0, time.UTC)),
		pruneFixture(pruneNewest, keptBytes, time.Date(2026, 9, 30, 18, 30, 0, 0, time.UTC)),
	)
	buildStoreFrom(t, home, filepath.Join(dir, storeSnapshot+".sqlite"))

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--dry-run")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Would delete 1 snapshot (1.2 MB), keeping the newest 3 and 20260801T120000Z, the store's snapshot:\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsKept(t, dir, storeSnapshot, pruneOldest, pruneMiddle, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_reports_a_folder_it_cannot_read(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	var removed []string
	spy := func(path string) error {
		removed = append(removed, path)
		return nil
	}

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, spy, "--dry-run")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
	assert.Empty(t, removed)
}

func Test_run_snapshots_prune_dry_run_is_interrupted_before_the_store_read(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, stdout, stderr := runCapture(ctx, []string{"snapshots", "prune", "--keep", "1", "--dry-run"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots prune interrupted\n", stderr.String())
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_dry_run_counts_the_cap_exactly(t *testing.T) {
	cases := []struct {
		name string
		keep string
		want string
	}{
		{"as many snapshots as the cap", "5", "Nothing to delete: 5 snapshots, within the newest 5\n"},
		{"one snapshot beyond the cap", "4", "Would delete 1 snapshot (1.2 MB), keeping the newest 4:\n  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))

			exitCode, stdout, stderr := runPrune(t, "--keep", c.keep, "--dry-run")

			require.Equal(t, 0, exitCode, stderr)
			assert.Equal(t, c.want, stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

// cannotTellRefusal is the prune refusal for a store that cannot say which snapshot built it, with reason as its cause.
func cannotTellRefusal(reason string) string {
	return "quarry: cannot tell which snapshot the store at " + storeShown + " was built from (" + reason + "), " +
		"so no snapshot was deleted; run quarry sync to rebuild the store, then prune again\n"
}

// thirteenSnapshots is one snapshot more than the default keep, oldest first, ids 20260901T090000Z to 20260913T090000Z.
func thirteenSnapshots() []snapshotFixture {
	fixtures := make([]snapshotFixture, 13)
	for i := range fixtures {
		fixtures[i] = snapshotFixture{id: fmt.Sprintf("202609%02dT090000Z", i+1), bytes: keptBytes}
	}
	fixtures[0].bytes = oldestBytes
	return fixtures
}

// runPruneRemoving runs quarry snapshots prune with args under ctx, its Server removing files through remove.
func runPruneRemoving(ctx context.Context, t *testing.T, remove func(string) error, args ...string) (int, string, string) {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
		return snapshot.NewServer(
			snapshot.WithSnapshotDir(snapshotsDirUnder(home)),
			snapshot.WithHome(home),
			snapshot.WithStoreProbe(duckstore.New(storeDirUnder(home))),
			snapshot.WithRemove(remove),
		), nil
	}

	exitCode := runWith(ctx, append([]string{"snapshots", "prune"}, args...), env)
	return exitCode, stdout.String(), stderr.String()
}

// cancellingRemove removes as os.Remove does, refuses the file named refused with EACCES,
// and cancels once it has dealt with the file named trigger.
func cancellingRemove(cancel context.CancelFunc, trigger, refused string) func(string) error {
	inner := refusingRemove(refused)
	return func(path string) error {
		err := inner(path)
		if filepath.Base(path) == trigger {
			cancel()
		}
		return err
	}
}

func Test_run_snapshots_prune_without_keep_deletes_beyond_the_newest_12(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, thirteenSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, "20260913T090000Z.sqlite"))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (1.2 MB), keeping the newest 12:\n"+
		"  20260901T090000Z  unknown  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, "20260901T090000Z")
	assert.FileExists(t, filepath.Join(dir, "20260902T090000Z.sqlite"))
}

func Test_run_snapshots_prune_names_itself_when_the_home_directory_cannot_be_resolved(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry snapshots prune again\n", stderr)
}

func Test_run_snapshots_prune_protects_nothing_when_there_is_no_store(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, ""+
		"Deleted 2 snapshots (3.5 MB), keeping the newest 3:\n"+
		"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
		"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
	requireSnapshotsGone(t, dir, pruneMiddle, pruneOldest)
}

func Test_run_snapshots_prune_says_nothing_to_delete_beside_an_unreadable_store(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a DuckDB file"), 0o600))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_says_nothing_to_delete_within_the_newest_one(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()[0])

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 1 snapshot, within the newest one\n", stdout)
	requireSnapshotsKept(t, dir, pruneOldest)
}

func Test_run_snapshots_prune_names_the_stores_snapshot_when_it_is_the_only_one_beyond_the_newest_n(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, thirteenSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, "20260901T090000Z.sqlite"))

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 13 snapshots, within the newest 12 and 20260901T090000Z, the store's snapshot\n", stdout)
	assert.FileExists(t, filepath.Join(dir, "20260901T090000Z.sqlite"))
}

func Test_run_snapshots_prune_says_nothing_to_delete_with_no_snapshots_folder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	exitCode, stdout, stderr := runPrune(t)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: no snapshots in "+snapshotsShown+"\n", stdout)
}

func Test_run_snapshots_prune_says_nothing_to_delete_when_the_only_candidate_is_already_gone(t *testing.T) {
	home := newHome(t)
	five := fiveSnapshots()
	dir := writeSnapshots(t, home, five[0], five[4])
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	vanishing := func(path string) error {
		err := os.Remove(path)
		if filepath.Base(path) == pruneOldest+".sqlite" {
			return &fs.PathError{Op: "remove", Path: path, Err: syscall.ENOENT}
		}
		return err
	}

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, vanishing, "--keep", "1")

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	assert.Equal(t, "Nothing to delete: 1 snapshot, within the newest one\n", stdout)
	requireSnapshotsGone(t, dir, pruneOldest)
}

func Test_run_snapshots_prune_prints_one_line_per_failure_and_nothing_on_stdout_when_none_succeeded(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runPrune(t, "--keep", "3")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMiddle+": permission denied\n"+
		"quarry: cannot delete snapshot "+pruneOldest+": permission denied\n", stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_says_it_was_interrupted_before_any_delete(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exitCode, stdout, stderr := runCapture(ctx, []string{"snapshots", "prune", "--keep", "1"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: snapshots prune interrupted\n", stderr.String())
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_prints_what_it_deleted_before_the_interrupt_line(t *testing.T) {
	cases := []struct {
		name       string
		keep       string
		wantStdout string
		wantStderr string
		wantGone   []string
		wantKept   []string
	}{
		{
			name: "one snapshot never attempted",
			keep: "3",
			wantStdout: "Deleted 1 snapshot (2.2 MB), keeping the newest 3:\n" +
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n",
			wantStderr: "quarry: snapshots prune interrupted; 1 snapshot was not deleted\n",
			wantGone:   []string{pruneMiddle},
			wantKept:   []string{pruneOldest, pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name: "several snapshots never attempted",
			keep: "2",
			wantStdout: "Deleted 1 snapshot (0.2 MB), keeping the newest 2:\n" +
				"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n",
			wantStderr: "quarry: snapshots prune interrupted; 2 snapshots were not deleted\n",
			wantGone:   []string{pruneMorning},
			wantKept:   []string{pruneOldest, pruneMiddle, pruneNoon, pruneNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			trigger := c.wantGone[0] + ".sqlite"

			exitCode, stdout, stderr := runPruneRemoving(ctx, t, cancellingRemove(cancel, trigger, ""), "--keep", c.keep)

			assert.Equal(t, 1, exitCode)
			assert.Equal(t, c.wantStderr, stderr)
			assert.Equal(t, c.wantStdout, stdout)
			requireSnapshotsGone(t, dir, c.wantGone...)
			requireSnapshotsKept(t, dir, c.wantKept...)
		})
	}
}

func Test_run_snapshots_prune_prints_the_failure_line_before_the_interrupt_line(t *testing.T) {
	pinLocalZone(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remove := cancellingRemove(cancel, pruneMorning+".sqlite", pruneMorning+".sqlite")

	exitCode, stdout, stderr := runPruneRemoving(ctx, t, remove, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"Deleted 1 snapshot (0.2 MB), keeping the newest one:\n"+
		"  "+pruneNoon+"  2026-09-30 10:15 EDT  0.2 MB\n", stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMorning+": permission denied\n"+
		"quarry: snapshots prune interrupted; 2 snapshots were not deleted\n", stderr)
}

func Test_run_snapshots_prune_refuses_a_store_with_no_import_history(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	editStore(t, home, "DELETE FROM import_runs")

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the store has no import history"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_store_of_another_format_naming_no_snapshot(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	writeStoreFixture(t, home, "CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);")

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("the store was built by another version of quarry"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

func Test_run_snapshots_prune_refuses_a_snapshots_folder_it_cannot_read(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	exitCode, stdout, stderr := runPrune(t)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: cannot read "+snapshotsShown+": permission denied\n", stderr)
}

func Test_run_snapshots_prune_reports_a_failed_stdout_write(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, thirteenSnapshots()...)
	var stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"snapshots", "prune"}, failingWriter{err: errNoSpace}, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "quarry: cannot write the result to stdout: no space left on device\n", stderr.String())
}

// recordedClass is one state of the snapshot path the store recorded: reason is why it cannot be
// read, "" when it can, and marked says whether the snapshot of the recorded ID is then the store's.
type recordedClass struct {
	name   string
	reason string
	marked bool
	// recorded is the path the store records for the snapshot with id.
	recorded func(home, id string) string
	// prepare puts the path's parent into the class's state.
	prepare func(t *testing.T, home string)
}

// backupPath is where every class but the first records a snapshot: <home>/Backup/<id>.sqlite.
func backupPath(home, id string) string { return filepath.Join(home, "Backup", id+".sqlite") }

// recordedClasses are the recorded-path states the cells cross: stats, gone, and three ways to fail to stat.
func recordedClasses() []recordedClass {
	return []recordedClass{
		{
			name: "stats", marked: true,
			recorded: func(home, id string) string { return filepath.Join(snapshotsDir(home), id+".sqlite") },
			prepare:  func(*testing.T, string) {},
		},
		{name: "is gone", marked: true, recorded: backupPath, prepare: func(*testing.T, string) {}},
		{
			name: "permission denied", reason: "permission denied", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				skipAsRoot(t)
				backup := filepath.Join(home, "Backup")
				require.NoError(t, os.Mkdir(backup, 0o700))
				require.NoError(t, os.Chmod(backup, 0o000))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
			},
		},
		{
			name: "a symlink loop", reason: "too many levels of symbolic links", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				backup := filepath.Join(home, "Backup")
				require.NoError(t, os.Symlink(backup, backup))
			},
		},
		{
			name: "a parent that is a file", reason: "not a directory", recorded: backupPath,
			prepare: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(home, "Backup"), nil, 0o600))
			},
		},
	}
}

// unreadableRecordedClasses are the classes whose recorded path cannot be read.
func unreadableRecordedClasses() []recordedClass {
	return slices.DeleteFunc(recordedClasses(), func(c recordedClass) bool { return c.reason == "" })
}

// readableRecordedClasses are the classes whose recorded path stats or is gone.
func readableRecordedClasses() []recordedClass {
	return slices.DeleteFunc(recordedClasses(), func(c recordedClass) bool { return c.reason != "" })
}

// buildStoreFromClass builds the store under home from the snapshot with id recorded as c has it, and returns that path.
func buildStoreFromClass(t *testing.T, home, id string, c recordedClass) string {
	t.Helper()
	c.prepare(t, home)
	recorded := c.recorded(home, id)
	buildStoreFrom(t, home, recorded)
	return recorded
}

// cannotReadPhrase is why c's recorded path cannot be read, naming it as shown: "" for a readable class.
func (c recordedClass) cannotReadPhrase(shown string) string {
	if c.reason == "" {
		return ""
	}
	return "cannot read " + shown + ": " + c.reason
}

// pruneRecordedCell runs quarry snapshots prune with args beside five snapshots and a store built from
// pruneMiddle recorded as c has it; it returns the snapshots folder, the recorded path and the run's result.
func pruneRecordedCell(t *testing.T, c recordedClass, args ...string) (string, string, int, string, string) {
	t.Helper()
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	recorded := buildStoreFromClass(t, home, pruneMiddle, c)
	exitCode, stdout, stderr := runPrune(t, args...)
	return dir, recorded, exitCode, stdout, stderr
}

// buildStoreFromUnreadable builds the store under home from <home>/Backup/<id>.sqlite, then makes
// Backup unsearchable, so the recorded path stats with EACCES.
func buildStoreFromUnreadable(t *testing.T, home, id string) {
	t.Helper()
	backup := filepath.Join(home, "Backup")
	require.NoError(t, os.Mkdir(backup, 0o700))
	buildStoreFrom(t, home, filepath.Join(backup, id+".sqlite"))
	require.NoError(t, os.Chmod(backup, 0o000))
	t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
}

func Test_run_snapshots_prune_refuses_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	skipAsRoot(t)
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFromUnreadable(t, home, pruneMiddle)

	exitCode, stdout, stderr := runPrune(t, "--keep", "1")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, cannotTellRefusal("cannot read ~/Backup/"+pruneMiddle+".sqlite: permission denied"), stderr)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
}

// pruneMode is a way to run prune, named for its table row.
type pruneMode struct {
	name string
	args []string
}

func Test_run_snapshots_prune_recorded_snapshot_cells(t *testing.T) {
	modes := []pruneMode{
		{"text", []string{"--keep", "1"}},
		{"dry run", []string{"--keep", "1", "--dry-run"}},
		{"json", []string{"--keep", "1", "--json"}},
		{"dry run json", []string{"--keep", "1", "--dry-run", "--json"}},
	}
	for _, c := range unreadableRecordedClasses() {
		for _, mode := range modes {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, mode.args...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, cannotTellRefusal(c.cannotReadPhrase("~/Backup/"+pruneMiddle+".sqlite")), stderr)
				requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
			})
		}
	}
}

func Test_run_snapshots_prune_deletes_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	modes := []pruneMode{
		{"text", []string{"--keep", "1"}},
		{"json", []string{"--keep", "1", "--json"}},
	}
	for _, c := range readableRecordedClasses() {
		for _, mode := range modes {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				dir, _, exitCode, _, stderr := pruneRecordedCell(t, c, mode.args...)

				require.Equal(t, 0, exitCode, stderr)
				assert.Empty(t, stderr)
				requireSnapshotsGone(t, dir, pruneOldest, pruneMorning, pruneNoon)
				requireSnapshotsKept(t, dir, pruneMiddle, pruneNewest)
			})
		}
	}
}

func Test_run_snapshots_prune_dry_run_lists_what_lies_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	for _, c := range readableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--keep", "1", "--dry-run")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, ""+
				"Would delete 3 snapshots (1.6 MB), keeping the newest one and 20260929T090011Z, the store's snapshot:\n"+
				"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
				"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_dry_run_json_lists_what_lies_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	for _, c := range readableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--keep", "1", "--dry-run", "--json")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, []string{pruneNoon, pruneMorning, pruneOldest}, jsonIDs(t, stdout, "would_delete"))
			assert.Empty(t, jsonIDs(t, stdout, "deleted"))
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_says_nothing_to_delete_within_the_newest_n_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	for _, c := range unreadableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_json_names_the_recorded_snapshot_without_a_warning_within_the_newest_n_when_it_cannot_be_read(t *testing.T) {
	for _, c := range unreadableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			_, recorded, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--json")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			doc := pruneDocument(t, stdout)
			assert.JSONEq(t, `{"id":"`+pruneMiddle+`","path":"`+recorded+`"}`, string(doc["store_snapshot"]))
			assert.JSONEq(t, `[]`, string(doc["warnings"]))
			assert.JSONEq(t, `[]`, string(doc["deleted"]))
		})
	}
}

// lockHeldPruneLine is the refusal a prune prints while another writer holds the lock.
const lockHeldPruneLine = "quarry: another quarry sync or quarry snapshots prune is running, so this prune deleted nothing; " +
	"run the command again once that one finishes\n"

// outputCells are the two stdout formats every edge row is asserted in.
var outputCells = []struct {
	name string
	flag []string
}{
	{name: "text", flag: nil},
	{name: "json", flag: []string{"--json"}},
}

// prunableStore is a quarry folder under the test's HOME with five snapshots, a store built from
// the newest, and one orphan manifest: what `prune --keep 3` deletes when nothing stops it.
type prunableStore struct {
	home   string
	dir    string
	orphan string
}

// newPrunableStore points HOME at a fresh prunableStore.
func newPrunableStore(t *testing.T) prunableStore {
	t.Helper()
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	orphan := filepath.Join(dir, "19990101T000000Z.json")
	require.NoError(t, os.WriteFile(orphan, []byte("{}"), 0o600))
	return prunableStore{home: home, dir: dir, orphan: orphan}
}

// holdLock takes the writer lock as a running sync would, and returns the func that releases it.
func (p prunableStore) holdLock(t *testing.T) func() {
	t.Helper()
	release, err := lockfile.New(lockPathUnder(storeDirUnder(p.home)), lockfile.ModeSync).Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(release)
	return release
}

// runQuarry runs quarry with args, returning its exit code, stdout and stderr.
func runQuarry(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exitCode := run(context.Background(), args, &out, &errOut)
	return exitCode, out.String(), errOut.String()
}

// jsonIDs decodes the ids of stdout's array under key.
func jsonIDs(t *testing.T, stdout, key string) []string {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	var entries []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(doc[key], &entries))
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	return ids
}

func Test_run_snapshots_prune_refuses_while_another_writer_holds_the_lock(t *testing.T) {
	for _, c := range outputCells {
		t.Run(c.name, func(t *testing.T) {
			p := newPrunableStore(t)
			p.holdLock(t)

			exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"snapshots", "prune", "--keep", "3"}, c.flag...))

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, lockHeldPruneLine, stderr.String())
			requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
			assert.FileExists(t, p.orphan)
		})
	}
}

func Test_run_snapshots_prune_proceeds_past_a_lock_left_by_an_earlier_run(t *testing.T) {
	p := newPrunableStore(t)
	release := p.holdLock(t)
	release()

	exitCode, _, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	requireSnapshotsGone(t, p.dir, pruneOldest, pruneMiddle)
	requireSnapshotsKept(t, p.dir, pruneMorning, pruneNoon, pruneNewest)
	assert.NoFileExists(t, p.orphan)
}

func Test_run_snapshots_prune_dry_run_runs_while_a_writer_holds_the_lock(t *testing.T) {
	cells := []struct {
		name   string
		flag   []string
		wantOK func(t *testing.T, stdout string)
	}{
		{name: "text", wantOK: func(t *testing.T, stdout string) {
			t.Helper()
			assert.Equal(t, ""+
				"Would delete 2 snapshots (3.5 MB), keeping the newest 3:\n"+
				"  20260929T090011Z  2026-09-29 05:00 EDT  2.2 MB\n"+
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
		}},
		{name: "json", flag: []string{"--json"}, wantOK: func(t *testing.T, stdout string) {
			t.Helper()
			assert.Equal(t, "true", string(pruneDocument(t, stdout)["dry_run"]))
			assert.Equal(t, []string{pruneMiddle, pruneOldest}, jsonIDs(t, stdout, "would_delete"))
			assert.Empty(t, jsonIDs(t, stdout, "deleted"))
		}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			p := newPrunableStore(t)
			p.holdLock(t)

			exitCode, stdout, stderr := runPrune(t, append([]string{"--keep", "3", "--dry-run"}, c.flag...)...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			c.wantOK(t, stdout)
			requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_refuses_usage_and_config_before_the_lock(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		config     string
		wantExit   int
		wantStderr string
	}{
		{
			name:     "keep 0 is a usage error",
			args:     []string{"--keep", "0"},
			wantExit: 2,
			wantStderr: "quarry: --keep must be 1 or more; the snapshot the store was built from is always kept; " +
				"Run 'quarry snapshots prune --help' for usage.\n",
		},
		{
			name:       "a malformed config",
			config:     "quicken.path = 12\n",
			wantExit:   1,
			wantStderr: "quarry: " + configShown + ": quicken.path must be a path in quotes, got 12" + configFix + "\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := newPrunableStore(t)
			writeConfig(t, p.home, c.config)
			p.holdLock(t)

			exitCode, stdout, stderr := runPrune(t, c.args...)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, c.wantStderr, stderr)
			requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_refuses_with_nothing_beyond_the_cap_while_locked(t *testing.T) {
	for _, c := range outputCells {
		t.Run(c.name, func(t *testing.T) {
			p := newPrunableStore(t)
			p.holdLock(t)

			exitCode, stdout, stderr := runPrune(t, c.flag...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, lockHeldPruneLine, stderr)
			requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_lists_while_a_writer_holds_the_lock(t *testing.T) {
	cells := []struct {
		name   string
		flag   []string
		wantOK func(t *testing.T, stdout string)
	}{
		{name: "text", wantOK: func(t *testing.T, stdout string) {
			t.Helper()
			assert.Equal(t, ""+
				"ID                  Taken                   Size  Source        Status\n"+
				"20260930T141502Z_2  2026-09-30 14:30 EDT  3.2 MB  Home.quicken  store\n"+
				"20260930T141502Z    2026-09-30 10:15 EDT  0.2 MB  Home.quicken\n"+
				"20260930T090000Z    2026-09-30 05:00 EDT  0.2 MB  Home.quicken\n"+
				"20260929T090011Z    2026-09-29 05:00 EDT  2.2 MB  Home.quicken\n"+
				"20260927T143005Z    2026-09-27 10:30 EDT  1.2 MB  Home.quicken\n"+
				"Total                                     7.1 MB\n", stdout)
		}},
		{name: "json", flag: []string{"--json"}, wantOK: func(t *testing.T, stdout string) {
			t.Helper()
			assert.ElementsMatch(t, []string{pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest}, jsonIDs(t, stdout, "snapshots"))
		}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			p := newPrunableStore(t)
			p.holdLock(t)

			exitCode, stdout, stderr := runQuarry(t, append([]string{"snapshots"}, c.flag...)...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			c.wantOK(t, stdout)
		})
	}
}

func Test_run_snapshots_prune_says_nothing_to_delete_with_no_quarry_folder_and_creates_nothing(t *testing.T) {
	cells := []struct {
		name       string
		flag       []string
		wantStdout string
	}{
		{name: "text", wantStdout: "Nothing to delete: no snapshots in " + snapshotsShown + "\n"},
		{name: "json", flag: []string{"--json"}, wantStdout: "" +
			"{\n" +
			"  \"dry_run\": false,\n" +
			"  \"keep\": 12,\n" +
			"  \"store_snapshot\": null,\n" +
			"  \"deleted\": [],\n" +
			"  \"would_delete\": [],\n" +
			"  \"failed\": [],\n" +
			"  \"warnings\": []\n" +
			"}\n"},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)

			exitCode, stdout, stderr := runPrune(t, c.flag...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, c.wantStdout, stdout)
			entries, err := os.ReadDir(home)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// requireNothingDeleted fails t unless all five snapshots and the orphan manifest are still there.
func (p prunableStore) requireNothingDeleted(t *testing.T) {
	t.Helper()
	requireSnapshotsKept(t, p.dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
	assert.FileExists(t, p.orphan)
}

func Test_run_snapshots_prune_refuses_an_unusable_lock_file_with_a_fix(t *testing.T) {
	for _, row := range unusableLockRows() {
		for _, cell := range outputCells {
			t.Run(row.name+" "+cell.name, func(t *testing.T) {
				p := newPrunableStore(t)
				row.arrange(t, p.home, storeDirUnder(p.home))

				exitCode, stdout, stderr := runPrune(t, append([]string{"--keep", "3"}, cell.flag...)...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, row.wantLine, stderr)
				p.requireNothingDeleted(t)
			})
		}
	}
}

func Test_run_snapshots_prune_lock_refuses_a_quarry_folder_that_is_a_file_naming_the_folder(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			home := newHome(t)
			quarryDir := storeDirUnder(home)
			require.NoError(t, os.MkdirAll(filepath.Dir(quarryDir), 0o700))
			require.NoError(t, os.WriteFile(quarryDir, []byte("not a folder"), 0o600))

			exitCode, stdout, stderr := runWithoutConfigFile(t, home, append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: "+quarryDirS+" is not a folder; rename or remove it, then run the command again\n", stderr)
			content, err := os.ReadFile(quarryDir)
			require.NoError(t, err)
			assert.Equal(t, "not a folder", string(content))
		})
	}
}

func Test_run_snapshots_prune_lock_refuses_a_quarry_folder_it_cannot_search_naming_the_folder_and_its_reason(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			skipAsRoot(t)
			p := newPrunableStore(t)
			quarryDir := storeDirUnder(p.home)
			require.NoError(t, os.Chmod(quarryDir, 0o600))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(quarryDir, 0o700)) })

			exitCode, stdout, stderr := runWithoutConfigFile(t, p.home, append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...)...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: cannot open "+quarryDirS+": permission denied; "+
				"make it readable and writable by your user, then run the command again\n", stderr)
			require.NoError(t, os.Chmod(quarryDir, 0o700))
			p.requireNothingDeleted(t)
		})
	}
}

// Needs runWith: the flock fault is a Locker option, not a state the filesystem can be put in.
func Test_run_snapshots_prune_refuses_when_the_disk_cannot_take_a_lock_with_a_fix(t *testing.T) {
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			p := newPrunableStore(t)
			var stdout, stderr bytes.Buffer
			env := testEnv(&stdout, &stderr)
			env.NewSnapshots = func(context.Context, string) (*snapshot.Server, error) {
				storeDir := storeDirUnder(p.home)
				notSupported := lockfile.WithFlock(func(int, int) error { return syscall.ENOTSUP })
				return snapshot.NewServer(
					snapshot.WithSnapshotDir(snapshotsDirUnder(p.home)),
					snapshot.WithHome(p.home),
					snapshot.WithStoreProbe(duckstore.New(storeDir)),
					snapshot.WithLocker(lockfile.New(lockPathUnder(storeDir), lockfile.ModePrune, notSupported)),
				), nil
			}

			exitCode := runWith(context.Background(), append([]string{"snapshots", "prune", "--keep", "3"}, cell.flag...), env)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: cannot lock "+lockShown+": operation not supported, so this prune deleted nothing; "+
				quarryDirS+" must be on a disk that supports file locks\n", stderr.String())
			p.requireNothingDeleted(t)
		})
	}
}

// The prune runs on its own goroutine: a lock file that blocks the open must fail the test, not hang it.
func Test_run_snapshots_prune_refuses_a_fifo_lock_file_without_hanging(t *testing.T) {
	type result struct {
		exitCode       int
		stdout, stderr string
	}
	for _, cell := range outputCells {
		t.Run(cell.name, func(t *testing.T) {
			p := newPrunableStore(t)
			require.NoError(t, syscall.Mkfifo(lockPathUnder(storeDirUnder(p.home)), 0o600))
			done := make(chan result, 1)

			go func() {
				exitCode, stdout, stderr := runPrune(t, append([]string{"--keep", "3"}, cell.flag...)...)
				done <- result{exitCode, stdout, stderr}
			}()

			select {
			case got := <-done:
				assert.Equal(t, 1, got.exitCode)
				assert.Empty(t, got.stdout)
				assert.Equal(t, "quarry: "+lockShown+" is not a regular file; remove it, then run the command again\n", got.stderr)
				p.requireNothingDeleted(t)
			case <-time.After(10 * time.Second):
				t.Fatal("prune did not return: its lock file is a fifo")
			}
		})
	}
}

func Test_run_snapshots_prune_deletes_with_a_read_only_lock_file(t *testing.T) {
	p := newPrunableStore(t)
	require.NoError(t, os.WriteFile(lockPathUnder(storeDirUnder(p.home)), nil, 0o400))

	exitCode, _, stderr := runPrune(t, "--keep", "3")

	require.Equal(t, 0, exitCode, stderr)
	requireSnapshotsGone(t, p.dir, pruneOldest, pruneMiddle)
	requireSnapshotsKept(t, p.dir, pruneMorning, pruneNoon, pruneNewest)
	assert.NoFileExists(t, p.orphan)
}

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
	return pruneStoreSnapshotAtJSON(id, dir+"/"+id+".sqlite")
}

// pruneStoreSnapshotAtJSON is the store_snapshot object with id and the recorded path.
func pruneStoreSnapshotAtJSON(id, path string) string {
	return "" +
		"  \"store_snapshot\": {\n" +
		"    \"id\": \"" + id + "\",\n" +
		"    \"path\": \"" + path + "\"\n" +
		"  },\n"
}

// pruneFailureJSON is one failed entry as the indented document prints it.
func pruneFailureJSON(dir, id, reason string) string {
	return "" +
		"    {\n" +
		"      \"id\": \"" + id + "\",\n" +
		"      \"path\": \"" + dir + "/" + id + ".sqlite\",\n" +
		"      \"reason\": \"" + reason + "\"\n" +
		"    }"
}

// refusingRemoveEach removes as os.Remove does, except that each file named in refused fails with EACCES.
func refusingRemoveEach(refused ...string) func(string) error {
	return func(path string) error {
		if slices.Contains(refused, filepath.Base(path)) {
			return refusingRemove(filepath.Base(path))(path)
		}
		return os.Remove(path)
	}
}

// pruneDocument decodes stdout's top-level keys as raw text, so a null and a missing key differ.
func pruneDocument(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
	return doc
}

func Test_run_snapshots_prune_json_prints_the_ruled_document_for_a_real_run(t *testing.T) {
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
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
	home := newHome(t)
	writeSnapshots(t, home, fiveSnapshots()...)

	exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "null", string(pruneDocument(t, stdout)["store_snapshot"]))
}

func Test_run_snapshots_prune_json_prints_the_stores_snapshot_and_does_not_delete_it_when_it_lies_beyond_the_newest_n(t *testing.T) {
	const storeSnapshot = "20260801T120000Z"
	home := newHome(t)
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

// Needs runPruneRemoving: the files that cannot be deleted are a Server option, not a folder mode.
func Test_run_snapshots_prune_json_lists_the_failed_deletes_newest_first_beside_the_deleted_ones_and_exits_1(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	remove := refusingRemoveEach(pruneOldest+".sqlite", pruneMorning+".sqlite")

	exitCode, stdout, stderr := runPruneRemoving(context.Background(), t, remove, "--keep", "1", "--json")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 1,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneNoon, keptBytes)+",\n"+
		pruneEntryJSON(dir, pruneMiddle, middleBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [\n"+
		pruneFailureJSON(dir, pruneMorning, "permission denied")+",\n"+
		pruneFailureJSON(dir, pruneOldest, "permission denied")+"\n"+
		"  ],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMorning+": permission denied\n"+
		"quarry: cannot delete snapshot "+pruneOldest+": permission denied\n", stderr)
	requireSnapshotsGone(t, dir, pruneNoon, pruneMiddle)
	requireSnapshotsKept(t, dir, pruneOldest, pruneMorning, pruneNewest)
}

func Test_run_snapshots_prune_json_prints_what_was_deleted_when_the_run_is_interrupted_and_exits_1(t *testing.T) {
	home := newHome(t)
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

func Test_run_snapshots_prune_json_prints_the_failure_lines_then_the_interrupt_line_and_the_document_when_a_delete_fails_before_the_interrupt(t *testing.T) {
	home := newHome(t)
	dir := writeSnapshots(t, home, fiveSnapshots()...)
	buildStoreFrom(t, home, filepath.Join(dir, pruneNewest+".sqlite"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remove := cancellingRemove(cancel, pruneMorning+".sqlite", pruneMorning+".sqlite")

	exitCode, stdout, stderr := runPruneRemoving(ctx, t, remove, "--keep", "1", "--json")

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, ""+
		"{\n"+
		"  \"dry_run\": false,\n"+
		"  \"keep\": 1,\n"+
		pruneStoreSnapshotJSON(dir, pruneNewest)+
		"  \"deleted\": [\n"+
		pruneEntryJSON(dir, pruneNoon, keptBytes)+"\n"+
		"  ],\n"+
		"  \"would_delete\": [],\n"+
		"  \"failed\": [\n"+
		pruneFailureJSON(dir, pruneMorning, "permission denied")+"\n"+
		"  ],\n"+
		"  \"warnings\": []\n"+
		"}\n", stdout)
	assert.Equal(t, ""+
		"quarry: cannot delete snapshot "+pruneMorning+": permission denied\n"+
		"quarry: snapshots prune interrupted; 2 snapshots were not deleted\n", stderr)
}

func Test_run_snapshots_prune_json_prints_no_document_when_interrupted_before_any_delete(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run("dry-run="+strconv.FormatBool(dryRun), func(t *testing.T) {
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			args := []string{"--keep", "1", "--json"}
			if dryRun {
				args = append(args, "--dry-run")
			}

			exitCode, stdout, stderr := runPruneRemoving(ctx, t, os.Remove, args...)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: snapshots prune interrupted\n", stderr)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_json_still_prints_the_document_with_empty_lists_and_a_null_store_snapshot_for_zero_snapshots(t *testing.T) {
	cases := []struct {
		name       string
		makeFolder bool
		dryRun     bool
	}{
		{name: "no snapshots folder"},
		{name: "no snapshots folder under --dry-run", dryRun: true},
		{name: "empty snapshots folder", makeFolder: true},
		{name: "empty snapshots folder under --dry-run", makeFolder: true, dryRun: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			if c.makeFolder {
				writeSnapshots(t, home)
			}
			args := []string{"--json"}
			if c.dryRun {
				args = append(args, "--dry-run")
			}

			exitCode, stdout, stderr := runPrune(t, args...)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, ""+
				"{\n"+
				"  \"dry_run\": "+strconv.FormatBool(c.dryRun)+",\n"+
				"  \"keep\": 12,\n"+
				"  \"store_snapshot\": null,\n"+
				"  \"deleted\": [],\n"+
				"  \"would_delete\": [],\n"+
				"  \"failed\": [],\n"+
				"  \"warnings\": []\n"+
				"}\n", stdout)
		})
	}
}

func Test_run_snapshots_prune_json_gives_the_store_snapshot_path_as_the_store_recorded_it(t *testing.T) {
	deletedMiddle := func(dir string) string {
		return "  \"deleted\": [\n" + pruneEntryJSON(dir, pruneMiddle, middleBytes) + "\n  ],\n"
	}
	deletedMiddleAndOldest := func(dir string) string {
		return "  \"deleted\": [\n" + pruneEntryJSON(dir, pruneMiddle, middleBytes) + ",\n" + pruneEntryJSON(dir, pruneOldest, oldestBytes) + "\n  ],\n"
	}
	cases := []struct {
		name        string
		recorded    func(t *testing.T, dir string) string
		storeID     string
		wantDeleted func(dir string) string
		wantKept    []string
	}{
		{
			name: "a path outside the folder naming a snapshot in it protects that snapshot",
			recorded: func(t *testing.T, _ string) string {
				t.Helper()
				return filepath.Join(t.TempDir(), pruneOldest+".sqlite")
			},
			storeID:     pruneOldest,
			wantDeleted: deletedMiddle,
			wantKept:    []string{pruneOldest, pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name: "a path outside the folder naming no snapshot in it protects nothing",
			recorded: func(t *testing.T, _ string) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "20260101T000000Z.sqlite")
			},
			storeID:     "20260101T000000Z",
			wantDeleted: deletedMiddleAndOldest,
			wantKept:    []string{pruneMorning, pruneNoon, pruneNewest},
		},
		{
			name: "a path through a symlink to the folder is not resolved",
			recorded: func(t *testing.T, dir string) string {
				t.Helper()
				alias := filepath.Join(t.TempDir(), "alias")
				require.NoError(t, os.Symlink(dir, alias))
				return filepath.Join(alias, pruneOldest+".sqlite")
			},
			storeID:     pruneOldest,
			wantDeleted: deletedMiddle,
			wantKept:    []string{pruneOldest, pruneMorning, pruneNoon, pruneNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			recorded := c.recorded(t, dir)
			buildStoreFrom(t, home, recorded)

			exitCode, stdout, stderr := runPrune(t, "--keep", "3", "--json")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, ""+
				"{\n"+
				"  \"dry_run\": false,\n"+
				"  \"keep\": 3,\n"+
				pruneStoreSnapshotAtJSON(c.storeID, recorded)+
				c.wantDeleted(dir)+
				"  \"would_delete\": [],\n"+
				"  \"failed\": [],\n"+
				"  \"warnings\": []\n"+
				"}\n", stdout)
			requireSnapshotsKept(t, dir, c.wantKept...)
		})
	}
}

func Test_run_snapshots_prune_json_prints_no_document_when_the_run_refuses(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, home string)
		args       []string
		wantExit   int
		wantStderr string
		locked     bool
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
		{
			name:       "snapshots folder cannot be read",
			setup:      func(*testing.T, string) {},
			args:       []string{"--json"},
			wantExit:   1,
			wantStderr: "quarry: cannot read " + snapshotsShown + ": permission denied\n",
			locked:     true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			dir := writeSnapshots(t, home, fiveSnapshots()...)
			c.setup(t, home)
			if c.locked {
				skipAsRoot(t)
				require.NoError(t, os.Chmod(dir, 0o000))
				t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			}

			exitCode, stdout, stderr := runPrune(t, c.args...)

			if c.locked {
				require.NoError(t, os.Chmod(dir, 0o700))
			}

			assert.Equal(t, c.wantExit, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, c.wantStderr, stderr)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_json_carries_a_config_warning_in_warnings_and_prefixed_on_stderr(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, "snapshot.keep = 3\n")
	warning := configShown + ": unknown key snapshot.keep; quarry ignores it"
	absoluteWarning := configPath(home) + ": unknown key snapshot.keep; quarry ignores it"

	exitCode, stdout, stderr := runPrune(t, "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.JSONEq(t, `["`+absoluteWarning+`"]`, string(pruneDocument(t, stdout)["warnings"]))
	assert.Equal(t, "quarry: warning: "+warning+"\n", stderr)
}

func Test_run_snapshots_prune_json_reports_the_keep_from_the_config_when_keep_is_not_given(t *testing.T) {
	home := newHome(t)
	writeSnapshots(t, home, fiveSnapshots()...)
	writeConfig(t, home, snapshotsKeepConfig(3))

	exitCode, stdout, stderr := runPrune(t, "--dry-run", "--json")

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "3", string(pruneDocument(t, stdout)["keep"]))
}
