// White-box: renderAccountsJSON's null forms and as_of handling are unexported
// formatting rules best driven directly over a store.AccountList.
package cli

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderAccountsJSON_renders_every_field_of_every_account(t *testing.T) {
	list := store.AccountList{
		AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		Accounts: []store.AccountBalance{
			{ID: "acct-1", Name: "Chequing", Type: "chequing", Currency: "CAD", Institution: new("First Bank"), Active: true, Balance: new(int64(1234567))},
			{ID: "acct-2", Name: "Visa", Type: "credit_card", Currency: "CAD", Closed: true, Active: true, Balance: new(int64(-120417))},
			{ID: "acct-3", Name: "Old Savings", Type: "savings", Currency: "USD", Balance: new(int64(0))},
			{ID: "acct-4", Name: "RRSP", Type: "retirement", Currency: "CAD", Institution: new("First Bank"), Active: true},
		},
	}

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	want := `{
  "as_of": "2026-09-29",
  "accounts": [
    {
      "id": "acct-1",
      "name": "Chequing",
      "type": "chequing",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "balance": "12345.67"
    },
    {
      "id": "acct-2",
      "name": "Visa",
      "type": "credit_card",
      "currency": "CAD",
      "institution": null,
      "closed": true,
      "active": true,
      "balance": "-1204.17"
    },
    {
      "id": "acct-3",
      "name": "Old Savings",
      "type": "savings",
      "currency": "USD",
      "institution": null,
      "closed": false,
      "active": false,
      "balance": "0.00"
    },
    {
      "id": "acct-4",
      "name": "RRSP",
      "type": "retirement",
      "currency": "CAD",
      "institution": "First Bank",
      "closed": false,
      "active": true,
      "balance": null
    }
  ],
  "warnings": []
}
`
	assert.Equal(t, want, string(got))
}

func Test_renderAccountsJSON_renders_no_accounts_as_an_empty_list(t *testing.T) {
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	assert.Equal(t, "{\n  \"as_of\": \"2026-09-29\",\n  \"accounts\": [],\n  \"warnings\": []\n}\n", string(got)) //nolint:testifylint // bytes are the contract
}

func Test_renderAccountsJSON_carries_the_warnings(t *testing.T) {
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(list, []string{"all 3 accounts are closed; pass --all to list them"})

	require.NoError(t, err)
	assert.Contains(t, string(got), "  \"warnings\": [\n    \"all 3 accounts are closed; pass --all to list them\"\n  ]\n")
}

func Test_renderAccountsJSON_renders_an_empty_institution_as_null(t *testing.T) {
	list := store.AccountList{
		AsOf:     time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC),
		Accounts: []store.AccountBalance{{ID: "acct-1", Institution: new("")}},
	}

	got, err := renderAccountsJSON(list, []string{})

	require.NoError(t, err)
	assert.Contains(t, string(got), "\"institution\": null,")
}

func Test_renderAccountsJSON_keeps_as_of_as_the_stored_day_in_any_local_zone(t *testing.T) {
	useZone(t, time.FixedZone("EDT", -4*60*60))
	list := store.AccountList{AsOf: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)}

	got, err := renderAccountsJSON(list, []string{})

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
