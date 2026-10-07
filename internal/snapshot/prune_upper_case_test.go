package snapshot_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upperCased renames id's .sqlite in dir to .SQLITE and returns the new path.
// A rename is needed because case-insensitive volumes keep the old name's case on a rewrite.
func upperCased(t *testing.T, dir, id string) string {
	t.Helper()
	upper := filepath.Join(dir, id+".SQLITE")
	require.NoError(t, os.Rename(filepath.Join(dir, id+".sqlite"), upper))
	return upper
}

// dirNames lists the names os.ReadDir returns for dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	upperCased(t, dir, idNewest)
	oldestPath := upperCased(t, dir, idOldest)
	rm := &fakeRemover{}
	require.Equal(t, []string{
		"20260927T143005Z.SQLITE", "20260927T143005Z.json",
		"20260929T090011Z.json", "20260929T090011Z.sqlite",
		"20260930T141502Z.SQLITE", "20260930T141502Z.json",
	}, dirNames(t, dir))

	pruned, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"20260927T143005Z.SQLITE", "20260927T143005Z.json"}, rm.calls)
	require.Len(t, pruned.Deleted, 1)
	assert.Equal(t, oldestPath, pruned.Deleted[0].Path)
	assert.Equal(t, []string{
		"20260929T090011Z.json", "20260929T090011Z.sqlite",
		"20260930T141502Z.SQLITE", "20260930T141502Z.json",
	}, dirNames(t, dir))
}

func Test_prune_removes_the_manifest_by_its_on_disk_name(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		snapshotExt string
		want        []string
	}{
		{
			name: "lower-case snapshot, upper-case manifest", snapshotExt: "sqlite",
			want: []string{"20260927T143005Z.sqlite", "20260927T143005Z.JSON"},
		},
		{
			name: "upper-case snapshot and manifest", snapshotExt: "SQLITE",
			want: []string{"20260927T143005Z.SQLITE", "20260927T143005Z.JSON"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest)
			renamedExtension(t, dir, idOldest, "sqlite", c.snapshotExt)
			renamedExtension(t, dir, idOldest, "json", "JSON")
			rm := &fakeRemover{}

			_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, c.want, rm.calls)
		})
	}
}

func Test_prune_never_sweeps_the_manifest_of_a_directory_named_as_an_upper_case_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idOldest+".SQLITE"), 0o700))
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, rm.calls)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_reports_an_upper_case_sqlite_snapshot_it_could_not_delete(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	oldestPath := upperCased(t, dir, idOldest)

	pruned, err := newPruneServer(home, nil, failing("20260927T143005Z.SQLITE", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, oldestPath, pruned.Failed[0].Entry.Path)
	assert.Equal(t, "permission denied", pruned.Failed[0].Reason)
	assert.Empty(t, pruned.Deleted)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_plan_prune_would_delete_an_upper_case_sqlite_snapshot_by_its_on_disk_path(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	oldestPath := upperCased(t, dir, idOldest)
	rm := &fakeRemover{}

	pruned, err := newPruneServer(home, nil, rm).PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.WouldDelete, 1)
	assert.Equal(t, oldestPath, pruned.WouldDelete[0].Path)
	assert.Empty(t, rm.calls)
}

func Test_prune_keeps_the_manifest_while_another_entry_is_named_as_the_snapshot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		create func(path string) error
	}{
		{name: "a regular upper-case variant", create: func(path string) error {
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err == nil {
				err = f.Close()
			}
			return err
		}},
		{name: "a directory", create: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "a symlink", create: func(path string) error { return os.Symlink("elsewhere", path) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest)
			if err := c.create(filepath.Join(dir, idOldest+".SQLITE")); errors.Is(err, fs.ErrExist) {
				t.Skip("the volume folds letter case, so a second name for the snapshot cannot exist")
			} else {
				require.NoError(t, err)
			}
			rm := &fakeRemover{}

			_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, []string{idOldest + ".sqlite"}, rm.calls)
			assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
		})
	}
}
