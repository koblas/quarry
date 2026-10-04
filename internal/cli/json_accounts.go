package cli

import (
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
)

// accountsDocument is accounts's --json stdout shape.
type accountsDocument struct {
	AsOf     string               `json:"as_of"`
	Currency string               `json:"currency"`
	Accounts []accountRowDocument `json:"accounts"`
	Warnings []string             `json:"warnings"`
}

// accountRowDocument is one entry of "accounts"; Institution, Balance and ConvertedBalance
// are null when absent, not valued, or unconverted (native listing or no rate).
type accountRowDocument struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	Currency         string  `json:"currency"`
	Institution      *string `json:"institution"`
	Closed           bool    `json:"closed"`
	Active           bool    `json:"active"`
	InReports        bool    `json:"in_reports"`
	LinkedTracking   bool    `json:"linked_tracking"`
	Balance          *string `json:"balance"`
	ConvertedBalance *string `json:"converted_balance"`
}

// renderAccountsJSON renders list as accounts's --json document; accounts is
// [] rather than null when list holds none.
func renderAccountsJSON(list report.AccountListing, warnings []string) ([]byte, error) {
	rows := make([]accountRowDocument, len(list.Accounts))
	for i, a := range list.Accounts {
		rows[i] = accountRowDocument{
			ID: a.ID, Name: a.Name, Type: a.Type, Currency: a.Currency,
			Institution: jsonNullInstitution(a.Institution),
			Closed:      a.Closed, Active: a.Active, InReports: !a.NotInReports,
			LinkedTracking: a.LinkedTracking,
			Balance:        jsonNullMoney(a.Balance), ConvertedBalance: jsonNullMoney(list.ConvertedBalance(a)),
		}
	}
	return marshalDocument(accountsDocument{AsOf: list.AsOf.Format(document.DateLayout), Currency: list.Currency.String(), Accounts: rows, Warnings: warnings})
}

// jsonNullInstitution is nil for a missing or empty institution name.
func jsonNullInstitution(name *string) *string {
	if name == nil {
		return nil
	}
	return document.NullString(*name)
}

// jsonNullMoney is document.Money of cents, or nil when cents is nil.
func jsonNullMoney(cents *int64) *string {
	if cents == nil {
		return nil
	}
	s := document.Money(*cents)
	return &s
}
