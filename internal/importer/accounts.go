package importer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

// accountTypeMap maps ZACCOUNT.ZTYPENAME to accounts.type (specification.md
// "Store enum values"). Any other ZTYPENAME is unmappable (S4 reason 2).
var accountTypeMap = map[string]string{
	"CHECKING":                   "chequing",
	"SAVINGS":                    "savings",
	"CREDITCARD":                 "credit_card",
	"ASSET":                      "asset",
	"LIABILITY":                  "liability",
	"HOMEEQUITY":                 "home_equity",
	"BROKERAGENORMAL":            "brokerage",
	"BROKERAGEOTHER":             "brokerage",
	"DEFERREDCOMPRETIREMENT401K": "retirement",
	"RETIREMENTIRA":              "retirement",
}

// validCurrencies are the only currencies quarry supports (P1-9).
var validCurrencies = map[string]bool{"CAD": true, "USD": true}

// accountRef is what later mapping steps need about an account without
// re-reading ZACCOUNT: its quarry id, name (for S4 subjects) and currency
// (a transaction's currency comes from its account).
type accountRef struct {
	ID       string
	Name     string
	Currency string
}

const accountsQuery = `
SELECT a.Z_PK, a.ZNAME, a.ZTYPENAME, a.ZCURRENCY, fi.ZNAME,
       COALESCE(a.ZCLOSED, 0), COALESCE(a.ZACTIVE, 0)
FROM ZACCOUNT a
LEFT JOIN ZFINANCIALINSTITUTION fi ON a.ZFINANCIALINSTITUTION = fi.Z_PK
WHERE COALESCE(a.ZDELETIONCOUNT, 0) = 0
ORDER BY a.ZNAME, a.Z_PK
`

// mapAccounts reads every non-deleted ZACCOUNT row. A row with no name, no
// type or no currency (S4 reason 10), an unmapped type (reason 2) or an
// unsupported currency (reason 1) is added to off and excluded from both
// results.
func mapAccounts(ctx context.Context, src Source, off *offenders) ([]store.Account, map[int64]accountRef, error) {
	var rows []store.Account
	refs := make(map[int64]accountRef)

	err := src.QueryRows(ctx, accountsQuery, nil, func(scan func(dest ...any) error) error {
		var pk int64
		var name, typ, currency, institution sql.NullString
		var closed, active bool
		if err := scan(&pk, &name, &typ, &currency, &institution, &closed, &active); err != nil {
			return err
		}

		if !name.Valid || name.String == "" {
			off.add(offender{class: 10, reason: reasonAccountNoName(pk), name: fmt.Sprintf("(source id %d)", pk), sourceID: pk})
			return nil
		}
		if !typ.Valid || typ.String == "" {
			off.add(offender{class: 10, reason: reasonAccountNoType(name.String), name: name.String, sourceID: pk})
			return nil
		}
		if !currency.Valid || currency.String == "" {
			off.add(offender{class: 10, reason: reasonAccountNoCurrency(name.String), name: name.String, sourceID: pk})
			return nil
		}
		quarryType, ok := accountTypeMap[typ.String]
		if !ok {
			off.add(offender{class: 2, reason: reasonAccountType(name.String, typ.String), name: name.String, sourceID: pk})
			return nil
		}
		if !validCurrencies[currency.String] {
			off.add(offender{class: 1, reason: reasonAccountCurrency(name.String, currency.String), name: name.String, sourceID: pk})
			return nil
		}

		id := fmt.Sprintf("acct-%d", pk)
		acct := store.Account{
			ID: id, SourceID: pk, Name: name.String, Type: quarryType, Currency: currency.String,
			Closed: closed, Active: active,
		}
		if institution.Valid {
			inst := institution.String
			acct.Institution = &inst
		}
		rows = append(rows, acct)
		refs[pk] = accountRef{ID: id, Name: name.String, Currency: currency.String}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read accounts: %w", err)
	}
	return rows, refs, nil
}
