package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// spendDocument is spend's --json stdout shape.
type spendDocument struct {
	Since         string                  `json:"since"`
	Until         string                  `json:"until"`
	By            string                  `json:"by"`
	Currency      string                  `json:"currency"`
	AccountFilter []accountFilterDocument `json:"account_filter"`
	Rows          []any                   `json:"rows"`
	Totals        []spendTotalDocument    `json:"totals"`
	Warnings      []string                `json:"warnings"`
}

// accountFilterDocument names one account a report was limited to.
type accountFilterDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// accountFilterDocuments is accounts as a report's "account_filter": [] rather than null when none.
func accountFilterDocuments(accounts []store.Account) []accountFilterDocument {
	filter := make([]accountFilterDocument, len(accounts))
	for i, a := range accounts {
		filter[i] = accountFilterDocument{ID: a.ID, Name: a.Name}
	}
	return filter
}

// spendCategoryRowDocument is one entry of "rows" grouped by category; Category
// is null for the group of splits with no category.
type spendCategoryRowDocument struct {
	Category *string `json:"category"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// spendPayeeRowDocument is one entry of "rows" grouped by payee; Payee is null
// for the group of splits with no payee.
type spendPayeeRowDocument struct {
	Payee    *string `json:"payee"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// spendTagRowDocument is one entry of "rows" grouped by tag; Tag is null for
// the group of splits with no tag.
type spendTagRowDocument struct {
	Tag      *string `json:"tag"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// spendMonthRowDocument is one entry of "rows" grouped by month; Month is
// YYYY-MM and Partial is true when the window cuts the month short.
type spendMonthRowDocument struct {
	Month    string `json:"month"`
	Currency string `json:"currency"`
	Spent    string `json:"spent"`
	Partial  bool   `json:"partial"`
}

// spendTotalDocument is one entry of "totals".
type spendTotalDocument struct {
	Currency string `json:"currency"`
	Spent    string `json:"spent"`
}

// renderSpendingJSON renders s as spend's --json document with warnings;
// rows and totals are [] rather than null when s holds none.
func renderSpendingJSON(s report.Spending, warnings []string) ([]byte, error) {
	rows := make([]any, len(s.Rows))
	for i, r := range s.Rows {
		rows[i] = spendRowDocumentFor(s.By, r, jsonMoney(r.Spent))
	}
	totals := make([]spendTotalDocument, len(s.Totals))
	for i, t := range s.Totals {
		totals[i] = spendTotalDocument{Currency: t.Currency, Spent: jsonMoney(t.Spent)}
	}
	return marshalDocument(spendDocument{
		Since:         s.Window.Since.Format(jsonDateLayout),
		Until:         s.Window.Until.Format(jsonDateLayout),
		By:            spendGroupings[s.By].name,
		Currency:      s.Currency.String(),
		AccountFilter: accountFilterDocuments(s.Accounts),
		Rows:          rows,
		Totals:        totals,
		Warnings:      warnings,
	})
}

// spendRowDocumentFor is the row of a spend grouped by group, its key named
// for the grouping; spent is the row's amount already formatted.
func spendRowDocumentFor(group store.SpendingGroup, r report.SpendingRow, spent string) any {
	switch group {
	case store.SpendByPayee:
		return spendPayeeRowDocument{Payee: r.Key, Currency: r.Currency, Spent: spent}
	case store.SpendByTag:
		return spendTagRowDocument{Tag: r.Key, Currency: r.Currency, Spent: spent}
	case store.SpendByMonth:
		return spendMonthRowDocument{Month: *r.Key, Currency: r.Currency, Spent: spent, Partial: r.Partial}
	case store.SpendByCategory:
	}
	return spendCategoryRowDocument{Category: r.Key, Currency: r.Currency, Spent: spent}
}
