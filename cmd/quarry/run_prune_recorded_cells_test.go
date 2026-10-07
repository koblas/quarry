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
	for _, c := range readableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			pinLocalZone(t)
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--keep", "1", "--dry-run")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, ""+
				"Would delete 3 snapshots (1.6 MB), keeping the newest one and 20260929T090011Z, the store's snapshot:\n"+
				"  20260930T141502Z  2026-09-30 10:15 EDT  0.2 MB\n"+
				"  20260930T090000Z  2026-09-30 05:00 EDT  0.2 MB\n"+
				"  20260927T143005Z  2026-09-27 10:30 EDT  1.2 MB\n", stdout)
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
	}
}

func Test_run_snapshots_prune_dry_run_json_lists_what_lies_beyond_the_newest_n_when_the_recorded_snapshot_stats_or_is_gone(t *testing.T) {
	for _, c := range readableRecordedClasses() {
		t.Run(c.name, func(t *testing.T) {
			dir, _, exitCode, stdout, stderr := pruneRecordedCell(t, c, "--keep", "1", "--dry-run", "--json")

			require.Equal(t, 0, exitCode, stderr)
			assert.Empty(t, stderr)
			assert.Equal(t, []string{pruneNoon, pruneMorning, pruneOldest}, jsonIDs(t, stdout, "would_delete"))
			assert.Empty(t, jsonIDs(t, stdout, "deleted"))
			requireSnapshotsKept(t, dir, pruneOldest, pruneMiddle, pruneMorning, pruneNoon, pruneNewest)
		})
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
