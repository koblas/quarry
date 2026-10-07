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

// Case-insensitive volumes resolve the lower-case name too: only the recorded path and the printed id tell them apart.
func Test_run_status_names_a_store_built_from_an_upper_case_sqlite_snapshot_by_its_id(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	upper := filepath.Join(snapshotsDir, id+".SQLITE")
	require.NoError(t, os.Rename(filepath.Join(snapshotsDir, id+".sqlite"), upper))
	require.Contains(t, dirNames(t, snapshotsDir), id+".SQLITE")
	require.NoError(t, os.Remove(storePathUnder(home)))
	var rebuildOut, rebuildErr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--from", id}, &rebuildOut, &rebuildErr), rebuildErr.String())

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Contains(t, stdout.String(), "\nSnapshot  "+id+", taken ")
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := run(context.Background(), []string{"status", "--json"}, &stdout, &stderr)

		require.Equal(t, 0, exitCode, stderr.String())
		var parsed struct {
			Snapshot struct {
				ID   string `json:"id"`
				Path string `json:"path"`
			} `json:"snapshot"`
		}
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &parsed))
		assert.Equal(t, id, parsed.Snapshot.ID)
		assert.Equal(t, upper, parsed.Snapshot.Path)
	})
}
