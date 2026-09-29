package duckstore_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/require"
)

// newBuiltStore builds a store of minimalRows in a fresh directory and returns a Store over it built with opts.
func newBuiltStore(t *testing.T, opts ...duckstore.Option) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	return duckstore.New(dir, opts...)
}

var errQueryFailed = errors.New("query failed")

// spyReadDB counts Close calls on a real read connection. queryFault and scanFault fail the read's own
// query (onQuery runs first; the first passQueries queries run for real); checkFaults fails one of the open's format checks, keyed by its query.
type spyReadDB struct {
	duckstore.ReadDB

	queryFault  error
	scanFault   error
	checkFaults map[string]error
	onQuery     func()
	closes      int
	passQueries int
	queries     int
}

func (s *spyReadDB) QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error {
	if slices.Contains(duckstore.FormatCheckQueries, query) {
		if fault, ok := s.checkFaults[query]; ok {
			return fault
		}
		return s.ReadDB.QueryRows(ctx, query, args, row)
	}
	s.startQuery()
	s.queries++
	if s.queries <= s.passQueries {
		return s.ReadDB.QueryRows(ctx, query, args, row)
	}
	if s.queryFault != nil {
		return s.queryFault
	}
	if s.scanFault != nil {
		return row(func(...any) error { return s.scanFault })
	}
	return s.ReadDB.QueryRows(ctx, query, args, row)
}

func (s *spyReadDB) QueryTable(ctx context.Context, query string, maxRows int) (duckdb.Table, error) {
	s.startQuery()
	if s.queryFault != nil {
		return duckdb.Table{}, s.queryFault
	}
	return s.ReadDB.QueryTable(ctx, query, maxRows)
}

func (s *spyReadDB) Close() error {
	s.closes++
	return s.ReadDB.Close()
}

func (s *spyReadDB) startQuery() {
	if s.onQuery != nil {
		s.onQuery()
	}
}

// spyOpener opens the store read-only for real and hands spy the connection to wrap.
func spyOpener(spy *spyReadDB) duckstore.Option {
	return duckstore.WithOpenReadOnly(func(ctx context.Context, path string) (duckstore.ReadDB, error) {
		db, err := duckdb.OpenReadOnly(ctx, path)
		if err != nil {
			return nil, err
		}
		spy.ReadDB = db
		return spy, nil
	})
}

// failingOpener is an opener that fails with fault.
func failingOpener(fault error) duckstore.Option {
	return duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		return nil, fault
	})
}

// ioFault is the error chain duckdb.DB returns for a failed read: the
// adapter's wrap around the driver's own error type.
func ioFault(op string) error {
	return fmt.Errorf("%s: %w", op, &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeIO, Msg: "IO Error: disk read failed"})
}

// skipAsRoot skips t under root, whom file modes do not stop.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
}
