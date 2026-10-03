package document_test

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

func Test_SearchWarnings_say_where_the_no_match_search_ran(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		span     store.TransactionRange
		want     string
	}{
		{
			name: "the store's transactions run",
			span: span,
			want: "no transactions match the search; the store's transactions run 2020-03-04 to 2025-12-31",
		},
		{
			name: "the store has none",
			want: "no transactions match the search; the store has no transactions",
		},
		{
			name:     "the named accounts' transactions run",
			accounts: []store.Account{included},
			span:     span,
			want:     "no transactions in the named accounts match the search; their transactions run 2020-03-04 to 2025-12-31",
		},
		{
			name:     "the named accounts have none",
			accounts: []store.Account{included},
			want:     "no transactions in the named accounts match the search; they have no transactions",
		},
		{
			name:     "a named left-out account is still searched",
			accounts: []store.Account{linked},
			span:     span,
			want:     "no transactions in the named accounts match the search; their transactions run 2020-03-04 to 2025-12-31",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.SearchWarnings(report.Search{Accounts: c.accounts, Transactions: c.span})

			assert.Equal(t, []string{c.want}, got)
		})
	}
}

func Test_SearchWarnings_is_an_empty_list_not_nil_when_something_matched(t *testing.T) {
	cases := []struct {
		name  string
		found report.Search
	}{
		{name: "everything listed", found: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 2, Transactions: span}}},
		{name: "cut by the limit", found: report.Search{Search: store.Search{Rows: make([]store.SearchRow, 2), Matched: 5, Transactions: span}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{}, document.SearchWarnings(c.found))
		})
	}
}
