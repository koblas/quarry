package snapshot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_dirDestination_prepare_creates_the_directory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "snapshots")
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func Test_dirDestination_backup_fails_when_the_directory_is_not_writable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	_, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.ErrorIs(t, err, fs.ErrPermission)
}

func Test_dirDestination_backup_reserves_the_next_suffix_when_the_current_second_is_taken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, name, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.Equal(t, "20260927T143005Z", name)
	_, err = dest.CommitSnapshot(t.Context(), partial)
	require.NoError(t, err)

	_, second, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.NoError(t, err)
	assert.Equal(t, "20260927T143005Z_2", second)
}

func Test_dirDestination_backup_reserves_the_next_suffix_when_the_first_two_names_are_taken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	firstPartial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	_, err = dest.CommitSnapshot(t.Context(), firstPartial)
	require.NoError(t, err)
	secondPartial, secondName, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.Equal(t, "20260927T143005Z_2", secondName)
	_, err = dest.CommitSnapshot(t.Context(), secondPartial)
	require.NoError(t, err)

	_, third, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.NoError(t, err)
	assert.Equal(t, "20260927T143005Z_3", third)
}

func Test_dirDestination_backup_reserves_the_next_suffix_when_only_the_manifest_final_is_taken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	manifestPartial, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))
	require.NoError(t, err)
	_, err = dest.CommitManifest(t.Context(), manifestPartial)
	require.NoError(t, err)

	_, name, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.NoError(t, err)
	assert.Equal(t, "20260927T143005Z_2", name)
}

func Test_dirDestination_backup_reserves_the_next_suffix_when_another_backup_is_already_in_flight(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".20260927T143005Z.sqlite.partial"), []byte("in flight"), 0o600))
	dest := snapshot.NewDirDestination(dir)

	_, name, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.NoError(t, err)
	assert.Equal(t, "20260927T143005Z_2", name)
}

func Test_dirDestination_backup_returns_immediately_when_the_partial_cannot_be_created_for_a_reason_other_than_a_collision(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	// The first candidate collides, forcing the loop to "_2" before the permission failure.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".20260927T143005Z.sqlite.partial"), []byte("in flight"), 0o600))
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	_, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.ErrorIs(t, err, fs.ErrPermission)
	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, ".20260927T143005Z_2.sqlite.partial", filepath.Base(pathErr.Path), "expected the failure on the second candidate, not a later one")
}

func Test_dirDestination_write_manifest_fails_when_the_directory_is_not_writable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	_, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))

	require.ErrorIs(t, err, fs.ErrPermission)
}

func Test_dirDestination_commit_snapshot_refuses_to_replace_an_existing_file(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.sqlite"), []byte("existing"), 0o600))

	_, err = dest.CommitSnapshot(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_commit_manifest_refuses_to_replace_an_existing_file(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.json"), []byte("existing"), 0o600))

	_, err = dest.CommitManifest(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_discard_removes_the_partial(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)

	err = dest.Discard(t.Context(), partial)

	require.NoError(t, err)
	_, statErr := os.Stat(partial)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_dirDestination_discard_fails_when_the_partial_cannot_be_removed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := snapshot.NewDirDestination(dir)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err = dest.Discard(t.Context(), partial)

	require.ErrorIs(t, err, fs.ErrPermission)
}

// writeAged writes name under dir with contents "x" and backdates its mtime
// by age, so the sweep's own os.ReadDir/time.Now cannot tell it apart from
// crash debris left age ago.
func writeAged(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	when := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, when, when))
	return path
}

func Test_dirDestination_prepare_removes_old_leftover_partials_matching_the_quarry_pattern(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const old = 2 * time.Hour
	matching := []string{
		".20260927T143005Z.sqlite.partial",
		".20260927T143005Z.json.partial",
		".20260927T143005Z_2.sqlite.partial",
		".20260927T143005Z.sqlite.partial-wal",
		".20260927T143005Z.sqlite.partial-shm",
		".20260927T143005Z.sqlite.partial-journal",
	}
	for _, name := range matching {
		writeAged(t, dir, name, old)
	}
	kept := []string{".DS_Store", ".20260927T143005Z.notes.partial", "20260927T143005Z.sqlite.partial"}
	for _, name := range kept {
		writeAged(t, dir, name, old)
	}
	writeAged(t, dir, "20260927T143005Z.sqlite", old)
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	for _, name := range matching {
		_, statErr := os.Stat(filepath.Join(dir, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, "expected %s removed", name)
	}
	for _, name := range append(kept, "20260927T143005Z.sqlite") {
		assert.FileExists(t, filepath.Join(dir, name))
	}
}

func Test_dirDestination_prepare_leaves_a_fresh_partial_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fresh := filepath.Join(dir, ".20260927T143005Z.sqlite.partial")
	require.NoError(t, os.WriteFile(fresh, []byte("x"), 0o600))
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	assert.FileExists(t, fresh)
}

func Test_dirDestination_prepare_removes_a_leftover_just_past_the_age_gate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 61*time.Minute)
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_dirDestination_prepare_leaves_a_leftover_just_inside_the_age_gate_alone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 59*time.Minute)
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	assert.FileExists(t, path)
}

func Test_dirDestination_prepare_still_succeeds_when_a_leftover_cannot_be_removed(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	path := writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 2*time.Hour)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	assert.FileExists(t, path)
}

func Test_dirDestination_prepare_still_succeeds_when_the_sweep_cannot_read_the_directory(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	dir := t.TempDir()
	writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 2*time.Hour)
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
}
