package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_search_without_since_or_until_searches_every_date(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-chq", 1)},
		chargeTxn{id: "old", account: "acct-chq", currency: "CAD", day: day(2025, time.December, 31), splits: []chargeSplit{{cents: -1000}}},
		chargeTxn{id: "future", account: "acct-chq", currency: "CAD", day: day(2027, time.January, 15), splits: []chargeSplit{{cents: -2000}}},
	))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"search", "--json"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	require.Len(t, doc.Transactions, 2)
	assert.Equal(t, []string{"txn-future", "txn-old"}, []string{doc.Transactions[0].TransactionID, doc.Transactions[1].TransactionID})
	assert.Nil(t, doc.Since)
	assert.Nil(t, doc.Until)
}
