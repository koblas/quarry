// White-box: jsonSQLCell's per-kind encoding is an unexported rule best driven
// directly over a store.QueryValue, one kind per case.
package cli

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodedCell renders a one-row, one-column document and returns the row's only cell as JSON text.
func encodedCell(t *testing.T, col store.QueryColumn, value store.QueryValue) string {
	t.Helper()
	result := report.QueryResult{
		Columns: []store.QueryColumn{col},
		Rows:    [][]store.QueryValue{{value}},
	}

	out, err := renderSQLJSON(result, 500, []string{})

	require.NoError(t, err)
	var doc struct {
		Rows [][]json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	return string(doc.Rows[0][0])
}

func Test_render_sql_json_encodes_each_native_kind(t *testing.T) {
	cases := []struct {
		name  string
		col   store.QueryColumn
		value store.QueryValue
		want  string
	}{
		{name: "NULL is null, not its text", col: store.QueryColumn{Type: "VARCHAR"}, value: store.QueryValue{Null: true, Text: "NULL"}, want: `null`},
		{name: "a boolean stays native", col: store.QueryColumn{Type: "BOOLEAN"}, value: store.QueryValue{Native: true, Text: "true"}, want: `true`},
		{name: "a signed integer stays native", col: store.QueryColumn{Type: "BIGINT"}, value: store.QueryValue{Native: int64(-5), Text: "-5"}, want: `-5`},
		{
			name: "the largest unsigned integer stays native", col: store.QueryColumn{Type: "UBIGINT"}, value: store.QueryValue{Native: uint64(math.MaxUint64), Text: "18446744073709551615"},
			want: `18446744073709551615`,
		},
		{name: "a DOUBLE stays native", col: store.QueryColumn{Type: "DOUBLE"}, value: store.QueryValue{Native: 1.5, Text: "1.5"}, want: `1.5`},
		{name: "a FLOAT keeps its short form", col: store.QueryColumn{Type: "FLOAT"}, value: store.QueryValue{Native: float32(0.1), Text: "0.1"}, want: `0.1`},
		{name: "negative zero keeps its sign", col: store.QueryColumn{Type: "DOUBLE"}, value: store.QueryValue{Native: math.Copysign(0, -1), Text: "-0"}, want: `-0`},
		{name: "DOUBLE NaN is nan", col: store.QueryColumn{Type: "DOUBLE"}, value: store.QueryValue{Native: math.NaN(), Text: "NaN"}, want: `"nan"`},
		{name: "DOUBLE +Inf is inf", col: store.QueryColumn{Type: "DOUBLE"}, value: store.QueryValue{Native: math.Inf(1), Text: "inf"}, want: `"inf"`},
		{name: "DOUBLE -Inf is -inf", col: store.QueryColumn{Type: "DOUBLE"}, value: store.QueryValue{Native: math.Inf(-1), Text: "-inf"}, want: `"-inf"`},
		{name: "FLOAT NaN is nan", col: store.QueryColumn{Type: "FLOAT"}, value: store.QueryValue{Native: float32(math.NaN()), Text: "NaN"}, want: `"nan"`},
		{name: "FLOAT +Inf is inf", col: store.QueryColumn{Type: "FLOAT"}, value: store.QueryValue{Native: float32(math.Inf(1)), Text: "inf"}, want: `"inf"`},
		{name: "FLOAT -Inf is -inf", col: store.QueryColumn{Type: "FLOAT"}, value: store.QueryValue{Native: float32(math.Inf(-1)), Text: "-inf"}, want: `"-inf"`},
		{
			name: "a DATE is its calendar day", col: store.QueryColumn{Type: "DATE"},
			value: store.QueryValue{Native: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Text: "2026-09-29"}, want: `"2026-09-29"`,
		},
		{
			name: "a TIMESTAMP is RFC 3339 with its nanoseconds", col: store.QueryColumn{Type: "TIMESTAMP_NS"},
			value: store.QueryValue{Native: time.Date(2026, 9, 29, 10, 30, 0, 123456789, time.UTC), Text: "2026-09-29 10:30:00.123456789"},
			want:  `"2026-09-29T10:30:00.123456789Z"`,
		},
		{
			name: "a TIMESTAMPTZ is converted to UTC", col: store.QueryColumn{Type: "TIMESTAMP WITH TIME ZONE"},
			value: store.QueryValue{Native: time.Date(2026, 9, 29, 10, 0, 0, 0, time.FixedZone("EDT", -4*60*60)), Text: "2026-09-29 10:00:00-04"},
			want:  `"2026-09-29T14:00:00Z"`,
		},
		{
			name: "a TIMESTAMP before year 1000 still encodes", col: store.QueryColumn{Type: "TIMESTAMP"},
			value: store.QueryValue{Native: time.Date(5, 1, 2, 3, 4, 5, 0, time.UTC), Text: "0005-01-02 03:04:05"}, want: `"0005-01-02T03:04:05Z"`,
		},
		{
			name: "a TIMESTAMP after year 9999 still encodes", col: store.QueryColumn{Type: "TIMESTAMP"},
			value: store.QueryValue{Native: time.Date(12345, 1, 2, 3, 4, 5, 0, time.UTC), Text: "12345-01-02 03:04:05"}, want: `"12345-01-02T03:04:05Z"`,
		},
		{name: "DECIMAL is its text", col: store.QueryColumn{Type: "DECIMAL(18,2)"}, value: store.QueryValue{Native: "12.50", Text: "12.50"}, want: `"12.50"`},
		{name: "an infinite date is its text", col: store.QueryColumn{Type: "DATE"}, value: store.QueryValue{Native: "infinity", Text: "infinity"}, want: `"infinity"`},
		{name: "a kind the encoder does not know is its text", col: store.QueryColumn{Type: "BLOB"}, value: store.QueryValue{Native: []byte{0x2a}, Text: `\x2A`}, want: `"\\x2A"`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, encodedCell(t, c.col, c.value))
		})
	}
}

func Test_render_sql_json_lists_columns_rows_and_the_cap(t *testing.T) {
	result := report.QueryResult{
		Columns: []store.QueryColumn{{Name: "name", Type: "VARCHAR"}, {Name: "balance", Type: "DECIMAL(18,2)"}},
		Rows: [][]store.QueryValue{
			{{Native: "Chequing", Text: "Chequing"}, {Native: "12.50", Text: "12.50"}},
			{{Native: "Visa", Text: "Visa"}, {Null: true, Text: "NULL"}},
		},
		Truncated: true,
	}

	got, err := renderSQLJSON(result, 2, []string{"showing the first 2 rows"})

	require.NoError(t, err)
	const want = `{
  "columns": [
    {
      "name": "name",
      "type": "VARCHAR"
    },
    {
      "name": "balance",
      "type": "DECIMAL(18,2)"
    }
  ],
  "rows": [
    [
      "Chequing",
      "12.50"
    ],
    [
      "Visa",
      null
    ]
  ],
  "row_count": 2,
  "limit": 2,
  "truncated": true,
  "warnings": [
    "showing the first 2 rows"
  ]
}
`
	assert.Equal(t, want, string(got)) //nolint:testifylint // the exact bytes, key order included, are the contract
}

func Test_render_sql_json_keeps_empty_lists_as_arrays(t *testing.T) {
	cases := []struct {
		name   string
		result store.QueryResult
		want   string
	}{
		{name: "no rows", result: store.QueryResult{Columns: []store.QueryColumn{{Name: "n", Type: "INTEGER"}}}, want: `"rows": [],`},
		{name: "no columns", result: store.QueryResult{}, want: `"columns": [],`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := renderSQLJSON(report.QueryResult{QueryResult: c.result}, 500, []string{})

			require.NoError(t, err)
			assert.Contains(t, string(got), c.want)
			assert.Contains(t, string(got), `"warnings": []`)
		})
	}
}
