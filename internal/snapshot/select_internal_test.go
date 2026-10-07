// White-box: selectFolder is unexported name-selection logic with many entry-type
// and letter-case combinations; a fake fs.DirEntry reaches each without a file per row.
package snapshot

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	selID    = "20260927T143005Z"
	selOther = "20260930T141502Z"
)

type fakeDirEntry struct {
	name string
	mode fs.FileMode
}

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return e.mode.IsDir() }
func (e fakeDirEntry) Type() fs.FileMode          { return e.mode.Type() }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil } //nolint:nilnil // never read by selectFolder

// entries builds regular files named names, in the order given.
func entries(names ...string) []fs.DirEntry {
	out := make([]fs.DirEntry, len(names))
	for i, name := range names {
		out[i] = fakeDirEntry{name: name}
	}
	return out
}

// withType replaces the entry named name by one of mode.
func withType(list []fs.DirEntry, name string, mode fs.FileMode) []fs.DirEntry {
	for i, e := range list {
		if e.Name() == name {
			list[i] = fakeDirEntry{name: name, mode: mode}
		}
	}
	return list
}

type selected struct{ snapshot, manifest string }

func chosen(sel folderSelection) []selected {
	out := make([]selected, len(sel.snapshots))
	for i, s := range sel.snapshots {
		out[i] = selected{snapshot: s.entry.Name(), manifest: s.manifest}
	}
	return out
}

func Test_select_folder_picks_one_snapshot_and_one_manifest_per_id(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []fs.DirEntry
		want    []selected
		strays  []string
	}{
		{
			name: "a lone upper-case snapshot", entries: entries(selID + ".SQLITE"),
			want: []selected{{selID + ".SQLITE", ""}},
		},
		{
			name: "exact lower case wins over upper case", entries: entries(selID+".SQLITE", selID+".sqlite"),
			want: []selected{{selID + ".sqlite", ""}}, strays: []string{selID + ".SQLITE"},
		},
		{
			name: "without a lower-case name the byte-order first wins", entries: entries(selID+".SQLITE", selID+".Sqlite"),
			want: []selected{{selID + ".SQLITE", ""}}, strays: []string{selID + ".Sqlite"},
		},
		{
			name: "byte order is read from the name, not the entry order", entries: entries(selID+".Sqlite", selID+".SQLITE"),
			want: []selected{{selID + ".SQLITE", ""}}, strays: []string{selID + ".Sqlite"},
		},
		{
			name: "three variants leave two strays", entries: entries(selID+".SQLITE", selID+".Sqlite", selID+".sqlite"),
			want: []selected{{selID + ".sqlite", ""}}, strays: []string{selID + ".SQLITE", selID + ".Sqlite"},
		},
		{
			name:    "a directory named as a snapshot loses to a regular variant and is no stray",
			entries: withType(entries(selID+".SQLITE", selID+".Sqlite"), selID+".SQLITE", fs.ModeDir),
			want:    []selected{{selID + ".Sqlite", ""}},
		},
		{
			name:    "a symlink named as a snapshot loses to a regular variant and is no stray",
			entries: withType(entries(selID+".SQLITE", selID+".Sqlite"), selID+".SQLITE", fs.ModeSymlink),
			want:    []selected{{selID + ".Sqlite", ""}},
		},
		{
			name:    "a non-regular snapshot alone is not a snapshot",
			entries: withType(entries(selID+".SQLITE"), selID+".SQLITE", fs.ModeDir), want: []selected{},
		},
		{
			name: "a numeric suffix is part of the ID", entries: entries(selID+"_2.SQLITE", selID+".sqlite"),
			want: []selected{{selID + "_2.SQLITE", ""}, {selID + ".sqlite", ""}},
		},
		{
			name:    "a manifest is chosen by the same rule: exact lower case first",
			entries: entries(selID+".JSON", selID+".Json", selID+".json", selID+".sqlite"),
			want:    []selected{{selID + ".sqlite", selID + ".json"}},
		},
		{
			name:    "without a lower-case manifest the byte-order first wins",
			entries: entries(selID+".Json", selID+".JSON", selID+".sqlite"),
			want:    []selected{{selID + ".sqlite", selID + ".JSON"}},
		},
		{
			name:    "a symlinked manifest competes",
			entries: withType(entries(selID+".json", selID+".JSON", selID+".SQLITE"), selID+".json", fs.ModeSymlink),
			want:    []selected{{selID + ".SQLITE", selID + ".json"}},
		},
		{
			name:    "a directory named as a manifest competes",
			entries: withType(entries(selID+".JSON", selID+".sqlite"), selID+".JSON", fs.ModeDir),
			want:    []selected{{selID + ".sqlite", selID + ".JSON"}},
		},
		{
			name:    "a lower-case t in the ID is neither a snapshot nor a manifest",
			entries: entries("20260927t143005Z.SQLITE", "20260927t143005Z.json"), want: []selected{},
		},
		{
			name:    "a lower-case z in the ID is neither a snapshot nor a manifest",
			entries: entries("20260927T143005z.sqlite", "20260927T143005z.json"), want: []selected{},
		},
		{
			name: "the long s folds to the extension", entries: entries(selID + ".ſqlite"),
			want: []selected{{selID + ".ſqlite", ""}},
		},
		{
			name:    "another extension, a partial and a non-ID name are ignored",
			entries: entries(selID+".db", "."+selID+".sqlite.partial", "notes.json", "notes.sqlite"), want: []selected{},
		},
		{
			name: "two IDs are two selections", entries: entries(selID+".SQLITE", selOther+".sqlite", selOther+".JSON"),
			want: []selected{{selID + ".SQLITE", ""}, {selOther + ".sqlite", selOther + ".JSON"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := selectFolder(c.entries)

			assert.Equal(t, c.want, chosen(got))
			assert.Equal(t, c.strays, got.strays)
		})
	}
}

func Test_select_folder_finds_orphan_manifests(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []fs.DirEntry
		want    []string
	}{
		{name: "a lone manifest", entries: entries(selID + ".json"), want: []string{selID + ".json"}},
		{
			name:    "every case variant of a manifest with no snapshot",
			entries: entries(selID+".JSON", selID+".json"), want: []string{selID + ".JSON", selID + ".json"},
		},
		{name: "beside another extension", entries: entries(selID+".json", selID+".db"), want: []string{selID + ".json"}},
		{name: "beside a snapshot in the same case", entries: entries(selID+".json", selID+".sqlite")},
		{name: "beside an upper-case snapshot", entries: entries(selID+".json", selID+".SQLITE")},
		{name: "beside an upper-case snapshot, manifest in upper case", entries: entries(selID+".JSON", selID+".SQLITE")},
		{
			name:    "beside a directory named as a snapshot",
			entries: withType(entries(selID+".json", selID+".SQLITE"), selID+".SQLITE", fs.ModeDir),
		},
		{
			name:    "beside a symlink named as a snapshot",
			entries: withType(entries(selID+".json", selID+".SQLITE"), selID+".SQLITE", fs.ModeSymlink),
		},
		{name: "beside a partial", entries: entries(selID+".json", "."+selID+".sqlite.partial")},
		{
			name: "beside an upper-case partial", entries: entries(selID+".json", "."+selID+".SQLITE.PARTIAL"),
			want: []string{selID + ".json"},
		},
		{name: "beside another ID's snapshot", entries: entries(selID+".json", selOther+".sqlite"), want: []string{selID + ".json"}},
		{name: "a lower-case t in the ID is no orphan", entries: entries("20260927t143005Z.json")},
		{name: "a lower-case z in the ID is no orphan", entries: entries("20260927T143005z.json")},
		{name: "a non-regular manifest alone", entries: withType(entries(selID+".json"), selID+".json", fs.ModeDir)},
		{name: "a symlinked manifest alone", entries: withType(entries(selID+".json"), selID+".json", fs.ModeSymlink)},
		{
			name: "only the regular variant of a mixed pair", entries: withType(entries(selID+".JSON", selID+".json"), selID+".json", fs.ModeDir),
			want: []string{selID + ".JSON"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := selectFolder(c.entries)

			assert.Equal(t, c.want, got.orphans)
		})
	}
}

// entryName is the on-disk name of s's entry, "" for the zero selection.
func entryName(s selectedSnapshot) string {
	if s.entry == nil {
		return ""
	}
	return s.entry.Name()
}

func Test_folder_selection_snapshot_lookup_skips_non_regular_entries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		entries   []fs.DirEntry
		id        string
		wantName  string
		wantFound bool
	}{
		{
			name: "a regular winner is found by its ID", entries: entries(selID+".SQLITE", selOther+".sqlite"),
			id: selID, wantName: selID + ".SQLITE", wantFound: true,
		},
		{
			name:    "a directory named as the ID is not found",
			entries: withType(entries(selID+".sqlite"), selID+".sqlite", fs.ModeDir), id: selID,
		},
		{
			name:    "a symlink named as the ID is not found",
			entries: withType(entries(selID+".sqlite"), selID+".sqlite", fs.ModeSymlink), id: selID,
		},
		{
			name:    "a non-regular sibling does not take the ID from a regular variant",
			entries: withType(entries(selID+".sqlite", selID+".SQLITE"), selID+".sqlite", fs.ModeSymlink),
			id:      selID, wantName: selID + ".SQLITE", wantFound: true,
		},
		{name: "an ID absent from the folder is not found", entries: entries(selOther + ".sqlite"), id: selID},
		{name: "the ID is not folded", entries: entries(selID + ".sqlite"), id: "20260927t143005z"},
		{name: "an empty folder holds nothing", entries: entries(), id: selID},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, found := selectFolder(c.entries).snapshot(c.id)

			assert.Equal(t, c.wantFound, found)
			assert.Equal(t, c.wantName, entryName(got))
		})
	}
}

func Test_select_manifest_picks_the_manifest_of_a_stem(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []fs.DirEntry
		stem    string
		want    string
	}{
		{name: "the lower-case manifest", entries: entries("X.json", "X.sqlite"), stem: "X", want: "X.json"},
		{name: "an upper-case extension alone", entries: entries("X.JSON"), stem: "X", want: "X.JSON"},
		{name: "lower case beats upper case", entries: entries("X.JSON", "X.json"), stem: "X", want: "X.json"},
		{name: "without lower case the byte-order first wins", entries: entries("X.Json", "X.JSON"), stem: "X", want: "X.JSON"},
		{
			name:    "a directory competes",
			entries: withType(entries("X.JSON"), "X.JSON", fs.ModeDir), stem: "X", want: "X.JSON",
		},
		{
			name:    "a symlink competes and an exact lower-case symlink wins",
			entries: withType(entries("X.JSON", "X.json"), "X.json", fs.ModeSymlink), stem: "X", want: "X.json",
		},
		{name: "a stem need not be an ID", entries: entries("latest.JSON"), stem: "latest", want: "latest.JSON"},
		{name: "the stem is exact, not folded", entries: entries("x.json"), stem: "X"},
		{name: "a snapshot extension before .json is not the stem", entries: entries("X.SQLITE.json"), stem: "X"},
		{name: "a trailing extension after .json is not a manifest", entries: entries("X.json.bak"), stem: "X"},
		{name: "a longer stem sharing the prefix is not a match", entries: entries("X2.json"), stem: "X"},
		{name: "no entries", entries: entries(), stem: "X"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.want, selectManifest(c.entries, c.stem))
		})
	}
}
