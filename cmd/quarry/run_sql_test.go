// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_prints_the_query_result_as_a_table(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	const query = `SELECT source_id AS id, name, holdings_value,
		CASE WHEN source_id = 1 THEN 'line one' || chr(10) || 'tab' || chr(9) || 'cr' || chr(13) END AS note
		FROM v_account_balances ORDER BY source_id`

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"id  name           holdings_value  note\n"+
		" 1  Chequing                 NULL  line one\\ntab\\tcr\\r\n"+
		" 2  US Chequing              NULL  NULL\n"+ //nolint:dupword // adjacent NULL cells are the expected row
		" 3  Old Savings              NULL  NULL\n"+ //nolint:dupword // adjacent NULL cells are the expected row
		" 4  RRSP                     0.00  NULL\n"+
		" 5  Visa Infinite            NULL  NULL\n", //nolint:dupword // adjacent NULL cells are the expected row
		stdout.String())
}

func Test_run_sql_prints_only_the_header_for_zero_rows(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "SELECT name, balance FROM v_account_balances WHERE false"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "name  balance\n", stdout.String())
}

func Test_run_sql_prints_at_most_limit_rows(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--limit", "2", "SELECT source_id FROM accounts ORDER BY source_id"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "source_id\n        1\n        2\n", stdout.String())
	assert.Equal(t, "quarry: warning: showing the first 2 rows; the query returned more; pass --limit 0 to print every row\n", stderr.String())
}

func Test_run_sql_reads_the_query_from_stdin(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer
	env := testEnv(&stdout, &stderr)
	env.Stdin = strings.NewReader("SELECT name\nFROM accounts\nWHERE source_id = 1;\n")

	exitCode := runWith(context.Background(), []string{"sql", "-"}, env)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "name\nChequing\n", stdout.String())
}

func Test_run_sql_reports_a_bad_query(t *testing.T) {
	exitCode, stdout, stderr := runSQLOnBuiltStore(t, "SELECT missing_column FROM accounts")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, "quarry: query failed: Binder Error: Referenced column \"missing_column\" not found in FROM clause!\n", stderr)
}

func Test_run_sql_refuses_to_change_the_store(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	storePath := filepath.Join(storeDirUnder(home), "quarry.duckdb")
	before := fileSum(t, storePath)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "CREATE TABLE notes (body VARCHAR)"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, sqlReadsOnlyTheStore, stderr.String())
	assert.Equal(t, before, fileSum(t, storePath))
}

func Test_run_sql_refuses_to_write_another_file(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out.csv")

	exitCode, stdout, stderr := runSQLOnBuiltStore(t, "COPY (SELECT 1) TO '"+target+"'")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, sqlReadsOnlyItsStore, stderr)
	assert.NoFileExists(t, target)
}

func Test_run_sql_refuses_to_read_another_file(t *testing.T) {
	source := filepath.Join(t.TempDir(), "in.csv")
	require.NoError(t, os.WriteFile(source, []byte("n\n1\n"), 0o600))

	exitCode, stdout, stderr := runSQLOnBuiltStore(t, "SELECT n FROM read_csv('"+source+"')")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, sqlReadsOnlyItsStore, stderr)
}

func Test_run_sql_refuses_to_attach_another_database(t *testing.T) {
	target := filepath.Join(t.TempDir(), "other.duckdb")

	exitCode, stdout, stderr := runSQLOnBuiltStore(t, "ATTACH '"+target+"' AS other")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, sqlReadsOnlyItsStore, stderr)
	assert.NoFileExists(t, target)
}

func Test_run_sql_refuses_to_install_an_extension(t *testing.T) {
	exitCode, stdout, stderr := runSQLOnBuiltStore(t, "INSTALL httpfs")

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout)
	assert.Equal(t, sqlReadsOnlyItsStore, stderr)
}

// threads=1 is a change DuckDB allows on an unlocked read-only connection; only the lock refuses it.
func Test_run_sql_refuses_to_change_a_setting(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		setting string
	}{
		{name: "one that opens external access", query: "SET enable_external_access=true", setting: "enable_external_access"},
		{name: "one that is otherwise harmless", query: "SET threads=1", setting: "threads"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exitCode, stdout, stderr := runSQLOnBuiltStore(t, c.query)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout)
			assert.Equal(t, "quarry: query failed: Invalid Input Error: Cannot change configuration option \""+c.setting+
				"\" - the configuration has been locked\n", stderr)
		})
	}
}

// Not parallel: it signals the whole test process. It proves the SIGINT wiring;
// the sleep does not prove the query had started (duckstore's tests cover that).
func Test_run_sql_reports_a_query_interrupted_by_sigint(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	ctx, stop := signalContext(context.Background())
	defer stop()
	var stdout, stderr bytes.Buffer
	exited := make(chan int, 1)
	go func() {
		exited <- run(ctx, []string{"sql", "SELECT count(*) FROM range(1000000000000)"}, &stdout, &stderr)
	}()
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))

	exitCode := exitCodeWithin(t, exited, 30*time.Second)
	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: query interrupted\n", stderr.String())
}

func Test_run_sql_refuses_when_home_is_unset(t *testing.T) {
	t.Setenv("HOME", "")

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "SELECT 1"})

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry sql again\n",
		stderr.String())
}

const (
	sqlReadsOnlyTheStore = "quarry: quarry sql only reads the store; change the data in Quicken and run quarry sync\n"
	sqlReadsOnlyItsStore = "quarry: quarry sql reads only quarry's store; other files, databases and extensions are turned off\n"
)

// runSQLOnBuiltStore syncs the accounts fixture under a fresh HOME, then runs sql query against it.
func runSQLOnBuiltStore(t *testing.T, query string) (int, string, string) {
	t.Helper()
	home := newHome(t)
	syncAccountsFixture(t, home)
	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", query})
	return exitCode, stdout.String(), stderr.String()
}

func fileSum(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return sha256.Sum256(raw)
}

// exitCodeWithin waits for run's exit code on exited, failing t if none arrives within timeout.
func exitCodeWithin(t *testing.T, exited <-chan int, timeout time.Duration) int {
	t.Helper()
	select {
	case code := <-exited:
		return code
	case <-time.After(timeout):
		t.Fatalf("run did not return within %s", timeout)
	}
	return 0
}

func Test_run_sql_csv_prints_null_as_an_empty_field_and_an_empty_string_as_a_quoted_pair(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	const query = `SELECT source_id, holdings_value,
		CASE WHEN source_id = 1 THEN '' WHEN source_id = 2 THEN 'US, "x"' END AS note
		FROM v_account_balances ORDER BY source_id`

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"source_id,holdings_value,note\n"+
		"1,,\"\"\n"+
		"2,,\"US, \"\"x\"\"\"\n"+
		"3,,\n"+
		"4,0.00,\n"+
		"5,,\n",
		stdout.String())
}

func Test_run_sql_csv_writes_a_row_of_one_null_column_as_a_quoted_pair_a_csv_reader_keeps(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", "SELECT holdings_value FROM v_account_balances WHERE source_id = 1"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "holdings_value\n\"\"\n", stdout.String())
	records, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"holdings_value"}, {""}}, records)
}

func Test_run_sql_csv_with_json_exits_2_naming_the_two_flags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", "--json", "SELECT 1"})

	assert.Equal(t, 2, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: --csv and --json cannot be used together; choose one output format\n", stderr.String())
}

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
	home := newHome(t)
	syncAccountsFixture(t, home)
	exitCode, stdout, stderr := runCapture(context.Background(), append([]string{"sql"}, args...))
	return exitCode, stdout.String(), stderr.String()
}

// fakeRates is a duckstore.RatesSource that returns its rates to every request, except those the request already has.
type fakeRates struct{ rates []store.Rate }

func (f fakeRates) Refresh(_ context.Context, req store.RatesRequest) (store.RatesRefresh, error) {
	var fresh []store.Rate
	for _, r := range f.rates {
		if r.Date.Before(req.Have.First) || r.Date.After(req.Have.Last) {
			fresh = append(fresh, r)
		}
	}
	return store.RatesRefresh{Rates: fresh, Added: len(fresh)}, nil
}

// replaceStoreWithRates is replaceStore with rates fetched into the store, skipping Quicken and the network.
func replaceStoreWithRates(t *testing.T, home string, rows store.Rows, rates ...store.Rate) {
	t.Helper()
	_, err := duckstore.New(storeDirUnder(home), duckstore.WithRates(fakeRates{rates: rates})).Replace(context.Background(), rows)
	require.NoError(t, err)
}

func Test_run_sql_views_carry_each_amount_converted_at_its_dates_rate(t *testing.T) {
	home := newHome(t)
	day := func(month time.Month, d int) time.Time { return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC) }
	replaceStoreWithRates(t, home,
		spendRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			spendSplit{id: "c0", account: "acct-cad", currency: "CAD", day: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), cents: -500},
			spendSplit{id: "u0", account: "acct-usd", currency: "USD", day: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), cents: -700},
			spendSplit{id: "u1", account: "acct-usd", currency: "USD", day: day(time.January, 2), cents: 10},
			spendSplit{id: "u2", account: "acct-usd", currency: "USD", day: day(time.January, 3), cents: -10},
			spendSplit{id: "c1", account: "acct-cad", currency: "CAD", day: day(time.January, 5), cents: -20},
			spendSplit{id: "c2", account: "acct-cad", currency: "CAD", day: day(time.January, 6), cents: 20},
		),
		store.Rate{Date: day(time.January, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(time.January, 5), USDCAD: money.Rate(1_600_000), Series: "FXUSDCAD"},
	)
	const query = `SELECT source, id, native, in_cad, in_usd, cad_type, usd_type FROM (
		SELECT 'balance' AS source, id, balance AS native, balance_cad AS in_cad, balance_usd AS in_usd,
			typeof(balance_cad) AS cad_type, typeof(balance_usd) AS usd_type FROM v_account_balances
		UNION ALL
		SELECT 'cash_flow', split_id, amount, amount_cad, amount_usd, typeof(amount_cad), typeof(amount_usd) FROM v_cash_flow
		UNION ALL
		SELECT 'spending', split_id, spent, spent_cad, spent_usd, typeof(spent_cad), typeof(spent_usd) FROM v_spending
	) ORDER BY source, id`

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "--csv", query})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"source,id,native,in_cad,in_usd,cad_type,usd_type\n"+
		"balance,acct-cad,-5.00,-5.00,-3.13,\"DECIMAL(38,2)\",\"DECIMAL(38,2)\"\n"+
		"balance,acct-usd,-7.00,-11.20,-7.00,\"DECIMAL(38,2)\",\"DECIMAL(38,2)\"\n"+
		"cash_flow,split-c0,-5.00,-5.00,,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-c1,-0.20,-0.20,-0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-c2,0.20,0.20,0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u0,-7.00,,-7.00,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u1,0.10,0.13,0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"cash_flow,split-u2,-0.10,-0.13,-0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-c0,5.00,5.00,,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-c1,0.20,0.20,0.13,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-u0,7.00,,7.00,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n"+
		"spending,split-u2,0.10,0.13,0.10,\"DECIMAL(18,2)\",\"DECIMAL(18,2)\"\n",
		stdout.String())
}

// viewRows is a store with one split of every kind the report views keep or
// leave out. s01-s02 and s08-s12 are the ones kept. It is not spendRows: the
// tests read split ids and bare category paths off the view, and it needs
// transfers, excluded transactions and system/income categories.
func viewRows() store.Rows {
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	txn := func(id string, amount int64) store.Transaction {
		return store.Transaction{
			ID: "txn-" + id, SourceID: 1, AccountID: "acct-1", Date: day,
			Amount: amount, Currency: "CAD", Status: "uncleared",
		}
	}
	split := func(id string, category *string, amount int64) store.Split {
		return store.Split{ID: "s" + id, SourceID: 1, TransactionID: "txn-s" + id, CategoryID: category, Amount: amount}
	}
	excluded := txn("s07", -700)
	excluded.ExcludedFromReports = true
	transferLeg := split("03", nil, -5000)
	transferLeg.TransferAccountID = new("acct-1")
	transferIn := split("04", nil, 5000)
	transferIn.TransferAccountID = new("acct-1")

	return store.Rows{
		Accounts: []store.Account{chequingAccount("acct-1", 1)},
		Categories: []store.Category{
			{ID: "cat-groceries", SourceID: 1, Name: "Groceries", FullPath: "Groceries", Kind: "expense"},
			{ID: "cat-salary", SourceID: 2, Name: "Salary", FullPath: "Salary", Kind: "income"},
			{ID: "cat-adjustment", SourceID: 3, Name: "Adjustment", FullPath: "Adjustment", Kind: "system"},
			{ID: "cat-returns", SourceID: 4, Name: "Returns", FullPath: "Returns", Kind: "expense"},
		},
		Transactions: []store.Transaction{
			txn("s01", -10000), txn("s02", 250000), txn("s03", -5000), txn("s04", 5000), txn("s05", -2000),
			txn("s06", -500), excluded, txn("s08", -300), txn("s09", 400), txn("s10", 3000),
			txn("s11", -1000), txn("s12", 2500),
		},
		Splits: []store.Split{
			split("01", new("cat-groceries"), -10000),
			split("02", new("cat-salary"), 250000),
			transferLeg,
			transferIn,
			split("05", nil, -2000),
			split("06", new("cat-adjustment"), -500),
			split("07", new("cat-groceries"), -700),
			split("08", nil, -300),
			split("09", nil, 400),
			split("10", new("cat-groceries"), 3000),
			split("11", new("cat-returns"), -1000),
			split("12", new("cat-returns"), 2500),
		},
		Transfers: []store.Transfer{
			{ID: "xfer-1", FromSplitID: "s03", ToSplitID: new("s04")},
			{ID: "xfer-2", FromSplitID: "s05"},
		},
		ImportRuns: []store.ImportRun{{
			ID: 1, StartedAt: day, FinishedAt: day,
			Snapshot: store.SnapshotRef{Path: "/snapshots/20260315T000000Z.sqlite", SHA256: "9f86", SchemaFingerprint: "sha256:abc", TakenAt: day},
		}},
	}
}

func Test_run_sql_cash_flow_keeps_only_real_income_and_spending(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, viewRows())

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "SELECT split_id, flow, amount FROM v_cash_flow ORDER BY split_id"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"split_id  flow      amount\n"+
		"s01       expense  -100.00\n"+
		"s02       income   2500.00\n"+
		"s08       expense    -3.00\n"+
		"s09       income      4.00\n"+
		"s10       expense    30.00\n"+
		"s11       expense   -10.00\n"+
		"s12       expense    25.00\n",
		stdout.String())
}

func Test_run_sql_spending_nets_refunds_against_their_category(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, viewRows())

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"sql", "SELECT category, sum(spent) AS spent FROM v_spending GROUP BY category ORDER BY category"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"category    spent\n"+
		"Groceries   70.00\n"+
		"Returns    -15.00\n"+
		"NULL         3.00\n",
		stdout.String())
}
