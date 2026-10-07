// White-box: selectFolder is unexported; a fake fs.DirEntry reaches entry types and case variants
// that a case-folding volume cannot hold side by side.
package snapshot

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_select_folder_marks_the_manifest_shared_when_another_entry_is_named_as_the_snapshot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		entries []fs.DirEntry
		want    []bool
	}{
		{name: "a lone snapshot", entries: entries(selID+".sqlite", selID+".json"), want: []bool{false}},
		{name: "a lone upper-case snapshot", entries: entries(selID+".SQLITE", selID+".JSON"), want: []bool{false}},
		{name: "a snapshot without a manifest", entries: entries(selID + ".sqlite"), want: []bool{false}},
		{name: "case variants of the manifest alone", entries: entries(selID+".sqlite", selID+".json", selID+".JSON"), want: []bool{false}},
		{name: "a regular stray", entries: entries(selID+".sqlite", selID+".SQLITE", selID+".json"), want: []bool{true}},
		{name: "three variants", entries: entries(selID+".sqlite", selID+".Sqlite", selID+".SQLITE", selID+".json"), want: []bool{true}},
		{
			name:    "a directory sibling",
			entries: withType(entries(selID+".sqlite", selID+".SQLITE", selID+".json"), selID+".SQLITE", fs.ModeDir),
			want:    []bool{true},
		},
		{
			name:    "a symlink sibling",
			entries: withType(entries(selID+".sqlite", selID+".SQLITE", selID+".json"), selID+".SQLITE", fs.ModeSymlink),
			want:    []bool{true},
		},
		{name: "a numeric suffix is another ID, not a sibling", entries: entries(selID+"_2.sqlite", selID+".sqlite"), want: []bool{false, false}},
		{
			name:    "only the ID with a sibling is shared",
			entries: entries(selID+".sqlite", selID+".SQLITE", selOther+".sqlite", selOther+".json"),
			want:    []bool{true, false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := selectFolder(c.entries)

			shared := make([]bool, len(got.snapshots))
			for i, s := range got.snapshots {
				shared[i] = s.manifestShared
			}
			assert.Equal(t, c.want, shared)
		})
	}
}

func Test_new_entry_carries_the_shared_manifest_flag_to_the_listing(t *testing.T) {
	t.Parallel()
	for _, shared := range []bool{true, false} {
		file := snapshotFile{
			entry: fakeDirEntry{name: selID + ".sqlite"}, id: selID, manifest: selID + ".json", manifestShared: shared,
			bytes: 7,
		}

		got := newEntry("/snapshots", file)

		assert.Equal(t, Entry{
			ID: selID, Path: "/snapshots/" + selID + ".sqlite", Bytes: 7,
			ManifestPath: "/snapshots/" + selID + ".json", manifestShared: shared,
		}, got)
	}
}
