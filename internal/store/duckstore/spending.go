package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// spendingByCategoryQuery reads spending per category and currency in the window, then one total
// per currency straight from v_spending; a total row is the one whose category grouping bit is set.
const spendingByCategoryQuery = `
SELECT category, currency, CAST(sum(spent) * 100 AS BIGINT), GROUPING(category)
FROM v_spending
WHERE date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
GROUP BY GROUPING SETS ((category, currency), (currency))
HAVING GROUPING(category) = 1 OR sum(spent) <> 0
ORDER BY GROUPING(category), category IS NOT NULL, lower(category), category, currency`

// ErrUnsupportedGrouping is what Spending returns for a grouping it cannot read.
var ErrUnsupportedGrouping = errors.New("spending grouping is not supported")

// Spending reads the spending in params.Window (both days counted) grouped
// by params.By, dropping a group that nets to zero. Rows sort with
// uncategorized first, then by category ignoring case; Totals keep every
// row's spending, one per currency. An unsupported grouping is an error;
// a store it cannot open or read is a *store.OpenError.
func (s *Store) Spending(ctx context.Context, params store.SpendingParams) (store.Spending, error) {
	if params.By != store.SpendByCategory {
		return store.Spending{}, fmt.Errorf("%w: %d", ErrUnsupportedGrouping, params.By)
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Spending{}, err
	}
	defer func() { _ = db.Close() }()

	var spending store.Spending
	args := []any{civilDay(params.Window.Since), civilDay(params.Window.Until)}
	err = db.QueryRows(ctx, spendingByCategoryQuery, args, func(scan func(dest ...any) error) error {
		var category sql.NullString
		var currency string
		var cents, grouping int64
		if err := scan(&category, &currency, &cents, &grouping); err != nil {
			return err
		}
		if grouping == 1 {
			spending.Totals = append(spending.Totals, store.SpendingTotal{Currency: currency, Spent: cents})
			return nil
		}
		spending.Rows = append(spending.Rows, store.SpendingRow{Key: nullStringPtr(category), Currency: currency, Spent: cents})
		return nil
	})
	if err != nil {
		return store.Spending{}, openFault(s.Path(), err)
	}
	return spending, nil
}

// civilDay is day's calendar date as text, which DuckDB reads as a DATE in no zone.
func civilDay(day time.Time) string {
	return day.Format(time.DateOnly)
}
