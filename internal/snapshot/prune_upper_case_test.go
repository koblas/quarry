package snapshot_test

import (
	"os"
	"path/filepath"
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
