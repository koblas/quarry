// White-box: renderPruned's keep phrase, alignment and empty-output rules are
// unexported formatting edges best driven directly.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
)

const (
	pruneHome     = "/Users/x"
	pruneDir      = "/Users/x/Library/Application Support/quarry/snapshots"
	pruneDirShown = "~/Library/Application Support/quarry/snapshots"
)

func deletedEntry(id string, bytes int64) snapshot.Entry {
	return snapshot.Entry{ID: id, Bytes: bytes, TakenAt: time.Date(2026, 9, 27, 14, 30, 5, 0, time.UTC)}
}

func Test_renderPruned_names_what_it_kept(t *testing.T) {
	useZone(t, time.UTC)
	one := []snapshot.Entry{deletedEntry("20260927T143005Z", 55_100_000)}
	two := []snapshot.Entry{deletedEntry("20260929T090011Z", 55_200_000), deletedEntry("20260927T143005Z", 55_100_000)}
	stored := &snapshot.Entry{ID: "20260801T120000Z", Store: true}
	cases := []struct {
		name   string
		pruned snapshot.Pruned
		want   string
	}{
		{
			name:   "keeping the newest one when N is 1",
			pruned: snapshot.Pruned{Keep: 1, Deleted: one},
			want:   "Deleted 1 snapshot (55.1 MB), keeping the newest one:\n",
		},
		{
			name:   "keeping the newest N when N is 2 or more",
			pruned: snapshot.Pruned{Keep: 12, Deleted: two},
			want:   "Deleted 2 snapshots (110.3 MB), keeping the newest 12:\n",
		},
		{
			name:   "grouping the thousands of N",
			pruned: snapshot.Pruned{Keep: 1200, Deleted: one},
			want:   "Deleted 1 snapshot (55.1 MB), keeping the newest 1,200:\n",
		},
		{
			name:   "naming the store's snapshot when it lies outside the newest N",
			pruned: snapshot.Pruned{Keep: 12, Deleted: one, StoreKept: stored},
			want:   "Deleted 1 snapshot (55.1 MB), keeping the newest 12 and 20260801T120000Z, the store's snapshot:\n",
		},
		{
			name:   "naming the store's snapshot beside the newest one",
			pruned: snapshot.Pruned{Keep: 1, Deleted: one, StoreKept: stored},
			want:   "Deleted 1 snapshot (55.1 MB), keeping the newest one and 20260801T120000Z, the store's snapshot:\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderPruned(c.pruned, pruneHome)

			assert.Contains(t, got, c.want)
		})
	}
}

func Test_renderPruned_aligns_the_rows(t *testing.T) {
	useZone(t, time.UTC)
	noManifest := snapshot.Entry{ID: "20260930T141502Z_2", Bytes: 165_500_000}
	dated := deletedEntry("20260929T090011Z", 55_200_000)
	small := deletedEntry("20260927T143005Z", 1_240_000)
	pruned := snapshot.Pruned{Keep: 1, Deleted: []snapshot.Entry{noManifest, dated, small}}

	got := renderPruned(pruned, pruneHome)

	assert.Equal(t, ""+
		"Deleted 3 snapshots (221.9 MB), keeping the newest one:\n"+
		"  20260930T141502Z_2  unknown               165.5 MB\n"+
		"  20260929T090011Z    2026-09-27 14:30 UTC   55.2 MB\n"+
		"  20260927T143005Z    2026-09-27 14:30 UTC    1.2 MB\n", got)
}

func Test_renderPruned_says_nothing_to_delete(t *testing.T) {
	stored := &snapshot.Entry{ID: "20260801T120000Z", Store: true}
	cases := []struct {
		name   string
		pruned snapshot.Pruned
		want   string
	}{
		{
			name:   "how many snapshots are within the newest N",
			pruned: snapshot.Pruned{Keep: 12, Dir: pruneDir, Snapshots: 5},
			want:   "Nothing to delete: 5 snapshots, within the newest 12\n",
		},
		{
			name:   "one snapshot is singular and the newest one",
			pruned: snapshot.Pruned{Keep: 1, Dir: pruneDir, Snapshots: 1},
			want:   "Nothing to delete: 1 snapshot, within the newest one\n",
		},
		{
			name:   "the store's snapshot is named when it is the one beyond the newest N",
			pruned: snapshot.Pruned{Keep: 12, Dir: pruneDir, Snapshots: 13, StoreKept: stored},
			want:   "Nothing to delete: 13 snapshots, within the newest 12 and 20260801T120000Z, the store's snapshot\n",
		},
		{
			name:   "no snapshots names the folder abbreviated",
			pruned: snapshot.Pruned{Keep: 12, Dir: pruneDir},
			want:   "Nothing to delete: no snapshots in " + pruneDirShown + "\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderPruned(c.pruned, pruneHome)

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_renderPruned_prints_nothing_when_every_delete_failed_or_the_run_was_interrupted(t *testing.T) {
	failure := snapshot.PruneFailure{Entry: deletedEntry("20260927T143005Z", 1_240_000), Reason: "permission denied"}
	cases := []struct {
		name   string
		pruned snapshot.Pruned
	}{
		{name: "every attempted delete failed", pruned: snapshot.Pruned{Keep: 1, Dir: pruneDir, Snapshots: 3, Failed: []snapshot.PruneFailure{failure}}},
		{name: "interrupted with nothing deleted", pruned: snapshot.Pruned{Keep: 1, Dir: pruneDir, Snapshots: 3, NotDeleted: 2}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderPruned(c.pruned, pruneHome)

			assert.Empty(t, got)
		})
	}
}

func Test_renderPruned_names_what_a_dry_run_would_delete(t *testing.T) {
	useZone(t, time.UTC)
	one := []snapshot.Entry{deletedEntry("20260927T143005Z", 55_100_000)}
	two := []snapshot.Entry{deletedEntry("20260929T090011Z", 55_200_000), deletedEntry("20260927T143005Z", 55_100_000)}
	cases := []struct {
		name   string
		pruned snapshot.Pruned
		want   string
	}{
		{
			name:   "one snapshot is singular",
			pruned: snapshot.Pruned{DryRun: true, Keep: 1, WouldDelete: one},
			want:   "Would delete 1 snapshot (55.1 MB), keeping the newest one:\n",
		},
		{
			name:   "several snapshots are plural",
			pruned: snapshot.Pruned{DryRun: true, Keep: 12, WouldDelete: two},
			want:   "Would delete 2 snapshots (110.3 MB), keeping the newest 12:\n",
		},
		{
			name: "the store's snapshot is named when it lies outside the newest N",
			pruned: snapshot.Pruned{
				DryRun: true, Keep: 12, WouldDelete: one, StoreKept: &snapshot.Entry{ID: "20260801T120000Z", Store: true},
			},
			want: "Would delete 1 snapshot (55.1 MB), keeping the newest 12 and 20260801T120000Z, the store's snapshot:\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderPruned(c.pruned, pruneHome)

			assert.Contains(t, got, c.want)
		})
	}
}

func Test_renderPruned_aligns_a_dry_runs_rows(t *testing.T) {
	useZone(t, time.UTC)
	noManifest := snapshot.Entry{ID: "20260930T141502Z_2", Bytes: 165_500_000}
	dated := deletedEntry("20260929T090011Z", 55_200_000)
	small := deletedEntry("20260927T143005Z", 1_240_000)
	pruned := snapshot.Pruned{DryRun: true, Keep: 1, WouldDelete: []snapshot.Entry{noManifest, dated, small}}

	got := renderPruned(pruned, pruneHome)

	assert.Equal(t, ""+
		"Would delete 3 snapshots (221.9 MB), keeping the newest one:\n"+
		"  20260930T141502Z_2  unknown               165.5 MB\n"+
		"  20260929T090011Z    2026-09-27 14:30 UTC   55.2 MB\n"+
		"  20260927T143005Z    2026-09-27 14:30 UTC    1.2 MB\n", got)
}

func Test_renderPruned_says_nothing_to_delete_for_a_dry_run_that_selects_nothing(t *testing.T) {
	cases := []struct {
		name   string
		pruned snapshot.Pruned
		want   string
	}{
		{
			name:   "snapshots within the newest N",
			pruned: snapshot.Pruned{DryRun: true, Keep: 12, Dir: pruneDir, Snapshots: 5},
			want:   "Nothing to delete: 5 snapshots, within the newest 12\n",
		},
		{
			name:   "no snapshots",
			pruned: snapshot.Pruned{DryRun: true, Keep: 12, Dir: pruneDir},
			want:   "Nothing to delete: no snapshots in " + pruneDirShown + "\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderPruned(c.pruned, pruneHome)

			assert.Equal(t, c.want, got)
		})
	}
}
