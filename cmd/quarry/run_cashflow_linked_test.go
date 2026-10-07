// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_cashflow_leaves_out_accounts_that_use_linked_account_tracking(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{
			chequingAccount("acct-chq", 1),
			{ID: "acct-401k", SourceID: 2, Name: "Netskope 401(k)", Type: "retirement", Currency: "CAD", Active: true, LinkedTracking: true},
		},
		spendSplit{id: "s01", account: "acct-chq", category: "cat-salary", currency: "CAD", day: day(2026, 3, 1), cents: 50000},
		spendSplit{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12000},
		spendSplit{id: "s03", account: "acct-401k", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -90000},
		spendSplit{id: "s04", account: "acct-401k", currency: "CAD", day: day(2026, 3, 12), cents: 40000},
	))
	period := []string{"--since", "2026-01", "--until", "2026-09"}
	var spendOut, cashFlowOut, stderr bytes.Buffer

	spendExit := runWith(context.Background(), append([]string{"spend"}, period...), spendEnv(&spendOut, &stderr))
	cashFlowExit := runWith(context.Background(), append([]string{"cashflow"}, period...), spendEnv(&cashFlowOut, &stderr))

	require.Equal(t, 0, spendExit, stderr.String())
	require.Equal(t, 0, cashFlowExit, stderr.String())
	assert.Equal(t, map[string]string{"CAD": "500.00"}, totalsColumn(cashFlowOut.String(), 2))
	assert.Equal(t, map[string]string{"CAD": "120.00"}, totalsColumn(cashFlowOut.String(), 3))
	assert.Equal(t, map[string]string{"CAD": "120.00"}, totalsColumn(spendOut.String(), 2))
	assert.Empty(t, stderr.String())
}
