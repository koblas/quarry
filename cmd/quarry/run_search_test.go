package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchSplitJSON is one entry of a search transaction's "splits".
type searchSplitJSON struct {
	Category *string `json:"category"`
	Memo     *string `json:"memo"`
	Amount   string  `json:"amount"`
	Transfer bool    `json:"transfer"`
}

// searchTransactionJSON is one entry of the search document's "transactions".
type searchTransactionJSON struct {
	TransactionID string            `json:"transaction_id"`
	Date          string            `json:"date"`
	AccountID     string            `json:"account_id"`
	Account       string            `json:"account"`
	Payee         *string           `json:"payee"`
	Memo          *string           `json:"memo"`
	Amount        string            `json:"amount"`
	Currency      string            `json:"currency"`
	Transfer      bool              `json:"transfer"`
	Excluded      bool              `json:"excluded"`
	Splits        []searchSplitJSON `json:"splits"`
}

// searchJSONDoc is quarry search --json's stdout.
type searchJSONDoc struct {
	Since         *string                 `json:"since"`
	Until         *string                 `json:"until"`
	AccountFilter []recurringIDName       `json:"account_filter"`
	Text          *string                 `json:"text"`
	Category      *string                 `json:"category"`
	Min           *string                 `json:"min"`
	Max           *string                 `json:"max"`
	Limit         int                     `json:"limit"`
	Matched       int                     `json:"matched"`
	Truncated     bool                    `json:"truncated"`
	Transactions  []searchTransactionJSON `json:"transactions"`
	Warnings      []string                `json:"warnings"`
}

// decodeSearchJSON reads stdout as the search document, refusing keys the document does not rule.
func decodeSearchJSON(t *testing.T, stdout string) searchJSONDoc {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	decoder.DisallowUnknownFields()
	var doc searchJSONDoc

	require.NoError(t, decoder.Decode(&doc), stdout)
	return doc
}

// searchSplit is one split of a searchTxn: "" category or memo means none, and transferTo is the
// account id of the other leg of a transfer; sourceID orders the splits of one transaction.
type searchSplit struct {
	category, memo, transferTo string
	sourceID                   int64
	cents                      int64
}

// searchTxn is one transaction of any number of splits, listed with the source id it carries:
// "" payee or memo means none, and the payee is stored under the id "payee-<name>".
type searchTxn struct {
	id, account, payee, memo string
	sourceID                 int64
	day                      time.Time
	excluded                 bool
	splits                   []searchSplit
}

// searchRows is spendRows' reference data plus txns (each priced at the sum of its splits) and a
// transfers row for each pair of split ids in pairs.
func searchRows(accounts []store.Account, pairs [][2]string, txns ...searchTxn) store.Rows {
	rows := spendRows(accounts)
	rows.Payees = nil
	currencies := map[string]string{}
	for _, a := range accounts {
		currencies[a.ID] = a.Currency
	}
	known := map[string]bool{}
	for _, tx := range txns {
		var payeeID *string
		if tx.payee != "" {
			id := "payee-" + tx.payee
			if !known[id] {
				known[id] = true
				rows.Payees = append(rows.Payees, store.Payee{ID: id, SourceID: int64(len(rows.Payees) + 1), Name: tx.payee})
			}
			payeeID = &id
		}
		var amount int64
		for j, sp := range tx.splits {
			rows.Splits = append(rows.Splits, store.Split{
				ID: fmt.Sprintf("split-%s-%d", tx.id, j), SourceID: sp.sourceID, TransactionID: "txn-" + tx.id,
				CategoryID: optional(sp.category), Memo: optional(sp.memo), Amount: sp.cents,
				TransferAccountID: optional(sp.transferTo),
			})
			amount += sp.cents
		}
		rows.Transactions = append(rows.Transactions, store.Transaction{
			ID: "txn-" + tx.id, SourceID: tx.sourceID, AccountID: tx.account, Date: tx.day, Amount: amount,
			Currency: currencies[tx.account], Status: "uncleared", PayeeID: payeeID, Memo: optional(tx.memo),
			ExcludedFromReports: tx.excluded,
		})
	}
	for i, pair := range pairs {
		to := pair[1]
		rows.Transfers = append(rows.Transfers, store.Transfer{ID: fmt.Sprintf("transfer-%d", i), FromSplitID: pair[0], ToSplitID: &to})
	}
	return rows
}

// optional is nil for "", else a pointer to s.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// manySearchTxns is n single-split CAD transactions on acct-chq named "n001".."nNNN", one day apart,
// the highest number newest.
func manySearchTxns(n int) []searchTxn {
	txns := make([]searchTxn, n)
	for i := range txns {
		txns[i] = searchTxn{
			id: fmt.Sprintf("n%03d", i+1), account: "acct-chq", sourceID: int64(i + 1), day: day(2025, time.January, 1).AddDate(0, 0, i),
			splits: []searchSplit{{sourceID: 1, cents: -100}},
		}
	}
	return txns
}

// searchStore: transactions by account, oldest first, with the dates txn-marked 02-14, txn-low and
// txn-high 03-10, txn-out and txn-in 03-20, txn-split 04-02.
func searchStore() store.Rows {
	chequing := chequingAccount("acct-chq", 1)
	savings := store.Account{ID: "acct-sav", SourceID: 2, Name: "Savings", Type: "savings", Currency: "CAD", Active: true}
	oldCard := store.Account{ID: "acct-old", SourceID: 3, Name: "Old Card", Type: "credit", Currency: "CAD", Active: true, NotInReports: true}
	tracked := store.Account{ID: "acct-trk", SourceID: 4, Name: "Tracked Loan", Type: "loan", Currency: "CAD", Active: true, LinkedTracking: true}
	return searchRows([]store.Account{chequing, savings, oldCard, tracked},
		[][2]string{{"split-out-0", "split-in-0"}},
		searchTxn{
			id: "linked", account: "acct-trk", sourceID: 1, day: day(2026, time.January, 20),
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -3000}},
		},
		searchTxn{
			id: "left-out", account: "acct-old", sourceID: 2, day: day(2026, time.February, 1),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -2500}},
		},
		searchTxn{
			id: "marked", account: "acct-chq", payee: "Bakery", memo: "hold for refund", sourceID: 5, day: day(2026, time.February, 14), excluded: true,
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -1999}},
		},
		searchTxn{
			id: "low", account: "acct-chq", payee: "Bakery", sourceID: 3, day: day(2026, time.March, 10),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -850}},
		},
		searchTxn{
			id: "high", account: "acct-chq", payee: "Costco", sourceID: 7, day: day(2026, time.March, 10),
			splits: []searchSplit{{category: "cat-groceries", sourceID: 1, cents: -4217}},
		},
		searchTxn{
			id: "out", account: "acct-chq", sourceID: 11, day: day(2026, time.March, 20),
			splits: []searchSplit{{transferTo: "acct-sav", sourceID: 1, cents: -50000}},
		},
		searchTxn{
			id: "in", account: "acct-sav", sourceID: 12, day: day(2026, time.March, 20),
			splits: []searchSplit{{transferTo: "acct-chq", sourceID: 1, cents: 50000}},
		},
		searchTxn{
			id: "split", account: "acct-chq", payee: "Costco", memo: "bulk run", sourceID: 20, day: day(2026, time.April, 2),
			splits: []searchSplit{
				{category: "cat-fuel", sourceID: 2, cents: -4000},
				{category: "cat-groceries", memo: "milk and eggs", sourceID: 1, cents: -6000},
			},
		},
	)
}

func Test_run_search_json_lists_every_transaction_newest_first_flagged_with_its_splits(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, searchStore(), []string{"search", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, searchJSONDoc{
		AccountFilter: []recurringIDName{},
		Limit:         500,
		Matched:       8,
		Transactions: []searchTransactionJSON{
			{
				TransactionID: "txn-split", Date: "2026-04-02", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Costco"), Memo: new("bulk run"), Amount: "-100.00", Currency: "CAD",
				Splits: []searchSplitJSON{
					{Category: new("Food:Groceries"), Memo: new("milk and eggs"), Amount: "-60.00"},
					{Category: new("Auto:Fuel"), Amount: "-40.00"},
				},
			},
			{
				TransactionID: "txn-in", Date: "2026-03-20", AccountID: "acct-sav", Account: "Savings",
				Amount: "500.00", Currency: "CAD", Transfer: true,
				Splits: []searchSplitJSON{{Amount: "500.00", Transfer: true}},
			},
			{
				TransactionID: "txn-out", Date: "2026-03-20", AccountID: "acct-chq", Account: "Chequing",
				Amount: "-500.00", Currency: "CAD", Transfer: true,
				Splits: []searchSplitJSON{{Amount: "-500.00", Transfer: true}},
			},
			{
				TransactionID: "txn-high", Date: "2026-03-10", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Costco"), Amount: "-42.17", Currency: "CAD",
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-42.17"}},
			},
			{
				TransactionID: "txn-low", Date: "2026-03-10", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Bakery"), Amount: "-8.50", Currency: "CAD",
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-8.50"}},
			},
			{
				TransactionID: "txn-marked", Date: "2026-02-14", AccountID: "acct-chq", Account: "Chequing",
				Payee: new("Bakery"), Memo: new("hold for refund"), Amount: "-19.99", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Auto:Fuel"), Amount: "-19.99"}},
			},
			{
				TransactionID: "txn-left-out", Date: "2026-02-01", AccountID: "acct-old", Account: "Old Card",
				Amount: "-25.00", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Food:Groceries"), Amount: "-25.00"}},
			},
			{
				TransactionID: "txn-linked", Date: "2026-01-20", AccountID: "acct-trk", Account: "Tracked Loan",
				Amount: "-30.00", Currency: "CAD", Excluded: true,
				Splits: []searchSplitJSON{{Category: new("Auto:Fuel"), Amount: "-30.00"}},
			},
		},
		Warnings: []string{},
	}, decodeSearchJSON(t, stdout.String()))
}

// runSearchOver replaces the store under a fresh HOME with rows, then runs args, returning the exit
// code, stdout and stderr.
func runSearchOver(t *testing.T, rows store.Rows, args []string) (int, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	replaceStore(t, newHome(t), rows)
	return runSpendCapture(context.Background(), args)
}

// searchedJSON runs quarry search --json with args against rows and returns the document, requiring exit 0.
func searchedJSON(t *testing.T, rows store.Rows, args ...string) searchJSONDoc {
	t.Helper()
	exitCode, stdout, stderr := runSearchOver(t, rows, append([]string{"search", "--json"}, args...))

	require.Equal(t, 0, exitCode, stderr.String())
	return decodeSearchJSON(t, stdout.String())
}

func transactionIDs(doc searchJSONDoc) []string {
	ids := make([]string, len(doc.Transactions))
	for i, tx := range doc.Transactions {
		ids[i] = tx.TransactionID
	}
	return ids
}

func Test_run_search_json_bounds_the_listing_by_since_and_until_and_echoes_their_days(t *testing.T) {
	doc := searchedJSON(t, searchStore(), "--since", "2026-01", "--until", "2026-03")

	assert.Equal(t, []string{"txn-in", "txn-out", "txn-high", "txn-low", "txn-marked", "txn-left-out", "txn-linked"}, transactionIDs(doc))
	assert.Equal(t, 7, doc.Matched)
	assert.Equal(t, new("2026-01-01"), doc.Since)
	assert.Equal(t, new("2026-03-31"), doc.Until)
}

func Test_run_search_json_bounds_the_listing_by_one_flag_alone(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantIDs   []string
		wantSince *string
		wantUntil *string
	}{
		{
			name: "since alone leaves the end open", args: []string{"--since", "2026-03"},
			wantIDs:   []string{"txn-split", "txn-in", "txn-out", "txn-high", "txn-low"},
			wantSince: new("2026-03-01"),
		},
		{
			name: "until alone leaves the start open", args: []string{"--until", "2026-02"},
			wantIDs:   []string{"txn-marked", "txn-left-out", "txn-linked"},
			wantUntil: new("2026-02-28"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, searchStore(), c.args...)

			assert.Equal(t, c.wantIDs, transactionIDs(doc))
			assert.Equal(t, c.wantSince, doc.Since)
			assert.Equal(t, c.wantUntil, doc.Until)
		})
	}
}

func Test_run_search_json_with_an_account_lists_only_its_transactions_and_echoes_it(t *testing.T) {
	doc := searchedJSON(t, searchStore(), "--account", "savings")

	assert.Equal(t, []string{"txn-in"}, transactionIDs(doc))
	assert.Equal(t, 1, doc.Matched)
	assert.Equal(t, []recurringIDName{{ID: "acct-sav", Name: "Savings"}}, doc.AccountFilter)
}

func Test_run_search_json_flags_a_named_left_out_accounts_transactions_excluded(t *testing.T) {
	doc := searchedJSON(t, searchStore(), "--account", "Tracked Loan")

	require.Len(t, doc.Transactions, 1)
	assert.Equal(t, "txn-linked", doc.Transactions[0].TransactionID)
	assert.True(t, doc.Transactions[0].Excluded)
	assert.False(t, doc.Transactions[0].Transfer)
}

func Test_run_search_json_lists_closed_account_and_usd_transactions_in_their_own_currency(t *testing.T) {
	closed := chequingAccount("acct-closed", 1)
	closed.Name, closed.Closed, closed.Active = "Closed Chequing", true, false
	rows := searchRows([]store.Account{closed, usdChequingAccount("acct-usd", 2)}, nil,
		searchTxn{
			id: "closed", account: "acct-closed", sourceID: 1, day: day(2020, time.May, 4),
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -1500}},
		},
		searchTxn{
			id: "usd", account: "acct-usd", sourceID: 2, day: day(2026, time.June, 1),
			splits: []searchSplit{{category: "cat-fuel", sourceID: 1, cents: -2500}},
		},
	)

	doc := searchedJSON(t, rows)

	require.Len(t, doc.Transactions, 2)
	assert.Equal(t, []string{"txn-usd", "USD", "-25.00"},
		[]string{doc.Transactions[0].TransactionID, doc.Transactions[0].Currency, doc.Transactions[0].Amount})
	assert.Equal(t, []string{"txn-closed", "CAD", "-15.00", "Closed Chequing"},
		[]string{doc.Transactions[1].TransactionID, doc.Transactions[1].Currency, doc.Transactions[1].Amount, doc.Transactions[1].Account})
}

func Test_run_search_refuses_a_since_that_is_not_a_date_before_opening_the_store(t *testing.T) {
	newHome(t)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "--since", "2024-13"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n", stderr.String())
}

func Test_run_search_refuses_a_since_after_the_until(t *testing.T) {
	newHome(t)

	exitCode, _, stderr := runSpendCapture(context.Background(), []string{"search", "--since", "2025", "--until", "2024"})

	assert.Equal(t, 2, exitCode)
	assert.Equal(t, "quarry: --since 2025 is after --until 2024\n", stderr.String())
}

func Test_run_search_prints_the_transactions_table(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, searchStore(), []string{"search"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Transactions in all accounts, all dates\n"+
		"\n"+
		"Date        Account             Payee       Category                   Memo                       Amount  Flags\n"+
		"2026-04-02  Chequing (CAD)      Costco      Food:Groceries, Auto:Fuel  bulk run / milk and eggs  -100.00\n"+
		"2026-03-20  Savings (CAD)       (no payee)  (transfer)                                            500.00  transfer\n"+
		"2026-03-20  Chequing (CAD)      (no payee)  (transfer)                                           -500.00  transfer\n"+
		"2026-03-10  Chequing (CAD)      Costco      Food:Groceries                                        -42.17\n"+
		"2026-03-10  Chequing (CAD)      Bakery      Food:Groceries                                         -8.50\n"+
		"2026-02-14  Chequing (CAD)      Bakery      Auto:Fuel                  hold for refund            -19.99  excluded\n"+
		"2026-02-01  Old Card (CAD)      (no payee)  Food:Groceries                                        -25.00  excluded\n"+
		"2026-01-20  Tracked Loan (CAD)  (no payee)  Auto:Fuel                                             -30.00  excluded\n"+
		"\n"+
		"8 matching transactions\n", stdout.String())
}

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
	exitCode, stdout, stderr := runSearchOver(t, textSearchStore(), []string{"search", "tip"})

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
			newHome(t)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

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
	exitCode, stdout, stderr := runSearchOver(t, amountSearchStore(), []string{"search", "--min", "100", "--max", "1000"})

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
		{name: "a signed min", args: []string{"--min", "-12"}, want: `quarry: --min "-12" is not an amount; ` + useDigits},
		{name: "a grouped max", args: []string{"--max", "1,234.56"}, want: `quarry: --max "1,234.56" is not an amount; ` + useDigits},
		{name: "an empty min is refused, not ignored", args: []string{"--min", ""}, want: `quarry: --min "" is not an amount; ` + useDigits},
		{name: "an empty max", args: []string{"--max", ""}, want: `quarry: --max "" is not an amount; ` + useDigits},
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
	newHome(t)

	assertSearchFailed(t, args, 2, want)
}

// categorySearchStore: one transaction per category, dated January 1..5 in the order listed,
// plus the category Travel that no split uses.
func categorySearchStore() store.Rows {
	categories := []struct {
		id, path string
		hidden   bool
	}{
		{"cat-food", "Food", false},
		{"cat-groceries", "Food:Groceries", false},
		{"cat-organic", "Food:Groceries:Organic", false},
		{"cat-foo", "Foo", false},
		{"cat-archive", "Archive", true},
		{"cat-travel", "Travel", false},
	}
	var txns []searchTxn
	var cats []store.Category
	var referenced []string
	for i, c := range categories {
		cats = append(cats, store.Category{ID: c.id, SourceID: int64(i + 1), Name: c.path, FullPath: c.path, Kind: "expense", Hidden: c.hidden})
		if c.id == "cat-travel" {
			continue
		}
		referenced = append(referenced, c.id)
		txns = append(txns, searchTxn{
			id: c.id, account: "acct-chq", sourceID: int64(i + 1), day: day(2026, time.January, i+1),
			splits: []searchSplit{{category: c.id, sourceID: 1, cents: -1000}},
		})
	}
	rows := searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, txns...)
	rows.Categories, rows.ReferencedCategoryIDs = cats, referenced
	return rows
}

func Test_run_search_category_lists_the_category_and_everything_under_it(t *testing.T) {
	cases := []struct {
		name         string
		category     string
		wantIDs      []string
		wantWarnings []string
	}{
		{
			name:     "a top category lists itself and every level under it, not its lookalike",
			category: "Food", wantIDs: []string{"txn-cat-organic", "txn-cat-groceries", "txn-cat-food"}, wantWarnings: []string{},
		},
		{
			name:     "any letter case names the same category",
			category: "food:groceries", wantIDs: []string{"txn-cat-organic", "txn-cat-groceries"}, wantWarnings: []string{},
		},
		{
			name:     "a category that only starts with the same letters is not under it",
			category: "Foo", wantIDs: []string{"txn-cat-foo"}, wantWarnings: []string{},
		},
		{
			name: "a hidden category counts", category: "Archive", wantIDs: []string{"txn-cat-archive"}, wantWarnings: []string{},
		},
		{
			name: "a known category no split uses gives the no-match warning", category: "Travel", wantIDs: []string{},
			wantWarnings: []string{"no transactions match the search; the store's transactions run 2026-01-01 to 2026-01-05"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, categorySearchStore(), "--category", c.category)

			assert.Equal(t, c.wantIDs, transactionIDs(doc))
			assert.Equal(t, len(c.wantIDs), doc.Matched)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
		})
	}
}

// narrowCategoryStore: five transactions that each differ from "hit" in one variable (account, text, amount or category).
func narrowCategoryStore() store.Rows {
	visa := store.Account{ID: "acct-visa", SourceID: 2, Name: "Visa", Type: "credit", Currency: "CAD", Active: true}
	groceries, other := "cat-groceries", "cat-foo"
	rows := searchRows([]store.Account{chequingAccount("acct-chq", 1), visa}, nil,
		searchTxn{id: "hit", account: "acct-chq", payee: "Gym", sourceID: 1, day: day(2026, time.March, 10), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-account", account: "acct-visa", payee: "Gym", sourceID: 2, day: day(2026, time.March, 11), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-text", account: "acct-chq", payee: "Bakery", sourceID: 3, day: day(2026, time.March, 12), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -6000}}},
		searchTxn{id: "other-amount", account: "acct-chq", payee: "Gym", sourceID: 4, day: day(2026, time.March, 13), splits: []searchSplit{{category: groceries, sourceID: 1, cents: -1000}}},
		searchTxn{id: "other-category", account: "acct-chq", payee: "Gym", sourceID: 5, day: day(2026, time.March, 14), splits: []searchSplit{{category: other, sourceID: 1, cents: -6000}}},
	)
	rows.Categories = []store.Category{
		{ID: "cat-food", SourceID: 3, Name: "Food", FullPath: "Food", Kind: "expense"},
		{ID: "cat-groceries", SourceID: 1, Name: "Food:Groceries", FullPath: "Food:Groceries", Kind: "expense"},
		{ID: "cat-foo", SourceID: 2, Name: "Foo", FullPath: "Foo", Kind: "expense"},
	}
	rows.ReferencedCategoryIDs = []string{groceries, other}
	return rows
}

func Test_run_search_category_narrows_with_account_text_and_amount(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "category alone", args: []string{"--category", "food"},
			want: []string{"txn-other-amount", "txn-other-text", "txn-other-account", "txn-hit"},
		},
		{name: "and an account", args: []string{"--category", "food", "--account", "Chequing"}, want: []string{"txn-other-amount", "txn-other-text", "txn-hit"}},
		{name: "and text", args: []string{"--category", "food", "gym"}, want: []string{"txn-other-amount", "txn-other-account", "txn-hit"}},
		{name: "and a minimum", args: []string{"--category", "food", "--min", "50"}, want: []string{"txn-other-text", "txn-other-account", "txn-hit"}},
		{
			name: "and all three", args: []string{"--category", "food", "--account", "Chequing", "--min", "50", "gym"},
			want: []string{"txn-hit"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, narrowCategoryStore(), c.args...)

			assert.Equal(t, c.want, transactionIDs(doc))
			assert.Equal(t, len(c.want), doc.Matched)
		})
	}
}

func Test_run_search_json_echoes_the_category_as_given_and_null_when_absent(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want *string
	}{
		{name: "not given", args: nil},
		{name: "as typed, in its own letter case", args: []string{"--category", "food:GROCERIES"}, want: new("food:GROCERIES")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := searchedJSON(t, narrowCategoryStore(), c.args...)

			assert.Equal(t, c.want, doc.Category)
		})
	}
}

func Test_run_search_text_names_the_category_in_the_caption(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, narrowCategoryStore(), []string{"search", "--category", "food:GROCERIES", "--min", "50"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Transactions in all accounts, all dates, category \"food:GROCERIES\", amount at least 50.00\n")
}

func Test_run_search_category_as_text(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "a known category no split uses prints the empty result and the no-match warning", args: []string{"--category", "Travel"},
			wantStdout: "Transactions in all accounts, all dates, category \"Travel\"\n\nDate  Account  Payee  Category  Memo  Amount  Flags\n\n0 matching transactions\n",
			wantStderr: "quarry: warning: no transactions match the search; the store's transactions run 2026-01-01 to 2026-01-05\n",
		},
		{
			name: "a misspelling with a named account is refused on the category", args: []string{"--account", "Chequing", "--category", "Fod"}, wantExit: 1,
			wantStderr: "quarry: no category named \"Fod\"; list them with quarry sql \"SELECT full_path FROM categories ORDER BY full_path\"\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runSearchOver(t, categorySearchStore(), append([]string{"search"}, c.args...))

			require.Equal(t, c.wantExit, exitCode, stderr.String())
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

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
			exitCode, stdout, stderr := runSearchOver(t, searchRows([]store.Account{chequingAccount("acct-chq", 1)}, nil, manySearchTxns(501)...), c.args)

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
	exitCode, stdout, stderr := runSearchOver(t, fiveSearchStore(), []string{"search", "--limit", "2"})

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
	exitCode, stdout, stderr := runSearchOver(t, fiveSearchStore(), []string{"search", "--json", "--limit", "2"})

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
			exitCode, stdout, stderr := runSearchOver(t, fiveSearchStore(), []string{"search", "--json", "--limit", c.limit})

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
			exitCode, stdout, stderr := runSearchOver(t, fiveSearchStore(), c.args)

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
			newHome(t)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_search_text_with_limit_counts_every_text_match_and_cuts_the_oldest(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, gymSearchStore(), []string{"search", "gym", "--limit", "1", "--json"})

	require.Equal(t, 0, exitCode, stderr.String())
	doc := decodeSearchJSON(t, stdout.String())
	assert.Equal(t, []string{"txn-visa-mar"}, transactionIDs(doc))
	assert.Equal(t, 3, doc.Matched)
	assert.True(t, doc.Truncated)
	assert.Equal(t, "quarry: warning: showing the newest 1 of 3 matching transactions; pass --limit 0 to list every one\n", stderr.String())
}

func Test_run_search_with_text_that_matches_nothing_prints_an_empty_result_and_the_no_match_warning(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, searchStore(), []string{"search", "--json", "zzz"})

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
			exitCode, stdout, stderr := runSearchOver(t, c.rows, append([]string{"search", "--json"}, c.args...))

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, []string{c.want}, decodeSearchJSON(t, stdout.String()).Warnings)
			assert.Equal(t, "quarry: warning: "+c.want+"\n", stderr.String())
		})
	}
}

func Test_run_search_text_with_no_match_prints_the_empty_listing_on_stdout_and_the_warning_on_stderr(t *testing.T) {
	exitCode, stdout, stderr := runSearchOver(t, searchStore(), []string{"search", "zzz"})

	require.Equal(t, 0, exitCode, stderr.String())
	want := "Transactions matching \"zzz\" in all accounts, all dates\n\n" +
		"Date  Account  Payee  Category  Memo  Amount  Flags\n" +
		"\n" +
		"0 matching transactions\n"
	assert.Equal(t, want, stdout.String())
	assert.Equal(t, "quarry: warning: no transactions match the search; the store's transactions run 2026-01-20 to 2026-04-02\n", stderr.String())
}

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
		badTextFF       = "quarry: search text \"\\xff\" is not valid UTF-8; set your terminal or script to UTF-8\n"
		badTextHalfRune = "quarry: search text \"caf\\xc3\" is not valid UTF-8; set your terminal or script to UTF-8\n"
		badCategoryFF   = "quarry: --category \"\\xff\" is not valid UTF-8; set your terminal or script to UTF-8\n"
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
		{name: "an empty since", args: []string{"--since", ""}, exit: 2, want: "quarry: --since \"\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"},
		{name: "an empty until", args: []string{"--until", ""}, exit: 2, want: "quarry: --until \"\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"},
		{name: "a since after the until", args: []string{"--since", "2025", "--until", "2024"}, exit: 2, want: "quarry: --since 2025 is after --until 2024\n"},
		{name: "an account no account is named", args: []string{"--account", "Nope"}, exit: 1, want: "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n"},
		{name: "an account name two accounts share", args: []string{"--account", "Visa"}, exit: 1, want: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n"},
		{name: "an empty account", args: []string{"--account", ""}, exit: 1, want: "quarry: no account named \"\"; run quarry accounts --all to list them\n"},
		{name: "a category no category is named", args: []string{"--category", "Fod"}, exit: 1, want: `quarry: no category named "Fod"` + unknownCategory},
		{name: "an empty category", args: []string{"--category", ""}, exit: 1, want: `quarry: no category named ""` + unknownCategory},
		{name: "a text that is not valid UTF-8", args: []string{"\xff"}, exit: 2, want: badTextFF},
		{name: "a text that ends in half a rune", args: []string{"caf\xc3"}, exit: 2, want: badTextHalfRune},
		{name: "a category that is not valid UTF-8", args: []string{"--category", "\xff"}, exit: 2, want: badCategoryFF},
		{name: "a --currency flag", args: []string{"--currency", "CAD"}, exit: 2, want: "quarry: unknown flag: --currency" + usage},
		{name: "a --csv flag", args: []string{"--csv"}, exit: 2, want: "quarry: unknown flag: --csv" + usage},
	}

	for _, c := range cases {
		for _, mode := range []struct {
			name string
			args []string
		}{{"as text", nil}, {"with --json", []string{"--json"}}} {
			t.Run(c.name+" "+mode.name, func(t *testing.T) {
				home := newHome(t)
				replaceStore(t, home, refusalSearchStore())

				assertSearchFailed(t, append(append([]string{"search"}, c.args...), mode.args...), c.exit, c.want)
			})
		}
	}
}

func Test_run_search_refuses_in_the_ruled_order(t *testing.T) {
	const (
		blank         = "quarry: search text is blank; leave it out to search by date, account, category or amount alone\n"
		negativeLimit = "quarry: --limit must be 0 or more; 0 prints every transaction\n"
		badMin        = "quarry: --min \"-12\" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99\n"
		badSince      = "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n"
		noAccount     = "quarry: no account named \"Nope\"; run quarry accounts --all to list them\n"
		noCategory    = "quarry: no category named \"Fod\"; list them with quarry sql \"SELECT full_path FROM categories ORDER BY full_path\"\n"
		badText       = "quarry: search text \"\\xff\" is not valid UTF-8; set your terminal or script to UTF-8\n"
		badCategory   = "quarry: --category \"\\xfe\" is not valid UTF-8; set your terminal or script to UTF-8\n"
	)
	cases := []struct {
		name string
		args []string
		exit int
		want string
	}{
		{name: "a bad since beats an unknown account", args: []string{"--since", "2024-13", "--account", "Nope"}, exit: 2, want: badSince},
		{name: "an unknown account beats an unknown category", args: []string{"--account", "Nope", "--category", "Fod"}, exit: 1, want: noAccount},
		{name: "an unknown category alone", args: []string{"--category", "Fod"}, exit: 1, want: noCategory},
		{name: "a text that is not valid UTF-8 beats a bad min", args: []string{"\xff", "--min", "-12"}, exit: 2, want: badText},
		{name: "a text that is not valid UTF-8 beats a category that is not", args: []string{"\xff", "--category", "\xfe"}, exit: 2, want: badText},
		{name: "blank text beats a category that is not valid UTF-8", args: []string{"", "--category", "\xfe"}, exit: 2, want: blank},
		{name: "a negative limit beats a category that is not valid UTF-8", args: []string{"--limit", "-1", "--category", "\xfe"}, exit: 2, want: negativeLimit},
		{name: "a category that is not valid UTF-8 beats a bad min", args: []string{"--category", "\xfe", "--min", "-12"}, exit: 2, want: badCategory},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			builtStore(t)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"search", "--json"}, c.args...))

			require.Equal(t, c.exit, exitCode, stderr.String())
			assert.Equal(t, c.want, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_search_reads_the_store_before_it_looks_up_the_category(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "an unknown category", args: []string{"--category", "Fod"}},
		{name: "a known category", args: []string{"--category", "Food"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)

			assertSearchFailed(t, append([]string{"search"}, c.args...), 1,
				"quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n")
		})
	}
}

func Test_run_search_refuses_text_that_is_not_valid_UTF_8_before_the_store_opens(t *testing.T) {
	assertSearchRefused(t, []string{"search", "\xff"},
		"quarry: search text \"\\xff\" is not valid UTF-8; set your terminal or script to UTF-8\n")
}

func Test_run_search_text_with_a_nul_byte_matches_nothing_and_exits_0(t *testing.T) {
	builtStore(t)

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"search", "a\x00b"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "0 matching transactions")
	assert.Contains(t, stderr.String(), "quarry: warning: no transactions match the search")
}

func Test_run_search_an_account_that_is_not_valid_UTF_8_is_an_unknown_account(t *testing.T) {
	builtStore(t)

	assertSearchFailed(t, []string{"search", "--account", "\xff"}, 1,
		"quarry: no account named \"\\xff\"; run quarry accounts --all to list them\n")
}

// builtStore points HOME at a fresh directory holding refusalSearchStore.
func builtStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	replaceStore(t, home, refusalSearchStore())
}

// assertSearchFailed runs args against the store under $HOME and requires wantExit, nothing on stdout and want on stderr.
func assertSearchFailed(t *testing.T, args []string, wantExit int, want string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCapture(context.Background(), args)

	require.Equal(t, wantExit, exitCode, stderr.String())
	assert.Equal(t, want, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_run_search_without_since_or_until_searches_every_date(t *testing.T) {
	doc := searchedJSON(t, chargeRows([]store.Account{chequingAccount("acct-chq", 1)},
		chargeTxn{id: "old", account: "acct-chq", currency: "CAD", day: day(2025, time.December, 31), splits: []chargeSplit{{cents: -1000}}},
		chargeTxn{id: "future", account: "acct-chq", currency: "CAD", day: day(2027, time.January, 15), splits: []chargeSplit{{cents: -2000}}},
	))

	require.Len(t, doc.Transactions, 2)
	assert.Equal(t, []string{"txn-future", "txn-old"}, []string{doc.Transactions[0].TransactionID, doc.Transactions[1].TransactionID})
	assert.Nil(t, doc.Since)
	assert.Nil(t, doc.Until)
}
