// White-box: a bundle vanishing between the scan and the stat cannot be staged
// through DiscoverBundle, so the origin's refusal is driven directly.
package snapshot

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_resolveBundlePath_names_quicken_path_when_a_discovered_bundle_has_vanished(t *testing.T) {
	t.Parallel()
	home := t.TempDir()

	_, err := resolveBundlePath(home, filepath.Join(home, "Documents", "Home.quicken"), discoveredRefusals)

	var re RefusalError
	require.ErrorAs(t, err, &re)
	assert.Equal(t, "~/Documents/Home.quicken does not exist; pass one with --quicken <path> "+
		"or set quicken.path in ~/Library/Application Support/quarry/config.toml", re.Error())
}
