package duckstore_test

import (
	"context"
	"errors"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
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
	st := newBuiltStore(t, failingOpener(fault))

	_, err := st.Query(t.Context(), "SELECT 1", 0)

	require.ErrorIs(t, err, fault)
	var openErr *store.OpenError
	assert.ErrorAs(t, err, &openErr)
}

func Test_query_reports_the_first_line_of_a_bad_query(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), "SELECT missing_column FROM accounts", 0)

	var queryErr *store.QueryError
	require.ErrorAs(t, err, &queryErr)
	assert.Equal(t, `Binder Error: Referenced column "missing_column" not found in FROM clause!`, queryErr.Reason)
}

func Test_query_reports_a_locked_setting_as_a_query_error(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), "SET enable_external_access=true", 0)

	var queryErr *store.QueryError
	require.ErrorAs(t, err, &queryErr)
	assert.Equal(t, `Invalid Input Error: Cannot change configuration option "enable_external_access" - the configuration has been locked`,
		queryErr.Reason)
}

func Test_query_refuses_what_a_read_may_not_do(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  error
	}{
		{name: "a write", query: "CREATE TABLE notes (body VARCHAR)", want: store.ErrReadOnlyQuery},
		{name: "another file", query: "SELECT * FROM read_csv('/etc/hosts')", want: store.ErrExternalAccess},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t)

			_, err := st.Query(t.Context(), c.query, 0)

			require.ErrorIs(t, err, c.want)
		})
	}
}

func Test_query_reports_an_interrupted_query(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: interruptFault(), onQuery: cancel}))

	_, err := st.Query(ctx, "SELECT 1", 0)

	require.ErrorIs(t, err, store.ErrQueryInterrupted)
	var openErr *store.OpenError
	assert.NotErrorAs(t, err, &openErr, "the open succeeded; only the query was interrupted")
}

func Test_query_reports_an_interrupt_error_under_a_live_context_as_a_query_error(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, spyOpener(&spyReadDB{queryFault: interruptFault()}))

	_, err := st.Query(t.Context(), "SELECT 1", 0)

	var queryErr *store.QueryError
	require.ErrorAs(t, err, &queryErr)
	assert.Equal(t, "context canceled", queryErr.Reason)
}

func Test_query_reports_an_open_interrupted_by_its_context(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, failingOpener(ioFault("open store read-only")))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := st.Query(ctx, "SELECT 1", 0)

	require.ErrorIs(t, err, store.ErrQueryInterrupted)
	var openErr *store.OpenError
	assert.ErrorAs(t, err, &openErr)
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
			st := newBuiltStore(t, spyOpener(spy))

			_, _ = st.Query(t.Context(), "SELECT 1", 0)

			assert.Equal(t, 1, spy.closes)
		})
	}
}

// interruptFault is the error chain the driver returns for a query its context interrupted.
func interruptFault() error {
	return errors.Join(context.Canceled, &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeInterrupt, Msg: "INTERRUPT Error: Interrupted!"})
}
