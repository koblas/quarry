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

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The store partial and its wal are backdated past the sweep's age gate; a
// fresh one could belong to another build still in flight.
func Test_run_removes_stale_store_leftovers_before_syncing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundle := v9fixture.OpenBundle(t, filepath.Join(home, "Documents"))
	storeDir := filepath.Join(home, "Library", "Application Support", "quarry")
	require.NoError(t, os.MkdirAll(storeDir, 0o700))

	partial := filepath.Join(storeDir, ".quarry-20260101T000000Z.duckdb.partial")
	require.NoError(t, os.WriteFile(partial, []byte("crash debris"), 0o600))
	partialWAL := partial + ".wal"
	require.NoError(t, os.WriteFile(partialWAL, []byte("wal"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(partial, old, old))
	require.NoError(t, os.Chtimes(partialWAL, old, old))
	staleWAL := filepath.Join(storeDir, "quarry.duckdb.wal")
	require.NoError(t, os.WriteFile(staleWAL, []byte("wal"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	assert.Empty(t, stderr.String())
	_, err := os.Stat(partial)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(partialWAL)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(staleWAL)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(storeDir, "quarry.duckdb"))
	assert.NoError(t, err)
}
