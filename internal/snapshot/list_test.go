package snapshot_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_list_orders_ids_with_a_numeric_suffix(t *testing.T) {
	t.Parallel()
	const huge = "_123456789012345678901234567890"
	cases := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "by value with the bare id oldest", ids: []string{idNewest, idNewest + "_2", idNewest + "_10", idMiddle},
			want: []string{idNewest + "_10", idNewest + "_2", idNewest, idMiddle},
		},
		{
			name: "a suffix too long for an integer by value", ids: []string{idNewest + "_9", idNewest + huge},
			want: []string{idNewest + huge, idNewest + "_9"},
		},
		{
			name: "a suffix with leading zeros by its value", ids: []string{idNewest + "_010", idNewest + "_11"},
			want: []string{idNewest + "_11", idNewest + "_010"},
		},
		{name: "one leading zero", ids: []string{idNewest + "_02", idNewest + "_2"}, want: []string{idNewest + "_2", idNewest + "_02"}},
		{
			name: "all-zero suffixes fall before the bare id", ids: []string{idNewest, idNewest + "_0", idNewest + "_000", idNewest + "_00"},
			want: []string{idNewest + "_000", idNewest + "_00", idNewest + "_0", idNewest},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := snapshotsFolder(t, home)
			for _, id := range c.ids {
				writeSnapshot(t, dir, id, 1000)
			}

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, entryIDs(listing))
		})
	}
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
	assert.Empty(t, listing.NoSnapshotsAbsolute)
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
			assert.Equal(t, "no snapshots in "+filepath.Join(home, "snapshots")+" yet; run quarry sync to take one", listing.NoSnapshotsAbsolute)
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
			restrictMode(t, dir, 0o000)
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
			restrictMode(t, dir, 0o400)
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

// threeSnapshots writes the oldest, middle and newest snapshots and returns the folder.
func threeSnapshots(t *testing.T, home string) string {
	t.Helper()
	dir := snapshotsFolder(t, home)
	for _, id := range []string{idOldest, idMiddle, idNewest} {
		writeSnapshot(t, dir, id, 1000)
	}
	return dir
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

func Test_list_returns_a_store_read_that_is_not_an_open_fault(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	_, err := newListServer(home, &fakeStoreProbe{builtFromErr: errStoreRead}).List(t.Context())

	require.ErrorIs(t, err, errStoreRead)
}

const (
	usesOnly    = "; quarry lists, prunes and uses only "
	renameOther = "; rename or remove the other"
)

// duplicatesOf lists a folder holding id's snapshot and manifest, plus variants, and returns the Listing.
func duplicatesOf(t *testing.T, ids []string, variants ...variant) (snapshot.Listing, string) {
	t.Helper()
	home := t.TempDir()
	dir := prunable(t, home, ids...)
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t, variants...)))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	return listing, dir
}

func Test_duplicate_warning_names_every_variant(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		variants []variant
		want     string
	}{
		{
			name:     "two names put the winner first",
			variants: []variant{regularVariant(idOldest+".SQLITE", idOldest+".sqlite")},
			want:     "holds both " + idOldest + ".sqlite and " + idOldest + ".SQLITE" + usesOnly + idOldest + ".sqlite" + renameOther,
		},
		{
			name:     "three names are in byte order",
			variants: []variant{regularVariant(idOldest+".Sqlite", idOldest+".sqlite"), regularVariant(idOldest+".SQLITE", idOldest+".sqlite")},
			want: "holds " + idOldest + ".SQLITE, " + idOldest + ".Sqlite and " + idOldest + ".sqlite" +
				usesOnly + idOldest + ".sqlite; rename or remove the others",
		},
		{
			name: "four names are in byte order",
			variants: []variant{
				regularVariant(idOldest+".Sqlite", idOldest+".sqlite"), regularVariant(idOldest+".SQLite", idOldest+".sqlite"),
				regularVariant(idOldest+".SQLITE", idOldest+".sqlite"),
			},
			want: "holds " + idOldest + ".SQLITE, " + idOldest + ".SQLite, " + idOldest + ".Sqlite and " + idOldest + ".sqlite" +
				usesOnly + idOldest + ".sqlite; rename or remove the others",
		},
		{
			name:     "a manifest variant names the manifest pair",
			variants: []variant{regularVariant(idOldest+".JSON", idOldest+".json")},
			want:     "holds both " + idOldest + ".json and " + idOldest + ".JSON" + usesOnly + idOldest + ".json" + renameOther,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			listing, dir := duplicatesOf(t, []string{idOldest}, c.variants...)

			assert.Equal(t, []string{"~/snapshots " + c.want}, listing.Duplicates)
			assert.Equal(t, []string{dir + " " + c.want}, listing.DuplicatesAbsolute)
		})
	}
}

func Test_duplicate_warning_gives_a_snapshot_with_both_kinds_of_variant_its_snapshot_line_first(t *testing.T) {
	t.Parallel()

	listing, _ := duplicatesOf(t, []string{idOldest},
		regularVariant(idOldest+".JSON", idOldest+".json"), regularVariant(idOldest+".SQLITE", idOldest+".sqlite"))

	assert.Equal(t, []string{
		"~/snapshots holds both " + idOldest + ".sqlite and " + idOldest + ".SQLITE" + usesOnly + idOldest + ".sqlite" + renameOther,
		"~/snapshots holds both " + idOldest + ".json and " + idOldest + ".JSON" + usesOnly + idOldest + ".json" + renameOther,
	}, listing.Duplicates)
}

func Test_duplicate_warning_lists_the_newest_affected_id_first(t *testing.T) {
	t.Parallel()

	listing, _ := duplicatesOf(t, []string{idOldest, idMiddle, idNewest},
		regularVariant(idOldest+".SQLITE", idOldest+".sqlite"), regularVariant(idNewest+".SQLITE", idNewest+".sqlite"))

	assert.Equal(t, []string{
		"~/snapshots holds both " + idNewest + ".sqlite and " + idNewest + ".SQLITE" + usesOnly + idNewest + ".sqlite" + renameOther,
		"~/snapshots holds both " + idOldest + ".sqlite and " + idOldest + ".SQLITE" + usesOnly + idOldest + ".sqlite" + renameOther,
	}, listing.Duplicates)
}

func Test_duplicate_warning_names_the_other_name_when_the_winner_sorts_first(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idOldest)
	upperCased(t, dir, idOldest)
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t, regularVariant(idOldest+".Sqlite", idOldest+".SQLITE"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{
		"~/snapshots holds both " + idOldest + ".SQLITE and " + idOldest + ".Sqlite" + usesOnly + idOldest + ".SQLITE" + renameOther,
	}, listing.Duplicates)
}

func Test_duplicate_warning_names_an_upper_case_winner_that_does_not_sort_last_of_three(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idOldest)
	upperCased(t, dir, idOldest)
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t,
		regularVariant(idOldest+".Sqlite", idOldest+".SQLITE"), regularVariant(idOldest+".SQLite", idOldest+".SQLITE"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{
		"~/snapshots holds " + idOldest + ".SQLITE, " + idOldest + ".SQLite and " + idOldest + ".Sqlite" +
			usesOnly + idOldest + ".SQLITE; rename or remove the others",
	}, listing.Duplicates)
}

func Test_list_gives_no_duplicate_warning_for_a_non_regular_variant(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		variant variant
	}{
		{name: "a directory named as the snapshot", variant: variant{name: idOldest + ".SQLITE", like: idOldest + ".sqlite", mode: fs.ModeDir}},
		{name: "a symlink named as the snapshot", variant: variant{name: idOldest + ".SQLITE", like: idOldest + ".sqlite", mode: fs.ModeSymlink}},
		{name: "a directory named as the manifest", variant: variant{name: idOldest + ".JSON", like: idOldest + ".json", mode: fs.ModeDir}},
		{name: "a symlink named as the manifest", variant: variant{name: idOldest + ".JSON", like: idOldest + ".json", mode: fs.ModeSymlink}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			listing, _ := duplicatesOf(t, []string{idOldest}, c.variant)

			assert.Equal(t, []string{idOldest}, entryIDs(listing))
			assert.Empty(t, listing.Duplicates)
			assert.Empty(t, listing.DuplicatesAbsolute)
		})
	}
}

func Test_list_gives_no_duplicate_warning_naming_a_manifest_that_is_not_a_regular_file(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idOldest)
	renamedExtension(t, dir, idOldest, "json", "Json")
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t,
		variant{name: idOldest + ".JSON", like: idOldest + ".Json", mode: fs.ModeDir},
		regularVariant(idOldest+".jSON", idOldest+".Json"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, entryIDs(listing))
	assert.Empty(t, listing.Duplicates)
}

func Test_list_gives_no_duplicate_warning_for_an_id_it_does_not_list(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T14:30:05Z"))
	srv := newListServer(home, nil, snapshot.WithReadDir(readDirWith(t,
		variant{name: idOldest + ".sqlite", like: idOldest + ".json", mode: fs.ModeDir},
		regularVariant(idOldest+".JSON", idOldest+".json"))))

	listing, err := srv.List(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []string{idNewest}, entryIDs(listing))
	assert.Empty(t, listing.Duplicates)
}

// cannotTellWarning opens every warning that the store's snapshot cannot be told.
const cannotTellWarning = "cannot tell which snapshot the store was built from: "

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

// markedIDs returns the IDs of the entries l marks as the store's, newest first.
func markedIDs(l snapshot.Listing) []string {
	ids := []string{}
	for _, entry := range l.Entries {
		if entry.Store {
			ids = append(ids, entry.ID)
		}
	}
	return ids
}

func Test_list_marks_exactly_the_snapshot_the_recorded_path_resolves_to(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		// recorded arranges links around the three snapshots in dir and returns the path the store recorded.
		recorded func(t *testing.T, home, dir string) string
		want     []string
	}{
		{
			name:     "a snapshot in the folder",
			recorded: func(_ *testing.T, _, dir string) string { return snapshotFile(dir, idMiddle) },
			want:     []string{idMiddle},
		},
		{
			name: "a snapshot in the folder named in another letter case, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return snapshotFile(dir, strings.ToLower(idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink outside the folder to a snapshot named with an upper-case extension, with a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				skipOnCaseSensitiveVolume(t, home)
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, filepath.Join(dir, idMiddle+".SQLITE"), filepath.Join(home, "latest.sqlite"))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink outside the folder whose target has a newer hard link",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "latest.sqlite"))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink in the folder under an older id whose target has a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkOlder))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink outside the folder named as a newer hard link of its target",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return symlink(t, snapshotFile(dir, idMiddle), snapshotFile(home, idLinkNewer))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink named as a snapshot to a hard link of it outside the folder under another name",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				outside := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "elsewhere.sqlite"))
				return symlink(t, outside, snapshotFile(filepath.Join(home, "aliases"), idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a symlink named as a snapshot with an upper-case extension to a hard link of it outside the folder",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				outside := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "elsewhere.sqlite"))
				return symlink(t, outside, filepath.Join(home, "aliases", idMiddle+".SQLITE"))
			},
			want: []string{idMiddle},
		},
		{
			name: "a snapshot in the folder with a newer hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkNewer))
				return snapshotFile(dir, idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a snapshot in the folder with an older hard link",
			recorded: func(t *testing.T, _, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkOlder))
				return snapshotFile(dir, idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a hard link outside the folder under no snapshot's id marks the newest snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
				return hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))
			},
			want: []string{idLinkBetween},
		},
		{
			name: "a hard link outside the folder under another snapshot's id marks the snapshot it links",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				return hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(home, idNewest))
			},
			want: []string{idMiddle},
		},
		{
			name: "a copy outside the folder under a snapshot's id",
			recorded: func(t *testing.T, home, _ string) string {
				t.Helper()
				return writeSnapshot(t, home, idMiddle, 1000)
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's in another letter case",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), strings.ToLower(idMiddle))
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's with an upper-case extension",
			recorded: func(_ *testing.T, home, _ string) string {
				return filepath.Join(home, "moved-away", idMiddle+".SQLITE")
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is a snapshot's under another extension",
			recorded: func(_ *testing.T, home, _ string) string {
				return filepath.Join(home, "moved-away", idMiddle+".db")
			},
			want: []string{idMiddle},
		},
		{
			name: "a path that is gone whose id is no snapshot's marks nothing",
			recorded: func(_ *testing.T, home, _ string) string {
				return snapshotFile(filepath.Join(home, "moved-away"), idOutside)
			},
			want: []string{},
		},
		{
			name: "a snapshot in the folder reached through a symlink to the folder",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				alias := symlink(t, dir, filepath.Join(home, "alias"))
				return snapshotFile(alias, idMiddle)
			},
			want: []string{idMiddle},
		},
		{
			name: "a copy outside the folder under no snapshot's id marks nothing",
			recorded: func(t *testing.T, home, _ string) string {
				t.Helper()
				return writeSnapshot(t, home, idOutside, 1000)
			},
			want: []string{},
		},
		{
			name: "a symlink named as one snapshot to another snapshot's file marks the file it resolves to",
			recorded: func(t *testing.T, home, dir string) string {
				t.Helper()
				return symlink(t, snapshotFile(dir, idNewest), snapshotFile(home, idMiddle))
			},
			want: []string{idNewest},
		},
		{
			name: "a path in the folder that is gone, under no snapshot's id, marks nothing",
			recorded: func(_ *testing.T, _, dir string) string {
				return snapshotFile(dir, idOutside)
			},
			want: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := threeSnapshots(t, home)
			recorded := c.recorded(t, home, dir)

			listing, err := newListServer(home, &fakeStoreProbe{builtFrom: recorded}).List(t.Context())

			require.NoError(t, err)
			assert.Equal(t, c.want, markedIDs(listing))
			assert.Equal(t, recorded, listing.StorePath)
			assert.Empty(t, listing.StoreWarning)
			assert.Empty(t, listing.StoreUnreadable)
		})
	}
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

func Test_list_keeps_the_on_disk_path_of_an_upper_case_manifest_it_cannot_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		write func(t *testing.T, path string)
	}{
		{name: "it is not JSON", write: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))
		}},
		{name: "it is a directory", write: func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.Mkdir(path, 0o700))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := snapshotsFolder(t, home)
			writeSnapshot(t, dir, idOldest, 1500)
			c.write(t, filepath.Join(dir, idOldest+".JSON"))
			require.ElementsMatch(t, []string{"20260927T143005Z.sqlite", "20260927T143005Z.JSON"}, dirNames(t, dir))

			listing, err := newListServer(home, nil).List(t.Context())

			require.NoError(t, err)
			require.Len(t, listing.Entries, 1)
			assert.Equal(t, filepath.Join(dir, "20260927T143005Z.JSON"), listing.Entries[0].ManifestPath)
			assert.Nil(t, listing.Entries[0].Manifest)
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
