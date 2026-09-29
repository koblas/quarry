package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeSQL(t *testing.T, fake fakeReportStore, stdout io.Writer, args ...string) error {
	t.Helper()
	return executeSQLWithStderr(t, fake, stdout, io.Discard, args...)
}

func executeSQLWithStderr(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	env := cli.Env{
		Stdout: stdout, Stderr: stderr,
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
	return cli.Execute(t.Context(), append([]string{"sql"}, args...), env)
}

func Test_sql_prints_the_result_as_a_table(t *testing.T) {
	var gotMaxRows int
	result := store.QueryResult{
		Columns: []store.QueryColumn{{Name: "name", Type: "VARCHAR"}, {Name: "n", Type: "INTEGER"}},
		Rows:    [][]store.QueryValue{{{Text: "Chequing"}, {Text: "7"}}},
	}
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{result: result, gotMaxRows: &gotMaxRows}, &stdout, "SELECT name, n FROM t")

	require.NoError(t, err)
	assert.Equal(t, "name      n\nChequing  7\n", stdout.String())
}

func Test_sql_passes_the_limit_to_the_query(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantMaxRows int
	}{
		{name: "500 rows unless set", args: []string{"SELECT 1"}, wantMaxRows: 501},
		{name: "the limit given", args: []string{"--limit", "7", "SELECT 1"}, wantMaxRows: 8},
		{name: "every row for limit 0", args: []string{"--limit", "0", "SELECT 1"}, wantMaxRows: 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotMaxRows int

			err := executeSQL(t, fakeReportStore{gotMaxRows: &gotMaxRows}, io.Discard, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.wantMaxRows, gotMaxRows)
		})
	}
}

func Test_sql_help_describes_the_command_and_the_limit_flag(t *testing.T) {
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{}, &stdout, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions.

Pass the query as one quoted argument, or - to read it from stdin. Amounts
are DECIMAL(18,2) in each account's own currency; negative is money leaving
the account. Transfers between your own accounts are in the transfers table
and splits.transfer_account_id, never in a category kind. List the tables
and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set); when there are more,
quarry says so on stderr. --limit 0 prints every row.`)
	assert.Contains(t, stdout.String(), "  quarry sql \"SELECT name, currency FROM accounts WHERE NOT closed\"\n")
	assert.Contains(t, stdout.String(), "  quarry sql --limit 0 --json - < monthly.sql\n")
	assert.Contains(t, stdout.String(), "      --limit n   print at most n rows (0 prints every row) (default 500)\n")
}

func Test_sql_returns_the_query_fault(t *testing.T) {
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{err: errStoreRead}, &stdout, "SELECT 1")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_sql_reports_each_query_refusal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "a write",
			err:  fmt.Errorf("%w: %w", store.ErrReadOnlyQuery, errStoreRead),
			want: "quarry sql only reads the store; change the data in Quicken and run quarry sync",
		},
		{
			name: "another file, database or extension",
			err:  fmt.Errorf("%w: %w", store.ErrExternalAccess, errStoreRead),
			want: "quarry sql reads only quarry's store; other files, databases and extensions are turned off",
		},
		{
			name: "any other query error",
			err:  &store.QueryError{Reason: "Binder Error: Referenced column \"x\" not found in FROM clause!"},
			want: "query failed: Binder Error: Referenced column \"x\" not found in FROM clause!",
		},
		{
			name: "an interrupted query",
			err:  fmt.Errorf("%w: %w", store.ErrQueryInterrupted, context.Canceled),
			want: "query interrupted",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := executeSQL(t, fakeReportStore{err: c.err}, io.Discard, "SELECT 1")

			require.EqualError(t, err, c.want)
		})
	}
}

func Test_sql_says_how_to_print_a_column_it_cannot_print(t *testing.T) {
	unprintable := &store.UnprintableValueError{Column: "doc", Type: "JSON"}
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{err: unprintable}, &stdout, "SELECT doc FROM t")

	require.EqualError(t, err, `cannot print column "doc" of type JSON; cast it in the query, e.g. CAST(doc AS VARCHAR)`)
	assert.Empty(t, stdout.String())
}

func Test_sql_returns_the_report_factory_fault(t *testing.T) {
	var stdout bytes.Buffer
	env := cli.Env{
		Stdout: &stdout, Stderr: io.Discard,
		NewReport: func(context.Context, string) (*report.Server, error) { return nil, errStoreRead },
	}

	err := cli.Execute(t.Context(), []string{"sql", "SELECT 1"}, env)

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_sql_returns_the_stdout_write_fault(t *testing.T) {
	err := executeSQL(t, fakeReportStore{}, failingWriter{err: errNoSpace}, "SELECT 1")

	require.ErrorIs(t, err, errNoSpace)
}

// rowsOfOne is n canned single-column rows.
func rowsOfOne(n int) store.QueryResult {
	result := store.QueryResult{Columns: []store.QueryColumn{{Name: "n", Type: "INTEGER"}}}
	for range n {
		result.Rows = append(result.Rows, []store.QueryValue{{Text: "1", Native: int64(1)}})
	}
	return result
}

func Test_sql_says_when_it_cuts_the_rows(t *testing.T) {
	const tail = "; the query returned more; pass --limit 0 to print every row"
	cases := []struct {
		name     string
		returned int
		args     []string
		wantNote []string
	}{
		{name: "more rows than the limit", returned: 3, args: []string{"--limit", "2"}, wantNote: []string{"showing the first 2 rows" + tail}},
		{name: "exactly the limit", returned: 2, args: []string{"--limit", "2"}, wantNote: []string{}},
		{name: "no limit", returned: 3, args: []string{"--limit", "0"}, wantNote: []string{}},
		{name: "a limit of one row", returned: 2, args: []string{"--limit", "1"}, wantNote: []string{"showing the first 1 row" + tail}},
		{name: "the default limit", returned: 501, args: nil, wantNote: []string{"showing the first 500 rows" + tail}},
		{name: "a limit in the thousands", returned: 1001, args: []string{"--limit", "1000"}, wantNote: []string{"showing the first 1,000 rows" + tail}},
	}

	for _, c := range cases {
		args := append(append([]string{}, c.args...), "SELECT 1")
		var wantStderr bytes.Buffer
		for _, note := range c.wantNote {
			wantStderr.WriteString("quarry: warning: " + note + "\n")
		}
		t.Run(c.name+", human", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(c.returned)}, &stdout, &stderr, args...)

			require.NoError(t, err)
			assert.Equal(t, wantStderr.String(), stderr.String())
		})
		t.Run(c.name+", json", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(c.returned)}, &stdout, &stderr, append([]string{"--json"}, args...)...)

			require.NoError(t, err)
			var got struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			assert.Equal(t, c.wantNote, got.Warnings)
			assert.Equal(t, wantStderr.String(), stderr.String())
		})
	}
}

func Test_sql_writes_no_warning_when_stdout_fails(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "human", args: []string{"--limit", "2", "SELECT 1"}},
		{name: "json", args: []string{"--json", "--limit", "2", "SELECT 1"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(3)}, failingWriter{err: errNoSpace}, &stderr, c.args...)

			require.ErrorIs(t, err, errNoSpace)
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_sql_json_writes_nothing_for_a_query_fault(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSQLWithStderr(t, fakeReportStore{err: errStoreRead}, &stdout, &stderr, "--json", "SELECT 1")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}
