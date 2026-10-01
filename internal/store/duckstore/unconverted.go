package duckstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// unconvertedView is the view a report reads and the column of it that holds an amount in the
// report currency, named without its _cad or _usd suffix.
type unconvertedView struct {
	name, column string
	// where narrows the view to the rows the report counts; empty counts them all.
	where string
}

var (
	spendingUnconverted = unconvertedView{name: "v_spending", column: "spent"}
	cashFlowUnconverted = unconvertedView{name: "v_cash_flow", column: "amount", where: "flow IN ('income', 'expense')"}
)

// unconvertedQuery counts the distinct transactions of view in the window and accounts whose
// convertedColumn is NULL, and gives the first rate date; a split in neither CAD nor USD counts nothing.
func unconvertedQuery(view unconvertedView, convertedColumn string, accounts accountFilter) string {
	filter := ""
	if view.where != "" {
		filter = " AND " + view.where
	}
	return fmt.Sprintf(`
SELECT count(DISTINCT transaction_id), (SELECT min(date) FROM fx_rates)
FROM %[1]s
WHERE %[2]s IS NULL AND currency IN ('CAD', 'USD')%[3]s
	AND date >= CAST($1 AS DATE) AND date <= CAST($2 AS DATE)%[4]s`,
		view.name, convertedColumn, filter, accounts.and("account_id"))
}

// read fills store.Unconverted for a report in currency over the window and accounts args name;
// a native report converts nothing, so it is the zero value without a query.
func (v unconvertedView) read(ctx context.Context, db ReadDB, currency money.Currency, accounts accountFilter, args []any) (store.Unconverted, error) {
	var column string
	switch currency { //nolint:exhaustive // Native, and any value outside the three, converts nothing
	case money.CAD:
		column = v.column + "_cad"
	case money.USD:
		column = v.column + "_usd"
	default:
		return store.Unconverted{}, nil
	}
	var unconverted store.Unconverted
	var firstRate sql.NullTime
	err := db.QueryRows(ctx, unconvertedQuery(v, column, accounts), args, func(scan func(dest ...any) error) error {
		return scan(&unconverted.Transactions, &firstRate)
	})
	unconverted.FirstRate = firstRate.Time
	return unconverted, err //nolint:wrapcheck // callers classify the driver's own error with openFault
}
