package duckdb_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_create_refuses_an_existing_path(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.duckdb")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))

	_, err := duckdb.Create(t.Context(), path)

	require.ErrorIs(t, err, duckdb.ErrExists)
}

func Test_create_makes_the_file_owner_only(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.duckdb")

	db, err := duckdb.Create(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func Test_create_fails_with_a_permission_error_in_a_read_only_directory(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "ro")
	require.NoError(t, os.Mkdir(roDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })
	path := filepath.Join(roDir, "data.duckdb")

	_, err := duckdb.Create(t.Context(), path)

	require.Error(t, err)
	assert.True(t, duckdb.IsPermission(err), "expected a permission-classified error, got %v", err)
}

// A regular file, not a directory, as a path segment makes os.Stat fail
// with ENOTDIR — an error Create must not mistake for "does not exist yet".
func Test_create_fails_when_a_path_segment_is_a_regular_file(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o600))
	path := filepath.Join(regular, "sub", "data.duckdb")

	_, err := duckdb.Create(t.Context(), path)

	require.Error(t, err)
	require.NotErrorIs(t, err, duckdb.ErrExists)
}

// sql.Open's own eager file-create/open already succeeded (the path is a
// writable temp dir), so a context cancelled before Create runs lands
// specifically on PingContext, not on sql.Open.
func Test_create_fails_when_the_context_is_already_cancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.duckdb")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := duckdb.Create(ctx, path)

	require.ErrorIs(t, err, context.Canceled)
}
