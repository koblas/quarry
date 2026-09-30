package snapshot_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingProbe is a fakeStoreProbe that counts BuiltFrom calls and runs onRead inside each.
type countingProbe struct {
	fakeStoreProbe

	calls  int
	onRead func()
}

func (p *countingProbe) BuiltFrom(ctx context.Context) (string, error) {
	p.calls++
	if p.onRead != nil {
		p.onRead()
	}
	return p.fakeStoreProbe.BuiltFrom(ctx)
}

func Test_prune_does_not_ask_the_store_when_nothing_lies_beyond_the_newest_n(t *testing.T) {
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
			assert.Zero(t, probe.calls)
		})
	}
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

func Test_prune_deletes_nothing_when_the_store_cannot_say_which_snapshot_built_it(t *testing.T) {
	t.Parallel()
	const storeFile = "/Users/x/Library/Application Support/quarry/quarry.duckdb"
	cases := []struct {
		name   string
		fault  *store.OpenError
		reason string
	}{
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
	for _, c := range cases {
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
