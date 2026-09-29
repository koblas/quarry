package duckdb_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
