// White-box: pairTransfers always hands describeOneSided its legs in split
// order, and builds rows and counts together, so no snapshot can present two
// tied legs out of order or checkTransferTotals a disagreeing pair.
package importer

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_describeOneSided_orders_legs_of_one_transaction_by_split_source_id(t *testing.T) {
	t.Parallel()
	rows := store.Rows{
		Accounts:     []store.Account{{ID: "acct-1", SourceID: 1, Name: "Chequing", Currency: "CAD", Active: true}},
		Transactions: []store.Transaction{{ID: "txn-1", SourceID: 1, AccountID: "acct-1", Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}},
		Splits: []store.Split{
			{ID: "split-9", SourceID: 9, TransactionID: "txn-1"},
			{ID: "split-10", SourceID: 10, TransactionID: "txn-1"},
		},
	}
	legs := []store.OneSidedTransfer{{ID: "xfer-10", SourceID: 10}, {ID: "xfer-9", SourceID: 9}}

	got := describeOneSided(newRowIndex(rows), rows.Splits, legs)

	assert.Equal(t, []string{"xfer-9", "xfer-10"}, []string{got[0].ID, got[1].ID})
}

func Test_checkTransferTotals_accepts_rows_that_are_paired_plus_one_sided(t *testing.T) {
	t.Parallel()
	rows := []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}, {ID: "xfer-3"}}
	check := store.TransferCheck{Paired: 2, OneSided: []store.OneSidedTransfer{{ID: "xfer-3"}}}

	err := checkTransferTotals(rows, check)

	require.NoError(t, err)
}

func Test_checkTransferTotals_refuses_rows_that_are_not_paired_plus_one_sided(t *testing.T) {
	t.Parallel()
	oneSided := []store.OneSidedTransfer{{ID: "xfer-3"}}
	cases := []struct {
		name  string
		rows  []store.Transfer
		check store.TransferCheck
	}{
		{
			name:  "a row neither paired nor one-sided",
			rows:  []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}, {ID: "xfer-3"}, {ID: "xfer-4"}},
			check: store.TransferCheck{Paired: 2, OneSided: oneSided},
		},
		{
			name:  "a count with no row behind it",
			rows:  []store.Transfer{{ID: "xfer-1"}, {ID: "xfer-2"}},
			check: store.TransferCheck{Paired: 2, OneSided: oneSided},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := checkTransferTotals(c.rows, c.check)

			assert.ErrorIs(t, err, errTransferTotals)
		})
	}
}
