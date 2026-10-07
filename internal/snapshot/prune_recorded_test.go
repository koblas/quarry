package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannotReadRefusal is the refusal for a store at ~/quarry/quarry.duckdb whose recorded snapshot cannot be read for detail.
func cannotReadRefusal(detail string) string {
	return "cannot tell which snapshot the store at ~/quarry/quarry.duckdb was built from (" + detail +
		"), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again"
}

func Test_prune_deletes_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	t.Parallel()
	for _, c := range unreadableRecordedPaths() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest)
			recorded := c.arrange(t, home)
			rm := &fakeRemover{}
			probe := &fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb"), builtFrom: recorded}

			pruned, err := newPruneServer(home, probe, rm).Prune(t.Context(), 1)

			require.EqualError(t, err, cannotReadRefusal("cannot read "+homeRelative(home, recorded)+": "+c.reason))
			assert.Empty(t, rm.calls)
			assert.Empty(t, pruned.Deleted)
			assert.FileExists(t, filepath.Join(dir, idOldest+".sqlite"))
		})
	}
}

func Test_plan_prune_refuses_and_plans_nothing_when_the_recorded_snapshot_cannot_be_read(t *testing.T) {
	t.Parallel()
	for _, c := range unreadableRecordedPaths() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, idNewest, idMiddle, idOldest)
			recorded := c.arrange(t, home)
			probe := &fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb"), builtFrom: recorded}

			planned, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(t.Context(), 1)

			require.EqualError(t, err, cannotReadRefusal("cannot read "+homeRelative(home, recorded)+": "+c.reason))
			assert.Empty(t, planned.WouldDelete)
		})
	}
}

func Test_prune_deletes_beyond_the_newest_n_when_the_recorded_snapshot_is_gone(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	gone := snapshotFile(filepath.Join(home, "moved-away"), idMiddle)
	probe := &fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb"), builtFrom: gone}

	pruned, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 1)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, gone, pruned.StorePath)
}

func Test_prune_reports_the_recorded_snapshot_within_the_newest_n_when_it_cannot_be_read(t *testing.T) {
	t.Parallel()
	for _, c := range unreadableRecordedPaths() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, idNewest, idMiddle, idOldest)
			recorded := c.arrange(t, home)
			rm := &fakeRemover{}
			probe := &fakeStoreProbe{path: filepath.Join(home, "quarry", "quarry.duckdb"), builtFrom: recorded}

			pruned, err := newPruneServer(home, probe, rm).Prune(t.Context(), 3)

			require.NoError(t, err)
			assert.Equal(t, recorded, pruned.StorePath)
			assert.Empty(t, pruned.Deleted)
			assert.Empty(t, rm.calls)
		})
	}
}
