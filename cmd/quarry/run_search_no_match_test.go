package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_search_with_text_that_matches_nothing_prints_an_empty_result_and_the_no_match_warning(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, searchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "--json", "zzz"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	assert.Equal(t, []searchTransactionJSON{}, doc.Transactions)
	assert.Equal(t, 0, doc.Matched)
	assert.Equal(t, "quarry: warning: no transactions match the search; the store's transactions run 2026-01-20 to 2026-04-02\n", stderr.String())
}

func Test_run_search_with_no_match_says_where_the_searched_transactions_run(t *testing.T) {
	savings := store.Account{ID: "acct-sav", SourceID: 2, Name: "Savings", Type: "savings", Currency: "CAD", Active: true}
	chequingOnly := searchRows([]store.Account{chequingAccount("acct-chq", 1), savings}, nil,
		searchTxn{id: "only", account: "acct-chq", sourceID: 1, day: day(2026, time.May, 4), splits: []searchSplit{{sourceID: 1, cents: -100}}})
	cases := []struct {
		name string
		rows store.Rows
		args []string
		want string
	}{
		{
			name: "the store's span", rows: searchStore(), args: []string{"zzz"},
			want: "no transactions match the search; the store's transactions run 2026-01-20 to 2026-04-02",
		},
		{
			name: "an empty store", rows: searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil), args: []string{"zzz"},
			want: "no transactions match the search; the store has no transactions",
		},
		{
			name: "a named account's span", rows: searchStore(), args: []string{"zzz", "--account", "Chequing"},
			want: "no transactions in the named accounts match the search; their transactions run 2026-02-14 to 2026-04-02",
		},
		{
			name: "a named account with no transactions", rows: chequingOnly, args: []string{"zzz", "--account", "Savings"},
			want: "no transactions in the named accounts match the search; they have no transactions",
		},
		{
			name: "a named left-out account keeps its own span", rows: searchStore(), args: []string{"zzz", "--account", "Old Card"},
			want: "no transactions in the named accounts match the search; their transactions run 2026-02-01 to 2026-02-01",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, c.rows)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"search", "--json"}, c.args...))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, []string{c.want}, decodeSearchJSON(t, stdout.String()).Warnings)
			assert.Equal(t, "quarry: warning: "+c.want+"\n", stderr.String())
		})
	}
}

func Test_run_search_text_with_no_match_prints_the_empty_listing_on_stdout_and_the_warning_on_stderr(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, searchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "zzz"})

	require.Equal(t, 0, exitCode, stderr.String())
	want := "Transactions matching \"zzz\" in all accounts, all dates\n\n" +
		"Date  Account  Payee  Category  Memo  Amount  Flags\n" +
		"\n" +
		"0 matching transactions\n"
	assert.Equal(t, want, stdout.String())
	assert.Equal(t, "quarry: warning: no transactions match the search; the store's transactions run 2026-01-20 to 2026-04-02\n", stderr.String())
}
