package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_plan_prune_removes_nothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	rm := &fakeRemover{}
	srv := newPruneServer(home, nil, rm)

	_, err := srv.PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, rm.calls)
	for _, id := range []string{idOldest, idFourth} {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}

	_, err = srv.Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest + ".sqlite", idOldest + ".json", idFourth + ".sqlite", idFourth + ".json"}, rm.calls)
}

func Test_plan_prune_leaves_orphan_manifests_alone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		keep int
	}{
		{"with snapshots beyond the newest two", 2},
		{"with nothing beyond the newest four", 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
			writeManifest(t, dir, idFifth, manifestTaken("2026-09-25T08:00:00Z"))
			rm := &fakeRemover{}

			_, err := newPruneServer(home, nil, rm).PlanPrune(t.Context(), c.keep)

			require.NoError(t, err)
			assert.Empty(t, rm.calls)
			assert.FileExists(t, filepath.Join(dir, idFifth+".json"))
		})
	}
}

func Test_plan_prune_lists_what_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	planned, err := newPruneServer(home, nil, &fakeRemover{}).PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	assert.True(t, planned.DryRun)
	assert.Equal(t, 2, planned.Keep)
	assert.Equal(t, filepath.Join(home, "snapshots"), planned.Dir)
	assert.Equal(t, 4, planned.Snapshots)
	assert.Equal(t, []string{idOldest, idFourth}, doomedIDs(planned.WouldDelete))
	assert.Empty(t, planned.Deleted)
	assert.Empty(t, planned.Failed)
}

func Test_plan_prune_keeps_the_stores_snapshot_out_of_would_delete(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	probe := &fakeStoreProbe{builtFrom: filepath.Join(dir, idOldest+".sqlite")}

	planned, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idFourth}, doomedIDs(planned.WouldDelete))
	require.NotNil(t, planned.StoreKept)
	assert.Equal(t, idOldest, planned.StoreKept.ID)
}

func Test_plan_prune_reads_the_store_once(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
	}{
		{"with a snapshot beyond the newest two", []string{idNewest, idMiddle, idOldest}},
		{"with nothing beyond the newest two", []string{idNewest, idMiddle}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, c.ids...)
			probe := &countingProbe{}

			_, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, 1, probe.calls)
		})
	}
}

func Test_plan_prune_reports_the_snapshot_path_the_store_recorded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
	}{
		{"with a snapshot beyond the newest two", []string{idNewest, idMiddle, idOldest}},
		{"with nothing beyond the newest two", []string{idNewest, idMiddle}},
		{"with no snapshots folder", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if len(c.ids) > 0 {
				prunable(t, home, c.ids...)
			}
			recorded := filepath.Join(home, "snapshots", idNewest+".sqlite")

			planned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).PlanPrune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, recorded, planned.StorePath)
		})
	}
}

func Test_plan_prune_plans_nothing_and_does_not_block_when_nothing_lies_beyond_the_newest_n_and_the_store_cannot_say(t *testing.T) {
	t.Parallel()
	for _, c := range storeReadFaults() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, idNewest, idMiddle)
			probe := &fakeStoreProbe{path: storeFile, builtFromErr: c.fault}

			planned, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, snapshot.Pruned{Keep: 2, Dir: filepath.Join(home, "snapshots"), Snapshots: 2, DryRun: true}, planned)
		})
	}
}

func Test_plan_prune_refuses_a_keep_below_one(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle)

	planned, err := newPruneServer(home, nil, &fakeRemover{}).PlanPrune(t.Context(), 0)

	require.ErrorIs(t, err, snapshot.ErrKeepBelowOne)
	assert.Equal(t, snapshot.Pruned{}, planned)
}

func Test_plan_prune_deletes_nothing_and_refuses_when_the_store_cannot_say_which_snapshot_built_it(t *testing.T) {
	t.Parallel()
	for _, c := range storeReadFaults() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			prunable(t, dir, idNewest, idMiddle, idOldest)
			probe := &fakeStoreProbe{path: storeFile, builtFromErr: c.fault}

			planned, err := snapshot.NewServer(
				snapshot.WithSnapshotDir(filepath.Join(dir, "snapshots")), snapshot.WithHome("/Users/x"),
				snapshot.WithStoreProbe(probe),
			).PlanPrune(t.Context(), 1)

			require.EqualError(t, err, "cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from ("+
				c.reason+"), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again")
			assert.Equal(t, snapshot.Pruned{}, planned)
		})
	}
}

func Test_plan_prune_refuses_a_folder_it_cannot_read(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	require.NoError(t, os.Chmod(dir, 0o000))
	probe := &countingProbe{}

	planned, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(t.Context(), 1)

	require.EqualError(t, err, "cannot read ~/snapshots: permission denied")
	assert.Equal(t, snapshot.Pruned{}, planned)
	assert.Zero(t, probe.calls)
}

func Test_plan_prune_is_interrupted_when_the_context_ended_before_the_listing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle)
	probe := &countingProbe{}
	ended, end := context.WithCancel(t.Context())
	end()

	planned, err := newPruneServer(home, probe, &fakeRemover{}).PlanPrune(ended, 2)

	require.EqualError(t, err, "snapshots prune interrupted")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, snapshot.Pruned{}, planned)
	assert.Zero(t, probe.calls)
}

func Test_plan_prune_is_interrupted_when_the_context_ended_during_the_store_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
	}{
		{"with a snapshot beyond the newest two", []string{idNewest, idMiddle, idOldest}},
		{"with nothing beyond the newest two", []string{idNewest, idMiddle}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, c.ids...)
			ctx, end := context.WithCancel(t.Context())

			planned, err := newPruneServer(home, &countingProbe{onRead: end}, &fakeRemover{}).PlanPrune(ctx, 2)

			require.EqualError(t, err, "snapshots prune interrupted")
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, snapshot.Pruned{}, planned)
		})
	}
}
