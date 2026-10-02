package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// QueryResult is the result of one query against the store.
type QueryResult struct {
	Columns []QueryColumn
	Rows    [][]QueryValue
}

// QueryColumn is one result column: its name and its DuckDB type name.
type QueryColumn struct {
	Name string
	Type string
}

// numericType matches every DuckDB integer, floating-point and DECIMAL type name.
var numericType = regexp.MustCompile(`^(TINYINT|SMALLINT|INTEGER|BIGINT|HUGEINT|BIGNUM|` +
	`UTINYINT|USMALLINT|UINTEGER|UBIGINT|UHUGEINT|FLOAT|DOUBLE|DECIMAL\(\d+,\d+\))$`)

// Numeric reports whether the column holds numbers: an integer, FLOAT,
// DOUBLE or DECIMAL type, not a LIST, ARRAY or STRUCT of them.
func (c QueryColumn) Numeric() bool {
	return numericType.MatchString(c.Type)
}

// QueryValue is one result cell. Text is DuckDB's own text for it ("NULL"
// for a NULL). Native is nil, a bool, int64, uint64, float32 (FLOAT),
// float64 (DOUBLE), a time.Time for a finite DATE or TIMESTAMP, or Text.
type QueryValue struct {
	Null   bool
	Text   string
	Native any
}

// UnprintableValueError is a query refused because a column holds a value
// quarry cannot print as DuckDB's text.
type UnprintableValueError struct {
	Column string
	Type   string
}

func (e *UnprintableValueError) Error() string {
	return `cannot print column "` + e.Column + `" of type ` + e.Type
}

// ErrReadOnlyQuery is a query refused because it would change the store.
var ErrReadOnlyQuery = errors.New("query would change the store")

// ErrExternalAccess is a query refused because it reaches another file, database or extension.
var ErrExternalAccess = errors.New("query reaches outside the store")

// ErrEmptyQuery is a query refused because it holds no statement.
var ErrEmptyQuery = errors.New("query holds no statement")

// ErrQueryInterrupted is a query stopped because its context was cancelled.
var ErrQueryInterrupted = errors.New("query interrupted")

// Interrupted is err reported as a query stopped by its context; errors.Is
// matches both ErrQueryInterrupted and err.
func Interrupted(err error) error {
	return fmt.Errorf("%w: %w", ErrQueryInterrupted, err)
}

// InterruptedBy is Interrupted(err) for a query ctx stopped: it also unwraps to ctx.Err(), so
// errors.Is tells a deadline from a cancel whatever err carries. Its text is Interrupted(err)'s.
func InterruptedBy(ctx context.Context, err error) error {
	return interruptedError{error: Interrupted(err), ctxErr: ctx.Err()}
}

// interruptedError is an Interrupted error that also unwraps to the context error behind it.
type interruptedError struct {
	error

	ctxErr error
}

func (e interruptedError) Unwrap() []error { return []error{e.error, e.ctxErr} }

// QueryError is a query the database rejected; Reason is the first line of
// the database's own message, verbatim.
type QueryError struct {
	Reason string
}

func (e *QueryError) Error() string {
	return e.Reason
}
