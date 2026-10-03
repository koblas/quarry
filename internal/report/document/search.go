package document

import (
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
)

// Search is search's --json document and the search tool's structured result.
type Search struct {
	Since         *string             `json:"since"`
	Until         *string             `json:"until"`
	AccountFilter []AccountFilter     `json:"account_filter"`
	Text          *string             `json:"text"`
	Category      *string             `json:"category"`
	Min           *string             `json:"min"`
	Max           *string             `json:"max"`
	Limit         int                 `json:"limit"`
	Matched       int                 `json:"matched"`
	Truncated     bool                `json:"truncated"`
	Transactions  []SearchTransaction `json:"transactions"`
	Warnings      []string            `json:"warnings"`
}

// SearchTransaction is one entry of "transactions".
type SearchTransaction struct {
	TransactionID string        `json:"transaction_id"`
	Date          string        `json:"date"`
	AccountID     string        `json:"account_id"`
	Account       string        `json:"account"`
	Payee         *string       `json:"payee"`
	Memo          *string       `json:"memo"`
	Amount        string        `json:"amount"`
	Currency      string        `json:"currency"`
	Transfer      bool          `json:"transfer"`
	Excluded      bool          `json:"excluded"`
	Splits        []SearchSplit `json:"splits"`
}

// SearchSplit is one entry of a transaction's "splits".
type SearchSplit struct {
	Category *string `json:"category"`
	Memo     *string `json:"memo"`
	Amount   string  `json:"amount"`
	Transfer bool    `json:"transfer"`
}

// NewSearch converts s into search's document with warnings; every array is [] rather than null when s holds none.
func NewSearch(s report.Search, warnings []string) Search {
	transactions := make([]SearchTransaction, len(s.Rows))
	for i, row := range s.Rows {
		transactions[i] = searchTransaction(row)
	}
	return Search{
		Since:         searchDay(s.Window.Since),
		Until:         searchDay(s.Window.Until),
		AccountFilter: NewAccountFilters(s.Accounts),
		Text:          s.Text,
		Limit:         s.Limit,
		Matched:       s.Matched,
		Truncated:     s.Truncated(),
		Transactions:  transactions,
		Warnings:      append([]string{}, warnings...),
	}
}

// searchDay is day's date, or nil for an open bound.
func searchDay(day *time.Time) *string {
	if day == nil {
		return nil
	}
	formatted := day.Format(DateLayout)
	return &formatted
}

// searchTransaction is the document entry for row.
func searchTransaction(row store.SearchRow) SearchTransaction {
	splits := make([]SearchSplit, len(row.Splits))
	for i, sp := range row.Splits {
		splits[i] = SearchSplit{Category: sp.Category, Memo: sp.Memo, Amount: Money(sp.Amount), Transfer: sp.Transfer}
	}
	return SearchTransaction{
		TransactionID: row.TransactionID,
		Date:          row.Date.Format(DateLayout),
		AccountID:     row.Account.ID,
		Account:       row.Account.Name,
		Payee:         row.Payee,
		Memo:          row.Memo,
		Amount:        Money(row.Amount),
		Currency:      row.Currency,
		Transfer:      row.Transfer,
		Excluded:      row.Excluded,
		Splits:        splits,
	}
}
