// White-box: renderSearch's caption, cell and footer rules are unexported layout, driven directly.
package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// searchRowOf is a CAD Chequing transaction on 2026-03-10 of -42.17 with one categorized split and no flags.
func searchRowOf() store.SearchRow {
	return store.SearchRow{
		TransactionID: "txn-1",
		Date:          time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC),
		Account:       store.Account{ID: "acct-1", Name: "Chequing", Currency: "CAD", Active: true},
		Payee:         new("Costco"),
		Amount:        -4217,
		Currency:      "CAD",
		Splits:        []store.SearchSplit{{Category: new("Food:Groceries"), Amount: -4217}},
	}
}

func foundRows(matched int, rows ...store.SearchRow) report.Search {
	var found report.Search
	found.Rows, found.Matched = rows, matched
	return found
}

func searchDay(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

func Test_renderSearch_pads_each_column_right_aligns_the_amount_and_trims_trailing_spaces(t *testing.T) {
	longer := searchRowOf()
	longer.Payee, longer.Amount = new("Bell Canada"), -184210
	longer.Splits = []store.SearchSplit{{Category: new("Utilities:Phone"), Amount: -184210}}
	longer.Memo = new("autopay")
	longer.Excluded = true
	shorter := searchRowOf()

	got := renderSearch(foundRows(2, longer, shorter))

	want := "Transactions in all accounts, all dates\n\n" +
		"Date        Account         Payee        Category         Memo        Amount  Flags\n" +
		"2026-03-10  Chequing (CAD)  Bell Canada  Utilities:Phone  autopay  -1,842.10  excluded\n" +
		"2026-03-10  Chequing (CAD)  Costco       Food:Groceries               -42.17\n" +
		"\n" +
		"2 matching transactions\n"
	assert.Equal(t, want, got)
}

func Test_renderSearch_footer_counts_every_match_not_the_rows_listed(t *testing.T) {
	got := renderSearch(foundRows(1234, searchRowOf()))

	assert.True(t, strings.HasSuffix(got, "\n\n1,234 matching transactions\n"), got)
}

func Test_renderSearch_footer_uses_the_singular_for_one_match(t *testing.T) {
	got := renderSearch(foundRows(1, searchRowOf()))

	assert.True(t, strings.HasSuffix(got, "\n\n1 matching transaction\n"), got)
}

func Test_renderSearch_prints_the_caption_header_and_a_zero_footer_when_nothing_matched(t *testing.T) {
	got := renderSearch(foundRows(0))

	want := "Transactions in all accounts, all dates\n\n" +
		"Date  Account  Payee  Category  Memo  Amount  Flags\n" +
		"\n" +
		"0 matching transactions\n"
	assert.Equal(t, want, got)
}

func Test_searchCaption_names_the_dates_the_window_bounds(t *testing.T) {
	cases := []struct {
		name   string
		window store.SearchWindow
		want   string
	}{
		{name: "no bound searches all dates", window: store.SearchWindow{}, want: "Transactions in all accounts, all dates"},
		{name: "since only reads from", window: store.SearchWindow{Since: searchDay(2026, time.January, 1)}, want: "Transactions in all accounts, from 2026-01-01"},
		{name: "until only reads through", window: store.SearchWindow{Until: searchDay(2025, time.December, 31)}, want: "Transactions in all accounts, through 2025-12-31"},
		{
			name:   "both bounds read as a range",
			window: store.SearchWindow{Since: searchDay(2026, time.January, 1), Until: searchDay(2026, time.March, 31)},
			want:   "Transactions in all accounts, 2026-01-01 to 2026-03-31",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, searchCaption(report.Search{Window: c.window}))
		})
	}
}

func Test_searchCaption_names_the_accounts_the_search_was_limited_to(t *testing.T) {
	s := report.Search{Accounts: []store.Account{{Name: "Chequing"}, {Name: "Sav\nings"}}}

	assert.Equal(t, `Transactions in Chequing, Sav\nings, all dates`, searchCaption(s))
}

func Test_searchCaption_names_the_amount_range_after_the_dates(t *testing.T) {
	cents := func(c int64) *int64 { return &c }
	cases := []struct {
		name    string
		amounts report.SearchAmounts
		want    string
	}{
		{name: "no bound adds nothing", amounts: report.SearchAmounts{}, want: "Transactions in all accounts, all dates"},
		{name: "both bounds are a range", amounts: report.SearchAmounts{Min: cents(2000), Max: cents(5000)}, want: "Transactions in all accounts, all dates, amount 20.00 to 50.00"},
		{name: "min alone is at least", amounts: report.SearchAmounts{Min: cents(2000)}, want: "Transactions in all accounts, all dates, amount at least 20.00"},
		{name: "max alone is at most", amounts: report.SearchAmounts{Max: cents(5000)}, want: "Transactions in all accounts, all dates, amount at most 50.00"},
		{name: "equal bounds are exactly", amounts: report.SearchAmounts{Min: cents(4217), Max: cents(4217)}, want: "Transactions in all accounts, all dates, amount exactly 42.17"},
		{name: "a bound of zero is still a bound", amounts: report.SearchAmounts{Min: cents(0)}, want: "Transactions in all accounts, all dates, amount at least 0.00"},
		{name: "amounts are grouped by thousands", amounts: report.SearchAmounts{Min: cents(123400), Max: cents(123401)}, want: "Transactions in all accounts, all dates, amount 1,234.00 to 1,234.01"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, searchCaption(report.Search{Amounts: c.amounts}))
		})
	}
}

func Test_searchCaption_puts_the_amount_after_the_text_accounts_and_dates(t *testing.T) {
	least := int64(10000)
	s := report.Search{
		Text:     new("costco"),
		Accounts: []store.Account{{Name: "Visa"}},
		Window:   store.SearchWindow{Since: searchDay(2025, time.January, 1)},
		Amounts:  report.SearchAmounts{Min: &least},
	}

	assert.Equal(t, `Transactions matching "costco" in Visa, from 2025-01-01, amount at least 100.00`, searchCaption(s))
}

func Test_searchCategoryCell_labels_each_split_once_in_split_order(t *testing.T) {
	cases := []struct {
		name   string
		splits []store.SearchSplit
		want   string
	}{
		{name: "no splits is uncategorized", splits: nil, want: "(uncategorized)"},
		{name: "a category shows its path", splits: []store.SearchSplit{{Category: new("Food:Groceries")}}, want: "Food:Groceries"},
		{name: "a split without a category is uncategorized", splits: []store.SearchSplit{{}}, want: "(uncategorized)"},
		{name: "a transfer leg is a transfer", splits: []store.SearchSplit{{Transfer: true}}, want: "(transfer)"},
		{
			name:   "splits keep their order",
			splits: []store.SearchSplit{{Category: new("Fuel")}, {Category: new("Food")}, {}},
			want:   "Fuel, Food, (uncategorized)",
		},
		{
			name:   "a repeated label shows once",
			splits: []store.SearchSplit{{Category: new("Food")}, {Category: new("Fuel")}, {Category: new("Food")}, {}, {}},
			want:   "Food, Fuel, (uncategorized)",
		},
		{name: "a path is escaped", splits: []store.SearchSplit{{Category: new("A\tB")}}, want: `A\tB`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, searchCategoryCell(c.splits))
		})
	}
}

func Test_searchMemoCell_lists_the_transaction_memo_then_each_other_split_memo(t *testing.T) {
	cases := []struct {
		name  string
		memo  *string
		memos []*string
		want  string
	}{
		{name: "no memo at all is blank", want: ""},
		{name: "the transaction memo alone", memo: new("bulk run"), want: "bulk run"},
		{name: "a split memo alone", memos: []*string{new("milk")}, want: "milk"},
		{name: "transaction memo first then split memos in order", memo: new("bulk run"), memos: []*string{new("milk"), new("eggs")}, want: "bulk run / milk / eggs"},
		{name: "a split memo equal to the transaction memo shows once", memo: new("bulk run"), memos: []*string{new("bulk run")}, want: "bulk run"},
		{name: "a repeated split memo shows once", memos: []*string{new("milk"), new("milk")}, want: "milk"},
		{name: "a nil or empty split memo adds nothing", memo: new("bulk run"), memos: []*string{nil, new("")}, want: "bulk run"},
		{name: "memos are escaped", memo: new("a\nb"), memos: []*string{new("c\rd")}, want: `a\nb / c\rd`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := searchRowOf()
			r.Memo = c.memo
			r.Splits = nil
			for _, m := range c.memos {
				r.Splits = append(r.Splits, store.SearchSplit{Memo: m})
			}

			assert.Equal(t, c.want, searchMemoCell(r))
		})
	}
}

func Test_searchFlagsCell_names_the_flags_a_transaction_carries(t *testing.T) {
	cases := []struct {
		name               string
		transfer, excluded bool
		want               string
	}{
		{name: "neither is blank", want: ""},
		{name: "transfer", transfer: true, want: "transfer"},
		{name: "excluded", excluded: true, want: "excluded"},
		{name: "both", transfer: true, excluded: true, want: "transfer, excluded"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := searchRowOf()
			r.Transfer, r.Excluded = c.transfer, c.excluded

			assert.Equal(t, c.want, searchFlagsCell(r))
		})
	}
}

func Test_renderSearch_labels_closed_and_usd_accounts_and_a_missing_payee(t *testing.T) {
	closed := searchRowOf()
	closed.Account = store.Account{Name: "Old Card", Currency: "USD", Closed: true}
	closed.Payee, closed.Amount = nil, 150000

	got := renderSearch(foundRows(1, closed))

	assert.Contains(t, got, "2026-03-10  Old Card (USD, closed)  (no payee)  Food:Groceries")
	assert.Contains(t, got, "  1,500.00\n")
}

func Test_renderSearch_escapes_account_payee_category_and_memo(t *testing.T) {
	r := searchRowOf()
	r.Account.Name = "Cheq\nuing"
	r.Payee = new("Cost\tco")
	r.Memo = new("a\rb")
	r.Splits = []store.SearchSplit{{Category: new("Fo\nod")}}

	got := renderSearch(foundRows(1, r))

	assert.Contains(t, got, `2026-03-10  Cheq\nuing (CAD)  Cost\tco  Fo\nod    a\rb  -42.17`)
	assert.Equal(t, 6, strings.Count(got, "\n"), got)
}

func Test_searchCaption_names_the_text_right_after_transactions(t *testing.T) {
	text := "costco"
	since := searchDay(2026, time.January, 1)
	cases := []struct {
		name  string
		found report.Search
		want  string
	}{
		{name: "text alone", found: report.Search{Text: &text}, want: `Transactions matching "costco" in all accounts, all dates`},
		{
			name: "text with accounts", found: report.Search{Text: &text, Accounts: []store.Account{{Name: "Chequing"}}},
			want: `Transactions matching "costco" in Chequing, all dates`,
		},
		{
			name:  "text with a since bound",
			found: report.Search{Text: &text, Window: store.SearchWindow{Since: since}},
			want:  `Transactions matching "costco" in all accounts, from 2026-01-01`,
		},
		{
			name:  "text with an until bound",
			found: report.Search{Text: &text, Window: store.SearchWindow{Until: since}},
			want:  `Transactions matching "costco" in all accounts, through 2026-01-01`,
		},
		{
			name:  "text with both bounds",
			found: report.Search{Text: &text, Window: store.SearchWindow{Since: since, Until: searchDay(2026, time.March, 31)}},
			want:  `Transactions matching "costco" in all accounts, 2026-01-01 to 2026-03-31`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, searchCaption(c.found))
		})
	}
}

func Test_searchCaption_quotes_text_that_has_a_quote_or_a_newline(t *testing.T) {
	text := "say \"hi\"\n"

	assert.Equal(t, `Transactions matching "say \"hi\"\n" in all accounts, all dates`, searchCaption(report.Search{Text: &text}))
}
