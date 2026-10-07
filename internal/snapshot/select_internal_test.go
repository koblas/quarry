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
		{name: "a lone upper-case snapshot", entries: entries(selID + ".SQLITE"),
			want: []selected{{selID + ".SQLITE", ""}}},
		{name: "exact lower case wins over upper case", entries: entries(selID+".SQLITE", selID+".sqlite"),
			want: []selected{{selID + ".sqlite", ""}}, strays: []string{selID + ".SQLITE"}},
		{name: "without a lower-case name the byte-order first wins", entries: entries(selID+".SQLITE", selID+".Sqlite"),
			want: []selected{{selID + ".SQLITE", ""}}, strays: []string{selID + ".Sqlite"}},
		{name: "byte order is read from the name, not the entry order", entries: entries(selID+".Sqlite", selID+".SQLITE"),
			want: []selected{{selID + ".SQLITE", ""}}, strays: []string{selID + ".Sqlite"}},
		{name: "three variants leave two strays", entries: entries(selID+".SQLITE", selID+".Sqlite", selID+".sqlite"),
			want: []selected{{selID + ".sqlite", ""}}, strays: []string{selID + ".SQLITE", selID + ".Sqlite"}},
		{name: "a directory named as a snapshot loses to a regular variant and is no stray",
			entries: withType(entries(selID+".SQLITE", selID+".Sqlite"), selID+".SQLITE", fs.ModeDir),
			want:    []selected{{selID + ".Sqlite", ""}}},
		{name: "a symlink named as a snapshot loses to a regular variant and is no stray",
			entries: withType(entries(selID+".SQLITE", selID+".Sqlite"), selID+".SQLITE", fs.ModeSymlink),
			want:    []selected{{selID + ".Sqlite", ""}}},
		{name: "a non-regular snapshot alone is not a snapshot",
			entries: withType(entries(selID+".SQLITE"), selID+".SQLITE", fs.ModeDir), want: []selected{}},
		{name: "a numeric suffix is part of the ID", entries: entries(selID+"_2.SQLITE", selID+".sqlite"),
			want: []selected{{selID + "_2.SQLITE", ""}, {selID + ".sqlite", ""}}},
		{name: "a manifest is chosen by the same rule: exact lower case first",
			entries: entries(selID+".JSON", selID+".Json", selID+".json", selID+".sqlite"),
			want:    []selected{{selID + ".sqlite", selID + ".json"}}},
		{name: "without a lower-case manifest the byte-order first wins",
			entries: entries(selID+".Json", selID+".JSON", selID+".sqlite"),
			want:    []selected{{selID + ".sqlite", selID + ".JSON"}}},
		{name: "a symlinked manifest competes",
			entries: withType(entries(selID+".json", selID+".JSON", selID+".SQLITE"), selID+".json", fs.ModeSymlink),
			want:    []selected{{selID + ".SQLITE", selID + ".json"}}},
		{name: "a directory named as a manifest competes",
			entries: withType(entries(selID+".JSON", selID+".sqlite"), selID+".JSON", fs.ModeDir),
			want:    []selected{{selID + ".sqlite", selID + ".JSON"}}},
		{name: "a lower-case t in the ID is neither a snapshot nor a manifest",
			entries: entries("20260927t143005Z.SQLITE", "20260927t143005Z.json"), want: []selected{}},
		{name: "a lower-case z in the ID is neither a snapshot nor a manifest",
			entries: entries("20260927T143005z.sqlite", "20260927T143005z.json"), want: []selected{}},
		{name: "the long s folds to the extension", entries: entries(selID + ".ſqlite"),
			want: []selected{{selID + ".ſqlite", ""}}},
		{name: "another extension, a partial and a non-ID name are ignored",
			entries: entries(selID+".db", "."+selID+".sqlite.partial", "notes.json", "notes.sqlite"), want: []selected{}},
		{name: "two IDs are two selections", entries: entries(selID+".SQLITE", selOther+".sqlite", selOther+".JSON"),
			want: []selected{{selID + ".SQLITE", ""}, {selOther + ".sqlite", selOther + ".JSON"}}},
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
		{name: "every case variant of a manifest with no snapshot",
			entries: entries(selID+".JSON", selID+".json"), want: []string{selID + ".JSON", selID + ".json"}},
		{name: "beside another extension", entries: entries(selID+".json", selID+".db"), want: []string{selID + ".json"}},
		{name: "beside a snapshot in the same case", entries: entries(selID+".json", selID+".sqlite")},
		{name: "beside an upper-case snapshot", entries: entries(selID+".json", selID+".SQLITE")},
		{name: "beside an upper-case snapshot, manifest in upper case", entries: entries(selID+".JSON", selID+".SQLITE")},
		{name: "beside a directory named as a snapshot",
			entries: withType(entries(selID+".json", selID+".SQLITE"), selID+".SQLITE", fs.ModeDir)},
		{name: "beside a symlink named as a snapshot",
			entries: withType(entries(selID+".json", selID+".SQLITE"), selID+".SQLITE", fs.ModeSymlink)},
		{name: "beside a partial", entries: entries(selID+".json", "."+selID+".sqlite.partial")},
		{name: "beside an upper-case partial", entries: entries(selID+".json", "."+selID+".SQLITE.PARTIAL"),
			want: []string{selID + ".json"}},
		{name: "beside another ID's snapshot", entries: entries(selID+".json", selOther+".sqlite"), want: []string{selID + ".json"}},
		{name: "a non-regular manifest alone", entries: withType(entries(selID+".json"), selID+".json", fs.ModeDir)},
		{name: "a symlinked manifest alone", entries: withType(entries(selID+".json"), selID+".json", fs.ModeSymlink)},
		{name: "only the regular variant of a mixed pair", entries: withType(entries(selID+".JSON", selID+".json"), selID+".json", fs.ModeDir),
			want: []string{selID + ".JSON"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := selectFolder(c.entries)

			assert.Equal(t, c.want, got.orphans)
		})
	}
}
