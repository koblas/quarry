package snapshot_test

import (
	"testing"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/koblas/quarry/internal/snapshot"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// threeStates is two new findings and one carried, as a store build reports them with no ignore list.
func threeStates() store.Result {
	return store.Result{
		Built:    true,
		Findings: finding.Counts{Open: 3, New: 2},
		FindingStates: []finding.State{
			{ID: "uncategorized:payee-1", New: true},
			{ID: "uncategorized:payee-2", New: true},
			{ID: "uncategorized:payee-3"},
		},
	}
}

func Test_sync_and_import_counts_an_ignored_open_finding_as_ignored_not_open_or_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()}, snapshot.WithIgnore([]string{"uncategorized:payee-1"}))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_every_finding_as_open_without_an_ignore_list(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, New: 2}, outcome.Store.Findings)
}

func Test_import_from_counts_an_ignored_open_finding_as_ignored_not_open_or_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()}, snapshot.WithIgnore([]string{"uncategorized:payee-1"}))
	manifest := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, outcome.Store.Findings)
}
