// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bundle is moved out of Documents so any read of Quicken, or discovery, would fail.
func Test_run_rebuilds_the_store_from_an_earlier_snapshot_without_quicken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	var syncStdout, syncStderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &syncStdout, &syncStderr))
	quarryDir := filepath.Join(home, "Library", "Application Support", "quarry")
	snapshotsDir := filepath.Join(quarryDir, "snapshots")
	manifestPath := onlyFileWithSuffix(t, snapshotsDir, ".json")
	manifestBefore, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	id := snapshotID(onlyFileWithSuffix(t, snapshotsDir, ".sqlite"))
	require.NoError(t, os.Rename(bundle.Dir, filepath.Join(home, "Elsewhere.quicken")))
	storePath := filepath.Join(quarryDir, "quarry.duckdb")
	require.NoError(t, os.Remove(storePath))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", id}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, syncStdout.String(), stdout.String())
	manifestAfter, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, manifestBefore, manifestAfter)
	assert.FileExists(t, storePath)
	onlyFileWithSuffix(t, snapshotsDir, ".sqlite")
	onlyFileWithSuffix(t, snapshotsDir, ".json")
}
