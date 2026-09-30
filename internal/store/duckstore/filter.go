package duckstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// accountFilter is the ids of the accounts a windowed read counts; empty counts every account.
type accountFilter []string

// and is the clause keeping only column values among the named accounts, or "" for every
// account. Its parameters are numbered after the window's two.
func (f accountFilter) and(column string) string {
	if len(f) == 0 {
		return ""
	}
	return " AND " + column + " IN (" + f.marks(3) + ")"
}

// args is one query argument per named account.
func (f accountFilter) args() []any {
	args := make([]any, len(f))
	for i, id := range f {
		args[i] = id
	}
	return args
}

// readArgs is the arguments of a windowed read: the window's first and last day, then the named accounts.
func readArgs(window store.Window, accounts accountFilter) []any {
	return append([]any{civilDay(window.Since), civilDay(window.Until)}, accounts.args()...)
}

// marks is one parameter per named account, numbered from first.
func (f accountFilter) marks(first int) string {
	marks := make([]string, len(f))
	for i := range f {
		marks[i] = fmt.Sprintf("$%d", first+i)
	}
	return strings.Join(marks, ", ")
}

// transactionRangeQuery is the first and last day of every transaction, or of the named reported
// accounts' transactions; its parameters are those accounts, numbered from $1.
func transactionRangeQuery(accounts accountFilter) string {
	if len(accounts) == 0 {
		return "SELECT min(date), max(date) FROM transactions"
	}
	return `SELECT min(t.date), max(t.date)
FROM transactions t JOIN accounts a ON a.id = t.account_id
WHERE ` + reportedAccount + ` AND t.account_id IN (` + accounts.marks(1) + ")"
}

// transactionRange reads the first and last day of the transactions transactionRangeQuery selects
// for accounts, through db; it is the zero range when there are none.
func transactionRange(ctx context.Context, db ReadDB, accounts accountFilter) (store.TransactionRange, error) {
	var first, last sql.NullTime
	err := db.QueryRows(ctx, transactionRangeQuery(accounts), accounts.args(), func(scan func(dest ...any) error) error {
		return scan(&first, &last)
	})
	return store.TransactionRange{First: first.Time, Last: last.Time}, err //nolint:wrapcheck // callers classify the driver's own error with openFault
}

// civilDay is day's calendar date as text, which DuckDB reads as a DATE in no zone.
func civilDay(day time.Time) string {
	return day.Format(time.DateOnly)
}
