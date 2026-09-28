// White-box: resolveFrom's ID-versus-path split has four input shapes, and
// only the ID form is reachable through ImportFrom without a real file per shape.
package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_resolveFrom_maps_each_value_shape_to_its_snapshot_and_manifest(t *testing.T) {
	const home = "/Users/x"
	const snapshotDir = "/Users/x/Library/Application Support/quarry/snapshots"
	cwd, err := os.Getwd()
	require.NoError(t, err)
	cases := []struct {
		name         string
		value        string
		wantSnapshot string
		wantManifest string
		wantIsPath   bool
	}{
		{name: "an ID joins the snapshots directory", value: "20260927T143005Z",
			wantSnapshot: snapshotDir + "/20260927T143005Z.sqlite", wantManifest: snapshotDir + "/20260927T143005Z.json",
			wantIsPath: false},
		{name: "a leading tilde expands against home", value: "~/x.sqlite",
			wantSnapshot: "/Users/x/x.sqlite", wantManifest: "/Users/x/x.json", wantIsPath: true},
		{name: "a slash makes a path even without the suffix", value: "/a/b",
			wantSnapshot: "/a/b", wantManifest: "/a/b.json", wantIsPath: true},
		{name: "a relative .sqlite name resolves against the working directory", value: "x.sqlite",
			wantSnapshot: filepath.Join(cwd, "x.sqlite"), wantManifest: filepath.Join(cwd, "x.json"), wantIsPath: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			snapshotPath, manifestPath, isPath, err := resolveFrom(home, snapshotDir, c.value)

			require.NoError(t, err)
			assert.Equal(t, c.wantSnapshot, snapshotPath)
			assert.Equal(t, c.wantManifest, manifestPath)
			assert.Equal(t, c.wantIsPath, isPath)
		})
	}
}
