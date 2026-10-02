package report

import (
	"errors"

	"github.com/koblas/quarry/internal/store"
)

// QueryFailureKind is why a query failed, as far as a surface can word it for the person who wrote it.
type QueryFailureKind int

// The reasons a query fails; each surface words its own copy for them.
const (
	// QueryFailureOther is any failure with no reason of its own: a refused store, a store fault.
	QueryFailureOther QueryFailureKind = iota
	// QueryFailureUnprintable is a result column quarry cannot print; Detail is the column.
	QueryFailureUnprintable
	// QueryFailureRejected is a query the database rejected; Detail is its own first line of reason.
	QueryFailureRejected
	// QueryFailureEmpty is a query holding no statement.
	QueryFailureEmpty
	// QueryFailureReadOnly is a query that would change the store.
	QueryFailureReadOnly
	// QueryFailureExternalAccess is a query reaching another file, database or extension.
	QueryFailureExternalAccess
	// QueryFailureInterrupted is a query cancelled before it finished.
	QueryFailureInterrupted
)

// QueryFailure is the reason a query failed. Err is the store error that decided Kind (the typed error, or the
// sentinel), nil for QueryFailureOther.
type QueryFailure struct {
	Kind   QueryFailureKind
	Detail string
	Err    error
}

// ClassifyQueryFailure names why err, as returned by Server.Query or the store, failed the query. A typed store
// error wins over a sentinel when err wraps both; anything else, a RefusalError included, is QueryFailureOther.
func ClassifyQueryFailure(err error) QueryFailure {
	if unprintable, ok := errors.AsType[*store.UnprintableValueError](err); ok {
		return QueryFailure{Kind: QueryFailureUnprintable, Detail: unprintable.Column, Err: unprintable}
	}
	if queryErr, ok := errors.AsType[*store.QueryError](err); ok {
		return QueryFailure{Kind: QueryFailureRejected, Detail: queryErr.Reason, Err: queryErr}
	}
	switch {
	case errors.Is(err, store.ErrEmptyQuery):
		return QueryFailure{Kind: QueryFailureEmpty, Err: store.ErrEmptyQuery}
	case errors.Is(err, store.ErrReadOnlyQuery):
		return QueryFailure{Kind: QueryFailureReadOnly, Err: store.ErrReadOnlyQuery}
	case errors.Is(err, store.ErrExternalAccess):
		return QueryFailure{Kind: QueryFailureExternalAccess, Err: store.ErrExternalAccess}
	case errors.Is(err, store.ErrQueryInterrupted):
		return QueryFailure{Kind: QueryFailureInterrupted, Err: store.ErrQueryInterrupted}
	}
	return QueryFailure{Kind: QueryFailureOther}
}
