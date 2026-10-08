package snapshot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDestination returns a directory Destination over a fresh folder, and that folder.
func newDestination(tb testing.TB) (snapshot.Destination, string) {
	tb.Helper()
	dir := tb.TempDir()
	return snapshot.NewDirDestination(dir), dir
}

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
	restrictMode(t, dir, 0o500)
	dest := snapshot.NewDirDestination(dir)

	_, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")

	require.ErrorIs(t, err, fs.ErrPermission)
}

func Test_dirDestination_backup_reserves_the_next_suffix_when_the_current_second_is_taken(t *testing.T) {
	t.Parallel()
	dest, _ := newDestination(t)
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
	dest, _ := newDestination(t)
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
	dest, _ := newDestination(t)
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
	skipUnderRoot(t)
	dir := t.TempDir()
	// The first candidate collides, forcing the loop to "_2" before the permission failure.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".20260927T143005Z.sqlite.partial"), []byte("in flight"), 0o600))
	restrictMode(t, dir, 0o500)
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
	restrictMode(t, dir, 0o500)
	dest := snapshot.NewDirDestination(dir)

	_, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))

	require.ErrorIs(t, err, fs.ErrPermission)
}

func Test_dirDestination_commit_snapshot_refuses_to_replace_an_existing_file(t *testing.T) {
	t.Parallel()
	dest, dir := newDestination(t)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.sqlite"), []byte("existing"), 0o600))

	_, err = dest.CommitSnapshot(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_commit_manifest_refuses_to_replace_an_existing_file(t *testing.T) {
	t.Parallel()
	dest, dir := newDestination(t)
	partial, err := dest.WriteManifest(t.Context(), "20260927T143005Z", []byte("{}"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "20260927T143005Z.json"), []byte("existing"), 0o600))

	_, err = dest.CommitManifest(t.Context(), partial)

	require.Error(t, err)
}

func Test_dirDestination_discard_removes_the_partial(t *testing.T) {
	t.Parallel()
	dest, _ := newDestination(t)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)

	err = dest.Discard(t.Context(), partial)

	require.NoError(t, err)
	_, statErr := os.Stat(partial)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func Test_dirDestination_discard_fails_when_the_partial_cannot_be_removed(t *testing.T) {
	t.Parallel()
	dest, dir := newDestination(t)
	partial, _, err := dest.Backup(t.Context(), &fakeSource{}, "20260927T143005Z")
	require.NoError(t, err)
	restrictMode(t, dir, 0o500)

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
		require.ErrorIs(t, statErr, os.ErrNotExist, "expected %s removed", name)
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
	skipUnderRoot(t)
	dir := t.TempDir()
	path := writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 2*time.Hour)
	restrictMode(t, dir, 0o500)
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
	assert.FileExists(t, path)
}

func Test_dirDestination_prepare_still_succeeds_when_the_sweep_cannot_read_the_directory(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	dir := t.TempDir()
	writeAged(t, dir, ".20260927T143005Z.sqlite.partial", 2*time.Hour)
	restrictMode(t, dir, 0o000)
	dest := snapshot.NewDirDestination(dir)

	err := dest.Prepare(t.Context())

	require.NoError(t, err)
}

const letterCaseName = "20260927T143005Z"

// listedEntry is a listing entry of the given mode that exists in no folder, so Info is never asked of it.
func listedEntry(name string, mode fs.FileMode) fs.DirEntry {
	return variantEntry{name: name, mode: mode}
}

func Test_dirDestination_backup_skips_an_id_the_folder_uses_in_another_letter_case(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		listing []fs.DirEntry
		err     error
		want    string
	}{
		{name: "upper-case snapshot extension", listing: []fs.DirEntry{listedEntry(letterCaseName+".SQLITE", 0)}, want: letterCaseName + "_2"},
		{name: "mixed-case snapshot extension on the second name too", listing: []fs.DirEntry{
			listedEntry(letterCaseName+".SQLITE", 0), listedEntry(letterCaseName+"_2.Sqlite", 0),
		}, want: letterCaseName + "_3"},
		{name: "upper-case manifest extension", listing: []fs.DirEntry{listedEntry(letterCaseName+".JSON", 0)}, want: letterCaseName + "_2"},
		{name: "directory named as the snapshot", listing: []fs.DirEntry{listedEntry(letterCaseName+".SQLITE", fs.ModeDir)}, want: letterCaseName + "_2"},
		{name: "another ID's files", listing: []fs.DirEntry{listedEntry("20260101T000000Z.SQLITE", 0)}, want: letterCaseName},
		{name: "listing fault", err: &fs.PathError{Op: "open", Path: "snapshots", Err: fs.ErrPermission}, want: letterCaseName},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dest := snapshot.NewDirDestinationReading(t.TempDir(), func(string) ([]fs.DirEntry, error) { return c.listing, c.err })

			_, name, err := dest.Backup(t.Context(), &fakeSource{}, letterCaseName)

			require.NoError(t, err)
			assert.Equal(t, c.want, name)
		})
	}
}

var firstPartial = regexp.MustCompile(`^\.(\d{8}T\d{6}Z)\.sqlite\.partial$`)

// readDirWithUpperCaseStrayForFirstPartial lists dir plus an upper-case snapshot named for the first partial found in it.
func readDirWithUpperCaseStrayForFirstPartial(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if match := firstPartial.FindStringSubmatch(entry.Name()); match != nil {
			entries = append(entries, listedEntry(match[1]+".SQLITE", 0))
		}
	}
	return entries, nil
}

func Test_sync_skips_an_id_the_folder_uses_in_another_letter_case(t *testing.T) {
	t.Parallel()
	bundle := v9fixture.OpenBundle(t, t.TempDir())
	snapshotsDir := filepath.Join(t.TempDir(), "quarry", "snapshots")
	srv := newServer(t, snapshotsDir, snapshot.WithReadDir(readDirWithUpperCaseStrayForFirstPartial))

	manifest, err := srv.Sync(t.Context(), bundle.Dir)

	require.NoError(t, err)
	assert.Regexp(t, `^\d{8}T\d{6}Z_2\.sqlite$`, filepath.Base(manifest.Snapshot.Path))
}

func Test_dirDestination_backup_skips_an_id_a_final_on_disk_uses_when_the_folder_cannot_be_listed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
	}{
		{name: "snapshot only", file: letterCaseName + ".sqlite"},
		{name: "manifest only", file: letterCaseName + ".json"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, c.file), []byte("x"), 0o600))
			unlistable := func(string) ([]fs.DirEntry, error) { return nil, fs.ErrPermission }
			dest := snapshot.NewDirDestinationReading(dir, unlistable)

			_, name, err := dest.Backup(t.Context(), &fakeSource{}, letterCaseName)

			require.NoError(t, err)
			assert.Equal(t, letterCaseName+"_2", name)
		})
	}
}
