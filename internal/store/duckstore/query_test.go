package duckstore_test

import (
	"context"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBuiltStore(t *testing.T, opts ...duckstore.Option) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	return duckstore.New(dir, opts...)
}

func Test_query_reads_a_built_store(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	got, err := st.Query(t.Context(), "SELECT name, balance FROM v_account_balances", 0)

	require.NoError(t, err)
	assert.Equal(t, store.QueryResult{
		Columns: []store.QueryColumn{{Name: "name", Type: "VARCHAR"}, {Name: "balance", Type: "DECIMAL(18,2)"}},
		Rows:    [][]store.QueryValue{{{Text: "Chequing", Native: "Chequing"}, {Text: "12.34", Native: "12.34"}}},
	}, got)
}

func Test_query_returns_at_most_max_rows(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	got, err := st.Query(t.Context(), "SELECT range FROM range(5)", 2)

	require.NoError(t, err)
	assert.Len(t, got.Rows, 2)
}

func Test_query_returns_the_open_fault(t *testing.T) {
	t.Parallel()
	fault := ioFault("open store read-only")
	st := duckstore.New(t.TempDir(), duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		return nil, fault
	}))

	_, err := st.Query(t.Context(), "SELECT 1", 0)

	require.ErrorIs(t, err, fault)
	assert.EqualError(t, err, "run query: "+fault.Error())
}

func Test_query_returns_the_driver_error_for_a_bad_query(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), "SELECT missing_column FROM accounts", 0)

	var derr *duckdbdriver.Error
	require.ErrorAs(t, err, &derr)
	assert.Equal(t, "run query: "+derr.Error(), err.Error())
}

func Test_query_refuses_a_value_it_cannot_print(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), `SELECT '{"a": 1}'::JSON AS doc`, 0)

	var unprintable *store.UnprintableValueError
	require.ErrorAs(t, err, &unprintable)
	assert.Equal(t, store.UnprintableValueError{Column: "doc", Type: "JSON"}, *unprintable)
}

func Test_query_closes_the_connection_on_success_and_on_a_query_fault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fault error
	}{
		{name: "after a successful query", fault: nil},
		{name: "after a query fault", fault: errQueryFailed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			spy := &spyReadDB{queryFault: c.fault}
			st := newBuiltStore(t, duckstore.WithOpenReadOnly(func(ctx context.Context, p string) (duckstore.ReadDB, error) {
				db, openErr := duckdb.OpenReadOnly(ctx, p)
				spy.ReadDB = db
				return spy, openErr
			}))

			_, _ = st.Query(t.Context(), "SELECT 1", 0)

			assert.Equal(t, 1, spy.closes)
		})
	}
}
