package duckstore

import (
	"context"
	"database/sql"
	"math/big"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// holdingsQueryFor reads v_holdings on $1, only in the named accounts when there are any (numbered from $2),
// in the order store.Holdings documents, names not case-folded.
func holdingsQueryFor(accounts accountFilter) string {
	filter := ""
	if len(accounts) > 0 {
		filter = " AND v.account_id IN (" + accounts.marks(2) + ")"
	}
	return holdingsSelect + filter + holdingsOrder
}

const holdingsSelect = `
SELECT v.account_id, v.security_id, a.name, a.source_id, a.closed, v.security, v.ticker, v.currency, s.source_id,
	CAST(CAST(v.shares AS DECIMAL(38,6)) * 1000000 AS BIGINT), CAST(CAST(v.price AS DECIMAL(38,6)) * 1000000 AS BIGINT), v.price_date,
	CAST(v.value * 100 AS HUGEINT), CAST(v.value_cad * 100 AS HUGEINT), CAST(v.value_usd * 100 AS HUGEINT),
	CAST(v.usd_cad * 1000000 AS BIGINT)
FROM v_holdings v
LEFT JOIN accounts a ON a.id = v.account_id
LEFT JOIN securities s ON s.id = v.security_id
WHERE v.date = CAST($1 AS DATE)`

const holdingsOrder = `
ORDER BY a.name, a.source_id, v.account_id, v.security, s.source_id, v.security_id`

// Holdings reads the holdings on params.AsOf in params.AccountIDs and the store's first rate date, as store.Holdings
// documents; a day with none, or one after today, is an empty result. A store it cannot open or read is a *store.OpenError.
func (s *Store) Holdings(ctx context.Context, params store.HoldingsParams) (store.Holdings, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Holdings{}, err
	}
	defer func() { _ = db.Close() }()

	var holdings store.Holdings
	accounts := accountFilter(params.AccountIDs)
	args := append([]any{civilDay(params.AsOf)}, accounts.args()...)
	err = db.QueryRows(ctx, holdingsQueryFor(accounts), args, func(scan func(dest ...any) error) error {
		holding, err := scanHolding(scan)
		if err != nil {
			return err
		}
		holdings.Holdings = append(holdings.Holdings, holding)
		return nil
	})
	if err == nil {
		holdings.FirstRate, err = firstRate(ctx, db)
	}
	if err != nil {
		return store.Holdings{}, openFault(s.Path(), err)
	}
	return holdings, nil
}

// scanHolding reads one holdingsQueryFor row.
func scanHolding(scan func(dest ...any) error) (store.Holding, error) {
	var h store.Holding
	var account, security, ticker, currency sql.NullString
	var accountSource, securitySource, price, usdCAD sql.NullInt64
	var closed sql.NullBool
	var priceDate sql.NullTime
	var value, valueCAD, valueUSD *big.Int
	err := scan(&h.AccountID, &h.SecurityID, &account, &accountSource, &closed, &security, &ticker, &currency, &securitySource,
		&h.Shares, &price, &priceDate, &value, &valueCAD, &valueUSD, &usdCAD)
	if err != nil {
		return store.Holding{}, err
	}
	h.Account, h.AccountSourceID, h.AccountClosed = account.String, accountSource.Int64, closed.Bool
	h.Security, h.Ticker, h.Currency = nullStringPtr(security), nullStringPtr(ticker), nullStringPtr(currency)
	h.SecuritySourceID, h.Price = nullInt64Ptr(securitySource), nullInt64Ptr(price)
	if priceDate.Valid {
		h.PriceDate = &priceDate.Time
	}
	h.Value, h.ValueCAD, h.ValueUSD = value, valueCAD, valueUSD
	h.USDCAD = money.Rate(usdCAD.Int64)
	return h, nil
}
