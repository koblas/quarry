package duckstore_test

import (
	"os"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func realStore(dir string) *duckstore.Store { return duckstore.New(dir) }

func faultyStore(f *faultDB) func(dir string) *duckstore.Store {
	return func(dir string) *duckstore.Store { return newFaultStore(dir, f) }
}

func Test_replace_keeps_the_previous_store_when_holding_shares_cannot_be_loaded(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		txns     []store.InvestmentTransaction
		newStore func(dir string) *duckstore.Store
		want     string
	}{
		{
			name:     "walk query",
			txns:     []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)},
			newStore: faultyStore(&faultDB{queryFaultOn: duckstore.HoldingWalkQuery, queryFault: errScratchBoom}),
			want:     "walk holdings",
		},
		{
			name: "append",
			txns: []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare)},
			newStore: faultyStore(&faultDB{appendFaultTable: "holding_shares", appendFault: &duckdbdriver.Error{
				Type: duckdbdriver.ErrorTypeConstraint, Msg: "Constraint Error: Duplicate key violates primary key constraint.",
			}}),
			want: "load holding_shares",
		},
		{
			name:     "split with a zero ratio",
			txns:     []store.InvestmentTransaction{buy(acctOne, secAcme, 1, shareDay, oneShare), splitOf(acctOne, secAcme, 2, shareNext, 0, oneShare)},
			newStore: realStore,
			want:     "split of sec-1 in acct-1",
		},
		{
			name: "split pushing the count one millionth past the column",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, shareDay, 333_333_333_333_333_334), splitOf(acctOne, secAcme, 2, shareNext, 3*oneShare, oneShare),
			},
			newStore: realStore,
			want:     "holding_shares sec-1 in acct-1",
		},
		{
			name: "split pushing a short count one millionth past the column",
			txns: []store.InvestmentTransaction{
				buy(acctOne, secAcme, 1, shareDay, -333_333_333_333_333_334), splitOf(acctOne, secAcme, 2, shareNext, 3*oneShare, oneShare),
			},
			newStore: realStore,
			want:     "holding_shares sec-1 in acct-1",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
			require.NoError(t, err)
			before, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			rows := minimalRows()
			rows.Transactions[0].Amount = 999
			rows.InvestmentTransactions = c.txns

			_, err = c.newStore(dir).Replace(t.Context(), rows)

			require.ErrorContains(t, err, c.want)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Equal(t, []string{"quarry.duckdb"}, direntNames(entries))
			after, err := os.ReadFile(replaced.Path)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func Test_replace_stores_a_share_count_at_the_column_maximum(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, shareDay, 333_333_333_333_333_333), splitOf(acctOne, secAcme, 2, shareNext, 3*oneShare, oneShare),
	}

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(shares AS VARCHAR) FROM holding_shares WHERE to_date IS NULL", "999999999999.999999")
}

func Test_replace_stores_a_short_share_count_at_the_column_minimum(t *testing.T) {
	t.Parallel()
	rows := minimalRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		buy(acctOne, secAcme, 1, shareDay, -333_333_333_333_333_333), splitOf(acctOne, secAcme, 2, shareNext, 3*oneShare, oneShare),
	}

	replaced, err := duckstore.New(t.TempDir()).Replace(t.Context(), rows)

	require.NoError(t, err)
	db := openReadOnly(t, replaced.Path)
	assertScalar(t, db, "SELECT CAST(shares AS VARCHAR) FROM holding_shares WHERE to_date IS NULL", "-999999999999.999999")
}
