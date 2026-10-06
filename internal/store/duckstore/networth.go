package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// netWorthSelect reads v_net_worth on each of the dates numbered from $1, closed by netWorthOrder.
const netWorthSelect = `
SELECT date, type, currency, accounts,
	CAST(balance * 100 AS HUGEINT), CAST(balance_cad * 100 AS HUGEINT), CAST(balance_usd * 100 AS HUGEINT)
FROM v_net_worth
WHERE date IN (`

// netWorthOrder is store.NetWorth's order; currency sorts alphabetically because an account's is CAD or USD.
const netWorthOrder = `)
ORDER BY date, type, currency`

// firstBalanceQuery is the earliest day a counted account has a transaction or a holding, as v_balances_daily starts it.
const firstBalanceQuery = `
SELECT min(f.d)
FROM (SELECT account_id, date AS d FROM transactions UNION ALL SELECT account_id, from_date FROM holding_shares) f
JOIN accounts a ON a.id = f.account_id
WHERE ` + reportedAccount

// NetWorth reads store.NetWorth for params.Dates; a date with no rows contributes none.
// A store it cannot open or read is a *store.OpenError.
func (s *Store) NetWorth(ctx context.Context, params store.NetWorthParams) (store.NetWorth, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.NetWorth{}, err
	}
	defer func() { _ = db.Close() }()

	return readNetWorth(ctx, db, s.Path(), params)
}

// readNetWorth is NetWorth over an open db of the store at path; every fault is a *store.OpenError.
func readNetWorth(ctx context.Context, db ReadDB, path string, params store.NetWorthParams) (store.NetWorth, error) {
	if len(params.Dates) == 0 {
		return store.NetWorth{}, nil
	}

	marks := make([]string, len(params.Dates))
	args := make([]any, len(params.Dates))
	for i, date := range params.Dates {
		marks[i], args[i] = fmt.Sprintf("CAST($%d AS DATE)", i+1), civilDay(date)
	}

	var read store.NetWorth
	err := db.QueryRows(ctx, netWorthSelect+strings.Join(marks, ", ")+netWorthOrder, args, func(scan func(dest ...any) error) error {
		var row store.NetWorthRow
		var date time.Time
		var balance, balanceCAD, balanceUSD *big.Int
		if err := scan(&date, &row.Type, &row.Currency, &row.Accounts, &balance, &balanceCAD, &balanceUSD); err != nil {
			return err
		}
		row.Date, row.Balance, row.BalanceCAD, row.BalanceUSD = date, balance, balanceCAD, balanceUSD
		read.Rows = append(read.Rows, row)
		return nil
	})
	if err != nil {
		return store.NetWorth{}, openFault(path, err)
	}
	err = db.QueryRows(ctx, unvaluedHoldingsSQL(reportedAccount+" AND h.date IN ("+strings.Join(marks, ", ")+")"), args,
		func(scan func(dest ...any) error) error {
			held, err := scanUnvaluedHolding(scan)
			read.Unvalued = append(read.Unvalued, held)
			return err
		})
	if err == nil {
		read.FirstRate, err = firstRate(ctx, db)
	}
	if err == nil {
		read.FirstBalance, err = firstBalance(ctx, db)
	}
	if err != nil {
		return store.NetWorth{}, openFault(path, err)
	}
	return read, nil
}

// firstBalance is the date of the first balance of a counted account through db; zero when there is none.
func firstBalance(ctx context.Context, db ReadDB) (time.Time, error) {
	var first sql.NullTime
	err := db.QueryRows(ctx, firstBalanceQuery, nil, func(scan func(dest ...any) error) error {
		return scan(&first)
	})
	return first.Time, err //nolint:wrapcheck // the caller classifies the driver's own error with openFault
}
