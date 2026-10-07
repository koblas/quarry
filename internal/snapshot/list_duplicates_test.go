package snapshot_test

import (
	"io/fs"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func Test_duplicate_warning_names_the_manifest_variants(t *testing.T) {
	t.Parallel()

	listing, dir := duplicatesOf(t, []string{idOldest}, regularVariant(idOldest+".JSON", idOldest+".json"))

	want := " holds both " + idOldest + ".json and " + idOldest + ".JSON" + usesOnly + idOldest + ".json" + renameOther
	assert.Equal(t, []string{"~/snapshots" + want}, listing.Duplicates)
	assert.Equal(t, []string{dir + want}, listing.DuplicatesAbsolute)
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
