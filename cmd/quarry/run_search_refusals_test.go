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

// refusalSearchStore holds two accounts named Visa, a chequing account and the categories Auto:Fuel and Food:Groceries.
func refusalSearchStore() store.Rows {
	return searchRows([]store.Account{
		chequingAccount("acct-chq", 1),
		{ID: "acct-812", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
		{ID: "acct-977", SourceID: 3, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
	}, nil, searchTxn{
		id: "one", account: "acct-chq", sourceID: 1, day: day(2026, time.March, 1),
		splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -1000}},
	})
}

func Test_run_search_refuses_bad_input_with_the_ruled_line_and_exit_code(t *testing.T) {
	const (
		notAmount       = "; use digits with up to 2 decimals and no sign, such as 25 or 19.99\n"
		unknownCategory = "; list them with quarry sql \"SELECT full_path FROM categories ORDER BY full_path\"\n"
		usage           = "; Run 'quarry search --help' for usage.\n"
	)
	cases := []struct {
		name string
		args []string
		exit int
		want string
	}{
		{name: "two texts", args: []string{"costco", "visa"}, exit: 2, want: "quarry: search takes one text; quote it as one argument\n"},
		{name: "a negative limit", args: []string{"--limit", "-1"}, exit: 2, want: "quarry: --limit must be 0 or more; 0 prints every transaction\n"},
		{name: "an empty text", args: []string{""}, exit: 2, want: "quarry: search text is blank; leave it out to search by date, account, category or amount alone\n"},
		{name: "a whitespace text", args: []string{"  "}, exit: 2, want: "quarry: search text is blank; leave it out to search by date, account, category or amount alone\n"},
		{name: "a signed min", args: []string{"--min", "-12"}, exit: 2, want: `quarry: --min "-12" is not an amount` + notAmount},
		{name: "a max that is not a number", args: []string{"--max", "abc"}, exit: 2, want: `quarry: --max "abc" is not an amount` + notAmount},
		{name: "a min above the max", args: []string{"--min", "50", "--max", "20"}, exit: 2, want: "quarry: --min 50 is more than --max 20\n"},
		{name: "a since that is not a date", args: []string{"--since", "2024-13"}, exit: 2, want: "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"},
		{name: "a since after the until", args: []string{"--since", "2025", "--until", "2024"}, exit: 2, want: "quarry: --since 2025 is after --until 2024\n"},
		{name: "an account no account is named", args: []string{"--account", "Nope"}, exit: 1, want: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n"},
		{name: "an account name two accounts share", args: []string{"--account", "Visa"}, exit: 1, want: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n"},
		{name: "an empty account", args: []string{"--account", ""}, exit: 1, want: "quarry: no account named \"\"; run quarry accounts --all to list them\n"},
		{name: "a category no category is named", args: []string{"--category", "Fod"}, exit: 1, want: `quarry: no category named "Fod"` + unknownCategory},
		{name: "an empty category", args: []string{"--category", ""}, exit: 1, want: `quarry: no category named ""` + unknownCategory},
		{name: "a --currency flag", args: []string{"--currency", "CAD"}, exit: 2, want: "quarry: unknown flag: --currency" + usage},
		{name: "a --csv flag", args: []string{"--csv"}, exit: 2, want: "quarry: unknown flag: --csv" + usage},
	}

	for _, c := range cases {
		for _, mode := range []struct {
			name string
			args []string
		}{{"as text", nil}, {"with --json", []string{"--json"}}} {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				replaceStore(t, home, refusalSearchStore())

				assertSearchFailed(t, append(append([]string{"search"}, c.args...), mode.args...), c.exit, c.want)
			})
		}
	}
}

// assertSearchFailed runs args against the store under $HOME and requires wantExit, nothing on stdout and want on stderr.
func assertSearchFailed(t *testing.T, args []string, wantExit int, want string) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), args, spendEnv(&stdout, &stderr))

	require.Equal(t, wantExit, exitCode, stderr.String())
	assert.Equal(t, want, stderr.String())
	assert.Empty(t, stdout.String())
}
