package snapshot_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idFourth = "20260926T080000Z"
)

func Test_prune_keeps_exactly_the_newest_n(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
		keep int
		want []string
	}{
		{"one beyond the newest two", []string{idNewest, idMiddle, idOldest}, 2, []string{idOldest}},
		{"two beyond the newest two", []string{idNewest, idMiddle, idOldest, idFourth}, 2, []string{idOldest, idFourth}},
		{"exactly the newest two", []string{idNewest, idMiddle}, 2, []string{}},
		{"fewer than the newest two", []string{idNewest}, 2, []string{}},
		{"a _10 counts as newer than a _2", []string{idNewest, idNewest + "_2", idNewest + "_10"}, 2, []string{idNewest}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			listing := listed(t, t.TempDir(), nil, c.ids...)

			doomed, _ := snapshot.SelectPrune(listing.Entries, c.keep)

			assert.Equal(t, c.want, doomedIDs(doomed))
		})
	}
}

func Test_prune_never_deletes_the_stores_snapshot_when_it_is_older_than_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	stored := filepath.Join(home, "snapshots", idOldest+".sqlite")
	listing := listed(t, home, &fakeStoreProbe{builtFrom: stored}, idNewest, idMiddle, idOldest, idFourth)

	doomed, kept := snapshot.SelectPrune(listing.Entries, 2)

	assert.Equal(t, []string{idFourth}, doomedIDs(doomed))
	require.NotNil(t, kept)
	assert.Equal(t, idOldest, kept.ID)
}

func Test_prune_reports_no_kept_store_snapshot_when_the_store_is_within_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	stored := filepath.Join(home, "snapshots", idMiddle+".sqlite")
	listing := listed(t, home, &fakeStoreProbe{builtFrom: stored}, idNewest, idMiddle, idOldest, idFourth)

	doomed, kept := snapshot.SelectPrune(listing.Entries, 2)

	assert.Equal(t, []string{idOldest, idFourth}, doomedIDs(doomed))
	assert.Nil(t, kept)
}

func Test_prune_never_deletes_the_stores_snapshot_recorded_under_a_path_that_no_longer_resolves(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	moved := filepath.Join(home, "moved-away", idOldest+".sqlite")
	listing := listed(t, home, &fakeStoreProbe{builtFrom: moved}, idNewest, idMiddle, idOldest)

	doomed, kept := snapshot.SelectPrune(listing.Entries, 1)

	assert.Equal(t, []string{idMiddle}, doomedIDs(doomed))
	require.NotNil(t, kept)
	assert.Equal(t, idOldest, kept.ID)
}

func Test_prune_protects_nothing_without_a_store_or_for_a_store_built_outside_the_folder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		probe func(home string) snapshot.StoreProbe
	}{
		{"no store", func(string) snapshot.StoreProbe { return nil }},
		{"a store built outside the folder", func(home string) snapshot.StoreProbe {
			return &fakeStoreProbe{builtFrom: filepath.Join(home, "elsewhere", idOutside+".sqlite")}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			listing := listed(t, home, c.probe(home), idNewest, idMiddle, idOldest)

			doomed, kept := snapshot.SelectPrune(listing.Entries, 2)

			assert.Equal(t, []string{idOldest}, doomedIDs(doomed))
			assert.Nil(t, kept)
		})
	}
}

func Test_prune_deletes_nothing_for_a_keep_below_one(t *testing.T) {
	t.Parallel()
	for name, keep := range map[string]int{"zero": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			listed(t, home, nil, idNewest, idMiddle)

			_, err := newListServer(home, nil).Prune(t.Context(), keep)

			require.ErrorIs(t, err, snapshot.ErrKeepBelowOne)
			assert.FileExists(t, filepath.Join(home, "snapshots", idMiddle+".sqlite"))
			assert.FileExists(t, filepath.Join(home, "snapshots", idNewest+".sqlite"))
		})
	}
}

func Test_prune_accepts_a_keep_of_one(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	listed(t, home, nil, idNewest, idMiddle)

	_, err := newListServer(home, nil).Prune(t.Context(), 1)

	require.NoError(t, err)
}

func Test_prune_deletes_the_snapshots_beyond_the_newest_n_with_their_manifests(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest, idFourth}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Equal(t, filepath.Join(home, "snapshots"), pruned.Dir)
	for _, id := range []string{idOldest, idFourth} {
		assert.NoFileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.NoFileExists(t, filepath.Join(dir, id+".json"))
	}
	for _, id := range []string{idNewest, idMiddle} {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
}

func Test_prune_deletes_the_snapshot_file_then_its_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest + ".sqlite", idOldest + ".json"}, rm.calls)
}

func Test_prune_leaves_the_manifest_when_the_snapshot_file_cannot_be_deleted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	rm := failing(idOldest+".sqlite", syscall.EACCES)

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.NotContains(t, rm.calls, idOldest+".json")
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_reports_a_snapshot_it_could_not_delete_and_goes_on_to_the_next(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".sqlite", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, idOldest, pruned.Failed[0].Entry.ID)
	assert.Equal(t, "permission denied", pruned.Failed[0].Reason)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 3, pruned.Snapshots)
}

func Test_prune_deletes_a_snapshot_that_has_no_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := snapshotsFolder(t, home)
	for _, id := range []string{idNewest, idMiddle, idOldest} {
		writeSnapshot(t, dir, id, 1000)
	}

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
}

func Test_prune_counts_a_snapshot_deleted_when_only_its_manifest_cannot_be_removed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".json", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_drops_a_snapshot_file_that_is_already_gone_and_removes_its_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)

	pruned, err := newPruneServer(home, nil, failing(idOldest+".sqlite", syscall.ENOENT)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
}

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

// idLinkFuture is a snapshot ID newer than any SyncAndImport mints and older than futureIDs.
const idLinkFuture = "20980101T000000Z"

func Test_prune_deletes_no_snapshot_that_is_the_file_the_store_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
	recorded := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))

	pruned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).Prune(t.Context(), 1)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	require.NotNil(t, pruned.StoreKept)
	assert.Equal(t, idLinkBetween, pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, idMiddle)
	assert.FileExists(t, snapshotFile(dir, idLinkBetween))
}

func Test_prune_dry_run_lists_no_snapshot_that_is_the_file_the_store_recorded(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	hardLink(t, snapshotFile(dir, idMiddle), snapshotFile(dir, idLinkBetween))
	recorded := hardLink(t, snapshotFile(dir, idMiddle), filepath.Join(home, "linked.sqlite"))

	planned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).PlanPrune(t.Context(), 1)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(planned.WouldDelete))
	require.NotNil(t, planned.StoreKept)
	assert.Equal(t, idLinkBetween, planned.StoreKept.ID)
}

func Test_import_from_deletes_no_snapshot_that_is_the_file_it_built_the_store_from(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	srv := newImportServer(t, home, builtStore(), snapshot.WithAutoPrune(1))
	taken := takeSnapshot(t, srv)
	builtID := snapshotIDFromPath(taken.Snapshot.Path)
	ids := futureIDs()
	dir := prunable(t, home, ids...)
	hardLink(t, taken.Snapshot.Path, snapshotFile(dir, idLinkFuture))
	outside := t.TempDir()
	hardLink(t, taken.Snapshot.Manifest, filepath.Join(outside, "linked.json"))
	recorded := hardLink(t, taken.Snapshot.Path, filepath.Join(outside, "linked.sqlite"))

	outcome, err := srv.ImportFrom(t.Context(), recorded)

	require.NoError(t, err)
	require.NotNil(t, outcome.Pruned)
	assert.Equal(t, []string{ids[0]}, doomedIDs(outcome.Pruned.Deleted))
	require.NotNil(t, outcome.Pruned.StoreKept)
	assert.Equal(t, idLinkFuture, outcome.Pruned.StoreKept.ID)
	assertSnapshotPairs(t, dir, true, builtID)
	assert.FileExists(t, snapshotFile(dir, idLinkFuture))
}

func Test_prune_reads_the_store_once_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
	}{
		{"as many snapshots as the newest two", []string{idNewest, idMiddle}},
		{"no snapshots folder", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if len(c.ids) > 0 {
				prunable(t, home, c.ids...)
			}
			probe := &countingProbe{}

			_, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, 1, probe.calls)
		})
	}
}

func Test_prune_never_blocks_when_nothing_lies_beyond_the_newest_n_and_the_store_cannot_say_which_snapshot_built_it(t *testing.T) {
	t.Parallel()
	for _, c := range storeReadFaults() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			prunable(t, home, idNewest, idMiddle)
			probe := &fakeStoreProbe{path: storeFile, builtFromErr: c.fault}

			pruned, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, snapshot.Pruned{Keep: 2, Dir: filepath.Join(home, "snapshots"), Snapshots: 2}, pruned)
		})
	}
}

func Test_prune_never_blocks_on_a_store_read_that_is_not_an_open_fault_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle)

	pruned, err := newPruneServer(home, &fakeStoreProbe{builtFromErr: errStoreRead}, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, snapshot.Pruned{Keep: 2, Dir: filepath.Join(home, "snapshots"), Snapshots: 2}, pruned)
}

func Test_prune_reports_the_snapshot_path_the_store_recorded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
		keep int
	}{
		{"nothing lies beyond the newest two", []string{idNewest, idMiddle}, 2},
		{"no snapshots folder", nil, 2},
		{"a snapshot lies beyond the newest two", []string{idNewest, idMiddle, idOldest}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if len(c.ids) > 0 {
				prunable(t, home, c.ids...)
			}
			recorded := filepath.Join(home, "snapshots", idNewest+".sqlite")

			pruned, err := newPruneServer(home, &fakeStoreProbe{builtFrom: recorded}, &fakeRemover{}).Prune(t.Context(), c.keep)

			require.NoError(t, err)
			assert.Equal(t, recorded, pruned.StorePath)
		})
	}
}

func Test_prune_is_interrupted_when_the_context_ended_during_the_store_read_with_nothing_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle)
	ctx, end := context.WithCancel(t.Context())

	pruned, err := newPruneServer(home, &countingProbe{onRead: end}, &fakeRemover{}).Prune(ctx, 2)

	require.EqualError(t, err, "snapshots prune interrupted")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, snapshot.Pruned{}, pruned)
}

func Test_prune_asks_the_store_when_a_snapshot_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	probe := &countingProbe{}

	_, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, 1, probe.calls)
}

func Test_prune_reports_the_folder_and_its_snapshot_count_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ids  []string
		want int
	}{
		{"as many snapshots as the newest two", []string{idNewest, idMiddle}, 2},
		{"no snapshots folder", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if len(c.ids) > 0 {
				prunable(t, home, c.ids...)
			}

			pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, snapshot.Pruned{Keep: 2, Dir: filepath.Join(home, "snapshots"), Snapshots: c.want}, pruned)
		})
	}
}

// storeFile is the store path the faults name; the Server's home is /Users/x.
const storeFile = "/Users/x/Library/Application Support/quarry/quarry.duckdb"

// storeReadFault is a store fault and the R3 reason a prune refusal gives for it.
type storeReadFault struct {
	name   string
	fault  *store.OpenError
	reason string
}

// storeReadFaults are the five ways a store cannot say which snapshot built it.
func storeReadFaults() []storeReadFault {
	return []storeReadFault{
		{"not a DuckDB file", &store.OpenError{Fault: store.OpenFaultNotDuckDB, Path: storeFile}, "the file is not a DuckDB database"},
		{"permission denied", &store.OpenError{Fault: store.OpenFaultPermission, Path: storeFile}, "permission denied"},
		{"locked by another program", &store.OpenError{Fault: store.OpenFaultLocked, Path: storeFile}, "another program has it open for writing"},
		{
			"no import history",
			&store.OpenError{Fault: store.OpenFaultOther, Path: storeFile, Reason: "the store has no import history"},
			"the store has no import history",
		},
		{
			"another format naming no snapshot",
			&store.OpenError{Fault: store.OpenFaultOtherFormat, Path: storeFile},
			"the store was built by another version of quarry",
		},
	}
}

func Test_prune_deletes_nothing_when_the_store_cannot_say_which_snapshot_built_it(t *testing.T) {
	t.Parallel()
	for _, c := range storeReadFaults() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			prunable(t, dir, idNewest, idMiddle, idOldest)
			rm := &fakeRemover{}
			probe := &fakeStoreProbe{path: storeFile, builtFromErr: c.fault}

			pruned, err := snapshot.NewServer(
				snapshot.WithSnapshotDir(filepath.Join(dir, "snapshots")), snapshot.WithHome("/Users/x"),
				snapshot.WithStoreProbe(probe), snapshot.WithRemove(rm.remove),
			).Prune(t.Context(), 1)

			require.EqualError(t, err, "cannot tell which snapshot the store at ~/Library/Application Support/quarry/quarry.duckdb was built from ("+
				c.reason+"), so no snapshot was deleted; run quarry sync to rebuild the store, then prune again")
			assert.Empty(t, rm.calls)
			assert.Empty(t, pruned.Deleted)
			assert.FileExists(t, filepath.Join(dir, "snapshots", idOldest+".sqlite"))
		})
	}
}

func Test_prune_deletes_beyond_the_newest_n_when_the_store_can_say_which_snapshot_built_it(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	probe := &fakeStoreProbe{builtFrom: filepath.Join(home, "snapshots", idNewest+".sqlite")}

	pruned, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
}

func Test_prune_has_nothing_to_delete_when_the_one_beyond_the_newest_n_is_the_stores(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	probe := &fakeStoreProbe{builtFrom: filepath.Join(dir, idOldest+".sqlite")}
	rm := &fakeRemover{}

	pruned, err := newPruneServer(home, probe, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, pruned.Deleted)
	assert.Empty(t, rm.calls)
	assert.Equal(t, 3, pruned.Snapshots)
	require.NotNil(t, pruned.StoreKept)
	assert.Equal(t, idOldest, pruned.StoreKept.ID)
}

func Test_prune_deletes_what_lies_beyond_the_newest_n_except_the_stores_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	probe := &fakeStoreProbe{builtFrom: filepath.Join(dir, idOldest+".sqlite")}

	pruned, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
	assert.FileExists(t, filepath.Join(dir, idOldest+".sqlite"))
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
	assert.NoFileExists(t, filepath.Join(dir, idFourth+".sqlite"))
}

func Test_prune_refuses_a_folder_it_cannot_read(t *testing.T) {
	t.Parallel()
	skipUnderRoot(t)
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	require.NoError(t, os.Chmod(dir, 0o000))
	probe := &countingProbe{}

	_, err := newPruneServer(home, probe, &fakeRemover{}).Prune(t.Context(), 1)

	require.EqualError(t, err, "cannot read ~/snapshots: permission denied")
	assert.Zero(t, probe.calls)
}

func Test_prune_is_interrupted_when_the_context_ended_before_the_listing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle)
	rm := &fakeRemover{}
	ended, end := context.WithCancel(t.Context())
	end()

	_, err := newPruneServer(home, &countingProbe{}, rm).Prune(ended, 2)

	require.EqualError(t, err, "snapshots prune interrupted")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, rm.calls)
}

func Test_prune_is_interrupted_when_the_context_ended_during_the_store_read(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}
	ctx, end := context.WithCancel(t.Context())
	probe := &countingProbe{onRead: end}

	_, err := newPruneServer(home, probe, rm).Prune(ctx, 1)

	require.EqualError(t, err, "snapshots prune interrupted")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, rm.calls)
}

func Test_prune_returns_a_store_read_that_is_not_an_open_fault(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest)
	rm := &fakeRemover{}

	_, err := newPruneServer(home, &fakeStoreProbe{builtFromErr: errStoreRead}, rm).Prune(t.Context(), 1)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, rm.calls)
}

const (
	idFifth = "20260925T080000Z"
	idSixth = "20260924T080000Z"
	// Names for the manifests and files Prune must leave alone.
	idDirManifest     = "20260101T000000Z"
	idLinkManifest    = "20260102T000000Z"
	idInFlightSyncing = "20260103T000000Z"
)

// cancelling returns a fakeRemover that ends the returned context once name has been removed.
func cancelling(t *testing.T, name string) (*fakeRemover, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	return &fakeRemover{cancelAfter: name, cancel: cancel}, ctx
}

func Test_prune_sweeps_only_orphan_manifests(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	writeManifest(t, dir, idFourth, manifestTaken("2026-09-26T08:00:00Z"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idDirManifest+".json"), 0o700))
	outside := filepath.Join(home, "outside.json")
	require.NoError(t, os.WriteFile(outside, []byte("{}"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, idLinkManifest+".json")))
	writeManifest(t, dir, idInFlightSyncing, manifestTaken("2026-01-03T00:00:00Z"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "."+idInFlightSyncing+".sqlite.partial"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.json"), []byte("{}"), 0o600))

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Empty(t, pruned.Failed)
	assert.NoFileExists(t, filepath.Join(dir, idFourth+".json"))
	for _, name := range []string{idNewest + ".json", idMiddle + ".json", idDirManifest + ".json", idLinkManifest + ".json", idInFlightSyncing + ".json", "notes.json"} {
		_, statErr := os.Lstat(filepath.Join(dir, name))
		require.NoError(t, statErr, name)
	}
	assert.FileExists(t, outside)
}

func Test_prune_sweeps_orphans_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))
	rm := &fakeRemover{}

	pruned, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, pruned.Deleted)
	assert.Equal(t, 2, pruned.Snapshots)
	assert.Equal(t, []string{idOldest + ".json"}, rm.calls)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_ignores_an_orphan_manifest_it_cannot_remove(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))

	pruned, err := newPruneServer(home, nil, failing(idOldest+".json", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, pruned.Failed)
	assert.Empty(t, pruned.Deleted)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_deletes_only_snapshot_files_inside_the_folder(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest)
	target := filepath.Join(home, "outside.sqlite")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, idMiddle+".sqlite")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idOldest+".sqlite"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "."+idFourth+".sqlite.partial"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stray.sqlite"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))

	pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 1)

	require.NoError(t, err)
	assert.Empty(t, pruned.Deleted)
	assert.Empty(t, pruned.Failed)
	assert.Equal(t, 1, pruned.Snapshots)
	assert.FileExists(t, target)
	for _, name := range []string{idMiddle + ".sqlite", idOldest + ".sqlite", "." + idFourth + ".sqlite.partial", "stray.sqlite", "sub"} {
		_, statErr := os.Lstat(filepath.Join(dir, name))
		require.NoError(t, statErr, name)
	}
}

func Test_prune_leaves_a_manifest_that_is_not_a_regular_file(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func(t *testing.T, path, outside string)
	}{
		{"an empty directory", func(t *testing.T, path, _ string) { t.Helper(); require.NoError(t, os.Mkdir(path, 0o700)) }},
		{"a symlink", func(t *testing.T, path, outside string) { t.Helper(); require.NoError(t, os.Symlink(outside, path)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle)
			writeSnapshot(t, dir, idOldest, 1000)
			outside := filepath.Join(home, "outside.json")
			require.NoError(t, os.WriteFile(outside, []byte("{}"), 0o600))
			c.build(t, filepath.Join(dir, idOldest+".json"), outside)

			pruned, err := newPruneServer(home, nil, &fakeRemover{}).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
			assert.Empty(t, pruned.Failed)
			assert.NoFileExists(t, filepath.Join(dir, idOldest+".sqlite"))
			_, statErr := os.Lstat(filepath.Join(dir, idOldest+".json"))
			require.NoError(t, statErr)
			assert.FileExists(t, outside)
		})
	}
}

func Test_prune_stops_deleting_once_interrupted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest, idFourth, idFifth)
	writeManifest(t, dir, idSixth, manifestTaken("2026-09-24T08:00:00Z"))
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.ErrorIs(t, err, context.Canceled)
	require.EqualError(t, err, "snapshots prune interrupted; 2 snapshots were not deleted")
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Equal(t, 2, pruned.NotDeleted)
	assert.NoFileExists(t, filepath.Join(dir, idOldest+".json"))
	for _, id := range []string{idFourth, idFifth} {
		assert.FileExists(t, filepath.Join(dir, id+".sqlite"))
		assert.FileExists(t, filepath.Join(dir, id+".json"))
	}
	assert.FileExists(t, filepath.Join(dir, idSixth+".json"))
}

func Test_prune_says_one_snapshot_was_not_deleted(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth)
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.EqualError(t, err, "snapshots prune interrupted; 1 snapshot was not deleted")
	assert.Equal(t, 1, pruned.NotDeleted)
}

func Test_prune_counts_only_unattempted_snapshots_after_a_failure_and_an_interrupt(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prunable(t, home, idNewest, idMiddle, idOldest, idFourth, idFifth, idSixth)
	rm, ctx := cancelling(t, idFourth+".sqlite")
	rm.faults = failing(idOldest+".sqlite", syscall.EACCES).faults

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.EqualError(t, err, "snapshots prune interrupted; 2 snapshots were not deleted")
	assert.Equal(t, 2, pruned.NotDeleted)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, idOldest, pruned.Failed[0].Entry.ID)
	assert.Equal(t, []string{idFourth}, doomedIDs(pruned.Deleted))
}

func Test_prune_finishes_without_sweeping_when_the_context_ends_after_the_last_delete(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	writeManifest(t, dir, idFourth, manifestTaken("2026-09-26T08:00:00Z"))
	rm, ctx := cancelling(t, idOldest+".sqlite")

	pruned, err := newPruneServer(home, nil, rm).Prune(ctx, 2)

	require.NoError(t, err)
	assert.Equal(t, []string{idOldest}, doomedIDs(pruned.Deleted))
	assert.Zero(t, pruned.NotDeleted)
	assert.FileExists(t, filepath.Join(dir, idFourth+".json"))
}

func Test_prune_deletes_an_upper_case_sqlite_snapshot_then_its_manifest_and_keeps_every_newer_manifest(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	upperCased(t, dir, idNewest)
	oldestPath := upperCased(t, dir, idOldest)
	rm := &fakeRemover{}
	require.Equal(t, []string{
		"20260927T143005Z.SQLITE", "20260927T143005Z.json",
		"20260929T090011Z.json", "20260929T090011Z.sqlite",
		"20260930T141502Z.SQLITE", "20260930T141502Z.json",
	}, dirNames(t, dir))

	pruned, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"20260927T143005Z.SQLITE", "20260927T143005Z.json"}, rm.calls)
	require.Len(t, pruned.Deleted, 1)
	assert.Equal(t, oldestPath, pruned.Deleted[0].Path)
	assert.Equal(t, []string{
		"20260929T090011Z.json", "20260929T090011Z.sqlite",
		"20260930T141502Z.SQLITE", "20260930T141502Z.json",
	}, dirNames(t, dir))
}

func Test_prune_removes_the_manifest_by_its_on_disk_name(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		snapshotExt string
		want        []string
	}{
		{
			name: "lower-case snapshot, upper-case manifest", snapshotExt: "sqlite",
			want: []string{"20260927T143005Z.sqlite", "20260927T143005Z.JSON"},
		},
		{
			name: "upper-case snapshot and manifest", snapshotExt: "SQLITE",
			want: []string{"20260927T143005Z.SQLITE", "20260927T143005Z.JSON"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest)
			renamedExtension(t, dir, idOldest, "sqlite", c.snapshotExt)
			renamedExtension(t, dir, idOldest, "json", "JSON")
			rm := &fakeRemover{}

			_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, c.want, rm.calls)
		})
	}
}

func Test_prune_removes_an_upper_case_manifest_it_cannot_read_with_its_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeSnapshot(t, dir, idOldest, 1000)
	renamedExtension(t, dir, idOldest, "sqlite", "SQLITE")
	require.NoError(t, os.WriteFile(filepath.Join(dir, idOldest+".JSON"), []byte("not json"), 0o600))
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"20260927T143005Z.SQLITE", "20260927T143005Z.JSON"}, rm.calls)
}

func Test_prune_sweeps_an_upper_case_orphan_manifest_by_its_on_disk_name(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))
	renamedExtension(t, dir, idOldest, "json", "JSON")
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"20260927T143005Z.JSON"}, rm.calls)
}

func Test_prune_never_sweeps_the_manifest_of_a_directory_named_as_an_upper_case_snapshot(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle)
	writeManifest(t, dir, idOldest, manifestTaken("2026-09-27T10:00:00Z"))
	require.NoError(t, os.Mkdir(filepath.Join(dir, idOldest+".SQLITE"), 0o700))
	rm := &fakeRemover{}

	_, err := newPruneServer(home, nil, rm).Prune(t.Context(), 2)

	require.NoError(t, err)
	assert.Empty(t, rm.calls)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_prune_reports_an_upper_case_sqlite_snapshot_it_could_not_delete(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	oldestPath := upperCased(t, dir, idOldest)

	pruned, err := newPruneServer(home, nil, failing("20260927T143005Z.SQLITE", syscall.EACCES)).Prune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.Failed, 1)
	assert.Equal(t, oldestPath, pruned.Failed[0].Entry.Path)
	assert.Equal(t, "permission denied", pruned.Failed[0].Reason)
	assert.Empty(t, pruned.Deleted)
	assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
}

func Test_plan_prune_would_delete_an_upper_case_sqlite_snapshot_by_its_on_disk_path(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	dir := prunable(t, home, idNewest, idMiddle, idOldest)
	oldestPath := upperCased(t, dir, idOldest)
	rm := &fakeRemover{}

	pruned, err := newPruneServer(home, nil, rm).PlanPrune(t.Context(), 2)

	require.NoError(t, err)
	require.Len(t, pruned.WouldDelete, 1)
	assert.Equal(t, oldestPath, pruned.WouldDelete[0].Path)
	assert.Empty(t, rm.calls)
}

func Test_prune_keeps_the_manifest_while_another_entry_is_named_as_the_snapshot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mode fs.FileMode
	}{
		{name: "a regular upper-case variant"},
		{name: "a directory", mode: fs.ModeDir},
		{name: "a symlink", mode: fs.ModeSymlink},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			dir := prunable(t, home, idNewest, idMiddle, idOldest)
			rm := &fakeRemover{}
			other := variant{name: idOldest + ".SQLITE", like: idOldest + ".sqlite", mode: c.mode}

			_, err := newPruneServer(home, nil, rm, snapshot.WithReadDir(readDirWith(t, other))).Prune(t.Context(), 2)

			require.NoError(t, err)
			assert.Equal(t, []string{idOldest + ".sqlite"}, rm.calls)
			assert.FileExists(t, filepath.Join(dir, idOldest+".json"))
		})
	}
}
