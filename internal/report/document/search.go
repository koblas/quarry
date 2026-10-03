package document

import "github.com/koblas/quarry/internal/report"

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

// NewSearch converts s into search's document with warnings.
func NewSearch(report.Search, []string) Search {
	return Search{}
}
