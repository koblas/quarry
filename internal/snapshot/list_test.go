package snapshot_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idOldest = "20260927T143005Z"
	idMiddle = "20260929T090011Z"
	idNewest = "20260930T141502Z"
	// idOutside names no snapshot in the folders the list tests build.
	idOutside = "20260801T120000Z"
)

// snapshotsFolder creates and returns the snapshots folder under home.
func snapshotsFolder(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, "snapshots")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return dir
}

// writeSnapshot writes id's .sqlite as a sparse file of size bytes.
func writeSnapshot(t *testing.T, dir, id string, size int64) string {
	t.Helper()
	path := filepath.Join(dir, id+".sqlite")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Truncate(path, size))
	return path
}

// writeManifest writes id's manifest through Manifest.Encode.
func writeManifest(t *testing.T, dir, id string, manifest snapshot.Manifest) {
	t.Helper()
	data, err := manifest.Encode()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600))
}

// manifestTaken is a manifest whose taken_at reads takenAt, for a Home.quicken source.
func manifestTaken(takenAt string) snapshot.Manifest {
	return snapshot.Manifest{
		Snapshot: snapshot.Info{Source: "/Users/x/Documents/Home.quicken", TakenAt: takenAt},
		Schema:   snapshot.SchemaInfo{Verified: true},
	}
}

// newListServer builds a Server over home's snapshots folder; probe is the store, nil for none.
func newListServer(home string, probe snapshot.StoreProbe) *snapshot.Server {
	opts := []snapshot.Option{snapshot.WithSnapshotDir(filepath.Join(home, "snapshots")), snapshot.WithHome(home)}
	if probe != nil {
		opts = append(opts, snapshot.WithStoreProbe(probe))
	}
	return snapshot.NewServer(opts...)
}

func entryIDs(l snapshot.Listing) []string {
	ids := make([]string, len(l.Entries))
	for i, e := range l.Entries {
		ids[i] = e.ID
	}
	return ids
}

func storeFlags(l snapshot.Listing) []bool {
	flags := make([]bool, len(l.Entries))
	for i, e := range l.Entries {
		flags[i] = e.Store
	}
	return flags
}

func Test_list_orders_by_id_even_when_taken_at_disagrees(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idOldest, 1000)
	writeManifest(t, dir, idOldest, manifestTaken("2026-10-05T10:00:00Z"))
	writeSnapshot(t, dir, idNewest, 1000)
	writeManifest(t, dir, idNewest, manifestTaken("2026-01-05T10:00:00Z"))

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest, idOldest}, entryIDs(listing))
	assert.Equal(t, time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), listing.Entries[0].TakenAt)
}

func Test_list_orders_a_numeric_suffix_by_value_with_the_bare_id_oldest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	for _, id := range []string{idNewest, idNewest + "_2", idNewest + "_10", idMiddle} {
		writeSnapshot(t, dir, id, 1000)
	}

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest + "_10", idNewest + "_2", idNewest, idMiddle}, entryIDs(listing))
}

func Test_list_orders_a_suffix_too_long_for_an_integer_by_value(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	const huge = "_123456789012345678901234567890"
	for _, id := range []string{idNewest + "_9", idNewest + huge} {
		writeSnapshot(t, dir, id, 1000)
	}

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest + huge, idNewest + "_9"}, entryIDs(listing))
}

func Test_list_totals_every_snapshot_including_one_with_no_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idOldest, 1000)
	writeSnapshot(t, dir, idMiddle, 2000)
	writeManifest(t, dir, idMiddle, manifestTaken("2026-09-29T09:00:11Z"))
	writeSnapshot(t, dir, idNewest, 4000)
	writeManifest(t, dir, idNewest, manifestTaken("2026-09-30T14:15:02Z"))

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.EqualValues(t, 7000, listing.TotalBytes)
	assert.Equal(t, []int64{4000, 2000, 1000}, []int64{listing.Entries[0].Bytes, listing.Entries[1].Bytes, listing.Entries[2].Bytes})
	assert.Empty(t, listing.NoSnapshots)
	assert.Equal(t, dir, listing.Dir)
}

func Test_list_reports_the_size_on_disk_not_the_size_the_manifest_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idMiddle, 2000)
	manifest := manifestTaken("2026-09-29T09:00:11Z")
	manifest.Snapshot.Bytes = 9999
	writeManifest(t, dir, idMiddle, manifest)

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.EqualValues(t, 2000, listing.Entries[0].Bytes)
	assert.EqualValues(t, 2000, listing.TotalBytes)
}

func Test_list_ignores_everything_that_is_not_a_snapshot_file(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idNewest, 1000)
	writeManifest(t, dir, idNewest, manifestTaken("2026-09-30T14:15:02Z"))
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T14:30:05Z"))
	writeSnapshot(t, dir, "."+idMiddle, 1000)
	require.NoError(t, os.Rename(filepath.Join(dir, "."+idMiddle+".sqlite"), filepath.Join(dir, "."+idMiddle+".sqlite.partial")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("stray"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "20260101T000000Z.sqlite"), 0o700))
	target := writeSnapshot(t, home, "elsewhere", 1000)
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "20260102T000000Z.sqlite")))

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest}, entryIDs(listing))
	assert.EqualValues(t, 1000, listing.TotalBytes)
}

func Test_list_gives_a_row_with_no_manifest_when_the_manifest_cannot_be_used(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		write func(t *testing.T, dir string)
	}{
		{name: "there is none", write: func(*testing.T, string) {}},
		{name: "it is not JSON", write: func(t *testing.T, dir string) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(dir, idMiddle+".json"), []byte("not json"), 0o600))
		}},
		{name: "it is a directory", write: func(t *testing.T, dir string) {
			t.Helper()
			require.NoError(t, os.Mkdir(filepath.Join(dir, idMiddle+".json"), 0o700))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := snapshotsFolder(t, home)
			writeSnapshot(t, dir, idMiddle, 2000)
			c.write(t, dir)

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			require.Len(t, listing.Entries, 1)
			assert.Nil(t, listing.Entries[0].Manifest)
			assert.True(t, listing.Entries[0].TakenAt.IsZero())
		})
	}
}

func Test_list_keeps_the_manifest_when_taken_at_does_not_parse(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idMiddle, 2000)
	writeManifest(t, dir, idMiddle, manifestTaken("last Tuesday"))

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	require.NotNil(t, listing.Entries[0].Manifest)
	assert.True(t, listing.Entries[0].Manifest.Schema.Verified)
	assert.True(t, listing.Entries[0].TakenAt.IsZero())
}

func Test_list_keeps_the_manifest_when_its_source_is_empty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	writeSnapshot(t, dir, idMiddle, 2000)
	manifest := manifestTaken("2026-09-29T09:00:11Z")
	manifest.Snapshot.Source = ""
	writeManifest(t, dir, idMiddle, manifest)

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	require.NotNil(t, listing.Entries[0].Manifest)
	assert.Empty(t, listing.Entries[0].Manifest.Snapshot.Source)
	assert.False(t, listing.Entries[0].TakenAt.IsZero())
}

func Test_list_says_how_to_take_a_snapshot_when_the_folder_holds_none(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		write func(t *testing.T, dir string)
	}{
		{name: "the folder does not exist", write: func(t *testing.T, dir string) {
			t.Helper()
			require.NoError(t, os.RemoveAll(dir))
		}},
		{name: "the folder is empty", write: func(*testing.T, string) {}},
		{name: "the folder holds only files that are not snapshots", write: func(t *testing.T, dir string) {
			t.Helper()
			writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T14:30:05Z"))
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".20260927T143005Z.sqlite.partial"), nil, 0o600))
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			c.write(t, snapshotsFolder(t, home))

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			assert.Empty(t, listing.Entries)
			assert.Zero(t, listing.TotalBytes)
			assert.Equal(t, "no snapshots in ~/snapshots yet; run quarry sync to take one", listing.NoSnapshots)
		})
	}
}

func Test_list_does_not_create_a_missing_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	_, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(home, "snapshots"))
}

func Test_list_refuses_a_folder_or_snapshot_it_cannot_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(t *testing.T, home string)
		want    string
	}{
		{name: "the folder cannot be read", arrange: func(t *testing.T, home string) {
			t.Helper()
			skipUnderRoot(t)
			dir := snapshotsFolder(t, home)
			require.NoError(t, os.Chmod(dir, 0o000))
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		}, want: "cannot read ~/snapshots: permission denied"},
		{name: "a file stands where the folder should be", arrange: func(t *testing.T, home string) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(home, "snapshots"), []byte("file"), 0o600))
		}, want: "cannot read ~/snapshots: not a directory"},
		{name: "a snapshot in a folder that cannot be searched has no size to read", arrange: func(t *testing.T, home string) {
			t.Helper()
			skipUnderRoot(t)
			dir := snapshotsFolder(t, home)
			writeSnapshot(t, dir, idMiddle, 2000)
			require.NoError(t, os.Chmod(dir, 0o400))
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		}, want: "cannot read ~/snapshots: permission denied"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			c.arrange(t, home)

			listing, err := newListServer(home, nil).List(t.Context())

			require.EqualError(t, err, c.want)
			assert.Empty(t, listing.Entries)
		})
	}
}

// skipUnderRoot skips t under root, whom file modes do not stop.
func skipUnderRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}

// threeSnapshots writes the oldest, middle and newest snapshots and returns the folder.
func threeSnapshots(t *testing.T, home string) string {
	t.Helper()
	dir := snapshotsFolder(t, home)
	for _, id := range []string{idOldest, idMiddle, idNewest} {
		writeSnapshot(t, dir, id, 1000)
	}
	return dir
}

func Test_list_marks_the_snapshot_the_store_was_built_from(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	recorded := filepath.Join(dir, idMiddle+".sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
	assert.Equal(t, recorded, listing.StorePath)
	assert.Empty(t, listing.StoreWarning)
	assert.Empty(t, listing.StoreUnreadable)
}

func Test_list_marks_nothing_and_says_nothing_without_a_store_probe(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)

	listing, err := newListServer(home, nil).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, false}, storeFlags(listing))
	assert.Empty(t, listing.StorePath)
	assert.Empty(t, listing.StoreWarning)
}

func Test_list_marks_nothing_and_says_nothing_when_there_is_no_store(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	missing := &store.OpenError{Fault: store.OpenFaultMissing, Path: filepath.Join(home, "quarry.duckdb")}

	listing, err := newListServer(home, &fakeStoreProbe{builtFromErr: missing}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, false}, storeFlags(listing))
	assert.Empty(t, listing.StorePath)
	assert.Empty(t, listing.StoreWarning)
}

func Test_list_marks_the_stores_snapshot_recorded_through_a_symlink(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	alias := filepath.Join(home, "alias")
	require.NoError(t, os.Symlink(dir, alias))
	recorded := filepath.Join(alias, idMiddle+".sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
	assert.Equal(t, recorded, listing.StorePath)
}

func Test_list_marks_nothing_for_a_store_built_from_a_snapshot_outside_the_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	outside := writeSnapshot(t, home, idOutside, 1000)

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: outside}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, false}, storeFlags(listing))
	assert.Equal(t, outside, listing.StorePath)
	assert.Empty(t, listing.StoreWarning)
}

func Test_list_marks_the_same_id_snapshot_when_the_store_was_built_from_a_copy_outside_the_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	outside := writeSnapshot(t, home, idMiddle, 1000)

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: outside}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
	assert.Equal(t, outside, listing.StorePath)
}

func Test_list_marks_the_same_id_snapshot_when_the_recorded_path_no_longer_resolves(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	moved := filepath.Join(home, "moved-away", idMiddle+".sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: moved}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true, false}, storeFlags(listing))
	assert.Equal(t, moved, listing.StorePath)
}

func Test_list_prefers_the_path_match_over_the_id_match(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	alias := filepath.Join(home, idMiddle+".sqlite")
	require.NoError(t, os.Symlink(filepath.Join(dir, idNewest+".sqlite"), alias))

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: alias}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, false}, storeFlags(listing))
}

func Test_list_marks_nothing_but_keeps_the_recorded_path_when_the_snapshot_was_deleted_by_hand(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	gone := filepath.Join(dir, "20260801T120000Z.sqlite")

	listing, err := newListServer(home, &fakeStoreProbe{builtFrom: gone}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{false, false, false}, storeFlags(listing))
	assert.Equal(t, gone, listing.StorePath)
	assert.Empty(t, listing.StoreWarning)
}

func Test_list_marks_the_snapshot_a_store_of_another_format_names(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := threeSnapshots(t, home)
	recorded := filepath.Join(dir, idNewest+".sqlite")
	otherFormat := &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: filepath.Join(home, "quarry.duckdb"), SnapshotPath: recorded}

	listing, err := newListServer(home, &fakeStoreProbe{builtFromErr: otherFormat}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []bool{true, false, false}, storeFlags(listing))
	assert.Empty(t, listing.StoreWarning)
}

func Test_list_warns_that_it_cannot_tell_which_snapshot_built_a_store_it_cannot_read(t *testing.T) {
	t.Parallel()
	const storeFile = "/Users/x/Library/Application Support/quarry/quarry.duckdb"
	cases := []struct {
		name  string
		fault *store.OpenError
		want  string
	}{
		{
			name: "not a DuckDB file", fault: &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storeFile},
			want: "the file is not a DuckDB database",
		},
		{
			name: "permission denied", fault: &store.OpenError{Fault: store.OpenFaultPermission, Path: storeFile},
			want: "permission denied",
		},
		{
			name: "locked by another program", fault: &store.OpenError{Fault: store.OpenFaultLocked, Path: storeFile},
			want: "another program has it open for writing",
		},
		{
			name: "another fault, naming the store by its ~ form", fault: &store.OpenError{
				Fault: store.OpenFaultOther, Path: storeFile, Reason: `Cannot open file "` + storeFile + `": Input/output error`,
			},
			want: `Cannot open file "~/Library/Application Support/quarry/quarry.duckdb": Input/output error`,
		},
		{
			name: "no import history", fault: &store.OpenError{Fault: store.OpenFaultOther, Path: storeFile, Reason: "the store has no import history"},
			want: "the store has no import history",
		},
		{
			name: "another format naming no snapshot", fault: &store.OpenError{Fault: store.OpenFaultOtherFormat, Path: storeFile},
			want: "the store was built by another version of quarry",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := "/Users/x"
			dir := t.TempDir()
			srv := snapshot.NewServer(
				snapshot.WithSnapshotDir(dir), snapshot.WithHome(home), snapshot.WithStoreProbe(&fakeStoreProbe{builtFromErr: c.fault}))
			writeSnapshot(t, dir, idMiddle, 1000)

			listing, err := srv.List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, []bool{false}, storeFlags(listing))
			assert.Empty(t, listing.StorePath)
			assert.Equal(t, c.want, listing.StoreUnreadable)
			assert.Equal(t, "cannot tell which snapshot the store was built from: "+c.want, listing.StoreWarning)
		})
	}
}

func Test_list_keeps_the_no_snapshots_note_and_the_store_warning_apart(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	notDuckDB := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry.duckdb")}

	listing, err := newListServer(home, &fakeStoreProbe{builtFromErr: notDuckDB}).List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "no snapshots in ~/snapshots yet; run quarry sync to take one", listing.NoSnapshots)
	assert.Equal(t, "cannot tell which snapshot the store was built from: the file is not a DuckDB database", listing.StoreWarning)
}

func Test_list_is_interrupted_rather_than_warning_when_the_context_ended_before_the_store_answered(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	threeSnapshots(t, home)
	fault := &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: filepath.Join(home, "quarry.duckdb")}
	srv := newListServer(home, &fakeStoreProbe{builtFromErr: fault})
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	listing, err := srv.List(cancelled)

	require.EqualError(t, err, "snapshots interrupted")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, listing.Entries)
}

var errStoreRead = errors.New("store read failed")

func Test_list_returns_a_store_read_that_is_not_an_open_fault(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	_, err := newListServer(home, &fakeStoreProbe{builtFromErr: errStoreRead}).List(t.Context())

	require.ErrorIs(t, err, errStoreRead)
}
