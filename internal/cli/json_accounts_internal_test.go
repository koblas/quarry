// White-box: renderAccountsJSON's null forms and as_of handling are unexported
// formatting rules best driven directly over a store.AccountList.
package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderAccountsJSON_renders_every_field_of_every_account(t *testing.T) {
	list := store.AccountList{
		AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		Accounts: []store.AccountBalance{
			{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Institution: new("First Bank"), Active: true, LinkedTracking: true, Balance: 1234567},
			{ID: "acct-2", Name: "Visa", Type: "credit_card", Currency: "CAD", Closed: true, Active: true, NotInReports: true, Balance: -120417},
			{ID: "acct-3", Name: "Old Savings", Type: "savings", Currency: "USD", Balance: 0},
			{ID: "acct-4", Name: "RRSP", Type: "retirement", Currency: "CAD", Institution: new("First Bank"), Active: true},
		},
	}

	got, err := renderAccountsJSON(report.AccountListing{AccountList: list}, []string{})

	require.NoError(t, err)
	want := `{
  "as_of": "2026-09-29",
  "currency": "native",
  "accounts": [
    {
      "id": "acct-1",
      "name": "Chequing",
      "type": "chequing",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "in_reports": true,
      "linked_tracking": true,
      "balance": "12345.67",
      "converted_balance": null
    },
    {
      "id": "acct-2",
      "name": "Visa",
      "type": "credit_card",
      "currency": "CAD",
      "institution": null,
      "closed": true,
      "active": true,
      "in_reports": false,
      "linked_tracking": false,
      "balance": "-1204.17",
      "converted_balance": null
    },
    {
      "id": "acct-3",
      "name": "Old Savings",
      "type": "savings",
      "currency": "USD",
      "institution": null,
      "closed": false,
      "active": false,
      "in_reports": true,
      "linked_tracking": false,
      "balance": "0.00",
      "converted_balance": null
    },
    {
      "id": "acct-4",
      "name": "RRSP",
      "type": "retirement",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "in_reports": true,
      "linked_tracking": false,
      "balance": "0.00",
      "converted_balance": null
    }
  ],
  "warnings": []
}
`
	assert.Equal(t, want, string(got))
}

func Test_renderAccountsJSON_renders_no_accounts_as_an_empty_list(t *testing.T) {
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(report.AccountListing{AccountList: list}, []string{})

	require.NoError(t, err)
	assert.Equal(t, "{\n  \"as_of\": \"2026-09-29\",\n  \"currency\": \"native\",\n  \"accounts\": [],\n  \"warnings\": []\n}\n", string(got)) //nolint:testifylint // bytes are the contract
}

func Test_renderAccountsJSON_carries_the_warnings(t *testing.T) {
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(report.AccountListing{AccountList: list}, []string{"all 3 accounts are closed; pass --all to list them"})

	require.NoError(t, err)
	assert.Contains(t, string(got), "  \"warnings\": [\n    \"all 3 accounts are closed; pass --all to list them\"\n  ]\n")
}

func Test_renderAccountsJSON_renders_an_empty_institution_as_null(t *testing.T) {
	list := store.AccountList{
		AsOf:     time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		Accounts: []store.AccountBalance{{ID: "acct-1", Institution: new("")}},
	}

	got, err := renderAccountsJSON(report.AccountListing{AccountList: list}, []string{})

	require.NoError(t, err)
	assert.Contains(t, string(got), "\"institution\": null,")
}

func Test_renderAccountsJSON_keeps_as_of_as_the_stored_day_in_any_local_zone(t *testing.T) {
	useZone(t, time.FixedZone("EDT", -4*60*60))
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(report.AccountListing{AccountList: list}, []string{})

	require.NoError(t, err)
	assert.Contains(t, string(got), "\"as_of\": \"2026-09-29\"")
}

func Test_allClosedNote(t *testing.T) {
	cases := []struct {
		name   string
		hidden int
		want   string
	}{
		{name: "exactly one account", hidden: 1, want: "the only account is closed; pass --all to list it"},
		{name: "exactly two accounts", hidden: 2, want: "all 2 accounts are closed; pass --all to list them"},
		{name: "several accounts", hidden: 3, want: "all 3 accounts are closed; pass --all to list them"},
		{name: "a count with a thousands separator", hidden: 1204, want: "all 1,204 accounts are closed; pass --all to list them"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, allClosedNote(c.hidden))
		})
	}
}

func jsonListing(currency money.Currency) report.AccountListing {
	usd := store.AccountBalance{
		ID: "acct-usd", Name: "US Chequing", Type: "chequing", Currency: "USD", Active: true,
		Balance: 800, BalanceCAD: new(int64(1000)), BalanceUSD: new(int64(800)),
	}
	brokerage := store.AccountBalance{ID: "acct-brk", Name: "Brokerage", Type: "brokerage", Currency: "USD", Active: true}
	return report.AccountListing{
		AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Accounts: []store.AccountBalance{usd, brokerage},
		Currency: currency,
	}
}

func Test_renderAccountsJSON_puts_currency_after_as_of_and_converted_balance_after_balance_in_every_mode(t *testing.T) {
	topLevel := []string{"as_of", "currency", "accounts", "warnings"}
	row := []string{"id", "name", "type", "currency", "institution", "closed", "active", "in_reports", "linked_tracking", "balance", "converted_balance"}
	cases := []struct {
		name     string
		currency money.Currency
	}{
		{name: "CAD", currency: money.CAD},
		{name: "USD", currency: money.USD},
		{name: "native", currency: money.Native},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := renderAccountsJSON(jsonListing(c.currency), []string{})

			require.NoError(t, err)
			assert.Equal(t, topLevel, topLevelKeys(t, got))
			var doc struct {
				Accounts []json.RawMessage `json:"accounts"`
			}
			require.NoError(t, json.Unmarshal(got, &doc))
			assert.Equal(t, row, topLevelKeys(t, doc.Accounts[0]))
			assert.Equal(t, row, topLevelKeys(t, doc.Accounts[1]))
		})
	}
}

func Test_renderAccountsJSON_reads_back_each_balance_and_its_converted_balance(t *testing.T) {
	cases := []struct {
		name     string
		currency money.Currency
		want     []*string
	}{
		{name: "CAD", currency: money.CAD, want: []*string{new("10.00"), nil}},
		{name: "USD", currency: money.USD, want: []*string{new("8.00"), nil}},
		{name: "native leaves every converted balance null", currency: money.Native, want: []*string{nil, nil}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := renderAccountsJSON(jsonListing(c.currency), []string{"a warning"})

			require.NoError(t, err)
			var doc struct {
				AsOf     string `json:"as_of"`
				Currency string `json:"currency"`
				Accounts []struct {
					Balance          *string `json:"balance"`
					ConvertedBalance *string `json:"converted_balance"`
				} `json:"accounts"`
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(got, &doc))
			assert.Equal(t, c.currency.String(), doc.Currency)
			require.Len(t, doc.Accounts, 2)
			assert.Equal(t, []*string{new("8.00"), new("0.00")}, []*string{doc.Accounts[0].Balance, doc.Accounts[1].Balance})
			assert.Equal(t, c.want, []*string{doc.Accounts[0].ConvertedBalance, doc.Accounts[1].ConvertedBalance})
			assert.Equal(t, []string{"a warning"}, doc.Warnings)
		})
	}
}

func Test_renderAccountsJSON_leaves_the_converted_balance_null_for_an_imported_balance_no_rate_converts(t *testing.T) {
	list := jsonListing(money.CAD)
	list.Accounts[0].BalanceCAD = nil

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	assert.Contains(t, string(got), "\"balance\": \"8.00\",\n      \"converted_balance\": null\n")
}

func Test_renderAccountsJSON_renders_no_accounts_in_the_reporting_currency_as_an_empty_list(t *testing.T) {
	list := report.AccountListing{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Currency: money.USD}

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	assert.Equal(t, "{\n  \"as_of\": \"2026-09-29\",\n  \"currency\": \"USD\",\n  \"accounts\": [],\n  \"warnings\": []\n}\n", string(got)) //nolint:testifylint // bytes are the contract
}
