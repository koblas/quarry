package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textSearchStore: one transaction per text rule, none sharing a word with another.
func textSearchStore() store.Rows {
	chequing := chequingAccount("acct-chq", 1)
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Costco Visa", Type: "credit", Currency: "CAD", Active: true}
	return searchRows([]store.Account{chequing, visa}, nil,
		searchTxn{id: "payee", account: "acct-chq", payee: "Costco", sourceID: 1, day: day(2026, time.March, 1), splits: []searchSplit{{sourceID: 1, cents: -100}}},
		searchTxn{id: "memo", account: "acct-chq", memo: "birthday gift", sourceID: 2, day: day(2026, time.March, 2), splits: []searchSplit{{sourceID: 1, cents: -200}}},
		searchTxn{id: "tip", account: "acct-chq", sourceID: 3, day: day(2026, time.March, 3), splits: []searchSplit{{memo: "tip", sourceID: 1, cents: -300}}},
		searchTxn{id: "cafe", account: "acct-chq", payee: "CAFÉ", sourceID: 4, day: day(2026, time.March, 4), splits: []searchSplit{{sourceID: 1, cents: -400}}},
		searchTxn{id: "off", account: "acct-chq", payee: "50 off", sourceID: 5, day: day(2026, time.March, 5), splits: []searchSplit{{sourceID: 1, cents: -500}}},
		searchTxn{id: "underscore", account: "acct-chq", payee: "a_b", sourceID: 6, day: day(2026, time.March, 6), splits: []searchSplit{{sourceID: 1, cents: -600}}},
		searchTxn{id: "other", account: "acct-chq", payee: "axb", sourceID: 7, day: day(2026, time.March, 7), splits: []searchSplit{{sourceID: 1, cents: -700}}},
		searchTxn{id: "backslash", account: "acct-chq", memo: `back\slash`, sourceID: 8, day: day(2026, time.March, 8), splits: []searchSplit{{sourceID: 1, cents: -800}}},
		searchTxn{id: "visa", account: "acct-visa", sourceID: 9, day: day(2026, time.March, 9), splits: []searchSplit{{sourceID: 1, cents: -900}}},
	)
}

func Test_run_search_text_lists_payee_memo_and_split_memo_matches_ignoring_case_with_every_character_literal(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "payee, ignoring case", text: "costco", want: []string{"txn-payee"}},
		{name: "transaction memo, ignoring case", text: "GIFT", want: []string{"txn-memo"}},
		{name: "split memo alone", text: "tip", want: []string{"txn-tip"}},
		{name: "a non-ASCII payee folds to lower case", text: "café", want: []string{"txn-cafe"}},
		{name: "percent is a character, not a wildcard", text: "5%", want: []string{}},
		{name: "underscore is a character, not one-character wildcard", text: "a_b", want: []string{"txn-underscore"}},
		{name: "backslash is a character", text: `\`, want: []string{"txn-backslash"}},
		{name: "the account name is not searched", text: "Visa", want: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, textSearchStore(), c.text)

			assert.Equal(t, c.want, transactionIDs(doc))
		})
	}
}

// gymSearchStore: payee "Gym" on Chequing in February and March and on Visa in March, and a Bakery
// payee on Chequing in March.
func gymSearchStore() store.Rows {
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit", Currency: "CAD", Active: true}
	return searchRows([]store.Account{chequingAccount("acct-chq", 1), visa}, nil,
		searchTxn{id: "chq-feb", account: "acct-chq", payee: "Gym", sourceID: 1, day: day(2026, time.February, 1), splits: []searchSplit{{sourceID: 1, cents: -100}}},
		searchTxn{id: "chq-mar", account: "acct-chq", payee: "Gym", sourceID: 2, day: day(2026, time.March, 10), splits: []searchSplit{{sourceID: 1, cents: -200}}},
		searchTxn{id: "visa-mar", account: "acct-visa", payee: "Gym", sourceID: 3, day: day(2026, time.March, 11), splits: []searchSplit{{sourceID: 1, cents: -300}}},
		searchTxn{id: "bakery", account: "acct-chq", payee: "Bakery", sourceID: 4, day: day(2026, time.March, 12), splits: []searchSplit{{sourceID: 1, cents: -400}}},
	)
}

func Test_run_search_json_echoes_the_text_as_given_and_null_when_none_was_given(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want *string
	}{
		{name: "text as typed, not folded", args: []string{"COSTCO"}, want: new("COSTCO")},
		{name: "no text", args: nil, want: nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, textSearchStore(), c.args...)

			assert.Equal(t, c.want, doc.Text)
		})
	}
}

func Test_run_search_shows_a_split_memo_only_match_in_the_memo_cell(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, textSearchStore())

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "tip"})

	require.Equal(t, 0, exitCode, stderr.String())
	want := "Transactions matching \"tip\" in all accounts, all dates\n\n" +
		"Date        Account         Payee       Category         Memo  Amount  Flags\n" +
		"2026-03-03  Chequing (CAD)  (no payee)  (uncategorized)  tip    -3.00\n" +
		"\n" +
		"1 matching transaction\n"
	assert.Equal(t, want, stdout.String())
}

func Test_run_search_json_lists_the_split_memo_of_a_split_memo_only_match(t *testing.T) {
	doc := searchedJSON(t, textSearchStore(), "tip")

	require.Len(t, doc.Transactions, 1)
	assert.Equal(t, []searchSplitJSON{{Memo: new("tip"), Amount: "-3.00"}}, doc.Transactions[0].Splits)
}

func Test_run_search_finds_text_that_starts_with_a_dash_after_the_double_dash(t *testing.T) {
	rows := searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil,
		searchTxn{id: "dash", account: "acct-chq", payee: "-50% off", sourceID: 1, day: day(2026, time.March, 1), splits: []searchSplit{{sourceID: 1, cents: -100}}},
		searchTxn{id: "plain", account: "acct-chq", payee: "50 off", sourceID: 2, day: day(2026, time.March, 2), splits: []searchSplit{{sourceID: 1, cents: -200}}},
	)

	doc := searchedJSON(t, rows, "--", "-50% off")

	assert.Equal(t, []string{"txn-dash"}, transactionIDs(doc))
	assert.Equal(t, new("-50% off"), doc.Text)
}

func Test_run_search_narrows_the_text_matches_by_account_and_since(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "text alone", args: []string{"gym"}, want: []string{"txn-visa-mar", "txn-chq-mar", "txn-chq-feb"}},
		{name: "text and account", args: []string{"gym", "--account", "Chequing"}, want: []string{"txn-chq-mar", "txn-chq-feb"}},
		{name: "text and since", args: []string{"gym", "--since", "2026-03"}, want: []string{"txn-visa-mar", "txn-chq-mar"}},
		{name: "text, account and since", args: []string{"gym", "--account", "Chequing", "--since", "2026-03"}, want: []string{"txn-chq-mar"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, gymSearchStore(), c.args...)

			assert.Equal(t, c.want, transactionIDs(doc))
			assert.Equal(t, len(c.want), doc.Matched)
		})
	}
}

func Test_run_search_refuses_two_texts_and_blank_text_before_reading_anything(t *testing.T) {
	const blank = "quarry: search text is blank; leave it out to search by date, account, category or amount alone\n"
	const twoTexts = "quarry: search takes one text; quote it as one argument\n"
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "two texts", args: []string{"search", "costco", "visa"}, want: twoTexts},
		{name: "an empty text", args: []string{"search", ""}, want: blank},
		{name: "a whitespace-only text", args: []string{"search", "  "}, want: blank},
		{name: "a blank text beats a bad since", args: []string{"search", "", "--since", "nonsense"}, want: blank},
		{name: "two texts with --json", args: []string{"search", "--json", "costco", "visa"}, want: twoTexts},
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
