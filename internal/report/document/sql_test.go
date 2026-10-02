package document_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
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

	out, err := json.Marshal(document.NewSQL(result, 500, nil))

	require.NoError(t, err)
	var doc struct {
		Rows [][]json.RawMessage `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(out, &doc))
	return string(doc.Rows[0][0])
}

func Test_NewSQL_encodes_each_native_kind(t *testing.T) {
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

func Test_NewSQL_counts_rows_and_never_leaves_a_list_null(t *testing.T) {
	result := report.QueryResult{
		Columns: []store.QueryColumn{{Name: "n", Type: "INTEGER"}},
		Rows:    [][]store.QueryValue{{{Native: int64(1), Text: "1"}}, {{Native: int64(2), Text: "2"}}},
	}
	empty := report.QueryResult{}

	got := document.NewSQL(result, 7, nil)
	none := document.NewSQL(empty, 7, nil)

	assert.Equal(t, 2, got.RowCount)
	assert.Equal(t, 7, got.Limit)
	assert.Equal(t, []document.SQLColumn{{Name: "n", Type: "INTEGER"}}, got.Columns)
	assert.Equal(t, [][]any{{int64(1)}, {int64(2)}}, got.Rows)
	assert.Equal(t, []string{}, got.Warnings)
	assert.Equal(t, []document.SQLColumn{}, none.Columns)
	assert.Equal(t, [][]any{}, none.Rows)
}

func Test_NewSQL_reports_truncation_and_passes_warnings_through(t *testing.T) {
	result := report.QueryResult{Truncated: true}

	got := document.NewSQL(result, 2, []string{"showing the first 2 rows"})

	assert.True(t, got.Truncated)
	assert.Equal(t, []string{"showing the first 2 rows"}, got.Warnings)
}
