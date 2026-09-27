package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Source whose Backup always succeeds without writing anything, so tests
// that only exercise Destination's own file handling need no real SQLite
// database.
type noopBackupSource struct{}

func (noopBackupSource) Open(context.Context, string) error   { return nil }
func (noopBackupSource) Probe(context.Context) error          { return nil }
func (noopBackupSource) Backup(context.Context, string) error { return nil }
func (noopBackupSource) Close() error                         { return nil }

func Test_dirDestination_prepare_creates_the_directory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "snapshots")
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func Test_dirDestination_backup_fails_when_the_directory_is_not_writable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	_, err := dest.Backup(t.Context(), noopBackupSource{}, "20260927T143005Z")

	require.Error(t, err)
}

func Test_dirDestination_write_manifest_fails_when_the_directory_is_not_writable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	_, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))

	require.Error(t, err)
}

func Test_dirDestination_commit_snapshot_refuses_to_replace_an_existing_file(t *testing.T) {
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, err := dest.Backup(t.Context(), noopBackupSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.sqlite"), []byte("existing"), 0o600))

	_, err = dest.CommitSnapshot(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_commit_manifest_refuses_to_replace_an_existing_file(t *testing.T) {
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.json"), []byte("existing"), 0o600))

	_, err = dest.CommitManifest(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_discard_removes_the_partial(t *testing.T) {
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, err := dest.Backup(t.Context(), noopBackupSource{}, "20260927T143005Z")
	require.NoError(t, err)

	err = dest.Discard(t.Context(), partial)

	require.NoError(t, err)
	_, statErr := os.Stat(partial)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_dirDestination_discard_fails_when_the_partial_cannot_be_removed(t *testing.T) {
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, err := dest.Backup(t.Context(), noopBackupSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err = dest.Discard(t.Context(), partial)

	require.Error(t, err)
}
