package document_test

import (
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

// warn builds one command's warnings from the parts every composer shares.
type warn func(accounts []store.Account, window store.Window, span store.TransactionRange, word string) []string

var (
	linked   = store.Account{ID: "a-1", Name: "Old 401(k)", LinkedTracking: true}
	hidden   = store.Account{ID: "a-2", Name: "Old Card", NotInReports: true}
	included = store.Account{ID: "a-3", Name: "Chequing"}

	window = store.Window{
		Since: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC),
	}
	span = store.TransactionRange{
		First: time.Date(2020, time.March, 4, 0, 0, 0, 0, time.UTC),
		Last:  time.Date(2025, time.December, 31, 0, 0, 0, 0, time.UTC),
	}
)

// emptyComposers builds each command's warnings over an empty result, with the empty-window subject it words.
func emptyComposers() []struct {
	name    string
	subject string
	warn    warn
} {
	return []struct {
		name    string
		subject string
		warn    warn
	}{
		{name: "spending", subject: "spending", warn: func(a []store.Account, w store.Window, s store.TransactionRange, word string) []string {
			return document.SpendingWarnings(report.Spending{Accounts: a, Window: w, Transactions: s}, word)
		}},
		{name: "cash flow", subject: "income or spending", warn: func(a []store.Account, w store.Window, s store.TransactionRange, word string) []string {
			return document.CashFlowWarnings(report.CashFlow{Accounts: a, Window: w, Transactions: s}, word)
		}},
		{name: "recurring", subject: "recurring charges", warn: func(a []store.Account, w store.Window, s store.TransactionRange, word string) []string {
			return document.RecurringWarnings(report.Recurring{Accounts: a, Window: w, Transactions: s}, word)
		}},
		{name: "anomalies", subject: "unusually large charges", warn: func(a []store.Account, w store.Window, s store.TransactionRange, word string) []string {
			return document.AnomaliesWarnings(report.Anomalies{Accounts: a, Window: w, Transactions: s}, word)
		}},
	}
}

func Test_warnings_name_the_command_by_word_in_the_left_out_lines(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn([]store.Account{linked, hidden}, window, span, "cash_flow")

			assert.Equal(t, []string{
				`account "Old 401(k)" uses linked account tracking in Quicken, so cash_flow leaves it out, as Quicken's reports do`,
				`account "Old Card" is not used in reports in Quicken, so cash_flow leaves it out; ` +
					`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`,
			}, got)
		})
	}
}

func Test_warnings_say_the_store_has_no_transactions_when_an_empty_window_covers_everything(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn(nil, window, store.TransactionRange{}, "x")

			assert.Equal(t, []string{"no " + c.subject + " from 2026-01-01 to 2026-09-29; the store has no transactions"}, got)
		})
	}
}

func Test_warnings_say_where_the_stores_transactions_run_when_an_empty_window_misses_them(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn(nil, window, span, "x")

			assert.Equal(t, []string{"no " + c.subject + " from 2026-01-01 to 2026-09-29; the store's transactions run 2020-03-04 to 2025-12-31"}, got)
		})
	}
}

func Test_warnings_say_where_the_named_accounts_transactions_run_when_an_empty_window_misses_them(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn([]store.Account{included}, window, span, "x")

			assert.Equal(t, []string{"no " + c.subject + " from 2026-01-01 to 2026-09-29 in the named accounts; their transactions run 2020-03-04 to 2025-12-31"}, got)
		})
	}
}

func Test_warnings_say_the_named_accounts_have_no_transactions_when_they_have_none(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn([]store.Account{included}, window, store.TransactionRange{}, "x")

			assert.Equal(t, []string{"no " + c.subject + " from 2026-01-01 to 2026-09-29 in the named accounts; they have no transactions"}, got)
		})
	}
}

func Test_warnings_skip_the_empty_window_note_when_every_named_account_is_left_out(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn([]store.Account{linked}, window, span, "x")

			assert.Len(t, got, 1)
		})
	}
}

func Test_warnings_keep_the_empty_window_note_when_a_named_account_is_included_beside_a_left_out_one(t *testing.T) {
	for _, c := range emptyComposers() {
		t.Run(c.name, func(t *testing.T) {
			got := c.warn([]store.Account{linked, included}, window, span, "x")

			assert.Len(t, got, 2)
		})
	}
}

func Test_warnings_are_an_empty_list_not_nil_for_a_result_with_rows(t *testing.T) {
	cases := []struct {
		name string
		got  []string
	}{
		{name: "spending", got: document.SpendingWarnings(report.Spending{Totals: []store.SpendingTotal{{Currency: "CAD"}}}, "x")},
		{name: "cash flow", got: document.CashFlowWarnings(report.CashFlow{Totals: []store.CashFlowTotal{{Currency: "CAD"}}}, "x")},
		{name: "recurring", got: document.RecurringWarnings(report.Recurring{Series: []report.Series{{}}}, "x")},
		{name: "anomalies", got: document.AnomaliesWarnings(report.Anomalies{Checked: 1}, "x")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.NotNil(t, c.got)
			assert.Empty(t, c.got)
		})
	}
}

func Test_anomalies_warnings_note_an_empty_window_only_when_no_charge_was_checked(t *testing.T) {
	cases := []struct {
		name    string
		checked int
		want    int
	}{
		{name: "none checked", checked: 0, want: 1},
		{name: "one checked", checked: 1, want: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.AnomaliesWarnings(report.Anomalies{Window: window, Checked: c.checked}, "x")

			assert.Len(t, got, c.want)
		})
	}
}

func Test_spending_warnings_note_split_tags_only_when_grouped_by_tag_with_a_split_carrying_several(t *testing.T) {
	cases := []struct {
		name  string
		by    store.SpendingGroup
		count int
		want  []string
	}{
		{name: "tag with two splits", by: store.SpendByTag, count: 2, want: []string{"2 splits carry more than one tag, so the rows add up to more than the total"}},
		{name: "tag with one split", by: store.SpendByTag, count: 1, want: []string{"1 split carries more than one tag, so the rows add up to more than the total"}},
		{name: "tag with none", by: store.SpendByTag, count: 0, want: []string{}},
		{name: "category with a count set", by: store.SpendByCategory, count: 3, want: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := report.Spending{By: c.by, MultiTagSplits: c.count, Totals: []store.SpendingTotal{{Currency: "CAD"}}}

			assert.Equal(t, c.want, document.SpendingWarnings(s, "x"))
		})
	}
}

func Test_spending_warnings_come_left_out_then_unconverted_then_split_tags_then_empty_window(t *testing.T) {
	s := report.Spending{
		Accounts:       []store.Account{linked, included},
		Window:         window,
		By:             store.SpendByTag,
		MultiTagSplits: 2,
		Currency:       money.CAD,
		Unconverted:    store.Unconverted{Transactions: 1},
	}

	got := document.SpendingWarnings(s, "spend")

	assert.Equal(t, []string{
		`account "Old 401(k)" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do`,
		"the store has no exchange rates, so amounts are listed in each account's own currency; run quarry sync to fetch them",
		"2 splits carry more than one tag, so the rows add up to more than the total",
		"no spending from 2026-01-01 to 2026-09-29 in the named accounts; they have no transactions",
	}, got)
}
