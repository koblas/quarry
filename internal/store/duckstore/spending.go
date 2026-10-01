package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// spendingQueryFor is a spending query for the accounts of a filter over source, a relation of
// v_spending's columns (see spendingSource); each takes the window's first and last day as $1
// and $2, then one parameter per named account.
type spendingQueryFor func(accounts accountFilter, source string) string

// spendingSource is the relation a spending read counts, in the shape of v_spending: native is
// v_spending itself; CAD and USD give each split its converted spent and that currency, except a
// split with no converted cell, which keeps its own spent and currency so a total never mixes.
func spendingSource(currency money.Currency) string {
	var converted, target string
	switch currency { //nolint:exhaustive // Native, and any value outside the three, reads v_spending as it is
	case money.CAD:
		converted, target = "spent_cad", "CAD"
	case money.USD:
		converted, target = "spent_usd", "USD"
	default:
		return "v_spending"
	}
	return fmt.Sprintf(`(SELECT split_id, account_id, date, month, category, payee,
	CASE WHEN %[1]s IS NULL THEN currency ELSE '%[2]s' END AS currency,
	COALESCE(%[1]s, spent) AS spent
	FROM v_spending)`, converted, target)
}

// spendingQuery reads per-group and per-currency-total spending, grouped by the key column and
// ordered by rowOrder after the total rows.
func spendingQuery(key, rowOrder string) spendingQueryFor {
	return func(accounts accountFilter, source string) string {
		return spendingQueryFrom(source, key, rowOrder, accounts)
	}
}

// spendingQueryFrom is the query of spendingQuery over source, a relation of v_spending's
// account_id, date, currency and spent columns plus the key column.
func spendingQueryFrom(source, key, rowOrder string, accounts accountFilter) string {
	return fmt.Sprintf(`
SELECT %[1]s, currency, CAST(sum(spent) * 100 AS BIGINT), GROUPING(%[1]s)
FROM %[3]s
WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)%[4]s
GROUP BY GROUPING SETS ((%[1]s, currency), (currency))
HAVING GROUPING(%[1]s) = 1 OR sum(spent) <> 0
ORDER BY GROUPING(%[1]s), %[2]s`, key, rowOrder, source, accounts.and("account_id"))
}

// spendingByMonthQuery keys each split by its month as YYYY-MM text, not a scanned DATE.
func spendingByMonthQuery(accounts accountFilter, source string) string {
	return spendingQueryFrom(
		"(SELECT account_id, date, currency, spent, strftime(month, '%Y-%m') AS month_key FROM "+source+")",
		"month_key", "month_key, currency", accounts)
}

// splitTagNames is a CTE of each split's distinct tag names; a link to a missing tag has none.
const splitTagNames = `WITH split_tag_names AS (
	SELECT DISTINCT st.split_id, t.name FROM split_tags st JOIN tags t ON t.id = st.tag_id
)`

// spendingByTagQuery reads spending per tag, then per-currency totals counting each split once.
func spendingByTagQuery(accounts accountFilter, source string) string {
	return splitTagNames + `
SELECT tag, currency, cents, grp FROM (
	SELECT n.name AS tag, s.currency, CAST(sum(s.spent) * 100 AS BIGINT) AS cents, 0 AS grp
	FROM ` + source + ` s LEFT JOIN split_tag_names n ON n.split_id = s.split_id
	WHERE s.date >= CAST($1 AS DATE) AND s.date <= CAST($2 AS DATE)` + accounts.and("s.account_id") + `
	GROUP BY n.name, s.currency
	HAVING sum(s.spent) <> 0
	UNION ALL
	SELECT NULL, currency, CAST(sum(spent) * 100 AS BIGINT), 1
	FROM ` + source + `
	WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)` + accounts.and("account_id") + `
	GROUP BY currency
)
ORDER BY grp, tag IS NOT NULL, lower(tag), tag, currency`
}

// multiTagSplitsQuery counts the splits in the window carrying more than one tag.
func multiTagSplitsQuery(accounts accountFilter) string {
	return splitTagNames + `
SELECT count(*) FROM (
	SELECT n.split_id FROM split_tag_names n
	WHERE n.split_id IN (
		SELECT split_id FROM v_spending WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)` + accounts.and("account_id") + `
	)
	GROUP BY n.split_id HAVING count(*) > 1
)`
}

// spendingQueries is the query of each grouping spending can read.
var spendingQueries = map[store.SpendingGroup]spendingQueryFor{
	// uncategorized first, then category ignoring case (a case-only tie by byte order), then currency
	store.SpendByCategory: spendingQuery("category", "category IS NOT NULL, lower(category), category, currency"),
	// each currency's biggest payee first; a tie by name ignoring case, then by byte order
	store.SpendByPayee: spendingQuery("payee", "currency, sum(spent) DESC, lower(payee), payee"),
	// no tag first, then tag name ignoring case (a case-only tie by byte order), then currency
	store.SpendByTag: spendingByTagQuery,
	// oldest month first, then currency
	store.SpendByMonth: spendingByMonthQuery,
}

// ErrUnsupportedGrouping is what Spending returns for a grouping it cannot read. The spend command
// refuses every --by value outside its grouping table before a read, so only a caller
// bypassing that table reaches it.
var ErrUnsupportedGrouping = errors.New("spending grouping is not supported")

// Spending reads the spending in params.Window (both days counted) grouped by params.By over
// params.AccountIDs (every account when empty), dropping a group netting to zero; Totals keep it.
// It fills MultiTagSplits and Transactions as store.Spending documents. An unknown grouping is
// ErrUnsupportedGrouping; a store it cannot open or read is a *store.OpenError.
func (s *Store) Spending(ctx context.Context, params store.SpendingParams) (store.Spending, error) {
	queryFor, ok := spendingQueries[params.By]
	if !ok {
		return store.Spending{}, fmt.Errorf("%w: %d", ErrUnsupportedGrouping, params.By)
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Spending{}, err
	}
	defer func() { _ = db.Close() }()

	var spending store.Spending
	accounts := accountFilter(params.AccountIDs)
	args := readArgs(params.Window, accounts)
	err = db.QueryRows(ctx, queryFor(accounts, spendingSource(params.Currency)), args, func(scan func(dest ...any) error) error {
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
		err = db.QueryRows(ctx, multiTagSplitsQuery(accounts), args, func(scan func(dest ...any) error) error {
			return scan(&spending.MultiTagSplits)
		})
	}
	if err == nil && len(spending.Totals) == 0 {
		spending.Transactions, err = transactionRange(ctx, db, accounts)
	}
	if err != nil {
		return store.Spending{}, openFault(s.Path(), err)
	}
	return spending, nil
}
