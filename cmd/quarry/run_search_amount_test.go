package main

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// amountSearchStore: one single-split transaction per amount, dated March 1..10 in the order
// listed, so each bound has a row on the value and a row one cent outside it.
func amountSearchStore() store.Rows {
	amounts := []struct {
		id    string
		cents int64
	}{
		{"charge-150", -15000},
		{"deposit-120", 12000},
		{"charge-99-99", -9999},
		{"charge-20", -2000},
		{"charge-20-01", -2001},
		{"deposit-20", 2000},
		{"deposit-50", 5000},
		{"deposit-50-01", 5001},
		{"deposit-42-17", 4217},
		{"charge-42-17", -4217},
	}
	txns := make([]searchTxn, len(amounts))
	for i, a := range amounts {
		txns[i] = searchTxn{
			id: a.id, account: "acct-chq", sourceID: int64(i + 1), day: day(2026, time.March, i+1),
			splits: []searchSplit{{sourceID: 1, cents: a.cents}},
		}
	}
	return searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, txns...)
}

func Test_run_search_min_and_max_compare_the_amount_without_its_sign(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "min 100 lists the charge and the deposit alike, not 99.99",
			args: []string{"--min", "100"},
			want: []string{"txn-deposit-120", "txn-charge-150"},
		},
		{
			name: "max 20 lists the 20.00 charge and deposit, not 20.01",
			args: []string{"--max", "20"},
			want: []string{"txn-deposit-20", "txn-charge-20"},
		},
		{
			name: "min 20 and max 50 are both inclusive",
			args: []string{"--min", "20", "--max", "50"},
			want: []string{"txn-charge-42-17", "txn-deposit-42-17", "txn-deposit-50", "txn-deposit-20", "txn-charge-20-01", "txn-charge-20"},
		},
		{
			name: "min and max both 42.17 list exactly that amount in either sign",
			args: []string{"--min", "42.17", "--max", "42.17"},
			want: []string{"txn-charge-42-17", "txn-deposit-42-17"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, amountSearchStore(), c.args...)

			assert.Equal(t, c.want, transactionIDs(doc))
			assert.Equal(t, len(c.want), doc.Matched)
		})
	}
}
