package duckstore

import (
	"context"
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

// NetWorth reads the v_net_worth rows on params.Dates, as store.NetWorth documents; a date with no rows
// contributes none. A store it cannot open or read is a *store.OpenError.
func (s *Store) NetWorth(ctx context.Context, params store.NetWorthParams) (store.NetWorth, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.NetWorth{}, err
	}
	defer func() { _ = db.Close() }()

	if len(params.Dates) == 0 {
		return store.NetWorth{}, nil
	}

	marks := make([]string, len(params.Dates))
	args := make([]any, len(params.Dates))
	for i, date := range params.Dates {
		marks[i], args[i] = fmt.Sprintf("CAST($%d AS DATE)", i+1), civilDay(date)
	}

	var read store.NetWorth
	err = db.QueryRows(ctx, netWorthSelect+strings.Join(marks, ", ")+netWorthOrder, args, func(scan func(dest ...any) error) error {
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
		return store.NetWorth{}, openFault(s.Path(), err)
	}
	return read, nil
}
