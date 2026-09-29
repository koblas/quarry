package cli

import (
	"github.com/koblas/quarry/internal/store"
)

// accountsDocument is accounts's --json stdout shape.
type accountsDocument struct {
	AsOf     string               `json:"as_of"`
	Accounts []accountRowDocument `json:"accounts"`
	Warnings []string             `json:"warnings"`
}

// accountRowDocument is one entry of "accounts"; Institution is null when the
// account has none, Balance when quarry cannot compute it.
type accountRowDocument struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Currency    string  `json:"currency"`
	Institution *string `json:"institution"`
	Closed      bool    `json:"closed"`
	Active      bool    `json:"active"`
	Balance     *string `json:"balance"`
}

// renderAccountsJSON renders list as accounts's --json document. as_of is
// the store's calendar day as-is, and accounts is [] rather than null when
// list holds none.
func renderAccountsJSON(list store.AccountList, warnings []string) ([]byte, error) {
	rows := make([]accountRowDocument, len(list.Accounts))
	for i, a := range list.Accounts {
		rows[i] = accountRowDocument{
			ID: a.ID, Name: a.Name, Type: a.Type, Currency: a.Currency,
			Institution: jsonNullInstitution(a.Institution),
			Closed:      a.Closed, Active: a.Active,
			Balance: jsonNullMoney(a.Balance),
		}
	}
	return marshalDocument(accountsDocument{AsOf: list.AsOf.Format(jsonDateLayout), Accounts: rows, Warnings: warnings})
}

// jsonNullInstitution is nil for a missing or empty institution name.
func jsonNullInstitution(name *string) *string {
	if name == nil {
		return nil
	}
	return jsonNullString(*name)
}

// jsonNullMoney is jsonMoney of cents, or nil when cents is nil.
func jsonNullMoney(cents *int64) *string {
	if cents == nil {
		return nil
	}
	s := jsonMoney(*cents)
	return &s
}
