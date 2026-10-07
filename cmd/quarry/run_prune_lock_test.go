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

	"github.com/koblas/quarry/internal/platform/lockfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), append([]string{"snapshots", "prune", "--keep", "3"}, c.flag...), &stdout, &stderr)

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
			home := t.TempDir()
			t.Setenv("HOME", home)

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
