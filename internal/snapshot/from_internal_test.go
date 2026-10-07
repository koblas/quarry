// White-box: locateFrom's ID-versus-path split and the path shapes behind it are
// unexported decisions, reached here without a Server, reference or importer.
package snapshot

import (
	"os"
	"path/filepath"
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

			snapshotPath, manifestPath, err := locateFrom(home, snapshotDir, c.value)

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

	snapshotPath, manifestPath, err := locateFrom(t.TempDir(), t.TempDir(), "x.SQLITE")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "x.SQLITE"), snapshotPath)
	assert.Equal(t, filepath.Join(cwd, "x.json"), manifestPath)
}
