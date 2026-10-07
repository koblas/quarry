package main

import (
	"context"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_search_prints_the_newest_500_of_501_matches_unless_limit_0(t *testing.T) {
	const cutLine = "quarry: warning: showing the newest 500 of 501 matching transactions; pass --limit 0 to list every one\n"
	cases := []struct {
		name          string
		args          []string
		wantRows      int
		wantOldest    string
		wantTruncated bool
		wantStderr    string
	}{
		{name: "default limit drops the oldest", args: []string{"search", "--json"}, wantRows: 500, wantOldest: "txn-n002", wantTruncated: true, wantStderr: cutLine},
		{name: "limit 0 lists every match", args: []string{"search", "--json", "--limit", "0"}, wantRows: 501, wantOldest: "txn-n001"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, manySearchTxns(501)...))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			require.Equal(t, 0, exitCode, stderr.String())
			doc := decodeSearchJSON(t, stdout.String())
			require.Len(t, doc.Transactions, c.wantRows)
			assert.Equal(t, "txn-n501", doc.Transactions[0].TransactionID)
			assert.Equal(t, c.wantOldest, doc.Transactions[c.wantRows-1].TransactionID)
			assert.Equal(t, 501, doc.Matched)
			assert.Equal(t, c.wantTruncated, doc.Truncated)
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// fiveSearchStore: transactions n001..n005 on Chequing, n005 the newest.
func fiveSearchStore() store.Rows {
	return searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, manySearchTxns(5)...)
}

func Test_run_search_text_with_limit_prints_the_newest_rows_with_the_full_match_count_and_the_cut_line(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, fiveSearchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "--limit", "2"})

	require.Equal(t, 0, exitCode, stderr.String())
	want := "Transactions in all accounts, all dates\n\n" +
		"Date        Account         Payee       Category         Memo  Amount  Flags\n" +
		"2025-01-05  Chequing (CAD)  (no payee)  (uncategorized)         -1.00\n" +
		"2025-01-04  Chequing (CAD)  (no payee)  (uncategorized)         -1.00\n" +
		"\n" +
		"5 matching transactions\n"
	assert.Equal(t, want, stdout.String())
	assert.Equal(t, "quarry: warning: showing the newest 2 of 5 matching transactions; pass --limit 0 to list every one\n", stderr.String())
}

func Test_run_search_json_with_limit_carries_the_cut_line_in_warnings_and_on_stderr(t *testing.T) {
	const cutLine = "showing the newest 2 of 5 matching transactions; pass --limit 0 to list every one"
	home := newHome(t)
	replaceStore(t, home, fiveSearchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "--json", "--limit", "2"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	assert.Equal(t, []string{"txn-n005", "txn-n004"}, transactionIDs(doc))
	assert.Equal(t, 5, doc.Matched)
	assert.True(t, doc.Truncated)
	assert.Equal(t, []string{cutLine}, doc.Warnings)
	assert.Equal(t, "quarry: warning: "+cutLine+"\n", stderr.String())
}

func Test_run_search_cuts_only_when_more_transactions_match_than_the_limit(t *testing.T) {
	const cutLine = "quarry: warning: showing the newest 4 of 5 matching transactions; pass --limit 0 to list every one\n"
	cases := []struct {
		name          string
		limit         string
		wantRows      int
		wantTruncated bool
		wantStderr    string
	}{
		{name: "limit equal to the matches lists them all", limit: "5", wantRows: 5},
		{name: "limit one below the matches cuts the oldest", limit: "4", wantRows: 4, wantTruncated: true, wantStderr: cutLine},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, fiveSearchStore())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "--json", "--limit", c.limit})

			require.Equal(t, 0, exitCode, stderr.String())
			doc := decodeSearchJSON(t, stdout.String())
			assert.Len(t, doc.Transactions, c.wantRows)
			assert.Equal(t, 5, doc.Matched)
			assert.Equal(t, c.wantTruncated, doc.Truncated)
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_search_json_echoes_the_limit_it_used(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "the default is 500", args: []string{"search", "--json"}, want: 500},
		{name: "limit 0 stays 0, meaning every match", args: []string{"search", "--json", "--limit", "0"}, want: 0},
		{name: "a limit as given", args: []string{"search", "--json", "--limit", "3"}, want: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, fiveSearchStore())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.want, decodeSearchJSON(t, stdout.String()).Limit)
		})
	}
}

func Test_run_search_refuses_the_search_flags_and_text_in_the_ruled_order_before_reading_anything(t *testing.T) {
	const (
		twoTexts      = "quarry: search takes one text; quote it as one argument\n"
		negativeLimit = "quarry: --limit must be 0 or more; 0 prints every transaction\n"
		blank         = "quarry: search text is blank; leave it out to search by date, account, category or amount alone\n"
	)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "a negative limit alone", args: []string{"search", "--limit", "-1"}, want: negativeLimit},
		{name: "two texts beat a negative limit", args: []string{"search", "costco", "visa", "--limit", "-1"}, want: twoTexts},
		{name: "a negative limit beats a blank text", args: []string{"search", "", "--limit", "-1"}, want: negativeLimit},
		{name: "a blank text beats a bad since", args: []string{"search", "", "--since", "nonsense"}, want: blank},
		{name: "a negative limit beats a bad since", args: []string{"search", "--limit", "-1", "--since", "nonsense"}, want: negativeLimit},
		{name: "a negative limit with --json", args: []string{"search", "--json", "--limit", "-1"}, want: negativeLimit},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_search_text_with_limit_counts_every_text_match_and_cuts_the_oldest(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, gymSearchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "gym", "--limit", "1", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	assert.Equal(t, []string{"txn-visa-mar"}, transactionIDs(doc))
	assert.Equal(t, 3, doc.Matched)
	assert.True(t, doc.Truncated)
	assert.Equal(t, "quarry: warning: showing the newest 1 of 3 matching transactions; pass --limit 0 to list every one\n", stderr.String())
}
