package duckstore_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedTxn is a one-split transaction of exactly cents, positive for a deposit and negative for a charge.
func signedTxn(id string, sourceID, cents int64) searchSpec {
	return searchSpec{id: id, sourceID: sourceID, parts: []searchPart{{category: new(catExpense), sourceID: 1, cents: cents}}}
}

// amountRows holds one transaction per amount of the table below, ids "txn-<name>".
func amountRows() store.Rows {
	rows := searchRowsFor()
	for i, t := range []struct {
		name  string
		cents int64
	}{
		{"c150", -15000},
		{"d120", 12000},
		{"c9999", -9999},
		{"c20", -2000},
		{"c2001", -2001},
		{"d20", 2000},
		{"d50", 5000},
		{"d5001", 5001},
		{"d4217", 4217},
		{"c4217", -4217},
		{"zero", 0},
	} {
		addSearch(&rows, signedTxn(t.name, int64(i+1), t.cents))
	}
	return rows
}

func Test_search_amount_bounds_compare_the_absolute_amount(t *testing.T) {
	t.Parallel()
	rows := amountRows()
	cases := []struct {
		name     string
		min, max *int64
		want     []string
	}{
		{name: "min finds a negative amount and a deposit by size", min: new(int64(12000)), want: []string{"txn-c150", "txn-d120"}},
		{name: "min is inclusive at the value", min: new(int64(15000)), want: []string{"txn-c150"}},
		{name: "min one cent above a value leaves it out", min: new(int64(15001))},
		{name: "min one cent below a value keeps it", min: new(int64(11999)), want: []string{"txn-c150", "txn-d120"}},
		{name: "min of one cent lists every amount but the zero one", min: new(int64(1)), want: []string{
			"txn-c150", "txn-d120", "txn-c9999", "txn-c20", "txn-c2001", "txn-d20", "txn-d50", "txn-d5001", "txn-d4217", "txn-c4217",
		}},
		{name: "min of zero lists the zero amount too", min: new(int64(0)), want: []string{
			"txn-c150", "txn-d120", "txn-c9999", "txn-c20", "txn-c2001", "txn-d20", "txn-d50", "txn-d5001", "txn-d4217", "txn-c4217", "txn-zero",
		}},
		{name: "max is inclusive at the value and ignores the sign", max: new(int64(2000)), want: []string{"txn-c20", "txn-d20", "txn-zero"}},
		{name: "max one cent below a value leaves it out", max: new(int64(1999)), want: []string{"txn-zero"}},
		{name: "max one cent above a value keeps it", max: new(int64(2001)), want: []string{"txn-c20", "txn-c2001", "txn-d20", "txn-zero"}},
		{name: "max of zero lists only the zero amount", max: new(int64(0)), want: []string{"txn-zero"}},
		{name: "min and max together keep both ends", min: new(int64(2000)), max: new(int64(5000)), want: []string{
			"txn-c20", "txn-c2001", "txn-d20", "txn-d50", "txn-d4217", "txn-c4217",
		}},
		{name: "equal min and max find exactly that amount", min: new(int64(4217)), max: new(int64(4217)), want: []string{"txn-d4217", "txn-c4217"}},
		{name: "equal min and max on a value with one holder", min: new(int64(5000)), max: new(int64(5000)), want: []string{"txn-d50"}},
		{name: "equal min and max on a value nobody holds", min: new(int64(4218)), max: new(int64(4218))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Min: c.min, Max: c.max})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
			assert.Equal(t, len(c.want), got.Matched)
		})
	}
}

func Test_search_amount_compares_the_transaction_not_its_splits(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{id: "two", sourceID: 1, parts: []searchPart{
		{category: new(catExpense), sourceID: 1, cents: -3000}, {category: new(catSearchFuel), sourceID: 2, cents: -4000},
	}})
	cases := []struct {
		name     string
		min, max *int64
		want     []string
	}{
		{name: "min at the transaction total finds it once", min: new(int64(7000)), want: []string{"txn-two"}},
		{name: "min above the total finds nothing", min: new(int64(7001))},
		{name: "max below the total finds nothing though each split is below it", max: new(int64(6999))},
		{name: "max at the total finds it once", max: new(int64(7000)), want: []string{"txn-two"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Min: c.min, Max: c.max})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
			assert.Equal(t, len(c.want), got.Matched)
		})
	}
}

func Test_search_amount_combines_with_text_window_and_two_accounts(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for _, g := range []struct {
		id      string
		payee   string
		account string
		day     int
		cents   int64
	}{
		{id: "g1", payee: "Gym", account: acctInReports, day: 10, cents: 6000},
		{id: "g2", payee: "Gym", account: acctSecond, day: 11, cents: 7000},
		{id: "g3", payee: "Gym", account: acctNotReports, day: 12, cents: 8000},
		{id: "g4", payee: "Bakery", account: acctInReports, day: 13, cents: 9000},
		{id: "g5", payee: "Gym", account: acctInReports, day: 5, cents: 6000},
		{id: "g6", payee: "Gym", account: acctInReports, day: 14, cents: 1000},
	} {
		spec := textTxn(&rows, g.id, int64(g.day), g.payee, "", "")
		spec.parts[0].cents = -g.cents
		spec.account, spec.date = g.account, day(2026, time.March, g.day)
		addSearch(&rows, spec)
	}
	since := day(2026, time.March, 8)

	got := searchOf(t, rows, store.SearchParams{
		Text: "gym", Window: store.SearchWindow{Since: &since}, AccountIDs: []string{acctInReports, acctSecond}, Min: new(int64(5000)),
	})

	assert.Equal(t, []string{"txn-g2", "txn-g1"}, searchedIDs(got))
	assert.Equal(t, 2, got.Matched)
}

func Test_search_amount_with_limit_counts_every_amount_match(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for i, cents := range []int64{-9000, 8000, -7000, 100} {
		spec := signedTxn(string(rune('a'+i)), int64(i+1), cents)
		spec.date = day(2026, time.March, 10+i)
		addSearch(&rows, spec)
	}

	got := searchOf(t, rows, store.SearchParams{Min: new(int64(7000)), Limit: 2})

	require.Equal(t, []string{"txn-c", "txn-b"}, searchedIDs(got))
	assert.Equal(t, 3, got.Matched)
}
