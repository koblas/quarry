package duckstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const catSearchFuel = "cat-search-fuel"

// searchRowsFor is accountRows (four reported accounts, one not in reports, one linked) with a fuel
// category and the payees "payee-gym" and "payee-bakery".
func searchRowsFor() store.Rows {
	rows := accountRows()
	rows.Categories = append(rows.Categories, expenseCategory(catSearchFuel, "Auto:Fuel"))
	rows.Payees = []store.Payee{{ID: payeeGym, SourceID: 1, Name: nameGym}, {ID: "payee-bakery", SourceID: 2, Name: "Bakery"}}
	return rows
}

func searchOf(t *testing.T, rows store.Rows, params store.SearchParams) store.Search {
	t.Helper()
	got, err := newStoreWith(t, rows).Search(t.Context(), params)
	require.NoError(t, err)
	return got
}

func searchedIDs(found store.Search) []string {
	ids := make([]string, len(found.Rows))
	for i, row := range found.Rows {
		ids[i] = row.TransactionID
	}
	return ids
}

// searchFlags are the flags of one listed transaction: its own, then each split's transfer flag.
type searchFlags struct {
	transfer, excluded bool
	splits             []bool
}

func flagsOf(row store.SearchRow) searchFlags {
	flags := searchFlags{transfer: row.Transfer, excluded: row.Excluded}
	for _, split := range row.Splits {
		flags.splits = append(flags.splits, split.Transfer)
	}
	return flags
}

func Test_search_flags_transfers_and_excluded_from_the_cash_flow_fragments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		spec  searchSpec
		xfers []store.Transfer
		want  searchFlags
	}{
		{
			name: "a transaction no exclusion touches has neither flag", spec: spend("case", 1, 500),
			want: searchFlags{splits: []bool{false}},
		},
		{
			name: "the from leg of a paired transfer", spec: spend("case", 1, 500),
			xfers: []store.Transfer{{ID: "x", FromSplitID: "case-0", ToSplitID: new("peer-leg")}},
			want:  searchFlags{transfer: true, splits: []bool{true}},
		},
		{
			name: "the to leg of a paired transfer", spec: spend("case", 1, 500),
			xfers: []store.Transfer{{ID: "x", FromSplitID: "peer-leg", ToSplitID: new("case-0")}},
			want:  searchFlags{transfer: true, splits: []bool{true}},
		},
		{
			name: "an unmatched transfer leg with no transfer account", spec: spend("case", 1, 500),
			xfers: []store.Transfer{{ID: "x", FromSplitID: "case-0"}},
			want:  searchFlags{transfer: true, splits: []bool{true}},
		},
		{
			name: "a split naming a transfer account that no transfer row records",
			spec: searchSpec{id: "case", sourceID: 1, parts: []searchPart{{transferTo: new(acctSecond), sourceID: 1, cents: -500}}},
			want: searchFlags{splits: []bool{false}},
		},
		{
			name: "a transaction whose second split is the transfer leg",
			spec: searchSpec{id: "case", sourceID: 1, parts: []searchPart{
				{category: new(catExpense), sourceID: 1, cents: -300}, {category: new(catExpense), sourceID: 2, cents: -200},
			}},
			xfers: []store.Transfer{{ID: "x", FromSplitID: "case-1"}},
			want:  searchFlags{transfer: true, splits: []bool{false, true}},
		},
		{
			name: "a transaction marked exclude from reports",
			spec: searchSpec{id: "case", sourceID: 1, excluded: true, parts: spend("case", 1, 500).parts},
			want: searchFlags{excluded: true, splits: []bool{false}},
		},
		{
			name: "a transaction in an account not in reports",
			spec: searchSpec{id: "case", sourceID: 1, account: acctNotReports, parts: spend("case", 1, 500).parts},
			want: searchFlags{excluded: true, splits: []bool{false}},
		},
		{
			name: "a transaction in an account using linked tracking",
			spec: searchSpec{id: "case", sourceID: 1, account: acctLinked, parts: spend("case", 1, 500).parts},
			want: searchFlags{excluded: true, splits: []bool{false}},
		},
		{
			name:  "a transfer leg that is also excluded",
			spec:  searchSpec{id: "case", sourceID: 1, excluded: true, parts: spend("case", 1, 500).parts},
			xfers: []store.Transfer{{ID: "x", FromSplitID: "case-0", ToSplitID: new("peer-leg")}},
			want:  searchFlags{transfer: true, excluded: true, splits: []bool{true}},
		},
		{
			name: "a split in a system category is not flagged",
			spec: searchSpec{id: "case", sourceID: 1, parts: []searchPart{{category: new(catSystem), sourceID: 1, cents: -500}}},
			want: searchFlags{splits: []bool{false}},
		},
		{
			name: "a zero-amount uncategorized split is not flagged",
			spec: searchSpec{id: "case", sourceID: 1, parts: []searchPart{{sourceID: 1}}},
			want: searchFlags{splits: []bool{false}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rows := searchRowsFor()
			addSearch(&rows, c.spec)
			rows.Transfers = append(rows.Transfers, c.xfers...)

			got := searchOf(t, rows, store.SearchParams{})

			require.Len(t, got.Rows, 1)
			assert.Equal(t, c.want, flagsOf(got.Rows[0]))
		})
	}
}

func Test_search_lists_newest_first_breaking_a_date_tie_by_source_id(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	older, lower, higher := spend("older", 1, 100), spend("lower", 5, 200), spend("higher", 9, 300)
	older.date, lower.date, higher.date = day(2026, time.March, 10), day(2026, time.March, 12), day(2026, time.March, 12)
	addSearch(&rows, older)
	addSearch(&rows, higher)
	addSearch(&rows, lower)

	got := searchOf(t, rows, store.SearchParams{})

	assert.Equal(t, []string{"txn-higher", "txn-lower", "txn-older"}, searchedIDs(got))
}

func Test_search_keeps_the_higher_source_id_of_a_same_date_pair_when_the_limit_cuts(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	older, lower, higher := spend("older", 1, 100), spend("lower", 5, 200), spend("higher", 9, 300)
	older.date, lower.date, higher.date = day(2026, time.March, 10), day(2026, time.March, 12), day(2026, time.March, 12)
	addSearch(&rows, older)
	addSearch(&rows, lower)
	addSearch(&rows, higher)

	got := searchOf(t, rows, store.SearchParams{Limit: 1})

	assert.Equal(t, []string{"txn-higher"}, searchedIDs(got))
}

func Test_search_counts_every_match_when_the_limit_cuts(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	newest := searchSpec{id: "newest", sourceID: 3, date: day(2026, time.March, 12), parts: []searchPart{
		{category: new(catExpense), sourceID: 1, cents: -300}, {category: new(catSearchFuel), sourceID: 2, cents: -200},
	}}
	middle, oldest := spend("middle", 2, 100), spend("oldest", 1, 100)
	middle.date, oldest.date = day(2026, time.March, 11), day(2026, time.March, 10)
	addSearch(&rows, oldest)
	addSearch(&rows, middle)
	addSearch(&rows, newest)
	cases := []struct {
		name        string
		limit       int
		wantRows    int
		wantMatched int
	}{
		{name: "a limit of one cuts two", limit: 1, wantRows: 1, wantMatched: 3},
		{name: "a limit equal to the matches cuts nothing", limit: 3, wantRows: 3, wantMatched: 3},
		{name: "a limit of zero lists every match", limit: 0, wantRows: 3, wantMatched: 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Limit: c.limit})

			assert.Equal(t, [2]int{c.wantRows, c.wantMatched}, [2]int{len(got.Rows), got.Matched})
		})
	}
}

func Test_search_keeps_two_transactions_sharing_a_date_and_source_id_apart(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for _, id := range []string{"a", "b"} {
		addSearch(&rows, searchSpec{id: id, sourceID: 4, parts: []searchPart{
			{memo: new(id + " first"), sourceID: 1, cents: -100},
			{memo: new(id + " second"), sourceID: 2, cents: -200},
		}})
	}

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 2)
	assert.Equal(t, []string{"txn-b", "txn-a"}, searchedIDs(got))
	assert.Equal(t, []store.SearchSplit{{Memo: new("b first"), Amount: -100}, {Memo: new("b second"), Amount: -200}}, got.Rows[0].Splits)
	assert.Equal(t, []store.SearchSplit{{Memo: new("a first"), Amount: -100}, {Memo: new("a second"), Amount: -200}}, got.Rows[1].Splits)
}

func Test_search_keeps_every_split_of_a_transaction_the_limit_keeps(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	newest := searchSpec{id: "newest", sourceID: 3, date: day(2026, time.March, 12), parts: []searchPart{
		{category: new(catExpense), sourceID: 1, cents: -300}, {category: new(catSearchFuel), sourceID: 2, cents: -200},
	}}
	older := spend("older", 1, 100)
	older.date = day(2026, time.March, 10)
	addSearch(&rows, older)
	addSearch(&rows, newest)

	got := searchOf(t, rows, store.SearchParams{Limit: 1})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, []store.SearchSplit{
		{Category: new("Groceries"), Amount: -300},
		{Category: new("Auto:Fuel"), Amount: -200},
	}, got.Rows[0].Splits)
}

func Test_search_lists_each_transactions_splits_in_source_order(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{id: "three", sourceID: 1, parts: []searchPart{
		{memo: new("third"), sourceID: 3, cents: -300},
		{memo: new("first"), sourceID: 1, cents: -100},
		{memo: new("second"), sourceID: 2, cents: -200},
	}})

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, []store.SearchSplit{
		{Memo: new("first"), Amount: -100},
		{Memo: new("second"), Amount: -200},
		{Memo: new("third"), Amount: -300},
	}, got.Rows[0].Splits)
}

func Test_search_lists_a_transaction_without_splits_with_none(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{id: "bare", sourceID: 1})

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 1)
	assert.Empty(t, got.Rows[0].Splits)
	assert.Equal(t, 1, got.Matched)
}

func Test_search_gives_no_memo_for_a_null_or_empty_memo(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{id: "null", sourceID: 1, parts: []searchPart{{sourceID: 1, cents: -100}}})
	addSearch(&rows, searchSpec{id: "empty", sourceID: 2, memo: new(""), parts: []searchPart{{memo: new(""), sourceID: 1, cents: -100}}})
	addSearch(&rows, searchSpec{id: "kept", sourceID: 3, memo: new("a memo"), parts: []searchPart{{memo: new("split memo"), sourceID: 1, cents: -100}}})

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 3)
	assert.Equal(t, []*string{new("a memo"), nil, nil}, []*string{got.Rows[0].Memo, got.Rows[1].Memo, got.Rows[2].Memo})
	assert.Equal(t, []*string{new("split memo"), nil, nil},
		[]*string{got.Rows[0].Splits[0].Memo, got.Rows[1].Splits[0].Memo, got.Rows[2].Splits[0].Memo})
}

func Test_search_gives_no_category_to_an_uncategorized_split_or_a_transfer_leg(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{id: "none", sourceID: 1, parts: []searchPart{
		{sourceID: 1, cents: -100},
		{transferTo: new(acctSecond), sourceID: 2, cents: -100},
		{category: new(catExpense), sourceID: 3, cents: -100},
	}})
	rows.Transfers = append(rows.Transfers, store.Transfer{ID: "x", FromSplitID: "none-1", ToSplitID: new("peer-leg")})

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 1)
	assert.Equal(t, []*string{nil, nil, new("Groceries")},
		[]*string{got.Rows[0].Splits[0].Category, got.Rows[0].Splits[1].Category, got.Rows[0].Splits[2].Category})
}

func Test_search_carries_the_transactions_account_payee_and_native_amount(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	addSearch(&rows, searchSpec{
		id: "one", sourceID: 1, account: acctSecond, payee: new(payeeGym), date: day(2026, time.March, 12),
		parts: []searchPart{{category: new(catExpense), sourceID: 1, cents: -4217}},
	})
	addSearch(&rows, searchSpec{id: "two", sourceID: 2, date: day(2026, time.March, 11)})

	got := searchOf(t, rows, store.SearchParams{})

	require.Len(t, got.Rows, 2)
	assert.Equal(t, store.SearchRow{
		TransactionID: "txn-one", Date: day(2026, time.March, 12),
		Account: store.Account{ID: acctSecond, Name: "Savings", Currency: "CAD", Active: true},
		Payee:   new(nameGym), Amount: -4217, Currency: "CAD",
		Splits: []store.SearchSplit{{Category: new("Groceries"), Amount: -4217}},
	}, got.Rows[0])
	assert.Nil(t, got.Rows[1].Payee)
}

func Test_search_bounds_dates_only_where_given(t *testing.T) {
	t.Parallel()
	first, last := day(2026, time.March, 11), day(2026, time.March, 13)
	middle := day(2026, time.March, 12)
	cases := []struct {
		name   string
		window store.SearchWindow
		want   []string
	}{
		{name: "neither bound lists every date", want: []string{"txn-d14", "txn-d13", "txn-d12", "txn-d11", "txn-d10"}},
		{name: "since alone lists from that day on", window: store.SearchWindow{Since: &middle}, want: []string{"txn-d14", "txn-d13", "txn-d12"}},
		{name: "until alone lists through that day", window: store.SearchWindow{Until: &middle}, want: []string{"txn-d12", "txn-d11", "txn-d10"}},
		{name: "both bounds list both days inclusive", window: store.SearchWindow{Since: &first, Until: &last}, want: []string{"txn-d13", "txn-d12", "txn-d11"}},
	}
	rows := searchRowsFor()
	for i, d := range []int{10, 11, 12, 13, 14} {
		spec := spend(fmt.Sprintf("d%02d", d), int64(i+1), 100)
		spec.date = day(2026, time.March, d)
		addSearch(&rows, spec)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Window: c.window})

			assert.Equal(t, c.want, searchedIDs(got))
		})
	}
}

func Test_search_keeps_only_the_named_accounts(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for i, account := range []string{acctInReports, acctSecond, acctNotReports} {
		spec := spend("on"+account, int64(i+1), 100)
		spec.account = account
		addSearch(&rows, spec)
	}
	for _, c := range []struct {
		name     string
		accounts []string
		want     []string
	}{
		{name: "none lists every account", want: []string{"txn-onacct-out", "txn-onacct-second", "txn-onacct-in"}},
		{name: "one lists that account only", accounts: []string{acctSecond}, want: []string{"txn-onacct-second"}},
		{name: "two list both", accounts: []string{acctInReports, acctSecond}, want: []string{"txn-onacct-second", "txn-onacct-in"}},
		{name: "an account not in reports is still searched", accounts: []string{acctNotReports}, want: []string{"txn-onacct-out"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{AccountIDs: c.accounts})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
		})
	}
}

func Test_search_spans_every_transaction_whatever_the_window_and_limit(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	old, recent := spend("old", 1, 100), spend("recent", 2, 100)
	old.date, recent.date = day(2003, time.January, 4), day(2027, time.February, 2)
	addSearch(&rows, old)
	addSearch(&rows, recent)
	after := day(2030, time.January, 1)

	got := searchOf(t, rows, store.SearchParams{Window: store.SearchWindow{Since: &after}, Limit: 1})

	assert.Empty(t, got.Rows)
	assert.Equal(t, store.TransactionRange{First: day(2003, time.January, 4), Last: day(2027, time.February, 2)}, got.Transactions)
}

func Test_search_spans_the_named_accounts_transactions_even_when_left_out_of_reports(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	inReports, leftOut := spend("in", 1, 100), spend("out", 2, 100)
	inReports.date = day(2003, time.January, 4)
	leftOut.account, leftOut.date = acctNotReports, day(2026, time.February, 3)
	addSearch(&rows, inReports)
	addSearch(&rows, leftOut)

	got := searchOf(t, rows, store.SearchParams{AccountIDs: []string{acctNotReports}})

	assert.Equal(t, store.TransactionRange{First: day(2026, time.February, 3), Last: day(2026, time.February, 3)}, got.Transactions)
}

func Test_search_gives_no_rows_and_the_zero_range_for_a_store_without_transactions(t *testing.T) {
	t.Parallel()

	got := searchOf(t, searchRowsFor(), store.SearchParams{})

	assert.Equal(t, store.Search{}, got)
}

func Test_search_returns_a_span_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	for _, c := range otherFaults("SELECT min", 1) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			_, err := st.Search(t.Context(), store.SearchParams{})

			assertOtherFault(t, err, c.reason)
			assert.ErrorIs(t, err, c.fault)
		})
	}
}

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

func Test_search_amount_compares_the_largest_amount_the_column_holds(t *testing.T) {
	t.Parallel()
	const top = int64(999999999999999999)
	rows := searchRowsFor()
	addSearch(&rows, signedTxn("deposit", 1, top))
	addSearch(&rows, signedTxn("charge", 2, -top))
	both := []listedAmount{{"txn-deposit", top}, {"txn-charge", -top}}
	cases := []struct {
		name     string
		min, max *int64
		want     []listedAmount
	}{
		{name: "min at the largest amount finds both signs", min: new(top), want: both},
		{name: "min one cent above it finds nothing", min: new(top + 1)},
		{name: "max at the largest amount finds both signs", max: new(top), want: both},
		{name: "max one cent below it finds nothing", max: new(top - 1)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Min: c.min, Max: c.max})

			assert.ElementsMatch(t, c.want, listedAmounts(got))
		})
	}
}

// listedAmount is a listed transaction's id with the amount the store returned for it.
type listedAmount struct {
	id     string
	amount int64
}

func listedAmounts(found store.Search) []listedAmount {
	listed := make([]listedAmount, len(found.Rows))
	for i, row := range found.Rows {
		listed[i] = listedAmount{row.TransactionID, row.Amount}
	}
	return listed
}

const (
	catFood      = "cat-food"
	catGroceries = "cat-groceries"
	catOrganic   = "cat-organic"
	catFoo       = "cat-foo"
	catArchive   = "cat-archive"
	catTravel    = "cat-travel"
)

// categoryRows holds a transaction named after each of Food, Food:Groceries, Food:Groceries:Organic, Foo and the
// hidden Archive, ids "txn-<name>"; the category Travel has none.
func categoryRows() store.Rows {
	rows := searchRowsFor()
	hidden := expenseCategory(catArchive, "Archive")
	hidden.Hidden = true
	rows.Categories = append(rows.Categories,
		expenseCategory(catFood, "Food"), expenseCategory(catGroceries, "Food:Groceries"), expenseCategory(catOrganic, "Food:Groceries:Organic"),
		expenseCategory(catFoo, "Foo"), hidden, expenseCategory(catTravel, "Travel"))
	for i, t := range []struct{ name, category string }{
		{"food", catFood}, {"groceries", catGroceries}, {"organic", catOrganic}, {"foo", catFoo}, {"archive", catArchive},
	} {
		addSearch(&rows, categorized(t.name, int64(i+1), t.category))
	}
	return rows
}

func Test_search_category_matches_the_category_and_everything_under_it(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	cases := []struct {
		name     string
		category string
		want     []string
	}{
		{name: "the category and every category under it", category: "Food", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "a lower-case argument", category: "food", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "an upper-case argument reaches the children", category: "FOOD", want: []string{"txn-food", "txn-groceries", "txn-organic"}},
		{name: "a child and its own child", category: "FOOD:GROCERIES", want: []string{"txn-groceries", "txn-organic"}},
		{name: "a grandchild alone", category: "food:groceries:organic", want: []string{"txn-organic"}},
		{name: "a sibling prefix is not the category", category: "Foo", want: []string{"txn-foo"}},
		{name: "half of a child's name is no category", category: "Food:Groc"},
		{name: "a hidden category counts", category: "Archive", want: []string{"txn-archive"}},
		{name: "a known category with no transactions", category: "Travel"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Category: &c.category})

			assert.ElementsMatch(t, c.want, searchedIDs(got))
			assert.Equal(t, len(c.want), got.Matched)
		})
	}
}

func Test_search_category_counts_a_transaction_once_when_several_splits_match(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	addSearch(&rows, searchSpec{id: "both", sourceID: 9, parts: []searchPart{
		{category: new(catFood), sourceID: 1, cents: -100}, {category: new(catGroceries), sourceID: 2, cents: -200},
	}})
	category := "Food"

	got := searchOf(t, rows, store.SearchParams{Category: &category, Limit: 1})

	assert.Equal(t, []string{"txn-both"}, searchedIDs(got))
	assert.Equal(t, 4, got.Matched)
	assert.Len(t, got.Rows[0].Splits, 2)
}

func Test_search_category_keeps_both_filters_with_named_accounts_and_text(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	for _, g := range []struct {
		id, payee, account, category string
		day                          int
	}{
		{"hit", "Gym", acctInReports, catGroceries, 20},
		{"other-account", "Gym", acctSecond, catGroceries, 21},
		{"other-text", "Bakery", acctInReports, catGroceries, 22},
		{"other-category", "Gym", acctInReports, catFoo, 23},
		{"other-named", "Gym", acctNotReports, catGroceries, 24},
	} {
		spec := categorized(g.id, 100+int64(g.day), g.category)
		payeeID := "cat-payee-" + g.id
		rows.Payees = append(rows.Payees, store.Payee{ID: payeeID, SourceID: int64(g.day), Name: g.payee})
		spec.payee, spec.account, spec.date = &payeeID, g.account, day(2026, time.April, g.day)
		addSearch(&rows, spec)
	}
	category := "Food"

	got := searchOf(t, rows, store.SearchParams{Category: &category, Text: "gym", AccountIDs: []string{acctInReports, acctNotReports}})

	assert.Equal(t, []string{"txn-other-named", "txn-hit"}, searchedIDs(got))
	assert.False(t, got.UnknownCategory)
	assert.Equal(t, store.TransactionRange{First: day(2026, time.March, 15), Last: day(2026, time.April, 24)}, got.Transactions)
}

func Test_search_category_flags_unknown_only_when_no_path_equals_it(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	cases := []struct {
		name     string
		category *string
		want     bool
	}{
		{name: "no category given", category: nil},
		{name: "a category with transactions", category: new("Food")},
		{name: "a category in other letter case", category: new("food:GROCERIES")},
		{name: "a known category with no transactions", category: new("Travel")},
		{name: "a misspelling", category: new("Fod"), want: true},
		{name: "the empty path", category: new(""), want: true},
		{name: "a path ending in the separator", category: new("Food:"), want: true},
		{name: "a wildcard character", category: new("%"), want: true},
		{name: "half of a name", category: new("Food:Groc"), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Category: c.category})

			assert.Equal(t, c.want, got.UnknownCategory)
		})
	}
}

func Test_search_category_flags_unknown_with_named_accounts_whether_or_not_they_have_transactions(t *testing.T) {
	t.Parallel()
	rows := categoryRows()
	cases := []struct {
		name     string
		category string
		accounts []string
		want     bool
	}{
		{name: "a misspelling with an account that has transactions", category: "Fod", accounts: []string{acctInReports}, want: true},
		{name: "a misspelling with an account that has none", category: "Fod", accounts: []string{acctSecond}, want: true},
		{name: "a misspelling with both", category: "Fod", accounts: []string{acctInReports, acctSecond}, want: true},
		{name: "a known category no split uses with both", category: "Travel", accounts: []string{acctInReports, acctSecond}},
		{name: "a known category with an account that has none", category: "Food", accounts: []string{acctSecond}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := searchOf(t, rows, store.SearchParams{Category: &c.category, AccountIDs: c.accounts})

			assert.Equal(t, c.want, got.UnknownCategory)
			assert.Empty(t, got.Rows)
		})
	}
}

func Test_search_category_known_with_no_transactions_in_the_store_is_not_unknown(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	rows.Categories = append(rows.Categories, expenseCategory(catTravel, "Travel"))
	category := "travel"

	got := searchOf(t, rows, store.SearchParams{Category: &category})

	assert.Equal(t, store.Search{}, got)
}

func Test_search_category_runs_two_statements(t *testing.T) {
	t.Parallel()
	category := "Food"
	spy := &spyReadDB{passQueries: 2, queryFault: errQueryFailed}
	st := newBuiltStore(t, spyOpener(spy))

	_, err := st.Search(t.Context(), store.SearchParams{Category: &category})

	require.NoError(t, err)
}

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

func Test_search_text_with_limit_counts_every_text_match(t *testing.T) {
	t.Parallel()
	rows := searchRowsFor()
	for i, g := range []struct{ id, payee string }{
		{"gym-old", "Gym"}, {"bakery", "Bakery"}, {"gym-mid", "Gym"}, {"gym-new", "Gym"}, {"bakery-new", "Bakery"},
	} {
		spec := textTxn(&rows, g.id, int64(i+1), g.payee, "", "")
		spec.date = day(2026, time.March, 10+i)
		addSearch(&rows, spec)
	}

	got := searchOf(t, rows, store.SearchParams{Text: "gym", Limit: 2})

	require.Equal(t, []string{"txn-gym-new", "txn-gym-mid"}, searchedIDs(got))
	assert.Equal(t, 3, got.Matched)
}
