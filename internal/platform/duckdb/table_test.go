package duckdb_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	duckdbdriver "github.com/duckdb/duckdb-go/v2"
	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_query_table_returns_columns_and_rows(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	table, err := db.QueryTable(t.Context(), "SELECT 1 AS n, 'a' AS s UNION ALL SELECT 2, NULL ORDER BY n", 0)

	require.NoError(t, err)
	assert.Equal(t, duckdb.Table{
		Columns: []duckdb.Column{{Name: "n", Type: "INTEGER"}, {Name: "s", Type: "VARCHAR"}},
		Rows: [][]duckdb.Value{
			{{Text: "1", Native: int64(1)}, {Text: "a", Native: "a"}},
			{{Text: "2", Native: int64(2)}, {Null: true, Text: "NULL"}},
		},
	}, table)
}

func Test_query_table_stops_at_max_rows(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	cases := []struct {
		name      string
		available int
		maxRows   int
		want      int
	}{
		{name: "exactly max rows available", available: 3, maxRows: 3, want: 3},
		{name: "one row more than max available", available: 4, maxRows: 3, want: 3},
		{name: "max rows 0 returns every row", available: 4, maxRows: 0, want: 4},
		{name: "a negative max rows returns every row", available: 4, maxRows: -1, want: 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), fmt.Sprintf("SELECT range FROM range(%d)", c.available), c.maxRows)

			require.NoError(t, err)
			assert.Len(t, table.Rows, c.want)
		})
	}
}

func Test_query_table_returns_the_driver_error(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	_, err := db.QueryTable(t.Context(), "SELECT missing_column", 0)

	var derr *duckdbdriver.Error
	require.ErrorAs(t, err, &derr)
	assert.Equal(t, duckdbdriver.ErrorTypeBinder, derr.Type)
}

func Test_query_table_refuses_a_type_the_driver_cannot_read(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	_, err := db.QueryTable(t.Context(), "SELECT 'x' AS label, 1::VARIANT AS payload", 0)

	var unprintable *duckdb.UnprintableValueError
	require.ErrorAs(t, err, &unprintable)
	assert.Equal(t, duckdb.UnprintableValueError{Column: "payload", Type: "VARIANT"}, *unprintable)
}

func Test_query_table_refuses_a_column_it_cannot_print(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	_, err := db.QueryTable(t.Context(), `SELECT 1 AS n, '{"a": 1}'::JSON AS doc`, 0)

	var unprintable *duckdb.UnprintableValueError
	require.ErrorAs(t, err, &unprintable)
	assert.Equal(t, duckdb.UnprintableValueError{Column: "doc", Type: "JSON"}, *unprintable)
	assert.EqualError(t, err, `cannot print column "doc" of type JSON`)
}

func Test_query_table_fails_when_the_context_is_cancelled_mid_iteration(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	var errCalls atomic.Int64
	ctx := scriptedContext{err: func() error {
		if errCalls.Add(1) > 500 {
			return context.Canceled
		}
		return nil
	}}

	table, err := db.QueryTable(ctx, "SELECT i FROM range(5000) t(i)", 0)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, table.Rows)
}
