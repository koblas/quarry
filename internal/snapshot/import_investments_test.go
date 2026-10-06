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

func Test_sync_and_import_hands_the_read_time_function_the_investments_the_build_wrote(t *testing.T) {
	t.Parallel()
	result := threeStatesWithAccount()
	result.Investments = store.Investments{
		Securities:   []store.Security{{ID: "sec-1", Name: "Acme Corp"}},
		Transactions: []store.InvestmentTransaction{{ID: "itxn-1", AccountID: "acct-1", SecurityID: new("sec-1"), Action: store.ActionAddShares}},
	}
	var got store.FindingList
	srv := newImportServer(t, t.TempDir(), &fakeImporter{result: result},
		snapshot.WithReadTimeFindings(func(list store.FindingList) []finding.State { got = list; return nil }))

	_, err := srv.SyncAndImport(t.Context(), v9fixture.OpenBundle(t, t.TempDir()).Dir)

	require.NoError(t, err)
	assert.Equal(t, result.Investments, got.Investments)
}
