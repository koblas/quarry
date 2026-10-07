// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pruneMode is a way to run prune, named for its table row.
type pruneMode struct {
	name string
	args []string
}

func Test_run_snapshots_prune_recorded_snapshot_cells(t *testing.T) {
	modes := []pruneMode{
		{"text", []string{"--keep", "1"}},
		{"dry run", []string{"--keep", "1", "--dry-run"}},
		{"json", []string{"--keep", "1", "--json"}},
		{"dry run json", []string{"--keep", "1", "--dry-run", "--json"}},
	}
	for _, c := range unreadableRecordedClasses() {
		for _, mode := range modes {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, mode.args...)

				assert.Equal(t, 1, exitCode)
				assert.Empty(t, stdout)
				assert.Equal(t, cannotTellRefusal(c.cannotReadPhrase("~/Backup/"+pruneMiddle+".sqlite")), stderr)
				requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
			})
		}
	}
}

func Test_run_snapshots_prune_deletes_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	modes := []pruneMode{
		{"text", []string{"--keep", "1"}},
		{"json", []string{"--keep", "1", "--json"}},
	}
	for _, c := range readableRecordedClasses() {
		for _, mode := range modes {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				dir, _, exitCode, _, stderr := pruneRecordedCell(t, c, mode.args...)

				require.Equal(t, 0, exitCode, stderr)
				assert.Empty(t, stderr)
				requireSnapshotsGone(t, dir, pruneOldest, pruneMorning, pruneNoon)
				requireSnapshotsKept(t, dir, pruneMiddle, pruneNewest)
			})
		}
	}
}

func Test_run_snapshots_prune_dry_run_lists_what_lies_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	modes := []pruneMode{
		{"text", []string{"--keep", "1", "--dry-run"}},
		{"json", []string{"--keep", "1", "--dry-run", "--json"}},
	}
	for _, c := range readableRecordedClasses() {
		for _, mode := range modes {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, mode.args...)

				require.Equal(t, 0, exitCode, stderr)
				assert.Empty(t, stderr)
				assert.Contains(t, stdout, pruneOldest)
				assert.Contains(t, stdout, pruneMorning)
				requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
			})
		}
	}
}

func Test_run_snapshots_prune_says_nothing_to_delete_within_the_newest_n_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	for _, c := range unreadableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c)

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, "Nothing to delete: 5 snapshots, within the newest 12\n", stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_json_names_the_recorded_snapshot_without_a_warning_within_the_newest_n_when_it_cannot_be_read(t *testing.T) {
	for _, c := range unreadableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			_, recorded, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--json")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			doc := pruneDocument(t, stdout)
			assert.JSONEq(t, `{"id":"`+pruneMiddle+`","path":"`+recorded+`"}`, string(doc["store_snapshot"]))
			assert.JSONEq(t, `[]`, string(doc["warnings"]))
			assert.JSONEq(t, `[]`, string(doc["deleted"]))
		})
	}
}
