package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"testing/iotest"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errStreamResetTwoLines = errors.New("stream reset\nby peer")
	errNoText              = errors.New("")
)

func executeSQL(t *testing.T, fake fakeReportStore, stdout io.Writer, args ...string) error {
	t.Helper()
	return executeSQLWithStderr(t, fake, stdout, io.Discard, args...)
}

func executeSQLWithStderr(t *testing.T, fake fakeReportStore, stdout, stderr io.Writer, args ...string) error {
	t.Helper()
	return cli.Execute(t.Context(), append([]string{"sql"}, args...), sqlEnv(fake, nil, stdout, stderr))
}

func executeSQLWithStdin(t *testing.T, fake fakeReportStore, stdin io.Reader, stdout io.Writer, args ...string) error {
	t.Helper()
	return cli.Execute(t.Context(), append([]string{"sql"}, args...), sqlEnv(fake, stdin, stdout, io.Discard))
}

func sqlEnv(fake fakeReportStore, stdin io.Reader, stdout, stderr io.Writer) cli.Env {
	return cli.Env{
		Stdin: stdin, Stdout: stdout, Stderr: stderr,
		NewReport: func(context.Context, string) (*report.Server, error) {
			return report.NewServer(report.WithStore(fake)), nil
		},
	}
}

func Test_sql_rejects_bad_usage(t *testing.T) {
	const (
		u5 = "sql needs a query; pass it as one quoted argument, or - to read it from stdin"
		u6 = "sql takes one query; quote it as one argument"
		u7 = "--limit must be 0 or more; 0 prints every row"
	)
	cases := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{name: "no query", args: nil, want: u5},
		{name: "a whitespace query", args: []string{" \t\n"}, want: u5},
		{name: "a whitespace query on stdin", args: []string{"-"}, stdin: " \n\t\n", want: u5},
		{name: "two queries", args: []string{"SELECT 1", "SELECT 2"}, want: u6},
		{name: "a limit just below zero", args: []string{"--limit", "-1", "SELECT 1"}, want: u7},
		{name: "no query before a negative limit", args: []string{"--limit", "-1"}, want: u5},
		{name: "two queries before a negative limit", args: []string{"--limit", "-1", "SELECT 1", "SELECT 2"}, want: u6},
		{name: "a negative limit before a blank query", args: []string{"--limit", "-1", " "}, want: u7},
		{name: "a negative limit before reading stdin", args: []string{"--limit", "-1", "-"}, stdin: "SELECT 1", want: u7},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotQuery string
			var stdout bytes.Buffer

			err := executeSQLWithStdin(t, fakeReportStore{gotQuery: &gotQuery}, strings.NewReader(c.stdin), &stdout, c.args...)

			var usage cli.UsageError
			require.ErrorAs(t, err, &usage)
			assert.Equal(t, c.want, usage.Error())
			assert.Empty(t, stdout.String())
			assert.Empty(t, gotQuery)
		})
	}
}

func Test_sql_reads_the_query_from_stdin_verbatim(t *testing.T) {
	const query = "  -- monthly\nSELECT 1;\n\n"
	var gotQuery string

	err := executeSQLWithStdin(t, fakeReportStore{gotQuery: &gotQuery}, strings.NewReader(query), io.Discard, "-")

	require.NoError(t, err)
	assert.Equal(t, query, gotQuery)
}

func Test_sql_passes_an_argument_query_verbatim(t *testing.T) {
	const query = " SELECT 1 \n"
	var gotQuery string

	err := executeSQL(t, fakeReportStore{gotQuery: &gotQuery}, io.Discard, query)

	require.NoError(t, err)
	assert.Equal(t, query, gotQuery)
}

func Test_sql_reports_a_stdin_read_fault(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "an OS fault on the file", err: &fs.PathError{Op: "read", Path: "/dev/stdin", Err: syscall.EIO}, want: "input/output error"},
		{name: "a fault from another reader, first line only", err: errStreamResetTwoLines, want: "stream reset"},
		{name: "a fault with no text", err: errNoText, want: "unknown error"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var gotQuery string
			var stdout bytes.Buffer

			err := executeSQLWithStdin(t, fakeReportStore{gotQuery: &gotQuery}, iotest.ErrReader(c.err), &stdout, "-")

			require.EqualError(t, err, "cannot read the query from stdin: "+c.want)
			require.ErrorIs(t, err, c.err)
			assert.NotErrorAs(t, err, new(cli.UsageError))
			assert.Empty(t, stdout.String())
			assert.Empty(t, gotQuery)
		})
	}
}

func Test_sql_reports_an_interrupt_while_reading_stdin(t *testing.T) {
	blocked, unblock := io.Pipe()
	t.Cleanup(func() { _ = unblock.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var gotQuery string
	var stdout bytes.Buffer

	err := cli.Execute(ctx, []string{"sql", "-"}, sqlEnv(fakeReportStore{gotQuery: &gotQuery}, blocked, &stdout, io.Discard))

	require.EqualError(t, err, "query interrupted")
	require.ErrorIs(t, err, store.ErrQueryInterrupted)
	assert.Empty(t, stdout.String())
	assert.Empty(t, gotQuery)
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

func Test_sql_help_describes_the_command_and_its_flags(t *testing.T) {
	var stdout bytes.Buffer

	err := executeSQL(t, fakeReportStore{}, &stdout, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `Run one SQL query against quarry's store and print the result. The store is
opened read-only: a query cannot change it, read or write other files, or
load extensions. A query too large for memory may spill to a temporary
directory beside the store; quarry removes it when it exits.

Pass the query as one quoted argument, or - to read it from stdin. A query
that starts with - (such as a -- comment) goes after --:

  quarry sql -- "-- monthly totals
  SELECT ..."

Amounts are DECIMAL(18,2) in each account's own currency; negative is money
leaving the account. v_cash_flow and v_spending also carry each amount in
CAD and in USD (amount_cad and amount_usd; spent_cad and spent_usd),
converted per split at the Bank of Canada rate for its date and rounded to
the cent, as quarry spend and quarry cashflow convert; they are NULL for a
date before the first rate. v_account_balances (v_balances_daily for
today) has balance_cad and balance_usd at today's rate. fx_rates holds
one rate per business day: usd_cad is the Canadian dollars in one US
dollar. For spending and income, query v_spending and v_cash_flow: they
already leave out transfers between your own accounts, Quicken's system
categories, transactions excluded from reports and accounts Quicken
leaves out of reports, so their totals match quarry spend and quarry
cashflow. A transfer leg is any split named in transfers.from_split_id
or transfers.to_split_id.

Each investment transaction that moves cash also has a row in transactions
(investment_transaction_id names it; NULL for a register entry), one split per
Quicken entry, so an account's cash is the sum of its transactions. In
v_cash_flow dividends, interest and capital-gain distributions are income;
buys, sells and share moves are neither. investment_transactions holds each
one's action, security and shares; its amount is DECIMAL(18,2) in the account's
own currency, negative when cash leaves the account; commission is
DECIMAL(18,4) in the account's own currency as Quicken recorded it (some
brokers charge fractions of a cent), NULL when there is none; shares is
DECIMAL(18,6) as Quicken recorded each transaction, negative when shares
leave. A split row carries split_new_shares and split_old_shares instead, so
a sum of shares is not a holding. prices holds each security's closing price
per day as Quicken recorded it, rounded to 6 decimals, in the security's
currency (securities.currency, NULL when Quicken records none).
holding_shares holds each account's count of each security, one row per span
of days it is unchanged and not zero (from_date through to_date, NULL while
still held), splits applied; these are the counts quarry sync checks against
Quicken. v_holdings has one row per holding per day held, through today:
price is the latest on or before date and price_date its day (NULL when
none), value is shares times price rounded to the cent, value_cad and
value_usd convert it at the rate for date, as quarry holdings does; filter
it by date. Neither includes cash in investment accounts. action is one of
add_shares, buy, capital_gain_long, capital_gain_short, dividend, interest,
margin_interest, misc_expense, misc_income, reinvest_dividend,
remove_shares, sell, split.

v_balances_daily has one row per account per day from its first transaction
or holding through today; cash is the sum of its transactions to that day,
holdings_value its holdings' value in its own currency (NULL outside
brokerage and retirement accounts), balance is cash plus holdings_value, as
quarry accounts and quarry networth use; filter by date. v_net_worth has one
row per day, account type and currency, adding up the balances of the
accounts Quicken's reports count, as quarry networth does; sum balance_cad
or balance_usd over one date for the total; a NULL there means no exchange
rate for that day.

findings holds what sync found to clean up in Quicken, and finding_items
the transactions, splits, payees or categories each one is about;
fixed_at is set once a finding is no longer found. Which findings you
ignored is set in the config file, not the store: quarry findings shows
each one's status.

List the tables and views with: quarry sql "SHOW TABLES"

At most --limit rows are printed (500 unless set, every row with --csv);
when there are more, quarry says so on stderr. --limit 0 prints every row.
With --csv, an empty field is NULL and "" is an empty string, except in a
one-column result, where NULL is also written as "" so no row is blank.`)
	assert.Contains(t, stdout.String(), "  quarry sql \"SELECT name, currency FROM accounts WHERE NOT closed\"\n")
	assert.Contains(t, stdout.String(), "  quarry sql --limit 0 --json - < monthly.sql\n")
	assert.Contains(t, stdout.String(), "  quarry sql --csv \"SELECT * FROM transactions\" > transactions.csv\n")
	assert.Contains(t, stdout.String(), "      --csv       print the rows as CSV, with a header line\n")
	assert.Contains(t, stdout.String(), "      --limit n   print at most n rows (500 unless set, every row with --csv; 0 prints every row)\n")
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
			err:  store.Interrupted(context.Canceled),
			want: "query interrupted",
		},
		{
			name: "a query holding no statement",
			err:  fmt.Errorf("%w: %w", store.ErrEmptyQuery, errStoreRead),
			want: "sql needs a query; pass it as one quoted argument, or - to read it from stdin",
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

func Test_sql_reports_a_failed_stdout_write(t *testing.T) {
	err := executeSQL(t, fakeReportStore{}, failingWriter{err: errNoSpace}, "SELECT 1")

	require.EqualError(t, err, "cannot write the result to stdout: write /dev/stdout: no space left on device")
	assert.ErrorIs(t, err, errNoSpace)
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
		name       string
		returned   int
		args       []string
		wantNote   []string
		wantStderr string
	}{
		{
			name: "more rows than the limit", returned: 3, args: []string{"--limit", "2", "SELECT 1"},
			wantNote: []string{"showing the first 2 rows" + tail}, wantStderr: "quarry: warning: showing the first 2 rows" + tail + "\n",
		},
		{name: "exactly the limit", returned: 2, args: []string{"--limit", "2", "SELECT 1"}, wantNote: []string{}},
		{name: "no limit", returned: 3, args: []string{"--limit", "0", "SELECT 1"}, wantNote: []string{}},
		{
			name: "a limit of one row", returned: 2, args: []string{"--limit", "1", "SELECT 1"},
			wantNote: []string{"showing the first 1 row" + tail}, wantStderr: "quarry: warning: showing the first 1 row" + tail + "\n",
		},
		{
			name: "the default limit", returned: 501, args: []string{"SELECT 1"},
			wantNote: []string{"showing the first 500 rows" + tail}, wantStderr: "quarry: warning: showing the first 500 rows" + tail + "\n",
		},
		{
			name: "a limit in the thousands", returned: 1001, args: []string{"--limit", "1000", "SELECT 1"},
			wantNote: []string{"showing the first 1,000 rows" + tail}, wantStderr: "quarry: warning: showing the first 1,000 rows" + tail + "\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name+", human", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(c.returned)}, &stdout, &stderr, c.args...)

			require.NoError(t, err)
			assert.Equal(t, c.wantStderr, stderr.String())
		})
		t.Run(c.name+", json", func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := executeSQLWithStderr(t, fakeReportStore{result: rowsOfOne(c.returned)}, &stdout, &stderr, append([]string{"--json"}, c.args...)...)

			require.NoError(t, err)
			var got struct {
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			assert.Equal(t, c.wantNote, got.Warnings)
			assert.Equal(t, c.wantStderr, stderr.String())
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
