package duckstore

import (
	"context"
	"database/sql"

	"github.com/koblas/quarry/internal/store"
)

// chargesQuery reads a row per transaction with positive v_spending, dated through $1, by date then source id.
const chargesQuery = `
WITH charge AS (
	SELECT transaction_id, count(*) AS splits, sum(spent) AS spent,
		count(category_id) AS categorised, count(DISTINCT category_id) AS categories,
		min(category_id) AS category_id, min(category) AS category
	FROM v_spending
	GROUP BY transaction_id
	HAVING sum(spent) > 0
)
SELECT t.id, t.source_id, t.date, a.id, a.name, a.currency, a.closed, a.active,
	t.payee_id, p.name, t.currency, CAST(c.spent * 100 AS BIGINT),
	CASE WHEN c.categorised = c.splits AND c.categories = 1 THEN c.category_id END,
	CASE WHEN c.categorised = c.splits AND c.categories = 1 THEN c.category END,
	c.splits
FROM charge c
JOIN transactions t ON t.id = c.transaction_id
JOIN accounts a ON a.id = t.account_id
LEFT JOIN payees p ON p.id = t.payee_id
WHERE t.date <= CAST($1 AS DATE)
ORDER BY t.date, t.source_id`

// Charges reads every charge dated through params.Through (that day included) and the span of
// the store's transactions, or of params.AccountIDs' reported accounts, as store.Charges
// documents. A store it cannot open or read is a *store.OpenError.
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
	if err != nil {
		return store.Charges{}, openFault(s.Path(), err)
	}
	return charges, nil
}

// scanCharge reads one chargesQuery row through scan.
func scanCharge(scan func(dest ...any) error) (store.Charge, error) {
	var charge store.Charge
	var payeeID, payee, categoryID, categoryPath sql.NullString
	err := scan(&charge.TransactionID, &charge.SourceID, &charge.Date,
		&charge.Account.ID, &charge.Account.Name, &charge.Account.Currency, &charge.Account.Closed, &charge.Account.Active,
		&payeeID, &payee, &charge.Currency, &charge.Amount, &categoryID, &categoryPath, &charge.ExpenseSplits)
	if err != nil {
		return store.Charge{}, err
	}
	charge.PayeeID, charge.Payee = nullStringPtr(payeeID), nullStringPtr(payee)
	if categoryID.Valid {
		charge.Category = &store.ChargeCategory{ID: categoryID.String, Path: categoryPath.String}
	}
	return charge, nil
}
