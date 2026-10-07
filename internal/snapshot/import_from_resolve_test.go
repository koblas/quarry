package snapshot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fromID      = "20260927T143005Z"
	fromOtherID = "20260930T141502Z"
	fromDDL     = "CREATE TABLE t (x INTEGER)"
)

// noSnapshotLine is the refusal for an ID that names no snapshot in ~/snapshots.
func noSnapshotLine(id string) string {
	return "no snapshot " + id + " in ~/snapshots; run quarry snapshots to list the ones kept"
}

func Test_import_from_an_id_refuses_a_snapshots_folder_it_cannot_list(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	cases := []struct {
		name string
		mode os.FileMode
	}{
		{name: "no permission at all", mode: 0o000},
		{name: "searchable but not readable", mode: 0o300},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			fake := &fakeImporter{}
			srv := newImportServer(t, home, fake)
			dir := filepath.Join(home, "snapshots")
			writeSnapshotPair(t, dir, fromID, fromDDL)
			require.NoError(t, os.Chmod(dir, c.mode))
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

			_, err := srv.ImportFrom(t.Context(), fromID)

			require.EqualError(t, err, "cannot read ~/snapshots: permission denied")
			assert.Empty(t, fake.calls)
		})
	}
}

func Test_import_from_an_id_refuses_a_snapshots_folder_that_is_a_file(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	require.NoError(t, os.WriteFile(filepath.Join(home, "snapshots"), []byte("x"), 0o600))

	_, err := srv.ImportFrom(t.Context(), fromID)

	require.EqualError(t, err, "cannot read ~/snapshots: not a directory")
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_id_finds_no_snapshot_in_a_folder_that_holds_only_other_ids(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	writeSnapshotPair(t, filepath.Join(home, "snapshots"), fromOtherID, fromDDL)

	_, err := srv.ImportFrom(t.Context(), fromID)

	require.EqualError(t, err, noSnapshotLine(fromID))
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_id_finds_no_snapshot_when_the_only_entry_is_a_directory(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "snapshots", fromID+".sqlite"), 0o700))

	_, err := srv.ImportFrom(t.Context(), fromID)

	require.EqualError(t, err, noSnapshotLine(fromID))
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_id_finds_no_snapshot_when_the_only_entry_is_a_symlink(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := filepath.Join(home, "snapshots")
	writeSnapshotPair(t, filepath.Join(home, "elsewhere"), fromID, fromDDL)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.Symlink(filepath.Join(home, "elsewhere", fromID+".sqlite"), filepath.Join(dir, fromID+".sqlite")))

	_, err := srv.ImportFrom(t.Context(), fromID)

	require.EqualError(t, err, noSnapshotLine(fromID))
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_id_does_not_fold_the_id_itself(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	writeSnapshotPair(t, filepath.Join(home, "snapshots"), fromID, fromDDL)
	lowered := strings.ToLower(fromID)

	_, err := srv.ImportFrom(t.Context(), lowered)

	require.EqualError(t, err, noSnapshotLine(lowered))
	assert.Empty(t, fake.calls)
}

func Test_import_from_an_id_finds_no_snapshot_for_a_hand_placed_name_that_is_not_an_id(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	renameTakenSnapshot(t, srv, "latest")

	_, err := srv.ImportFrom(t.Context(), "latest")

	require.EqualError(t, err, noSnapshotLine("latest"))
	assert.Empty(t, fake.calls)
}

func Test_import_from_a_path_resolves_a_hand_placed_name_that_is_not_an_id(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := renameTakenSnapshot(t, srv, "latest")

	outcome, err := srv.ImportFrom(t.Context(), filepath.Join(dir, "latest.sqlite"))

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "latest.sqlite"), outcome.Manifest.Snapshot.Path)
	require.Len(t, fake.calls, 1)
}

func Test_import_from_an_id_resolves_an_upper_case_snapshot_and_manifest_by_their_on_disk_names(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	taken := takeSnapshot(t, srv)
	id := snapshotIDFromPath(taken.Snapshot.Path)
	dir := filepath.Dir(taken.Snapshot.Path)
	require.NoError(t, os.Rename(taken.Snapshot.Path, filepath.Join(dir, id+".SQLITE")))
	require.NoError(t, os.Rename(taken.Snapshot.Manifest, filepath.Join(dir, id+".JSON")))
	require.Equal(t, []string{id + ".JSON", id + ".SQLITE"}, dirNames(t, dir))

	outcome, err := srv.ImportFrom(t.Context(), id)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, id+".SQLITE"), outcome.Manifest.Snapshot.Path)
	assert.Equal(t, filepath.Join(dir, id+".JSON"), outcome.Manifest.Snapshot.Manifest)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, filepath.Join(dir, id+".SQLITE"), fake.calls[0].Path)
}

func Test_import_from_an_id_reports_no_manifest_when_the_snapshot_has_none_in_any_case(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := filepath.Join(home, "snapshots")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, fromID+".SQLITE"), []byte("x"), 0o600))

	_, err := srv.ImportFrom(t.Context(), fromID)

	require.EqualError(t, err, "~/snapshots/"+fromID+".SQLITE is not a quarry snapshot "+
		"(no .json manifest next to it); pass a snapshot taken by quarry sync with --from <snapshot>")
	assert.Empty(t, fake.calls)
}

func Test_import_from_a_path_refuses_a_parent_folder_it_cannot_list(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := filepath.Join(home, "Backups")
	writeSnapshotPair(t, dir, "x", fromDDL)
	require.NoError(t, os.Chmod(dir, 0o300))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := srv.ImportFrom(t.Context(), filepath.Join(dir, "x.sqlite"))

	require.EqualError(t, err, "cannot read ~/Backups: permission denied")
	assert.Empty(t, fake.calls)
}

func Test_import_from_a_path_names_the_upper_case_manifest_it_cannot_read(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := filepath.Join(home, "Backups")
	writeSnapshotPair(t, dir, "x", fromDDL)
	manifest := filepath.Join(dir, "x.JSON")
	require.NoError(t, os.Rename(filepath.Join(dir, "x.json"), manifest))
	require.Equal(t, []string{"x.JSON", "x.sqlite"}, dirNames(t, dir))
	require.NoError(t, os.Chmod(manifest, 0o000))
	t.Cleanup(func() { _ = os.Chmod(manifest, 0o600) })

	_, err := srv.ImportFrom(t.Context(), filepath.Join(dir, "x.sqlite"))

	require.EqualError(t, err, "cannot read ~/Backups/x.JSON: permission denied; check the file's permissions")
	assert.Empty(t, fake.calls)
}

func Test_import_from_a_path_reports_no_manifest_when_none_exists_in_any_case(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	fake := &fakeImporter{}
	srv := newImportServer(t, home, fake)
	dir := filepath.Join(home, "Backups")
	writeSnapshotPair(t, dir, "x", fromDDL)
	require.NoError(t, os.Remove(filepath.Join(dir, "x.json")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "X2.JSON"), []byte("{}"), 0o600))

	_, err := srv.ImportFrom(t.Context(), filepath.Join(dir, "x.sqlite"))

	require.EqualError(t, err, "~/Backups/x.sqlite is not a quarry snapshot "+
		"(no .json manifest next to it); pass a snapshot taken by quarry sync with --from <snapshot>")
	assert.Empty(t, fake.calls)
}

// renameTakenSnapshot takes a snapshot, renames it and its manifest to name, and returns the snapshots folder.
func renameTakenSnapshot(t *testing.T, srv *snapshot.Server, name string) string {
	t.Helper()
	taken := takeSnapshot(t, srv)
	dir := filepath.Dir(taken.Snapshot.Path)
	require.NoError(t, os.Rename(taken.Snapshot.Path, filepath.Join(dir, name+".sqlite")))
	require.NoError(t, os.Rename(taken.Snapshot.Manifest, filepath.Join(dir, name+".json")))
	return dir
}
