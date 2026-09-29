package atomicfile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/atomicfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_create_refuses_an_existing_file(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o600))

	_, err := atomicfile.Create(path, 0o600)

	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrExist))
}

func Test_commit_moves_the_partial_into_place(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	partial := filepath.Join(dir, ".partial")
	dest := filepath.Join(dir, "final")
	require.NoError(t, os.WriteFile(partial, []byte("payload"), 0o600))

	err := atomicfile.Commit(partial, dest)

	require.NoError(t, err)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
	_, err = os.Stat(partial)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}

func Test_commit_refuses_to_replace_an_existing_file(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	partial := filepath.Join(dir, ".partial")
	dest := filepath.Join(dir, "final")
	require.NoError(t, os.WriteFile(partial, []byte("new"), 0o600))
	require.NoError(t, os.WriteFile(dest, []byte("original"), 0o600))

	err := atomicfile.Commit(partial, dest)

	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrExist))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

// The partial's directory is made read-only after writing, so the hard
// link succeeds but removing the old entry cannot.
func Test_commit_succeeds_when_the_partial_cannot_be_removed_after_linking(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	partialDir := filepath.Join(t.TempDir(), "partial-dir")
	require.NoError(t, os.MkdirAll(partialDir, 0o755))
	partial := filepath.Join(partialDir, ".partial")
	dest := filepath.Join(t.TempDir(), "final")
	require.NoError(t, os.WriteFile(partial, []byte("payload"), 0o600))
	require.NoError(t, os.Chmod(partialDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(partialDir, 0o755) })

	err := atomicfile.Commit(partial, dest)

	require.NoError(t, err)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
}

// destDir is unreadable (still writable+executable, all Link needs) before
// Commit, so Link succeeds but fsyncing the directory afterward cannot open it.
func Test_commit_succeeds_when_the_destination_directory_cannot_be_synced(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	destDir := filepath.Join(t.TempDir(), "dest-dir")
	require.NoError(t, os.MkdirAll(destDir, 0o755))
	partial := filepath.Join(t.TempDir(), ".partial")
	dest := filepath.Join(destDir, "final")
	require.NoError(t, os.WriteFile(partial, []byte("payload"), 0o600))
	require.NoError(t, os.Chmod(destDir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(destDir, 0o755) })

	err := atomicfile.Commit(partial, dest)

	require.NoError(t, err)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
}

func Test_commit_fails_when_the_partial_is_missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	partial := filepath.Join(dir, ".partial")
	dest := filepath.Join(dir, "final")

	err := atomicfile.Commit(partial, dest)

	require.Error(t, err)
	assert.True(t, errors.Is(err, fs.ErrNotExist))
}
