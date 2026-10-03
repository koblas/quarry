package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textTxn is a one-split transaction of the given payee name, memo and split memo; "" means none.
func textTxn(rows *store.Rows, id string, sourceID int64, payee, memo, splitMemo string) searchSpec {
	spec := spend(id, sourceID, 100)
	if payee != "" {
		payeeID := "text-payee-" + id
		rows.Payees = append(rows.Payees, store.Payee{ID: payeeID, SourceID: 10 + sourceID, Name: payee})
		spec.payee = &payeeID
	}
	if memo != "" {
		spec.memo = &memo
	}
	if splitMemo != "" {
		spec.parts[0].memo = &splitMemo
	}
	return spec
}

func Test_search_text_matches_payee_memo_and_split_memo_each_alone(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, textTxn(&rows, "payee", 1, "Fitness Gym", "", ""))
	addSearch(&rows, textTxn(&rows, "memo", 2, "", "monthly pass", ""))
	addSearch(&rows, textTxn(&rows, "split", 3, "", "", "locker fee"))
	addSearch(&rows, textTxn(&rows, "bare", 4, "", "", ""))
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "payee name", text: "gym", want: []string{"txn-payee"}},
		{name: "transaction memo", text: "pass", want: []string{"txn-memo"}},
		{name: "split memo", text: "locker", want: []string{"txn-split"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Text: c.text})

			assert.Equal(t, c.want, searchedIDs(got))
			assert.Equal(t, 1, got.Matched)
		})
	}
}

func Test_search_text_ignores_letter_case_both_ways(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                         string
		payee, memo, splitMemo, text string
	}{
		{name: "upper payee, lower text", payee: "CAFÉ", text: "café"},
		{name: "lower payee, upper text", payee: "café", text: "CAFÉ"},
		{name: "upper memo, lower text", memo: "CAFÉ", text: "café"},
		{name: "lower memo, upper text", memo: "café", text: "CAFÉ"},
		{name: "upper split memo, lower text", splitMemo: "CAFÉ", text: "café"},
		{name: "lower split memo, upper text", splitMemo: "café", text: "CAFÉ"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := searchRowsFor()
			addSearch(&rows, textTxn(&rows, "cafe", 1, c.payee, c.memo, c.splitMemo))

			got := searchOf(t, rows, store.SearchParams{Text: c.text})

			assert.Equal(t, []string{"txn-cafe"}, searchedIDs(got))
		})
	}
}

func Test_search_text_treats_percent_underscore_and_backslash_literally(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, textTxn(&rows, "fifty", 1, "50 off", "", ""))
	addSearch(&rows, textTxn(&rows, "pct", 2, "100% pure", "", ""))
	addSearch(&rows, textTxn(&rows, "under", 3, "a_b", "", ""))
	addSearch(&rows, textTxn(&rows, "other", 4, "axb", "", ""))
	addSearch(&rows, textTxn(&rows, "slash", 5, "", `back\slash`, ""))
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "percent is not a wildcard", text: "5%", want: nil},
		{name: "percent matches only a percent sign", text: "%", want: []string{"txn-pct"}},
		{name: "underscore is not a one-character wildcard", text: "a_b", want: []string{"txn-under"}},
		{name: "backslash is a character", text: `\`, want: []string{"txn-slash"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Text: c.text})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
		})
	}
}

func Test_search_text_does_not_match_account_or_category_names(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	spec := textTxn(&rows, "named", 1, "Fitness Gym", "", "")
	spec.account = acctSecond
	addSearch(&rows, spec)
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "account name", text: "Savings"},
		{name: "category name", text: "Groceries"},
		{name: "payee name control", text: "Fitness", want: []string{"txn-named"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Text: c.text})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
		})
	}
}

func Test_search_text_lists_a_transaction_once_and_counts_it_once_when_several_splits_match(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	spec := textTxn(&rows, "many", 1, "", "tip jar", "tip one")
	spec.parts = append(spec.parts,
		searchPart{category: new(catExpense), memo: new("tip two"), sourceID: 2, cents: -100},
		searchPart{category: new(catExpense), memo: new("tip three"), sourceID: 3, cents: -100})
	addSearch(&rows, spec)
	addSearch(&rows, textTxn(&rows, "one", 2, "", "", "tip"))

	got := searchOf(t, rows, store.SearchParams{Text: "tip"})

	require.Equal(t, []string{"txn-one", "txn-many"}, searchedIDs(got))
	assert.Equal(t, 2, got.Matched)
	assert.Len(t, got.Rows[1].Splits, 3)
}

func Test_search_text_is_not_trimmed(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, textTxn(&rows, "gym", 1, "Gym", "", ""))
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "leading space", text: " gym"},
		{name: "trailing space", text: "gym "},
		{name: "no space", text: "gym", want: []string{"txn-gym"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Text: c.text})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
		})
	}
}

func Test_search_text_combines_with_window_and_two_accounts(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for _, g := range []struct {
		id      string
		payee   string
		account string
		day     int
	}{
		{id: "g1", payee: "Gym", account: acctInReports, day: 10},
		{id: "g2", payee: "Gym", account: acctSecond, day: 11},
		{id: "g3", payee: "Gym", account: acctNotReports, day: 12},
		{id: "g4", payee: "Bakery", account: acctInReports, day: 13},
		{id: "g5", payee: "Gym", account: acctInReports, day: 5},
	} {
		spec := textTxn(&rows, g.id, int64(g.day), g.payee, "", "")
		spec.account, spec.date = g.account, day(2026, time.March, g.day)
		addSearch(&rows, spec)
	}
	since := day(2026, time.March, 8)

	got := searchOf(t, rows, store.SearchParams{
		Text: "gym", Window: store.SearchWindow{Since: &since}, AccountIDs: []string{acctInReports, acctSecond},
	})

	assert.Equal(t, []string{"txn-g2", "txn-g1"}, searchedIDs(got))
	assert.Equal(t, 2, got.Matched)
}
