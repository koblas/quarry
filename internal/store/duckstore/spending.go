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

// splitTagNames is a CTE of each split's distinct tag names; a link to a missing tag has none.
const splitTagNames = `WITH split_tag_names AS (
	SELECT DISTINCT st.split_id, t.name FROM split_tags st JOIN tags t ON t.id = st.tag_id
)`

// spendingByTagQuery reads spending per tag, then per-currency totals counting each split once.
const spendingByTagQuery = splitTagNames + `
SELECT tag, currency, cents, grp FROM (
	SELECT n.name AS tag, s.currency, CAST(sum(s.spent) * 100 AS BIGINT) AS cents, 0 AS grp
	FROM v_spending s LEFT JOIN split_tag_names n ON n.split_id = s.split_id
	WHERE s.date >= CAST($1 AS DATE) AND s.date <= CAST($2 AS DATE)
	GROUP BY n.name, s.currency
	HAVING sum(s.spent) <> 0
	UNION ALL
	SELECT NULL, currency, CAST(sum(spent) * 100 AS BIGINT), 1
	FROM v_spending
	WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)
	GROUP BY currency
)
ORDER BY grp, tag IS NOT NULL, lower(tag), tag, currency`

// multiTagSplitsQuery counts the splits in the window carrying more than one tag.
const multiTagSplitsQuery = splitTagNames + `
SELECT count(*) FROM (
	SELECT n.split_id FROM split_tag_names n
	WHERE n.split_id IN (
		SELECT split_id FROM v_spending WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)
	)
	GROUP BY n.split_id HAVING count(*) > 1
)`

// spendingQueries is the query of each grouping spending can read; each takes the window's
// first and last day as its two arguments.
var spendingQueries = map[store.SpendingGroup]string{
	// uncategorized first, then category ignoring case (a case-only tie by byte order), then currency
	store.SpendByCategory: spendingQuery("category", "category IS NOT NULL, lower(category), category, currency"),
	// each currency's biggest payee first; a tie by name ignoring case, then by byte order
	store.SpendByPayee: spendingQuery("payee", "currency, sum(spent) DESC, lower(payee), payee"),
	// no tag first, then tag name ignoring case (a case-only tie by byte order), then currency
	store.SpendByTag: spendingByTagQuery,
}

// ErrUnsupportedGrouping is what Spending returns for a grouping it cannot read.
var ErrUnsupportedGrouping = errors.New("spending grouping is not supported")

// Spending reads the spending in params.Window (both days counted) grouped by params.By,
// dropping a group that nets to zero; Totals keep it, one per currency. Grouping by tag also
// counts the splits carrying several tags. An unsupported grouping is an error, and a store
// it cannot open or read is a *store.OpenError.
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
	if err == nil && params.By == store.SpendByTag {
		err = db.QueryRows(ctx, multiTagSplitsQuery, args, func(scan func(dest ...any) error) error {
			return scan(&spending.MultiTagSplits)
		})
	}
	if err != nil {
		return store.Spending{}, openFault(s.Path(), err)
	}
	return spending, nil
}

// civilDay is day's calendar date as text, which DuckDB reads as a DATE in no zone.
func civilDay(day time.Time) string {
	return day.Format(time.DateOnly)
}
