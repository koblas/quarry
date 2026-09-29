package cli

import (
	"github.com/koblas/quarry/internal/report"
)

// spendByCategory is the "by" value of a spend grouped by category, the only
// grouping spend reads.
const spendByCategory = "category"

// spendDocument is spend's --json stdout shape.
type spendDocument struct {
	Since         string                 `json:"since"`
	Until         string                 `json:"until"`
	By            string                 `json:"by"`
	AccountFilter []spendAccountDocument `json:"account_filter"`
	Rows          []spendRowDocument     `json:"rows"`
	Totals        []spendTotalDocument   `json:"totals"`
	Warnings      []string               `json:"warnings"`
}

// spendAccountDocument names one account spend was limited to.
type spendAccountDocument struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// spendRowDocument is one entry of "rows"; Category is null for the group of
// splits with no category.
type spendRowDocument struct {
	Category *string `json:"category"`
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
	rows := make([]spendRowDocument, len(s.Rows))
	for i, r := range s.Rows {
		rows[i] = spendRowDocument{Category: r.Key, Currency: r.Currency, Spent: jsonMoney(r.Spent)}
	}
	totals := make([]spendTotalDocument, len(s.Totals))
	for i, t := range s.Totals {
		totals[i] = spendTotalDocument{Currency: t.Currency, Spent: jsonMoney(t.Spent)}
	}
	return marshalDocument(spendDocument{
		Since:         s.Window.Since.Format(jsonDateLayout),
		Until:         s.Window.Until.Format(jsonDateLayout),
		By:            spendByCategory,
		AccountFilter: []spendAccountDocument{},
		Rows:          rows,
		Totals:        totals,
		Warnings:      []string{},
	})
}
