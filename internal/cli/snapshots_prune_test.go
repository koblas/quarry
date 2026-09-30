package cli_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_prune_help_says_what_prune_keeps(t *testing.T) {
	const long = `Delete all but the newest snapshots now, as sync does after each successful
sync. prune keeps --keep snapshots, or snapshots.keep from
~/Library/Application Support/quarry/config.toml (12 unless set). The
snapshot the store was built from is never deleted, even when it is older.

With --dry-run, prune lists what it would delete and deletes nothing.
`

	got := snapshotsHelp(t, "snapshots", "prune", "--help")

	assert.Contains(t, got, long)
}

func Test_prune_help_shows_examples_and_the_keep_flag(t *testing.T) {
	const examples = `Examples:
  quarry snapshots prune --dry-run
  quarry snapshots prune --keep 3
`
	const keepFlag = "--keep n   keep the newest n snapshots (default: snapshots.keep in the config file, 12 unless set)\n"

	got := snapshotsHelp(t, "snapshots", "prune", "--help")

	assert.Contains(t, got, examples)
	assert.Contains(t, got, keepFlag)
}

func Test_snapshots_help_lists_prune(t *testing.T) {
	const listed = "  prune       Delete all but the newest snapshots\n"

	got := snapshotsHelp(t, "snapshots", "--help")

	assert.Contains(t, got, listed)
}
