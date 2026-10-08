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

func Test_query_table_text_matches_duckdb_cast(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)
	cases := []struct {
		name   string
		expr   string
		native string
	}{
		{name: "DECIMAL zero keeps its scale", expr: "CAST(0 AS DECIMAL(18,2))", native: "string 0.00"},
		{name: "DECIMAL below one", expr: "CAST(-0.05 AS DECIMAL(18,2))", native: "string -0.05"},
		{name: "DECIMAL keeps a trailing zero", expr: "CAST(12.5 AS DECIMAL(18,2))", native: "string 12.50"},
		{name: "DECIMAL(38,2) from SUM", expr: "(SELECT SUM(x) FROM (VALUES (CAST(1234.5 AS DECIMAL(18,2)))) t(x))", native: "string 1234.50"},
		{name: "DECIMAL of scale zero", expr: "CAST(12 AS DECIMAL(4,0))", native: "string 12"},
		{name: "HUGEINT from SUM", expr: "(SELECT SUM(x) FROM (VALUES (9223372036854775807::BIGINT), (1::BIGINT)) t(x))", native: "string 9223372036854775808"},
		{name: "UHUGEINT", expr: "CAST(340282366920938463463374607431768211455 AS UHUGEINT)", native: "string 340282366920938463463374607431768211455"},
		{name: "BIGNUM", expr: "123456789012345678901234567890::BIGNUM", native: "string 123456789012345678901234567890"},
		{name: "TINYINT", expr: "CAST(-128 AS TINYINT)", native: "int64 -128"},
		{name: "SMALLINT", expr: "CAST(-32768 AS SMALLINT)", native: "int64 -32768"},
		{name: "INTEGER", expr: "CAST(-2147483648 AS INTEGER)", native: "int64 -2147483648"},
		{name: "BIGINT", expr: "CAST(-9223372036854775808 AS BIGINT)", native: "int64 -9223372036854775808"},
		{name: "UTINYINT", expr: "CAST(255 AS UTINYINT)", native: "uint64 255"},
		{name: "USMALLINT", expr: "CAST(65535 AS USMALLINT)", native: "uint64 65535"},
		{name: "UINTEGER", expr: "CAST(4294967295 AS UINTEGER)", native: "uint64 4294967295"},
		{name: "UBIGINT", expr: "CAST(18446744073709551615 AS UBIGINT)", native: "uint64 18446744073709551615"},
		{name: "DOUBLE whole number", expr: "1.0::DOUBLE", native: "float64 1"},
		{name: "DOUBLE fraction", expr: "0.1::DOUBLE", native: "float64 0.1"},
		{name: "DOUBLE large whole number", expr: "123456789.0::DOUBLE", native: "float64 1.23456789e+08"},
		{name: "DOUBLE just below the exponent threshold", expr: "1e15::DOUBLE", native: "float64 1e+15"},
		{name: "DOUBLE at the exponent threshold", expr: "1e16::DOUBLE", native: "float64 1e+16"},
		{name: "DOUBLE below 1e-4", expr: "1e-05::DOUBLE", native: "float64 1e-05"},
		{name: "DOUBLE at 1e-4", expr: "0.0001::DOUBLE", native: "float64 0.0001"},
		{name: "DOUBLE nan", expr: "'nan'::DOUBLE", native: "float64 NaN"},
		{name: "DOUBLE negative infinity", expr: "'-inf'::DOUBLE", native: "float64 -Inf"},
		{name: "DOUBLE negative zero", expr: "-0.0::DOUBLE", native: "float64 -0"},
		{name: "DOUBLE from AVG", expr: "(SELECT AVG(x) FROM (VALUES (1), (2), (4)) t(x))", native: "float64 2.3333333333333335"},
		{name: "FLOAT whole number", expr: "1.0::FLOAT", native: "float32 1"},
		{name: "FLOAT at 1e-4", expr: "0.0001::FLOAT", native: "float32 0.0001"},
		{name: "FLOAT rounded to its own precision", expr: "123456789::FLOAT", native: "float32 1.2345679e+08"},
		{name: "FLOAT infinity", expr: "'inf'::FLOAT", native: "float32 +Inf"},
		{name: "BOOLEAN", expr: "true", native: "bool true"},
		{name: "VARCHAR", expr: "'plain text'", native: "string plain text"},
		{name: "ENUM", expr: "'a'::ENUM('a', 'b')", native: "string a"},
		{name: "BIT", expr: "'10101'::BIT", native: "string 10101"},
		{name: "DATE", expr: "DATE '2026-09-29'", native: "time.Time 2026-09-29 00:00:00 +0000 UTC"},
		{name: "DATE before year 1000", expr: "DATE '0033-01-02'", native: "time.Time 0033-01-02 00:00:00 +0000 UTC"},
		{name: "DATE before year 1", expr: "CAST(DATE '0001-01-01' - INTERVAL 1 DAY AS DATE)", native: "time.Time 0000-12-31 00:00:00 +0000 UTC"},
		{name: "DATE after year 9999", expr: "DATE '12345-01-02'", native: "time.Time 12345-01-02 00:00:00 +0000 UTC"},
		{name: "DATE infinity", expr: "DATE 'infinity'", native: "string infinity"},
		{name: "DATE negative infinity", expr: "DATE '-infinity'", native: "string -infinity"},
		{name: "TIME with a fraction", expr: "TIME '03:04:05.5'", native: "string 03:04:05.5"},
		{name: "TIMETZ with an hour offset", expr: "TIMETZ '03:04:05+02'", native: "string 03:04:05+02"},
		{name: "TIMETZ with a seconds offset", expr: "TIMETZ '03:04:05.25-02:30:15'", native: "string 03:04:05.25-02:30:15"},
		{name: "TIMETZ offset of zero minutes and some seconds drops the minutes", expr: "TIMETZ '12:00:00-00:00:30'", native: "string 12:00:00-00:30"},
		{name: "TIMETZ offset of one minute and some seconds", expr: "TIMETZ '12:00:00+00:01:30'", native: "string 12:00:00+00:01:30"},
		{name: "TIMESTAMP with hundredths", expr: "TIMESTAMP '2026-01-02 03:04:05.12'", native: "time.Time 2026-01-02 03:04:05.12 +0000 UTC"},
		{name: "TIMESTAMP with one microsecond", expr: "TIMESTAMP '2026-01-02 03:04:05.000001'", native: "time.Time 2026-01-02 03:04:05.000001 +0000 UTC"},
		{name: "TIMESTAMP before year 1", expr: "TIMESTAMP '0033-01-02 03:04:05' - INTERVAL 40 YEARS", native: "time.Time -0007-01-02 03:04:05 +0000 UTC"},
		{name: "TIMESTAMP infinity", expr: "TIMESTAMP 'infinity'", native: "string infinity"},
		{name: "TIMESTAMP negative infinity", expr: "TIMESTAMP '-infinity'", native: "string -infinity"},
		{name: "TIMESTAMP_S", expr: "TIMESTAMP_S '2026-01-02 03:04:05'", native: "time.Time 2026-01-02 03:04:05 +0000 UTC"},
		{name: "TIMESTAMP_S infinity", expr: "TIMESTAMP_S 'infinity'", native: "string infinity"},
		{name: "TIMESTAMP_S negative infinity", expr: "TIMESTAMP_S '-infinity'", native: "string -infinity"},
		{name: "TIMESTAMP_MS", expr: "TIMESTAMP_MS '2026-01-02 03:04:05.123'", native: "time.Time 2026-01-02 03:04:05.123 +0000 UTC"},
		{name: "TIMESTAMP_MS infinity", expr: "TIMESTAMP_MS 'infinity'", native: "string infinity"},
		{name: "TIMESTAMP_MS negative infinity", expr: "TIMESTAMP_MS '-infinity'", native: "string -infinity"},
		{name: "TIMESTAMP_NS", expr: "TIMESTAMP_NS '2026-01-02 03:04:05.123456789'", native: "time.Time 2026-01-02 03:04:05.123456789 +0000 UTC"},
		{name: "TIMESTAMP_NS infinity", expr: "TIMESTAMP_NS 'infinity'", native: "string infinity"},
		{name: "TIMESTAMP_NS negative infinity", expr: "TIMESTAMP_NS '-infinity'", native: "string -infinity"},
		{name: "TIMESTAMPTZ", expr: "TIMESTAMPTZ '2026-01-02 03:04:05.5+00'", native: "time.Time 2026-01-02 03:04:05.5 +0000 UTC"},
		{name: "TIMESTAMPTZ infinity", expr: "TIMESTAMPTZ 'infinity'", native: "string infinity"},
		{name: "TIMESTAMPTZ negative infinity", expr: "TIMESTAMPTZ '-infinity'", native: "string -infinity"},
		{name: "UUID", expr: "'eeccb8c5-9943-b2bb-bb5e-222f4e14b687'::UUID", native: "string eeccb8c5-9943-b2bb-bb5e-222f4e14b687"},
		{name: "BLOB with a zero byte", expr: `'\x00ab'::BLOB`, native: `string \x00ab`},
		{name: "BLOB with quotes and a backslash", expr: `'a\x5C''"b'::BLOB`, native: `string a\x5C\x27\x22b`},
		{name: "INTERVAL with every part", expr: "INTERVAL '1 year 2 months 3 days 04:05:06.5'", native: "string 1 year 2 months 3 days 04:05:06.5"},
		{name: "INTERVAL of minus one day", expr: "INTERVAL '-1 day'", native: "string -1 day"},
		{name: "INTERVAL of zero", expr: "INTERVAL '0 seconds'", native: "string 00:00:00"},
		{name: "INTERVAL of minus one hour", expr: "INTERVAL '-1 hours'", native: "string -01:00:00"},
		{name: "INTERVAL past 99 hours", expr: "INTERVAL '200 hours'", native: "string 200:00:00"},
		{name: "INTERVAL of minus fourteen months", expr: "INTERVAL '-14 months'", native: "string -1 year -2 months"},
		{name: "INTERVAL with mixed signs", expr: "INTERVAL '-1 year 2 days -00:00:00.000001'", native: "string -1 year 2 days -00:00:00.000001"},
		{name: "LIST of integers with a NULL", expr: "[1, 2, NULL]", native: "string [1, 2, NULL]"},
		{name: "LIST of plain strings", expr: "['a', 'b c']", native: "string [a, b c]"},
		{name: "empty LIST", expr: "[]::INTEGER[]", native: "string []"},
		{name: "ARRAY", expr: "array_value(1, 2)", native: "string [1, 2]"},
		{name: "ARRAY of strings prints them bare", expr: "array_value('', 'a b ', 'NULL', 'x''y')", native: "string [, a b , NULL, x'y]"},
		{name: "ARRAY of TIMESTAMPs is bare", expr: "array_value(TIMESTAMP '2026-01-01 01:02:03')", native: "string [2026-01-01 01:02:03]"},
		{name: "ARRAY in a LIST keeps its elements bare", expr: "[array_value('b,c')]", native: "string [[b,c]]"},
		{name: "ARRAY in a STRUCT keeps its elements bare", expr: "{'k': array_value('x,y')}", native: "string {'k': [x,y]}"},
		{name: "LIST in an ARRAY keeps its quoting", expr: "array_value(['a,b'])", native: "string [['a,b']]"},
		{name: "STRUCT in an ARRAY keeps its quoting", expr: "array_value({'k': 'x,y'})", native: "string [{'k': 'x,y'}]"},
		{name: "LIST of LISTs", expr: "[[1, 2], [3]]", native: "string [[1, 2], [3]]"},
		{name: "MAP", expr: "MAP {'a': 1, 'b': NULL}", native: "string {a=1, b=NULL}"},
		{name: "UNION", expr: "union_value(num := 2)", native: "string 2"},
		{name: "STRUCT in declared field order", expr: "{'z': 1, 'a': 'x', 'm': NULL}", native: "string {'z': 1, 'a': x, 'm': NULL}"},
		{name: "STRUCT with quoted and nested field names", expr: `{'we"ird': 1, 'a b': {'y': 2, 'x': 3}}`, native: `string {'we"ird': 1, 'a b': {'y': 2, 'x': 3}}`},
		{name: "STRUCT field name with a quote and a backslash", expr: `{'it''s\': 1}`, native: `string {'it\'s\\': 1}`},
		{name: "LIST of STRUCTs", expr: "[{'z': 1, 'a': 2}]", native: "string [{'z': 1, 'a': 2}]"},
		{name: "nested empty string is quoted", expr: "['']", native: "string ['']"},
		{name: "nested leading space is quoted", expr: "[' a']", native: "string [' a']"},
		{name: "nested trailing tab is quoted", expr: "['a' || chr(9)]", native: "string ['a\t']"},
		{name: "nested null word is quoted", expr: "['Null']", native: "string ['Null']"},
		{name: "nested special character is quoted", expr: "['a,b']", native: "string ['a,b']"},
		{name: "nested quote and backslash are escaped", expr: `['it''s\:']`, native: `string ['it\'s\\:']`},
		{name: "nested backslash alone is not quoted", expr: `['a\b']`, native: `string [a\b]`},
		{name: "nested MAP key and value are quoted", expr: "MAP {'a=b': 'c d '}", native: "string {'a=b'='c d '}"},
		{name: "nested STRUCT value is quoted", expr: "{'k': 'x,y'}", native: "string {'k': 'x,y'}"},
		{name: "nested TIMESTAMP is quoted", expr: "[TIMESTAMP '2026-01-01 01:02:03']", native: "string ['2026-01-01 01:02:03']"},
		{name: "nested DATE is not quoted", expr: "[DATE '2026-01-01']", native: "string [2026-01-01]"},
		{name: "nested UNION value is not quoted", expr: "[union_value(s := 'a,b')]", native: "string [a,b]"},
		{name: "NULL", expr: "CAST(NULL AS INTEGER)", native: "<nil> <nil>"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			table, err := db.QueryTable(t.Context(), fmt.Sprintf("SELECT %s AS v, CAST(%s AS VARCHAR) AS s", c.expr, c.expr), 0)

			require.NoError(t, err)
			require.Len(t, table.Rows, 1)
			assert.Equal(t, table.Rows[0][1].Text, table.Rows[0][0].Text)
			assert.Equal(t, c.native, fmt.Sprintf("%T %v", table.Rows[0][0].Native, table.Rows[0][0].Native))
		})
	}
}

func Test_query_table_prints_time_24_00_00_as_midnight(t *testing.T) {
	t.Parallel()
	db, _ := newOpenDatabase(t)

	table, err := db.QueryTable(t.Context(), "SELECT TIME '24:00:00'", 0)

	require.NoError(t, err)
	assert.Equal(t, "00:00:00", table.Rows[0][0].Text)
}

// Not parallel: it swaps the process-local zone, which DuckDB's TimeZone mirrors via SET GLOBAL.
// TZ cannot do it: Go and DuckDB each read the zone once per process.
func Test_query_table_prints_a_timestamptz_offset_in_whole_minutes(t *testing.T) {
	cases := []struct {
		name string
		zone string
		want string
	}{
		{name: "a negative offset of 30 seconds or more is truncated", zone: "America/Chicago", want: "1800-06-01 06:09:24-05:50"},
		{name: "a positive offset of 30 seconds or more is truncated", zone: "Asia/Tokyo", want: "1800-06-01 21:18:59+09:18"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, _ := newOpenDatabase(t)
			useZone(t, db, c.zone)

			table, err := db.QueryTable(t.Context(),
				"SELECT TIMESTAMPTZ '1800-06-01 12:00:00+00' AS v, CAST(TIMESTAMPTZ '1800-06-01 12:00:00+00' AS VARCHAR) AS s", 0)

			require.NoError(t, err)
			assert.Equal(t, []string{c.want, c.want}, []string{table.Rows[0][0].Text, table.Rows[0][1].Text})
		})
	}
}
