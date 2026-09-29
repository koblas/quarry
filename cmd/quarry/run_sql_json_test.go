// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_json_returns_typed_values(t *testing.T) {
	const query = `SELECT 12.50::DECIMAL(18,2) AS amount,
		170141183460469231731687303715884105727::HUGEINT AS big,
		42::INTEGER AS count,
		1.5::DOUBLE AS ratio,
		true AS flag,
		DATE '2026-09-29' AS day,
		TIMESTAMP '2026-09-29 10:30:00' AS at,
		NULL::VARCHAR AS nothing`

	exitCode, stdout, stderr := runSQLArgsOnBuiltStore(t, "--json", query)

	require.Equal(t, 0, exitCode, stderr)
	assert.Empty(t, stderr)
	const want = `{
  "columns": [
    {
      "name": "amount",
      "type": "DECIMAL(18,2)"
    },
    {
      "name": "big",
      "type": "HUGEINT"
    },
    {
      "name": "count",
      "type": "INTEGER"
    },
    {
      "name": "ratio",
      "type": "DOUBLE"
    },
    {
      "name": "flag",
      "type": "BOOLEAN"
    },
    {
      "name": "day",
      "type": "DATE"
    },
    {
      "name": "at",
      "type": "TIMESTAMP"
    },
    {
      "name": "nothing",
      "type": "VARCHAR"
    }
  ],
  "rows": [
    [
      "12.50",
      "170141183460469231731687303715884105727",
      42,
      1.5,
      true,
      "2026-09-29",
      "2026-09-29T10:30:00Z",
      null
    ]
  ],
  "row_count": 1,
  "limit": 500,
  "truncated": false,
  "warnings": []
}
`
	assert.Equal(t, want, stdout) //nolint:testifylint // the exact bytes, key order included, are the contract
}

func Test_run_sql_json_caps_the_rows_and_says_so(t *testing.T) {
	const cutWarning = "showing the first 2 rows; the query returned more; pass --limit 0 to print every row"
	cases := []struct {
		name         string
		returned     int
		limit        int
		wantPrinted  int
		wantCut      bool
		wantWarnings []string
		wantStderr   string
	}{
		{name: "more rows than the limit", returned: 3, limit: 2, wantPrinted: 2, wantCut: true, wantWarnings: []string{cutWarning}, wantStderr: "quarry: warning: " + cutWarning + "\n"},
		{name: "exactly the limit", returned: 2, limit: 2, wantPrinted: 2, wantWarnings: []string{}},
		{name: "no limit", returned: 3, limit: 0, wantPrinted: 3, wantWarnings: []string{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			query := "SELECT range AS n FROM range(" + strconv.Itoa(c.returned) + ")"

			exitCode, stdout, stderr := runSQLArgsOnBuiltStore(t, "--json", "--limit", strconv.Itoa(c.limit), query)

			require.Equal(t, 0, exitCode, stderr)
			var doc struct {
				Rows      []json.RawMessage `json:"rows"`
				RowCount  int               `json:"row_count"`
				Limit     int               `json:"limit"`
				Truncated bool              `json:"truncated"`
				Warnings  []string          `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal([]byte(stdout), &doc))
			assert.Len(t, doc.Rows, c.wantPrinted)
			assert.Equal(t, c.wantPrinted, doc.RowCount)
			assert.Equal(t, c.limit, doc.Limit)
			assert.Equal(t, c.wantCut, doc.Truncated)
			assert.Equal(t, c.wantWarnings, doc.Warnings)
			assert.Equal(t, c.wantStderr, stderr)
		})
	}
}

// runSQLArgsOnBuiltStore syncs the accounts fixture under a fresh HOME, then runs sql with args against it.
func runSQLArgsOnBuiltStore(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), append([]string{"sql"}, args...), &stdout, &stderr)
	return exitCode, stdout.String(), stderr.String()
}
