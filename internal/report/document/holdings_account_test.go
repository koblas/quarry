package document_test

import (
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	accountTypeChequing = "chequing"
	accountTypeSavings  = "savings"
)

func namedAccount(id, name, accountType string) store.Account {
	return store.Account{ID: id, Name: name, Type: accountType}
}

func notInvestmentLine(name string) string {
	return `account "` + name + `" is not a brokerage or retirement account, so it has no holdings`
}

func Test_HoldingsWarnings_name_each_named_account_that_is_not_an_investment_account(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		want     []string
	}{
		{
			name: "in the table's account order, not the order given",
			accounts: []store.Account{
				namedAccount("a-1", "Zeta Chequing", accountTypeChequing),
				namedAccount("a-2", "Alpha Savings", accountTypeSavings),
			},
			want: []string{notInvestmentLine("Alpha Savings"), notInvestmentLine("Zeta Chequing"), namedNothingLine},
		},
		{
			name: "by plain name, so a lower-case name follows an upper-case one",
			accounts: []store.Account{
				namedAccount("a-1", "alpha", accountTypeChequing),
				namedAccount("a-2", "Zeta", accountTypeSavings),
			},
			want: []string{notInvestmentLine("Zeta"), notInvestmentLine("alpha"), namedNothingLine},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.HoldingsWarnings(report.Holdings{Accounts: c.accounts, Currency: money.CAD, AsOf: emptyAsOf})

			assert.Equal(t, c.want, got)
		})
	}
}

func Test_HoldingsWarnings_keep_the_given_order_of_the_named_accounts_for_the_caller(t *testing.T) {
	zeta, alpha := namedAccount("a-1", "Zeta", accountTypeChequing), namedAccount("a-2", "Alpha", accountTypeChequing)
	h := report.Holdings{Accounts: []store.Account{zeta, alpha}, Currency: money.CAD}

	_ = document.HoldingsWarnings(h)

	assert.Equal(t, []store.Account{zeta, alpha}, h.Accounts)
}

func Test_HoldingsWarnings_leave_out_a_named_investment_account_whatever_its_state(t *testing.T) {
	closed := namedAccount("a-3", "Old Brokerage", store.AccountTypeBrokerage)
	closed.Closed = true
	hidden := namedAccount("a-4", "Hidden Brokerage", store.AccountTypeBrokerage)
	hidden.NotInReports = true
	h := report.Holdings{
		Rows:     []store.Holding{holdingsTestRow()},
		Accounts: []store.Account{namedAccount("a-1", "Brokerage", store.AccountTypeBrokerage), namedAccount("a-2", "RRSP", store.AccountTypeRetirement), closed, hidden},
		Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	assert.Empty(t, got)
}

func Test_HoldingsWarnings_put_the_non_investment_line_before_the_no_price_line(t *testing.T) {
	row := holdingsTestRow()
	row.Price = nil
	h := report.Holdings{
		Rows:     []store.Holding{row},
		Accounts: []store.Account{namedAccount("a-1", "Chequing", accountTypeChequing), namedAccount(row.AccountID, row.Account, store.AccountTypeBrokerage)},
		Currency: money.CAD,
	}

	got := document.HoldingsWarnings(h)

	require.Len(t, got, 2)
	assert.Equal(t, notInvestmentLine("Chequing"), got[0])
	assert.Contains(t, got[1], "no price on or before")
}

func Test_holdings_json_account_filter_lists_the_named_accounts(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		want     string
	}{
		{
			name: "in the order given",
			accounts: []store.Account{
				namedAccount("a-2", "Zeta", store.AccountTypeBrokerage),
				namedAccount("a-1", "Alpha", accountTypeChequing),
			},
			want: `[{"id":"a-2","name":"Zeta"},{"id":"a-1","name":"Alpha"}]`,
		},
		{name: "as [] when none is named", accounts: nil, want: `[]`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got struct {
				AccountFilter json.RawMessage `json:"account_filter"`
			}

			require.NoError(t, json.Unmarshal([]byte(indented(t, document.NewHoldings(report.Holdings{Accounts: c.accounts, Currency: money.CAD}, nil))), &got))

			assert.JSONEq(t, c.want, string(got.AccountFilter))
		})
	}
}
