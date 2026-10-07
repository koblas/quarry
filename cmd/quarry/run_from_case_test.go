// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The snapshot is renamed after the sync and its on-disk name asserted: a case-insensitive volume
// resolves the lower-case name too, so only the recorded path tells the two apart.
func Test_run_sync_from_an_id_rebuilds_the_store_from_an_upper_case_sqlite_snapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	lower := onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	id := snapshotID(lower)
	upper := filepath.Join(snapshotsDir, id+".SQLITE")
	require.NoError(t, os.Rename(lower, upper))
	require.Contains(t, dirNames(t, snapshotsDir), id+".SQLITE")
	require.NoError(t, os.Remove(storePathUnder(home)))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", id}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, map[string]string{"1": upper}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_path FROM import_runs"))
	var status, statusErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &status, &statusErr), statusErr.String())
	var parsed struct {
		Snapshot struct {
			Path string `json:"path"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(status.Bytes(), &parsed))
	assert.Equal(t, upper, parsed.Snapshot.Path)
}

// Case-insensitive volumes resolve a differently-cased name too, so every row renames first and asserts the
// on-disk names; the recorded path is what tells the spellings apart.
func Test_run_sync_from_a_path_finds_its_snapshot_and_manifest_in_any_letter_case(t *testing.T) {
	cases := []struct {
		name string
		// arrange renames or links files in dir around id's snapshot and returns the --from value and the path the store should record.
		arrange   func(t *testing.T, home, dir, id string) (from, recorded string)
		wantNames func(id string) []string
	}{
		{
			name: "an upper-case snapshot named by its absolute path",
			arrange: func(t *testing.T, _, dir, id string) (string, string) {
				t.Helper()
				upper := filepath.Join(dir, id+".SQLITE")
				require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
				return upper, upper
			},
			wantNames: func(id string) []string { return []string{id + ".SQLITE", id + ".json"} },
		},
		{
			name: "an upper-case snapshot named relative to the working directory",
			arrange: func(t *testing.T, _, dir, id string) (string, string) {
				t.Helper()
				upper := filepath.Join(dir, id+".SQLITE")
				require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
				t.Chdir(dir)
				return id + ".SQLITE", upper
			},
			wantNames: func(id string) []string { return []string{id + ".SQLITE", id + ".json"} },
		},
		{
			name: "a lower-case snapshot beside an upper-case manifest",
			arrange: func(t *testing.T, _, dir, id string) (string, string) {
				t.Helper()
				require.NoError(t, os.Rename(filepath.Join(dir, id+".json"), filepath.Join(dir, id+".JSON")))
				return filepath.Join(dir, id+".sqlite"), filepath.Join(dir, id+".sqlite")
			},
			wantNames: func(id string) []string { return []string{id + ".JSON", id + ".sqlite"} },
		},
		{
			name: "an upper-case snapshot beside an upper-case manifest",
			arrange: func(t *testing.T, _, dir, id string) (string, string) {
				t.Helper()
				upper := filepath.Join(dir, id+".SQLITE")
				require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
				require.NoError(t, os.Rename(filepath.Join(dir, id+".json"), filepath.Join(dir, id+".JSON")))
				return upper, upper
			},
			wantNames: func(id string) []string { return []string{id + ".JSON", id + ".SQLITE"} },
		},
		{
			name: "a symlink named as the snapshot, which only the ID form refuses",
			arrange: func(t *testing.T, home, dir, id string) (string, string) {
				t.Helper()
				target := filepath.Join(home, "elsewhere", id+".sqlite")
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
				require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), target))
				link := symlink(t, target, filepath.Join(dir, id+".sqlite"))
				return link, link
			},
			wantNames: func(id string) []string { return []string{id + ".json", id + ".sqlite"} },
		},
	}

	for _, c := range cases {
		for _, format := range [][]string{nil, {"--json"}} {
			t.Run(c.name+" "+strings.Join(format, " "), func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
				dir := filepath.Join(storeDirUnder(home), "snapshots")
				id := snapshotID(onlyFileWithSuffix(t, dir, ".sqlite"))
				from, recorded := c.arrange(t, home, dir, id)
				require.Equal(t, c.wantNames(id), dirNames(t, dir))
				require.NoError(t, os.Remove(storePathUnder(home)))

				exitCode, stdout, stderr := runSyncFrom(t, from, format...)

				require.Equal(t, 0, exitCode, stderr)
				assert.NotEmpty(t, stdout)
				assert.Empty(t, stderr)
				assert.Equal(t, map[string]string{"1": recorded}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_path FROM import_runs"))
			})
		}
	}
}
