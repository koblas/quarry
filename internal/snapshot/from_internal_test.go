// White-box: locateFrom's ID-versus-path split and the path shapes behind it are
// unexported decisions, reached here without a Server, reference or importer.
package snapshot

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isPathForm_tells_a_file_name_from_an_id(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "an ID", value: "20260927T143005Z", want: false},
		{name: "a lower-case .sqlite name", value: "x.sqlite", want: true},
		{name: "an upper-case .SQLITE name", value: "X.SQLITE", want: true},
		{name: "a mixed-case .Sqlite name", value: "X.Sqlite", want: true},
		{name: "a slash without the extension", value: "/a/b", want: true},
		{name: "a leading tilde with a slash", value: "~/x", want: true},
		{name: "a .json name is not a snapshot file", value: "x.json", want: false},
		{name: "the extension only in the middle", value: "x.sqlite.bak", want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, isPathForm(c.value))
		})
	}
}

func Test_locateFrom_maps_each_path_shape_to_its_snapshot_and_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotDir := filepath.Join(home, "snapshots")
	files := []string{"x.sqlite", "b", "X.SQLITE"}
	for _, name := range files {
		require.NoError(t, os.WriteFile(filepath.Join(home, name), []byte("x"), 0o600))
	}
	cases := []struct {
		name         string
		value        string
		wantSnapshot string
		wantManifest string
	}{
		{
			name: "a leading tilde expands against home", value: "~/x.sqlite",
			wantSnapshot: filepath.Join(home, "x.sqlite"), wantManifest: filepath.Join(home, "x.json"),
		},
		{
			name: "a slash makes a path even without the suffix", value: filepath.Join(home, "b"),
			wantSnapshot: filepath.Join(home, "b"), wantManifest: filepath.Join(home, "b.json"),
		},
		{
			name: "an upper-case extension is stripped for the manifest", value: filepath.Join(home, "X.SQLITE"),
			wantSnapshot: filepath.Join(home, "X.SQLITE"), wantManifest: filepath.Join(home, "X.json"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			snapshotPath, manifestPath, err := NewServer(WithHome(home), WithSnapshotDir(snapshotDir)).locateFrom(c.value)

			require.NoError(t, err)
			assert.Equal(t, c.wantSnapshot, snapshotPath)
			assert.Equal(t, c.wantManifest, manifestPath)
		})
	}
}

func Test_locateFrom_resolves_a_relative_name_against_the_working_directory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.SQLITE"), []byte("x"), 0o600))
	t.Chdir(dir)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	snapshotPath, manifestPath, err := NewServer(WithHome(t.TempDir()), WithSnapshotDir(t.TempDir())).locateFrom("x.SQLITE")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "x.SQLITE"), snapshotPath)
	assert.Equal(t, filepath.Join(cwd, "x.json"), manifestPath)
}

// listing is a read-dir seam that returns names as regular files whatever folder it is asked for.
func listing(names ...string) func(string) ([]fs.DirEntry, error) {
	return func(string) ([]fs.DirEntry, error) { return entries(names...), nil }
}

func Test_locate_by_id_takes_the_winner_through_the_read_dir_seam(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	snapshotDir := filepath.Join(home, "snapshots")
	srv := NewServer(WithHome(home), WithSnapshotDir(snapshotDir),
		WithReadDir(listing(selID+".Sqlite", selID+".SQLITE", selID+".JSON")))

	snapshotPath, manifestPath, err := srv.locateFrom(selID)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(snapshotDir, selID+".SQLITE"), snapshotPath)
	assert.Equal(t, filepath.Join(snapshotDir, selID+".JSON"), manifestPath)
}

func Test_locate_by_path_finds_the_manifest_through_the_read_dir_seam(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, "X.sqlite")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	srv := NewServer(WithHome(home), WithSnapshotDir(filepath.Join(home, "snapshots")), WithReadDir(listing("X.JSON")))

	snapshotPath, manifestPath, err := srv.locateFrom(path)

	require.NoError(t, err)
	assert.Equal(t, path, snapshotPath)
	assert.Equal(t, filepath.Join(home, "X.JSON"), manifestPath)
}

func Test_locate_refuses_with_the_unreadable_folder_copy_when_the_read_dir_seam_fails(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	backups := filepath.Join(home, "Backups")
	require.NoError(t, os.Mkdir(backups, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(backups, "x.sqlite"), []byte("x"), 0o600))
	denied := func(string) ([]fs.DirEntry, error) {
		return nil, &fs.PathError{Op: "open", Path: "dir", Err: syscall.EACCES}
	}
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "an ID names the snapshots folder", value: selID, want: "cannot read ~/snapshots: permission denied"},
		{name: "a path names its parent folder", value: filepath.Join(backups, "x.sqlite"), want: "cannot read ~/Backups: permission denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := NewServer(WithHome(home), WithSnapshotDir(filepath.Join(home, "snapshots")), WithReadDir(denied))

			_, _, err := srv.locateFrom(c.value)

			require.EqualError(t, err, c.want)
		})
	}
}
