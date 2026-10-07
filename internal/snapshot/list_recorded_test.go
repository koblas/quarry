package snapshot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannotTellWarning opens every warning that the store's snapshot cannot be told.
const cannotTellWarning = "cannot tell which snapshot the store was built from: "

// unreadableRecorded is a recorded snapshot path that fails to stat for reason, named idMiddle so
// the entry of that ID in the folder is the one a name match would mark.
type unreadableRecorded struct {
	name   string
	reason string
	// arrange builds the fault under home and returns the recorded path.
	arrange func(t *testing.T, home string) string
}

// unreadableRecordedPaths are the ways the recorded path fails to stat for a reason other than not existing.
func unreadableRecordedPaths() []unreadableRecorded {
	return []unreadableRecorded{
		{
			name: "permission denied", reason: "permission denied",
			arrange: func(t *testing.T, home string) string {
				t.Helper()
				skipUnderRoot(t)
				backup := filepath.Join(home, "Backup")
				require.NoError(t, os.Mkdir(backup, 0o700))
				require.NoError(t, os.Chmod(backup, 0o000))
				t.Cleanup(func() { assert.NoError(t, os.Chmod(backup, 0o700)) })
				return filepath.Join(backup, idMiddle+".sqlite")
			},
		},
		{
			name: "a symlink loop", reason: "too many levels of symbolic links",
			arrange: func(t *testing.T, home string) string {
				t.Helper()
				loop := filepath.Join(home, idMiddle+".sqlite")
				return symlink(t, loop, loop)
			},
		},
		{
			name: "a parent that is a file", reason: "not a directory",
			arrange: func(t *testing.T, home string) string {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(home, "Backup"), nil, 0o600))
				return filepath.Join(home, "Backup", idMiddle+".sqlite")
			},
		},
		{
			name: "a name too long", reason: "file name too long",
			arrange: func(_ *testing.T, home string) string {
				return filepath.Join(home, strings.Repeat("a", 256), idMiddle+".sqlite")
			},
		},
	}
}

// homeRelative shows path under home as ~/...
func homeRelative(home, path string) string { return "~" + strings.TrimPrefix(path, home) }

func Test_list_marks_nothing_and_warns_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	t.Parallel()
	for _, c := range unreadableRecordedPaths() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			threeSnapshots(t, home)
			recorded := c.arrange(t, home)

			listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, []string{}, markedIDs(listing))
			assert.Equal(t, recorded, listing.StorePath)
			assert.Equal(t, "cannot read "+homeRelative(home, recorded)+": "+c.reason, listing.StoreUnreadable)
			assert.Equal(t, cannotTellWarning+"cannot read "+homeRelative(home, recorded)+": "+c.reason, listing.StoreWarning)
			assert.Equal(t, cannotTellWarning+"cannot read "+recorded+": "+c.reason, listing.StoreWarningAbsolute)
		})
	}
}

func Test_list_marks_the_recorded_id_and_warns_of_nothing_when_the_recorded_path_stats_or_is_gone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		recorded func(home, dir string) string
	}{
		{"a path that stats", func(_, dir string) string { return snapshotFile(dir, idMiddle) }},
		{"a path that is gone", func(home, _ string) string { return snapshotFile(filepath.Join(home, "moved-away"), idMiddle) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := threeSnapshots(t, home)

			listing, err := newListServer(home, &fakeStoreProbe{builtFrom: c.recorded(home, dir)}).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, []string{idMiddle}, markedIDs(listing))
			assert.Empty(t, listing.StoreUnreadable)
			assert.Empty(t, listing.StoreWarning)
			assert.Empty(t, listing.StoreWarningAbsolute)
		})
	}
}

func Test_list_names_the_same_warning_in_both_forms_when_the_store_cannot_be_read(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	fault := &store.OpenError{Fault: store.OpenFaultPermission, Path: filepath.Join(home, "quarry.duckdb")}

	listing, err := newListServer(home, &fakeStoreProbe{builtFromErr: fault}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, cannotTellWarning+"permission denied", listing.StoreWarning)
	assert.Equal(t, cannotTellWarning+"permission denied", listing.StoreWarningAbsolute)
}
