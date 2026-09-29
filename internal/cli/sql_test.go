package cli_test

import (
	"bytes"
	"context"
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
	env := cli.Env{
		Stdout: stdout, Stderr: io.Discard,
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

func Test_sql_help_describes_the_limit_flag(t *testing.T) {
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{}, &stdout, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Run one SQL query against quarry's store and print the result.")
	assert.Contains(t, stdout.String(), "  quarry sql --limit 0 --json - < monthly.sql\n")
	assert.Contains(t, stdout.String(), "      --limit n   print at most n rows (0 prints every row) (default 500)\n")
}

func Test_sql_returns_the_query_fault(t *testing.T) {
	var gotMaxRows int
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{err: errStoreRead, gotMaxRows: &gotMaxRows}, &stdout, "SELECT 1")

	require.ErrorIs(t, err, errStoreRead)
	assert.Empty(t, stdout.String())
}

func Test_sql_says_how_to_print_a_column_it_cannot_print(t *testing.T) {
	var gotMaxRows int
	unprintable := &store.UnprintableValueError{Column: "doc", Type: "JSON"}
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{err: unprintable, gotMaxRows: &gotMaxRows}, &stdout, "SELECT doc FROM t")

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
	var gotMaxRows int

	err := executeSQL(t, fakeReportStore{gotMaxRows: &gotMaxRows}, failingWriter{err: errNoSpace}, "SELECT 1")

	require.ErrorIs(t, err, errNoSpace)
}
