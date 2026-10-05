package duckstore

import (
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// unvaluedHoldingsSQL selects the v_holdings rows of investment accounts a that have no value in the account's
// currency, further restricted by where, in date, account, security order.
func unvaluedHoldingsSQL(where string) string {
	return `
SELECT h.date, a.id, a.name, h.security_id, h.security, h.currency, h.price IS NOT NULL
FROM v_holdings h
JOIN accounts a ON a.id = h.account_id
WHERE a.type IN (` + investmentTypesSQL() + `) AND ` + valuedInAccountCurrencySQL() + ` IS NULL AND ` + where + `
ORDER BY h.date, lower(a.name), a.name, a.id, lower(h.security), h.security, h.security_id`
}

// scanUnvaluedHolding reads one row of unvaluedHoldingsSQL.
func scanUnvaluedHolding(scan func(dest ...any) error) (store.UnvaluedHolding, error) {
	var date time.Time
	var security, currency sql.NullString
	var held store.UnvaluedHolding
	if err := scan(&date, &held.AccountID, &held.Account, &held.SecurityID, &security, &currency, &held.Priced); err != nil {
		return store.UnvaluedHolding{}, err
	}
	held.Date, held.Security, held.Currency = date, security.String, nullStringPtr(currency)
	return held, nil
}
