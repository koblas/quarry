package cli_test

import (
	"bytes"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotsHelp(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer

	err := cli.Execute(t.Context(), args, cli.Env{Stdout: &stdout, Stderr: &stderr})

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
	return stdout.String()
}

func Test_snapshots_help_says_what_snapshots_lists_and_how_old_ones_go(t *testing.T) {
	const long = `List the snapshots quarry sync has taken, newest first: when each was
taken, its size, the Quicken file it came from, and which one the store was
built from. Snapshots live in ~/Library/Application Support/quarry/snapshots.

After each successful sync, quarry deletes the oldest snapshots beyond the
newest 12, never the one the store was built from. Snapshots of every
Quicken file count toward the same 12. To keep a different number, set
snapshots.keep in ~/Library/Application Support/quarry/config.toml:

  [snapshots]
  keep = 24

Status is "store" for the snapshot the store was built from, "schema
differs" for one quarry cannot import, and "no manifest" for one that
cannot be used with --from. Rebuild the store from a listed snapshot with
quarry sync --from <ID>.
`

	got := snapshotsHelp(t, "snapshots", "--help")

	assert.Contains(t, got, long)
}

func Test_snapshots_help_shows_examples(t *testing.T) {
	const examples = `Examples:
  quarry snapshots
  quarry snapshots prune --dry-run
`

	got := snapshotsHelp(t, "snapshots", "--help")

	assert.Contains(t, got, examples)
}
