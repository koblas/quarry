// White-box: accountsFXWarnings is unexported; the line's exact wording is the rule.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_accountsFXWarnings_says_why_a_row_shows_no_rate(t *testing.T) {
	asOf := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	first := time.Date(2099, time.January, 2, 0, 0, 0, 0, time.UTC)
	usd := store.AccountBalance{Currency: "USD", Balance: new(int64(800)), BalanceUSD: new(int64(800))}
	cad := store.AccountBalance{Currency: "CAD", Balance: new(int64(800)), BalanceCAD: new(int64(800))}
	cases := []struct {
		name     string
		currency money.Currency
		account  store.AccountBalance
		first    time.Time
		want     string
	}{
		{
			name: "no rates in CAD", currency: money.CAD, account: usd,
			want: "the store has no exchange rates, so USD balances show no rate in the In CAD column; run quarry sync to fetch them",
		},
		{
			name: "no rates in USD", currency: money.USD, account: cad,
			want: "the store has no exchange rates, so CAD balances show no rate in the In USD column; run quarry sync to fetch them",
		},
		{
			name: "rates only after today in CAD", currency: money.CAD, account: usd, first: first,
			want: "the first exchange rate in the store, 2099-01-02, is dated after today, so USD balances show no rate in the In CAD column; check the Mac's date and time",
		},
		{
			name: "rates only after today in USD", currency: money.USD, account: cad, first: first,
			want: "the first exchange rate in the store, 2099-01-02, is dated after today, so CAD balances show no rate in the In USD column; check the Mac's date and time",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := report.AccountListing{AsOf: asOf, FirstRate: c.first, Accounts: []store.AccountBalance{c.account}, Currency: c.currency}

			assert.Equal(t, []string{c.want}, accountsFXWarnings(l))
		})
	}
}

func Test_accountsFXWarnings_is_silent_unless_a_listed_row_shows_no_rate(t *testing.T) {
	asOf := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	past := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	future := time.Date(2099, time.January, 2, 0, 0, 0, 0, time.UTC)
	cad := store.AccountBalance{Currency: "CAD", Balance: new(int64(800)), BalanceCAD: new(int64(800))}
	usdNoRate := store.AccountBalance{Currency: "USD", Balance: new(int64(800))}
	notImported := store.AccountBalance{Currency: "USD"}
	cases := []struct {
		name     string
		currency money.Currency
		first    time.Time
		accounts []store.AccountBalance
	}{
		{name: "an all-CAD store with no rates in CAD", currency: money.CAD, accounts: []store.AccountBalance{cad}},
		{name: "a not imported cross-currency account with no rates", currency: money.CAD, accounts: []store.AccountBalance{notImported}},
		{name: "a not imported cross-currency account with rates only after today", currency: money.CAD, first: future, accounts: []store.AccountBalance{notImported}},
		{name: "no accounts with no rates", currency: money.CAD},
		{name: "native with no rates", currency: money.Native, accounts: []store.AccountBalance{usdNoRate}},
		{name: "a rate on or before today does not explain the missing cell", currency: money.CAD, first: past, accounts: []store.AccountBalance{usdNoRate}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := report.AccountListing{AsOf: asOf, FirstRate: c.first, Accounts: c.accounts, Currency: c.currency}

			assert.Empty(t, accountsFXWarnings(l))
		})
	}
}
