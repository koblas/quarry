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

// searchPart is one split of a searchSpec; sourceID orders the splits of one transaction and nil
// category, memo or transferTo mean none.
type searchPart struct {
	category, memo, transferTo *string
	sourceID                   int64
	cents                      int64
}

// searchSpec is one transaction of any number of splits; account defaults to acctInReports, date to 2026-03-15
// and the currency to CAD. Its splits are named "<id>-<index>".
type searchSpec struct {
	id       string
	sourceID int64
	account  string
	date     time.Time
	payee    *string
	memo     *string
	excluded bool
	parts    []searchPart
}

// addSearch appends spec's transaction and its splits to rows.
func addSearch(rows *store.Rows, spec searchSpec) {
	account, date := spec.account, spec.date
	if account == "" {
		account = acctInReports
	}
	if date.IsZero() {
		date = day(2026, time.March, 15)
	}
	var amount int64
	for i, part := range spec.parts {
		amount += part.cents
		rows.Splits = append(rows.Splits, store.Split{
			ID: spec.id + "-" + string(rune('0'+i)), SourceID: part.sourceID, TransactionID: "txn-" + spec.id,
			CategoryID: part.category, Memo: part.memo, Amount: part.cents, TransferAccountID: part.transferTo,
		})
	}
	rows.Transactions = append(rows.Transactions, store.Transaction{
		ID: "txn-" + spec.id, SourceID: spec.sourceID, AccountID: account, Date: date, PayeeID: spec.payee,
		Memo: spec.memo, Amount: amount, Currency: "CAD", Status: "uncleared", ExcludedFromReports: spec.excluded,
	})
}

// searchRowsFor is accountRows (four reported accounts, one not in reports, one linked) with a fuel
// category and the payees "payee-gym" and "payee-bakery".
func searchRowsFor() store.Rows {
	rows := accountRows()
	rows.Categories = append(rows.Categories, expenseCategory(catSearchFuel, "Auto:Fuel"))
	rows.Payees = []store.Payee{{ID: payeeGym, SourceID: 1, Name: nameGym}, {ID: "payee-bakery", SourceID: 2, Name: "Bakery"}}
	return rows
}

// spend is the one-split spec of an expense of cents in catExpense.
func spend(id string, sourceID, cents int64) searchSpec {
	return searchSpec{id: id, sourceID: sourceID, parts: []searchPart{{category: new(catExpense), sourceID: 1, cents: -cents}}}
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

func Test_search_returns_the_span_query_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault(`query rows "SELECT min"`)
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, queryFault: fault}))

	_, err := st.Search(t.Context(), store.SearchParams{})

	assertOtherFault(t, err, "disk read failed")
	assert.ErrorIs(t, err, fault)
}

func Test_search_returns_a_span_scan_fault_as_another_fault(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{passQueries: 1, scanFault: errScanFailed}))

	_, err := st.Search(t.Context(), store.SearchParams{})

	assertOtherFault(t, err, errScanFailed.Error())
	assert.ErrorIs(t, err, errScanFailed)
}
