// White-box: a case-sensitive volume cannot be assumed (macOS temp dirs fold case), so the
// selected entry is built by hand and deleteSnapshot is fed it directly.
package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_delete_snapshot_keeps_a_manifest_another_entry_of_the_id_may_still_need(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		manifestShared bool
		want           []string
	}{
		{name: "a shared manifest stays", manifestShared: true, want: []string{selID + ".sqlite"}},
		{name: "an unshared manifest goes with its snapshot", manifestShared: false, want: []string{selID + ".sqlite", selID + ".json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			entry := Entry{
				ID: selID, Path: filepath.Join(dir, selID+".sqlite"),
				ManifestPath: filepath.Join(dir, selID+".json"), manifestShared: c.manifestShared,
			}
			require.NoError(t, os.WriteFile(entry.ManifestPath, []byte("{}"), 0o600))
			var removed []string
			srv := &Server{remove: func(path string) error {
				removed = append(removed, filepath.Base(path))
				return nil
			}}
			pruned := Pruned{Snapshots: 3}

			srv.deleteSnapshot(entry, &pruned)

			assert.Equal(t, c.want, removed)
			assert.Equal(t, []Entry{entry}, pruned.Deleted)
			assert.Equal(t, 2, pruned.Snapshots)
		})
	}
}
