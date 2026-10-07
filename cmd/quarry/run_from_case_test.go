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
