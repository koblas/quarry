package cli_test

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// csvTransactions is n rows of id, payee, memo and the CSV body they print as.
// Row 2 has a comma and quotes in its payee and a NULL memo; row 3 an empty-string memo.
func csvTransactions(n int) ([][]store.QueryValue, string) {
	rows := make([][]store.QueryValue, 0, n)
	var body strings.Builder
	for id := 1; id <= n; id++ {
		payee, memo := "Payee", "note"
		payeeCSV, memoCSV := payee, memo
		memoNull := false
		switch id {
		case 2:
			payee, payeeCSV = `Smith, "Jo"`, `"Smith, ""Jo"""`
			memo, memoCSV, memoNull = "NULL", "", true
		case 3:
			memo, memoCSV = "", `""`
		}
		rows = append(rows, []store.QueryValue{{Text: strconv.Itoa(id)}, {Text: payee}, {Text: memo, Null: memoNull}})
		_, _ = fmt.Fprintf(&body, "%d,%s,%s\n", id, payeeCSV, memoCSV)
	}
	return rows, body.String()
}

func Test_sql_csv_prints_every_row_with_a_header(t *testing.T) {
	rows, wantBody := csvTransactions(600)
	result := store.QueryResult{
		Columns: []store.QueryColumn{{Name: "id", Type: "INTEGER"}, {Name: "payee", Type: "VARCHAR"}, {Name: "memo", Type: "VARCHAR"}},
		Rows:    rows,
	}
	gotMaxRows := -1
	var stdout, stderr bytes.Buffer

	err := executeSQLWithStderr(t, fakeReportStore{result: result, gotMaxRows: &gotMaxRows}, &stdout, &stderr,
		"--csv", "SELECT id, payee, memo FROM transactions")

	require.NoError(t, err)
	assert.Equal(t, "id,payee,memo\n"+wantBody, stdout.String())
	assert.Equal(t, 0, gotMaxRows)
	assert.Empty(t, stderr.String())
}

func Test_sql_csv_tells_null_from_the_empty_string_and_the_word_null(t *testing.T) {
	result := store.QueryResult{
		Columns: []store.QueryColumn{{Name: "a", Type: "VARCHAR"}, {Name: "b", Type: "VARCHAR"}, {Name: "c", Type: "VARCHAR"}},
		Rows:    [][]store.QueryValue{{{Text: "NULL", Null: true}, {Text: ""}, {Text: "NULL"}}},
	}
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{result: result}, &stdout, "--csv", "SELECT a, b, c FROM t")

	require.NoError(t, err)
	assert.Equal(t, "a,b,c\n,\"\",NULL\n", stdout.String())
}

func Test_sql_csv_prints_the_header_alone_when_the_query_returns_no_rows(t *testing.T) {
	result := store.QueryResult{Columns: []store.QueryColumn{{Name: "id", Type: "INTEGER"}, {Name: "payee", Type: "VARCHAR"}}}
	var stdout, stderr bytes.Buffer

	err := executeSQLWithStderr(t, fakeReportStore{result: result}, &stdout, &stderr, "--csv", "SELECT id, payee FROM t WHERE false")

	require.NoError(t, err)
	assert.Equal(t, "id,payee\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func Test_sql_csv_honours_an_explicit_limit_equal_to_the_default(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "with --csv", args: []string{"--csv", "--limit", "500", "SELECT 1"}},
		{name: "without --csv", args: []string{"--limit", "500", "SELECT 1"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotMaxRows := -1

			err := executeSQL(t, fakeReportStore{gotMaxRows: &gotMaxRows}, io.Discard, c.args...)

			require.NoError(t, err)
			assert.Equal(t, 501, gotMaxRows)
		})
	}
}

func Test_sql_csv_prints_every_row_for_limit_0(t *testing.T) {
	gotMaxRows := -1

	err := executeSQL(t, fakeReportStore{gotMaxRows: &gotMaxRows}, io.Discard, "--csv", "--limit", "0", "SELECT 1")

	require.NoError(t, err)
	assert.Equal(t, 0, gotMaxRows)
}

func Test_sql_csv_cuts_the_rows_at_an_explicit_limit_and_says_so(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(3)}, &stdout, &stderr, "--csv", "--limit", "2", "SELECT 1")

	require.NoError(t, err)
	assert.Equal(t, "n\n1\n1\n", stdout.String())
	assert.Equal(t, "quarry: warning: showing the first 2 rows; the query returned more; pass --limit 0 to print every row\n", stderr.String())
}

func Test_sql_csv_refuses_to_combine_with_json(t *testing.T) {
	const want = "--csv and --json cannot be used together; choose one output format"
	cases := []struct {
		name string
		args []string
	}{
		{name: "csv first", args: []string{"--csv", "--json", "-"}},
		{name: "json first", args: []string{"--json", "--csv", "-"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotQuery string
			stdin := strings.NewReader("SELECT 1")
			var stdout bytes.Buffer

			err := executeSQLWithStdin(t, fakeReportStore{gotQuery: &gotQuery}, stdin, &stdout, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, want, usage.Error())
			assert.Empty(t, stdout.String())
			assert.Empty(t, gotQuery)
			assert.Equal(t, len("SELECT 1"), stdin.Len())
		})
	}
}

func Test_sql_csv_with_json_loses_to_the_other_usage_errors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "no query", args: []string{"--csv", "--json"}, want: "sql needs a query; pass it as one quoted argument, or - to read it from stdin"},
		{name: "a negative limit", args: []string{"--csv", "--json", "--limit", "-1", "SELECT 1"}, want: "--limit must be 0 or more; 0 prints every row"},
		{name: "a blank query", args: []string{"--csv", "--json", "  "}, want: "sql needs a query; pass it as one quoted argument, or - to read it from stdin"},
		{name: "two queries", args: []string{"--csv", "--json", "SELECT 1", "SELECT 2"}, want: "sql takes one query; quote it as one argument"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := executeSQL(t, fakeReportStore{}, io.Discard, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, usage.Error())
		})
	}
}

func Test_sql_csv_writes_no_warning_when_stdout_fails(t *testing.T) {
	var stderr bytes.Buffer

	err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(3)}, failingWriter{err: errNoSpace}, &stderr, "--csv", "--limit", "2", "SELECT 1")

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.Empty(t, stderr.String())
}

func Test_sql_csv_prints_nothing_when_the_query_fails(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "a store fault", err: errStoreRead, want: errStoreRead.Error()},
		{
			name: "a column it cannot print",
			err:  &store.UnprintableValueError{Column: "doc", Type: "JSON"},
			want: `cannot print column "doc" of type JSON; cast it in the query, e.g. CAST(doc AS VARCHAR)`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{err: c.err}, &stdout, &stderr, "--csv", "SELECT 1")

			require.EqualError(t, err, c.want)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func Test_sql_csv_of_a_one_column_result_keeps_every_row_when_read_back_as_csv(t *testing.T) {
	result := store.QueryResult{
		Columns: []store.QueryColumn{{Name: "memo", Type: "VARCHAR"}},
		Rows:    [][]store.QueryValue{{{Text: "NULL", Null: true}}, {{Text: "a"}}, {{Text: "NULL", Null: true}}},
	}
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{result: result}, &stdout, "--csv", "SELECT memo FROM t")

	require.NoError(t, err)
	records, readErr := csv.NewReader(&stdout).ReadAll()
	require.NoError(t, readErr)
	assert.Equal(t, [][]string{{"memo"}, {""}, {"a"}, {""}}, records)
}
