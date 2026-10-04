// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cash-only investment rows keep the share gate out of the way.
func Test_run_accounts_shows_not_valued_for_brokerage_and_retirement_accounts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokerage := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	ira := b.Account(v9fixture.AccountRow{Name: "IRA", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	addTransaction(b, chequing, "100.00", day)
	dividend := new(int64(10))
	for _, account := range []int64{brokerage, ira} {
		pk := b.InvestmentTransaction(v9fixture.TransactionRow{Account: account, PostedDate: &day, Type: dividend, Amount: "12.00"})
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "12.00"})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	var stdout, stderr, jsonOut bytes.Buffer

	textCode := run(context.Background(), []string{"accounts", "--currency", "native"}, &stdout, &stderr)
	jsonCode := run(context.Background(), []string{"accounts", "--json", "--currency", "native"}, &jsonOut, &stderr)

	require.Equal(t, 0, textCode, stderr.String())
	require.Equal(t, 0, jsonCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"Account    Type        Currency     Balance  Status\n"+
		"Brokerage  brokerage   CAD       not valued\n"+
		"Chequing   chequing    CAD           100.00\n"+
		"IRA        retirement  CAD       not valued\n",
		stdout.String())
	assert.NotContains(t, stdout.String(), "not imported")
	var doc accountsJSON
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc))
	require.Len(t, doc.Accounts, 3)
	assert.Equal(t, "Brokerage", doc.Accounts[0].Name)
	assert.Nil(t, doc.Accounts[0].Balance)
}
