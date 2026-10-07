package snapshot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
