package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// spendDocument is spend's --json stdout shape.
type spendDocument struct {
	Since         string                 `json:"since"`
	Until         string                 `json:"until"`
	By            string                 `json:"by"`
	AccountFilter []spendAccountDocument `json:"account_filter"`
	Rows          []any                  `json:"rows"`
	Totals        []spendTotalDocument   `json:"totals"`
	Warnings      []string               `json:"warnings"`
}

// spendAccountDocument names one account spend was limited to.
type spendAccountDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

// spendTotalDocument is one entry of "totals".
type spendTotalDocument struct {
	Currency string `json:"currency"`
	Spent    string `json:"spent"`
}

// renderSpendingJSON renders s as spend's --json document; rows and totals
// are [] rather than null when s holds none.
func renderSpendingJSON(s report.Spending) ([]byte, error) {
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
		AccountFilter: []spendAccountDocument{},
		Rows:          rows,
		Totals:        totals,
		Warnings:      []string{},
	})
}

// spendRowDocumentFor is the row of a spend grouped by group, its key named
// for the grouping; spent is the row's amount already formatted.
func spendRowDocumentFor(group store.SpendingGroup, r store.SpendingRow, spent string) any {
	if group == store.SpendByPayee {
		return spendPayeeRowDocument{Payee: r.Key, Currency: r.Currency, Spent: spent}
	}
	return spendCategoryRowDocument{Category: r.Key, Currency: r.Currency, Spent: spent}
}
