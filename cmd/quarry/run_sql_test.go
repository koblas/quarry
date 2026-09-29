// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sql_prints_the_query_result_as_a_table(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	const query = `SELECT source_id AS id, name, balance,
		CASE WHEN source_id = 1 THEN 'line one' || chr(10) || 'tab' || chr(9) || 'cr' || chr(13) END AS note
		FROM v_account_balances ORDER BY source_id`
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", query}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, ""+
		"id  name            balance  note\n"+
		" 1  Chequing       12345.67  line one\\ntab\\tcr\\r\n"+
		" 2  US Chequing     8310.00  NULL\n"+
		" 3  Old Savings        0.00  NULL\n"+
		" 4  RRSP               NULL  NULL\n"+ //nolint:dupword // two adjacent NULL cells are the expected row
		" 5  Visa Infinite  -1204.17  NULL\n",
		stdout.String())
}

func Test_run_sql_prints_only_the_header_for_zero_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT name, balance FROM v_account_balances WHERE false"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "name  balance\n", stdout.String())
}

func Test_run_sql_prints_at_most_limit_rows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "--limit", "2", "SELECT source_id FROM accounts ORDER BY source_id"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "source_id\n        1\n        2\n", stdout.String())
	assert.Equal(t, "quarry: warning: showing the first 2 rows; the query returned more; pass --limit 0 to print every row\n", stderr.String())
}

func Test_run_sql_reads_the_query_from_stdin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	storePath := filepath.Join(storeDirUnder(home), "quarry.duckdb")
	before := fileSum(t, storePath)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "CREATE TABLE notes (body VARCHAR)"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sql", "SELECT 1"}, &stdout, &stderr)

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
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), []string{"sql", query}, &stdout, &stderr)
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
