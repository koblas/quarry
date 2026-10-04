package duckstore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// storeRelations are the tables and views a built store holds, for sql users to query.
func storeRelations() []string {
	return []string{
		"accounts", "categories", "finding_items", "findings", "fx_rates", "import_runs", "payees", "prices", "securities", "split_tags", "splits",
		"store_info", "tags", "transactions", "transfers", "v_account_balances", "v_cash_flow", "v_spending",
	}
}

func Test_query_show_tables_lists_every_table_and_view(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	got, err := st.Query(t.Context(), "SHOW TABLES", 0)

	require.NoError(t, err)
	names := make([]string, len(got.Rows))
	for i, row := range got.Rows {
		names[i] = row[0].Text
	}
	assert.ElementsMatch(t, storeRelations(), names)
}

func Test_query_prints_every_column_of_each_table_and_view(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	for _, name := range storeRelations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := st.Query(t.Context(), "SELECT * FROM "+name, 0) //nolint:unqueryvet // every column is the point

			require.NoError(t, err)
			assert.NotEmpty(t, got.Rows)
		})
	}
}

func Test_query_returns_at_most_max_rows(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	got, err := st.Query(t.Context(), "SELECT range FROM range(5)", 2)

	require.NoError(t, err)
	assert.Len(t, got.Rows, 2)
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

func Test_query_reports_a_file_of_unknown_type_named_as_a_table_as_a_query_error(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), "SELECT 1 FROM '/etc/hosts'", 0)

	var queryErr *store.QueryError
	require.ErrorAs(t, err, &queryErr)
	assert.Equal(t, "Catalog Error: Table with name /etc/hosts does not exist!", queryErr.Reason)
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
		{name: "a known file type named as a table", query: "SELECT * FROM 'x.parquet'", want: store.ErrExternalAccess},
		{name: "a lone semicolon", query: ";", want: store.ErrEmptyQuery},
		{name: "a lone comment", query: "-- note", want: store.ErrEmptyQuery},
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

func Test_query_interrupted_by_its_deadline_is_not_a_cancel(t *testing.T) {
	t.Parallel()
	const slowQuery = "SELECT sum(a.range * b.range) FROM range(1000000) a, range(1000000) b"
	cases := []struct {
		name  string
		start func(t *testing.T) (context.Context, *duckstore.Store)
		is    error
		isNot error
	}{
		{
			name: "a query past its deadline",
			start: func(t *testing.T) (context.Context, *duckstore.Store) {
				t.Helper()
				ctx := deadlineIn(t, 500*time.Millisecond)
				return ctx, newBuiltStore(t, spyOpener(&spyReadDB{queryFault: driverInterrupt(), onQuery: func() { <-ctx.Done() }}))
			},
			is: context.DeadlineExceeded, isNot: context.Canceled,
		},
		{
			name: "a query cancelled",
			start: func(t *testing.T) (context.Context, *duckstore.Store) {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				return ctx, newBuiltStore(t, spyOpener(&spyReadDB{queryFault: driverInterrupt(), onQuery: cancel}))
			},
			is: context.Canceled, isNot: context.DeadlineExceeded,
		},
		{
			name: "an open past its deadline",
			start: func(t *testing.T) (context.Context, *duckstore.Store) {
				t.Helper()
				return deadlineIn(t, -time.Second), newBuiltStore(t, failingOpener(ioFault("open store read-only")))
			},
			is: context.DeadlineExceeded, isNot: context.Canceled,
		},
		{
			name: "an open cancelled",
			start: func(t *testing.T) (context.Context, *duckstore.Store) {
				t.Helper()
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx, newBuiltStore(t, failingOpener(ioFault("open store read-only")))
			},
			is: context.Canceled, isNot: context.DeadlineExceeded,
		},
		{
			name: "a real slow query past its deadline",
			start: func(t *testing.T) (context.Context, *duckstore.Store) {
				t.Helper()
				return deadlineIn(t, time.Second), newBuiltStore(t)
			},
			is: context.DeadlineExceeded, isNot: context.Canceled,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ctx, st := c.start(t)

			_, err := st.Query(ctx, slowQuery, 0)

			require.ErrorIs(t, err, store.ErrQueryInterrupted)
			require.ErrorIs(t, err, c.is)
			assert.NotErrorIs(t, err, c.isNot)
		})
	}
}

// deadlineIn is a context whose deadline is d from now, a negative d being already past.
func deadlineIn(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	t.Cleanup(cancel)
	return ctx
}

func Test_query_refuses_a_value_it_cannot_print(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)

	_, err := st.Query(t.Context(), `SELECT '{"a": 1}'::JSON AS doc`, 0)

	var unprintable *store.UnprintableValueError
	require.ErrorAs(t, err, &unprintable)
	assert.Equal(t, store.UnprintableValueError{Column: "doc", Type: "JSON"}, *unprintable)
}

// driverInterrupt is the error the driver returns for a query it interrupted mid-run, which does not carry the context's error.
func driverInterrupt() error {
	return &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeInterrupt, Msg: "INTERRUPT Error: Interrupted!"}
}

// interruptFault is a driver interrupt error that also carries the context's own error.
func interruptFault() error {
	return errors.Join(context.Canceled, &duckdbdriver.Error{Type: duckdbdriver.ErrorTypeInterrupt, Msg: "INTERRUPT Error: Interrupted!"})
}
