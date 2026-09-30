package duckstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/koblas/quarry/internal/store"
)

// cashFlowKeys is the SQL text of each period's key, matching Go's 2006-01 and 2006 layouts.
var cashFlowKeys = map[store.CashFlowPeriod]string{
	store.CashFlowByMonth: "strftime(date, '%Y-%m')",
	store.CashFlowByYear:  "strftime(date, '%Y')",
}

// ErrUnsupportedPeriod is what CashFlow returns for a period unit it cannot read. The cashflow
// command refuses every --by value other than month and year before a read, so only a caller
// bypassing that table reaches it.
var ErrUnsupportedPeriod = errors.New("cash-flow period is not supported")

// cashFlowQuery reads per-period and per-currency-total income, spending and net in cents from
// v_cash_flow, and the savings rate as BIGINT tenths / 10.0 (always finite), or NULL.
func cashFlowQuery(key string, accounts accountFilter) string {
	// Integer rounding half away from zero never yields a negative zero; income <= 0 has no rate.
	return fmt.Sprintf(`
SELECT period_key, currency, income, spent, income - spent,
	CASE WHEN income > 0 THEN CAST(sign(income - spent) * ((2000 * abs(CAST(income - spent AS HUGEINT)) + income) // (2 * income)) AS BIGINT) / 10.0 END,
	grp
FROM (
	SELECT period_key, currency,
		CAST(COALESCE(sum(CASE WHEN flow = 'income' THEN amount END), 0) * 100 AS BIGINT) AS income,
		CAST(COALESCE(sum(CASE WHEN flow = 'expense' THEN -amount END), 0) * 100 AS BIGINT) AS spent,
		GROUPING(period_key) AS grp
	FROM (SELECT account_id, date, currency, flow, amount, %s AS period_key FROM v_cash_flow)
	WHERE date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)%s
	GROUP BY GROUPING SETS ((period_key, currency), (currency))
)
ORDER BY grp, period_key, currency`, key, accounts.and("account_id"))
}

// CashFlow reads income and spending in params.Window (both days counted), per period and
// currency, counting only params.AccountIDs when any (every account otherwise); Totals give one
// row per currency. A window with no income or spending also gets Transactions, as Spending does.
// An unsupported period is ErrUnsupportedPeriod, and a store it cannot open or read is a *store.OpenError.
func (s *Store) CashFlow(ctx context.Context, params store.CashFlowParams) (store.CashFlow, error) {
	key, ok := cashFlowKeys[params.By]
	if !ok {
		return store.CashFlow{}, fmt.Errorf("%w: %d", ErrUnsupportedPeriod, params.By)
	}
	db, err := s.openRead(ctx)
	if err != nil {
		return store.CashFlow{}, err
	}
	defer func() { _ = db.Close() }()

	var flow store.CashFlow
	accounts := accountFilter(params.AccountIDs)
	err = db.QueryRows(ctx, cashFlowQuery(key, accounts), readArgs(params.Window, accounts), func(scan func(dest ...any) error) error {
		var period sql.NullString
		var currency string
		var income, spent, net, grouping int64
		var rate sql.NullFloat64
		if err := scan(&period, &currency, &income, &spent, &net, &rate, &grouping); err != nil {
			return err
		}
		var ratePct *float64
		if rate.Valid {
			ratePct = &rate.Float64
		}
		if grouping == 1 {
			flow.Totals = append(flow.Totals, store.CashFlowTotal{Currency: currency, Income: income, Spent: spent, Net: net, SavingsRatePct: ratePct})
			return nil
		}
		flow.Rows = append(flow.Rows, store.CashFlowRow{Period: period.String, Currency: currency, Income: income, Spent: spent, Net: net, SavingsRatePct: ratePct})
		return nil
	})
	if err == nil && len(flow.Totals) == 0 {
		flow.Transactions, err = transactionRange(ctx, db, accounts)
	}
	if err != nil {
		return store.CashFlow{}, openFault(s.Path(), err)
	}
	return flow, nil
}
