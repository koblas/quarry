package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	emptyPrefix = "quarry: warning: no spending from 2026-01-01 to 2026-09-29"
	emptyE1a    = "no spending from 2026-01-01 to 2026-09-29 in the named accounts; their transactions run 2019-03-02 to 2024-11-30"
)

func Test_spend_says_when_the_window_holds_nothing(t *testing.T) {
	cases := []struct {
		name string
		span store.TransactionRange
		args []string
		want string
	}{
		{
			name: "the store has transactions elsewhere",
			span: span(t, "2003-01-04", "2026-09-26"),
			want: emptyPrefix + "; the store's transactions run 2003-01-04 to 2026-09-26\n",
		},
		{
			name: "the store has no transactions",
			want: emptyPrefix + "; the store has no transactions\n",
		},
		{
			name: "the named accounts have transactions elsewhere",
			span: span(t, "2019-03-02", "2024-11-30"),
			args: []string{"--account", chequingID},
			want: "quarry: warning: " + emptyE1a + "\n",
		},
		{
			name: "the named accounts have no transactions",
			args: []string{"--account", chequingID},
			want: emptyPrefix + " in the named accounts; they have no transactions\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := namedAccounts()
			fake.spending = store.Spending{Transactions: c.span}

			err := executeSpend(t, fake, spendNow, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.want, stderr.String())
		})
	}
}

func Test_spend_json_puts_the_empty_window_note_in_warnings_unprefixed(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2003-01-04", "2026-09-26")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--json")

	require.NoError(t, err)
	assert.JSONEq(t, `{"since":"2026-01-01","until":"2026-09-29","by":"category","currency":"CAD","account_filter":[],
		"rows":[],"totals":[],"warnings":["no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2026-09-26"]}`,
		stdout.String())
}

func Test_spend_says_nothing_of_an_empty_window_when_every_named_account_is_left_out_of_reports(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", oldBankID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+leftOutWarning("Old Bank")+"\n",
		stderr.String())
}

func Test_spend_says_nothing_of_an_empty_window_when_every_named_account_is_left_out_and_one_is_linked(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSpend(t, namedAccounts(), spendNow, &stdout, &stderr, "--account", "Old Card", "--account", linkedID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+linkedTrackingWarning("Netskope 401(k)")+"\n",
		stderr.String())
}

func Test_spend_puts_the_empty_window_note_after_the_linked_tracking_warning_when_a_reported_account_is_named(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2019-03-02", "2024-11-30")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr, "--account", linkedID, "--account", chequingID)

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+linkedTrackingWarning("Netskope 401(k)")+"\nquarry: warning: "+emptyE1a+"\n", stderr.String())
}

func Test_spend_puts_the_empty_window_note_after_the_left_out_of_reports_warnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Transactions: span(t, "2019-03-02", "2024-11-30")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr,
		"--account", "Old Card", "--account", chequingID, "--json")

	require.NoError(t, err)
	assert.Equal(t, "quarry: warning: "+leftOutWarning("Old Card")+"\nquarry: warning: "+emptyE1a+"\n", stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, []string{leftOutWarning("Old Card"), emptyE1a}, doc.Warnings)
}

func Test_spend_says_nothing_of_an_empty_window_when_a_currency_nets_to_zero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	fake := namedAccounts()
	fake.spending = store.Spending{Totals: []store.SpendingTotal{{Currency: "CAD", Spent: 0}}, Transactions: span(t, "2003-01-04", "2026-09-26")}

	err := executeSpend(t, fake, spendNow, &stdout, &stderr)

	require.NoError(t, err)
	assert.Empty(t, stderr.String())
}
