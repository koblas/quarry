package duckstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// chargesQuery reads a row per transaction with positive v_spending, dated through $1, by date then source id.
// Its converted cells are the per-split sums of v_spending's, and its rate is exact millionths.
const chargesQuery = `
WITH charge AS (
	SELECT transaction_id, count(*) AS splits, sum(spent) AS spent,
		sum(spent_cad) AS spent_cad, sum(spent_usd) AS spent_usd, min(usd_cad) AS usd_cad,
		count(category_id) AS categorised, count(DISTINCT category_id) AS categories,
		min(category_id) AS category_id, min(category) AS category
	FROM v_spending
	GROUP BY transaction_id
	HAVING sum(spent) > 0
)
SELECT t.id, t.source_id, t.date, a.id, a.name, a.currency, a.closed, a.active,
	t.payee_id, p.name, t.currency, CAST(c.spent * 100 AS BIGINT),
	CAST(c.spent_cad * 100 AS BIGINT), CAST(c.spent_usd * 100 AS BIGINT), CAST(c.usd_cad * 1000000 AS BIGINT),
	CASE WHEN c.categorised = c.splits AND c.categories = 1 THEN c.category_id END,
	CASE WHEN c.categorised = c.splits AND c.categories = 1 THEN c.category END,
	c.splits
FROM charge c
JOIN transactions t ON t.id = c.transaction_id
JOIN accounts a ON a.id = t.account_id
LEFT JOIN payees p ON p.id = t.payee_id
WHERE t.date <= CAST($1 AS DATE)
ORDER BY t.date, t.source_id`

// firstRateQuery reads the date of the store's first exchange rate.
const firstRateQuery = "SELECT min(date) FROM fx_rates"

// Charges reads every charge dated through params.Through (that day included), the span of
// the store's transactions, or of params.AccountIDs' reported accounts, and the first rate's
// date, as store.Charges documents. A store it cannot open or read is a *store.OpenError.
func (s *Store) Charges(ctx context.Context, params store.ChargeParams) (store.Charges, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Charges{}, err
	}
	defer func() { _ = db.Close() }()

	var charges store.Charges
	err = db.QueryRows(ctx, chargesQuery, []any{civilDay(params.Through)}, func(scan func(dest ...any) error) error {
		charge, err := scanCharge(scan)
		if err != nil {
			return err
		}
		charges.Rows = append(charges.Rows, charge)
		return nil
	})
	if err == nil {
		charges.Transactions, err = transactionRange(ctx, db, accountFilter(params.AccountIDs))
	}
	if err == nil {
		charges.FirstRate, err = firstRate(ctx, db)
	}
	if err != nil {
		return store.Charges{}, openFault(s.Path(), err)
	}
	return charges, nil
}

// firstRate is the date of the first exchange rate through db; zero when the store has none.
func firstRate(ctx context.Context, db ReadDB) (time.Time, error) {
	var first sql.NullTime
	err := db.QueryRows(ctx, firstRateQuery, nil, func(scan func(dest ...any) error) error {
		return scan(&first)
	})
	return first.Time, err //nolint:wrapcheck // callers classify the driver's own error with openFault
}

// scanCharge reads one chargesQuery row through scan.
func scanCharge(scan func(dest ...any) error) (store.Charge, error) {
	var charge store.Charge
	var payeeID, payee, categoryID, categoryPath sql.NullString
	var amountCAD, amountUSD, usdCAD sql.NullInt64
	err := scan(&charge.TransactionID, &charge.SourceID, &charge.Date,
		&charge.Account.ID, &charge.Account.Name, &charge.Account.Currency, &charge.Account.Closed, &charge.Account.Active,
		&payeeID, &payee, &charge.Currency, &charge.Amount, &amountCAD, &amountUSD, &usdCAD, &categoryID, &categoryPath, &charge.ExpenseSplits)
	if err != nil {
		return store.Charge{}, err
	}
	charge.PayeeID, charge.Payee = nullStringPtr(payeeID), nullStringPtr(payee)
	charge.AmountCAD, charge.AmountUSD = nullInt64Ptr(amountCAD), nullInt64Ptr(amountUSD)
	charge.USDCAD = money.Rate(usdCAD.Int64)
	if categoryID.Valid {
		charge.Category = &store.ChargeCategory{ID: categoryID.String, Path: categoryPath.String}
	}
	return charge, nil
}
