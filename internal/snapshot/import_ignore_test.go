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

const unclassifiedID = "unclassified-account:acct-1"

// threeStatesWithAccount is threeStates with the one account a build wrote.
func threeStatesWithAccount() store.Result {
	result := threeStates()
	result.Accounts = []store.Account{{ID: "acct-1", Type: store.AccountTypeBrokerage}}
	return result
}

// readTimeStates is a read-time function computing one unclassified finding for each account it is given.
func readTimeStates(list store.FindingList) []finding.State {
	states := make([]finding.State, 0, len(list.Accounts))
	for _, a := range list.Accounts {
		states = append(states, finding.State{ID: "unclassified-account:" + a.ID})
	}
	return states
}

func Test_sync_and_import_counts_a_read_time_finding_open_and_never_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()}, snapshot.WithReadTimeFindings(readTimeStates))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 4, New: 2}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_the_same_without_a_read_time_function_whatever_accounts_the_build_wrote(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()})

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, New: 2}, outcome.Store.Findings)
}

func Test_sync_and_import_counts_an_ignored_read_time_finding_as_ignored(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()},
		snapshot.WithReadTimeFindings(readTimeStates), snapshot.WithIgnore([]string{unclassifiedID}))

	outcome, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 3, Ignored: 1, New: 2}, outcome.Store.Findings)
}

func Test_import_from_counts_a_read_time_finding_open_and_never_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStatesWithAccount()}, snapshot.WithReadTimeFindings(readTimeStates))
	manifest := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 4, New: 2}, outcome.Store.Findings)
}

func Test_import_from_counts_an_ignored_open_finding_as_ignored_not_open_or_new(t *testing.T) {
	t.Parallel()
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: threeStates()}, snapshot.WithIgnore([]string{"uncategorized:payee-1"}))
	manifest := takeSnapshot(t, srv)

	outcome, err := srv.ImportFrom(t.Context(), snapshotIDFromPath(manifest.Snapshot.Path))

	require.NoError(t, err)
	assert.Equal(t, finding.Counts{Open: 2, Ignored: 1, New: 1}, outcome.Store.Findings)
}
