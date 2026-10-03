package document

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// Spending is spend's --json document and the spending tool's structured result.
type Spending struct {
	Since         string          `json:"since"`
	Until         string          `json:"until"`
	By            string          `json:"by"`
	Currency      string          `json:"currency"`
	AccountFilter []AccountFilter `json:"account_filter"`
	Rows          []any           `json:"rows"`
	Totals        []SpendingTotal `json:"totals"`
	Warnings      []string        `json:"warnings"`
}

// SpendingCategoryRow is one entry of "rows" grouped by category; Category
// is null for the group of splits with no category.
type SpendingCategoryRow struct {
	Category *string `json:"category"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// SpendingPayeeRow is one entry of "rows" grouped by payee; Payee is null
// for the group of splits with no payee.
type SpendingPayeeRow struct {
	Payee    *string `json:"payee"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// SpendingTagRow is one entry of "rows" grouped by tag; Tag is null for
// the group of splits with no tag.
type SpendingTagRow struct {
	Tag      *string `json:"tag"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// SpendingMonthRow is one entry of "rows" grouped by month; Month is
// YYYY-MM and Partial is true when the window cuts the month short.
type SpendingMonthRow struct {
	Month    string `json:"month"`
	Currency string `json:"currency"`
	Spent    string `json:"spent"`
	Partial  bool   `json:"partial"`
}

// SpendingTotal is one entry of "totals".
type SpendingTotal struct {
	Currency string `json:"currency"`
	Spent    string `json:"spent"`
}

// NewSpending converts s into spend's document with warnings; rows, totals and
// account_filter are [] rather than null when s holds none.
func NewSpending(s report.Spending, warnings []string) Spending {
	rows := make([]any, len(s.Rows))
	for i, r := range s.Rows {
		rows[i] = spendingRow(s.By, r, Money(r.Spent))
	}
	totals := make([]SpendingTotal, len(s.Totals))
	for i, t := range s.Totals {
		totals[i] = SpendingTotal{Currency: t.Currency, Spent: Money(t.Spent)}
	}
	return Spending{
		Since:         s.Window.Since.Format(DateLayout),
		Until:         s.Window.Until.Format(DateLayout),
		By:            s.By.String(),
		Currency:      s.Currency.String(),
		AccountFilter: NewAccountFilters(s.Accounts),
		Rows:          rows,
		Totals:        totals,
		Warnings:      append([]string{}, warnings...),
	}
}

// spendingRow is the row of a spend grouped by group, its key named
// for the grouping; spent is the row's amount already formatted.
func spendingRow(group store.SpendingGroup, r report.SpendingRow, spent string) any {
	switch group {
	case store.SpendByPayee:
		return SpendingPayeeRow{Payee: r.Key, Currency: r.Currency, Spent: spent}
	case store.SpendByTag:
		return SpendingTagRow{Tag: r.Key, Currency: r.Currency, Spent: spent}
	case store.SpendByMonth:
		return SpendingMonthRow{Month: *r.Key, Currency: r.Currency, Spent: spent, Partial: r.Partial}
	case store.SpendByCategory:
	}
	return SpendingCategoryRow{Category: r.Key, Currency: r.Currency, Spent: spent}
}
