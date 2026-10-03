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

func Test_run_search_json_echoes_min_and_max_normalized_and_null_when_absent(t *testing.T) {
	cases := []struct {
		name             string
		args             []string
		wantMin, wantMax *string
	}{
		{name: "neither given", args: nil},
		{name: "min only is normalized to two decimals", args: []string{"--min", "12.5"}, wantMin: new("12.50")},
		{name: "max only", args: []string{"--max", "7"}, wantMax: new("7.00")},
		{name: "both", args: []string{"--min", "1", "--max", "9.99"}, wantMin: new("1.00"), wantMax: new("9.99")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, amountSearchStore(), c.args...)

			assert.Equal(t, c.wantMin, doc.Min)
			assert.Equal(t, c.wantMax, doc.Max)
		})
	}
}

func Test_run_search_text_names_the_amount_range_in_the_caption(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStore(t, home, amountSearchStore())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"search", "--min", "100", "--max", "1000"}, spendEnv(&stdout, &stderr))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Transactions in all accounts, all dates, amount 100.00 to 1,000.00\n")
}

const useDigits = "use digits with up to 2 decimals and no sign, such as 25 or 19.99"

func Test_run_search_refuses_a_bad_amount_with_the_ruled_line_before_opening_the_store(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "a signed min", args: []string{"--min", "-12"}, want: `quarry: --min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "a grouped max", args: []string{"--max", "1,234.56"}, want: `quarry: --max "1,234.56" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "an empty min is refused, not ignored", args: []string{"--min", ""}, want: `quarry: --min "" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "an empty max", args: []string{"--max", ""}, want: `quarry: --max "" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99`},
		{name: "seventeen integer digits", args: []string{"--min", "12345678901234567"}, want: `quarry: --min "12345678901234567" is not an amount; ` + useDigits},
		{name: "a min above the max", args: []string{"--min", "50", "--max", "20"}, want: "quarry: --min 50 is more than --max 20"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSearchRefused(t, append([]string{"search"}, c.args...), c.want+"\n")
		})
	}
}

func Test_run_search_refuses_min_above_max_before_a_bad_since(t *testing.T) {
	assertSearchRefused(t, []string{"search", "--min", "50", "--max", "20", "--since", "nonsense"}, "quarry: --min 50 is more than --max 20\n")
}

func Test_run_search_refuses_a_bad_min_before_a_bad_since_and_before_a_bad_max(t *testing.T) {
	const badMin = "quarry: --min \"-12\" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99\n"
	const badMax = "quarry: --max \"abc\" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99\n"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "a bad min beats a bad since", args: []string{"search", "--min", "-12", "--since", "nonsense"}, want: badMin},
		{name: "a bad min beats a bad max", args: []string{"search", "--max", "abc", "--min", "-12"}, want: badMin},
		{name: "a bad max beats a bad since", args: []string{"search", "--max", "abc", "--since", "nonsense"}, want: badMax},
		{name: "a bad max beats a min above the max", args: []string{"search", "--min", "50", "--max", "abc"}, want: badMax},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertSearchRefused(t, c.args, c.want)
		})
	}
}

func Test_run_search_refuses_blank_text_before_a_bad_min(t *testing.T) {
	assertSearchRefused(t, []string{"search", "", "--min", "-12"},
		"quarry: search text is blank; leave it out to search by date, account, category or amount alone\n")
}

// assertSearchRefused runs args against an empty home and requires exit 2, nothing on stdout and want on stderr.
func assertSearchRefused(t *testing.T, args []string, want string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), args, spendEnv(&stdout, &stderr))

	assert.Equal(t, 2, exitCode)
	assert.Equal(t, want, stderr.String())
	assert.Empty(t, stdout.String())
}
