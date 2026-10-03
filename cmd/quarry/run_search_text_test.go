package main

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
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
