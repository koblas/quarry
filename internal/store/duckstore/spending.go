package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// spendingQuery reads per-group and per-currency-total spending from v_spending, grouped by
// the key column and ordered by rowOrder after the total rows.
func spendingQuery(key, rowOrder string) string {
	return fmt.Sprintf(`
SELECT %[1]s, currency, CAST(sum(spent) * 100 AS BIGINT), GROUPING(%[1]s)
FROM v_spending
WHERE date >= CAST(? AS DATE) AND date <= CAST(? AS DATE)
GROUP BY GROUPING SETS ((%[1]s, currency), (currency))
HAVING GROUPING(%[1]s) = 1 OR sum(spent) <> 0
ORDER BY GROUPING(%[1]s), %[2]s`, key, rowOrder)
}

// spendingQueries is the query of each grouping spending can read.
var spendingQueries = map[store.SpendingGroup]string{
	// uncategorized first, then category ignoring case (a case-only tie by byte order), then currency
	store.SpendByCategory: spendingQuery("category", "category IS NOT NULL, lower(category), category, currency"),
	// each currency's biggest payee first; a tie by name ignoring case, then by byte order
	store.SpendByPayee: spendingQuery("payee", "currency, sum(spent) DESC, lower(payee), payee"),
}

// ErrUnsupportedGrouping is what Spending returns for a grouping it cannot read.
var ErrUnsupportedGrouping = errors.New("spending grouping is not supported")

// Spending reads the spending in params.Window (both days counted) grouped by params.By,
// dropping a group that nets to zero; Totals keep it, one per currency. An unsupported
// grouping is an error; a store it cannot open or read is a *store.OpenError.
func (s *Store) Spending(ctx context.Context, params store.SpendingParams) (store.Spending, error) {
	query, ok := spendingQueries[params.By]
	if !ok {
		return store.Spending{}, fmt.Errorf("%w: %d", ErrUnsupportedGrouping, params.By)
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Spending{}, err
	}
	defer func() { _ = db.Close() }()

	var spending store.Spending
	args := []any{civilDay(params.Window.Since), civilDay(params.Window.Until)}
	err = db.QueryRows(ctx, query, args, func(scan func(dest ...any) error) error {
		var key sql.NullString
		var currency string
		var cents, grouping int64
		if err := scan(&key, &currency, &cents, &grouping); err != nil {
			return err
		}
		if grouping == 1 {
			spending.Totals = append(spending.Totals, store.SpendingTotal{Currency: currency, Spent: cents})
			return nil
		}
		spending.Rows = append(spending.Rows, store.SpendingRow{Key: nullStringPtr(key), Currency: currency, Spent: cents})
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
