package snapshot_test

import (
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/snapshot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idFourth = "20260926T080000Z"
)

// listed writes a snapshot per id into home's folder and lists them with probe as the store.
func listed(t *testing.T, home string, probe snapshot.StoreProbe, ids ...string) snapshot.Listing {
	t.Helper()
	dir := snapshotsFolder(t, home)
	for _, id := range ids {
		writeSnapshot(t, dir, id, 1000)
	}
	listing, err := newListServer(home, probe).List(t.Context())
	require.NoError(t, err)
	return listing
}

func doomedIDs(entries []snapshot.Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}

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
