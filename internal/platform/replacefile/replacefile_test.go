package replacefile_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/platform/replacefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_write_creates_the_file(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "target")

	err := replacefile.Write(path, []byte("new"), 0o600)

	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func Test_write_replaces_an_existing_file_whole(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(path, []byte("the old contents, longer than the new"), 0o600))

	err := replacefile.Write(path, []byte("new"), 0o600)

	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

func Test_write_gives_the_file_the_requested_mode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "target")

	err := replacefile.Write(path, []byte("new"), 0o644)

	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), info.Mode().Perm())
}

func Test_write_leaves_only_the_target_in_the_folder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := replacefile.Write(filepath.Join(dir, "target"), []byte("new"), 0o600)

	require.NoError(t, err)
	assert.Equal(t, []string{"target"}, names(t, dir))
}

func Test_write_leaves_the_old_file_and_no_temp_when_the_rename_fails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	require.NoError(t, os.MkdirAll(filepath.Join(path, "inside"), 0o755))

	err := replacefile.Write(path, []byte("new"), 0o600)

	var linkErr *os.LinkError
	require.ErrorAs(t, err, &linkErr)
	require.ErrorContains(t, err, path)
	assert.Equal(t, []string{"target"}, names(t, dir))
	assert.Equal(t, []string{"inside"}, names(t, path))
}

func Test_write_fails_when_the_folder_is_read_only(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := replacefile.Write(filepath.Join(dir, "target"), []byte("new"), 0o600)

	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Empty(t, names(t, dir))
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
