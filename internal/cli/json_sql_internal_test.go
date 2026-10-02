// White-box: renderSQLJSON is unexported; these pin the bytes it writes.
package cli

import (
	"testing"

	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
