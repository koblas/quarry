// White-box: pairTransfers always hands describeOneSided its legs in split
// order, so no snapshot can present two tied legs to its sort out of order.
package importer

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_describeOneSided_orders_legs_of_one_transaction_by_split_source_id(t *testing.T) {
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
