package duckstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// accountsQuery reads today, the earliest rate and every cash, holdings value and balance in cents, native, CAD and USD.
const accountsQuery = `
SELECT d.as_of, d.first_rate, v.id, v.source_id, v.name, v.type, v.currency, v.institution, v.closed, v.active,
	NOT a.in_reports, a.linked_tracking, CAST(v.cash * 100 AS BIGINT), CAST(v.holdings_value * 100 AS BIGINT),
	CAST(v.balance * 100 AS BIGINT), CAST(v.balance_cad * 100 AS BIGINT), CAST(v.balance_usd * 100 AS BIGINT)
FROM (SELECT current_date AS as_of, ` + firstRateSubquery + ` AS first_rate) d
LEFT JOIN v_account_balances v ON true
LEFT JOIN accounts a ON a.id = v.id
ORDER BY lower(v.name), v.name, v.source_id`

// Accounts reads every account, closed ones included, with balances native and in CAD and USD, the store's
// today as AsOf, the holdings those balances leave out as Unvalued and the earliest rate as FirstRate,
// sorted by name ignoring case, then name, then source id. A store it cannot open or read is a *store.OpenError.
func (s *Store) Accounts(ctx context.Context) (store.AccountList, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.AccountList{}, err
	}
	defer func() { _ = db.Close() }()

	var list store.AccountList
	err = db.QueryRows(ctx, accountsQuery, nil, func(scan func(dest ...any) error) error {
		var asOf time.Time
		var firstRate sql.NullTime
		var id, name, typ, currency, institution sql.NullString
		var sourceID, cash, holdingsValue, balance, balanceCAD, balanceUSD sql.NullInt64
		var closed, active, notInReports, linkedTracking sql.NullBool
		if err := scan(&asOf, &firstRate, &id, &sourceID, &name, &typ, &currency, &institution, &closed, &active,
			&notInReports, &linkedTracking, &cash, &holdingsValue, &balance, &balanceCAD, &balanceUSD); err != nil {
			return err
		}
		list.AsOf = asOf
		list.FirstRate = firstRate.Time
		if !id.Valid {
			return nil
		}
		acct := store.AccountBalance{
			ID: id.String, SourceID: sourceID.Int64, Name: name.String, Type: typ.String, Currency: currency.String,
			Institution: nullStringPtr(institution), Closed: closed.Bool, Active: active.Bool, NotInReports: notInReports.Bool,
			LinkedTracking: linkedTracking.Bool,
			Balance:        balance.Int64, Cash: cash.Int64, HoldingsValue: nullInt64Ptr(holdingsValue),
			BalanceCAD: nullInt64Ptr(balanceCAD), BalanceUSD: nullInt64Ptr(balanceUSD),
		}
		list.Accounts = append(list.Accounts, acct)
		return nil
	})
	if err != nil {
		return store.AccountList{}, openFault(s.Path(), err)
	}
	err = db.QueryRows(ctx, unvaluedHoldingsSQL("h.date = current_date"), nil, func(scan func(dest ...any) error) error {
		held, err := scanUnvaluedHolding(scan)
		list.Unvalued = append(list.Unvalued, held)
		return err
	})
	if err != nil {
		return store.AccountList{}, openFault(s.Path(), err)
	}
	return list, nil
}

// nullInt64Ptr returns n's value, or nil when n is NULL.
func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// nullStringPtr returns s's value, or nil when s is NULL.
func nullStringPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	return &s.String
}
