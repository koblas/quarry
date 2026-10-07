package snapshot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renamedExtension renames id's file from extension from to extension to.
// A rename is needed because case-insensitive volumes keep the old name's case on a rewrite.
func renamedExtension(t *testing.T, dir, id, from, to string) {
	t.Helper()
	require.NoError(t, os.Rename(filepath.Join(dir, id+"."+from), filepath.Join(dir, id+"."+to)))
}

func Test_list_gives_an_upper_case_sqlite_snapshot_its_on_disk_path_and_manifest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                     string
		snapshotExt, manifestExt string
		wantSnapshot             string
		wantManifest             string
	}{
		{
			name: "upper-case snapshot, lower-case manifest", snapshotExt: "SQLITE", manifestExt: "json",
			wantSnapshot: "20260927T143005Z.SQLITE", wantManifest: "20260927T143005Z.json",
		},
		{
			name: "lower-case snapshot, upper-case manifest", snapshotExt: "sqlite", manifestExt: "JSON",
			wantSnapshot: "20260927T143005Z.sqlite", wantManifest: "20260927T143005Z.JSON",
		},
		{
			name: "upper-case snapshot and manifest", snapshotExt: "SQLITE", manifestExt: "JSON",
			wantSnapshot: "20260927T143005Z.SQLITE", wantManifest: "20260927T143005Z.JSON",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := snapshotsFolder(t, home)
			writeSnapshot(t, dir, idOldest, 1500)
			writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T14:30:05Z"))
			renamedExtension(t, dir, idOldest, "sqlite", c.snapshotExt)
			renamedExtension(t, dir, idOldest, "json", c.manifestExt)
			require.ElementsMatch(t, []string{c.wantSnapshot, c.wantManifest}, dirNames(t, dir))

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			require.Len(t, listing.Entries, 1)
			entry := listing.Entries[0]
			assert.Equal(t, idOldest, entry.ID)
			assert.Equal(t, filepath.Join(dir, c.wantSnapshot), entry.Path)
			assert.Equal(t, filepath.Join(dir, c.wantManifest), entry.ManifestPath)
			require.NotNil(t, entry.Manifest)
			assert.Equal(t, "2026-09-27T14:30:05Z", entry.Manifest.Snapshot.TakenAt)
			assert.EqualValues(t, 1500, entry.Bytes)
			assert.EqualValues(t, 1500, listing.TotalBytes)
		})
	}
}

func Test_list_lists_no_directory_or_symlink_named_as_an_upper_case_snapshot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func(t *testing.T, path, outside string)
	}{
		{"a directory", func(t *testing.T, path, _ string) { t.Helper(); require.NoError(t, os.Mkdir(path, 0o700)) }},
		{"a symlink", func(t *testing.T, path, outside string) { t.Helper(); require.NoError(t, os.Symlink(outside, path)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := snapshotsFolder(t, home)
			writeSnapshot(t, dir, idNewest, 1000)
			outside := writeSnapshot(t, home, "elsewhere", 2000)
			c.build(t, filepath.Join(dir, idOldest+".SQLITE"), outside)

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, []string{idNewest}, entryIDs(listing))
			assert.EqualValues(t, 1000, listing.TotalBytes)
		})
	}
}
