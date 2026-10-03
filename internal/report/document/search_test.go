package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func searchDay(month time.Month, dayOfMonth int) *time.Time {
	d := time.Date(2026, month, dayOfMonth, 0, 0, 0, 0, time.UTC)
	return &d
}

// searched is a search of Chequing, 2026-01-01 to 2026-03-31, matching 3 transactions of which 1 is listed.
func searched(rows ...store.SearchRow) report.Search {
	return report.Search{
		Rows: rows, Matched: 3,
		Window:   store.SearchWindow{Since: searchDay(time.January, 1), Until: searchDay(time.March, 31)},
		Accounts: []store.Account{{ID: "acct-1", Name: "Chequing"}},
		Limit:    1,
	}
}

func costcoRow() store.SearchRow {
	return store.SearchRow{
		TransactionID: "txn-1", Date: *searchDay(time.March, 2),
		Account: store.Account{ID: "acct-1", Name: "Chequing"},
		Payee:   new("Costco"), Memo: new("bulk run"), Amount: -10000, Currency: "CAD", Excluded: true,
		Splits: []store.SearchSplit{
			{Category: new("Food:Groceries"), Memo: new("milk"), Amount: -6000},
			{Amount: -4000, Transfer: true},
		},
	}
}

func Test_NewSearch_writes_every_key_in_the_ruled_order(t *testing.T) {
	got := indented(t, document.NewSearch(searched(costcoRow()), []string{"showing the newest 1"}))

	//nolint:testifylint // key order is the contract under test; JSONEq ignores it
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-03-31",
  "account_filter": [
    {
      "id": "acct-1",
      "name": "Chequing"
    }
  ],
  "text": null,
  "category": null,
  "min": null,
  "max": null,
  "limit": 1,
  "matched": 3,
  "truncated": true,
  "transactions": [
    {
      "transaction_id": "txn-1",
      "date": "2026-03-02",
      "account_id": "acct-1",
      "account": "Chequing",
      "payee": "Costco",
      "memo": "bulk run",
      "amount": "-100.00",
      "currency": "CAD",
      "transfer": false,
      "excluded": true,
      "splits": [
        {
          "category": "Food:Groceries",
          "memo": "milk",
          "amount": "-60.00",
          "transfer": false
        },
        {
          "category": null,
          "memo": null,
          "amount": "-40.00",
          "transfer": true
        }
      ]
    }
  ],
  "warnings": [
    "showing the newest 1"
  ]
}
`, got)
}

func Test_NewSearch_gives_null_for_an_absent_payee_memo_split_category_and_open_bounds(t *testing.T) {
	row := store.SearchRow{TransactionID: "txn-2", Date: *searchDay(time.March, 3), Account: store.Account{ID: "acct-1", Name: "Chequing"}, Currency: "CAD", Splits: []store.SearchSplit{{Amount: -100}}}
	search := report.Search{Rows: []store.SearchRow{row}, Matched: 1, Limit: 500}

	got := document.NewSearch(search, nil)

	assert.Nil(t, got.Since)
	assert.Nil(t, got.Until)
	assert.Nil(t, got.Transactions[0].Payee)
	assert.Nil(t, got.Transactions[0].Memo)
	assert.Nil(t, got.Transactions[0].Splits[0].Category)
	assert.Nil(t, got.Transactions[0].Splits[0].Memo)
}

func Test_NewSearch_writes_empty_arrays_for_no_accounts_transactions_splits_or_warnings(t *testing.T) {
	bare := store.SearchRow{TransactionID: "txn-3", Date: *searchDay(time.March, 3), Account: store.Account{ID: "acct-1", Name: "Chequing"}, Currency: "CAD"}
	cases := []struct {
		name   string
		search report.Search
		path   string
	}{
		{name: "account_filter", search: report.Search{}, path: `"account_filter": []`},
		{name: "transactions", search: report.Search{}, path: `"transactions": []`},
		{name: "splits", search: report.Search{Search: store.Search{Rows: []store.SearchRow{bare}, Matched: 1}}, path: `"splits": []`},
		{name: "warnings", search: report.Search{}, path: `"warnings": []`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := indented(t, document.NewSearch(c.search, nil))

			assert.Contains(t, got, c.path)
		})
	}
}

func Test_NewSearch_marks_truncated_only_when_the_limit_cut_matches(t *testing.T) {
	cases := []struct {
		name    string
		matched int
		listed  int
		want    bool
	}{
		{name: "cut", matched: 3, listed: 1, want: true},
		{name: "everything listed", matched: 2, listed: 2},
		{name: "nothing matched", matched: 0, listed: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			search := report.Search{Rows: make([]store.SearchRow, c.listed), Matched: c.matched}

			got := document.NewSearch(search, nil)

			assert.Equal(t, c.want, got.Truncated)
		})
	}
}
