// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// phaseOneImportRunsDDL is a store as Phase 1 left it: an import run naming its snapshot, no store_info.
const phaseOneImportRunsDDL = `CREATE TABLE import_runs (id BIGINT, snapshot_path VARCHAR);
INSERT INTO import_runs VALUES (1, '/Users/dave/Library/Application Support/quarry/snapshots/20260927T143005Z.sqlite');`

// writeStoreFixture builds a DuckDB file at the store path under home from ddl, closed before any read.
func writeStoreFixture(t *testing.T, home, ddl string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	db, err := duckdb.Create(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), ddl)
	require.NoError(t, err)
	require.NoError(t, db.CheckpointClose(t.Context()))
}

// removingOpener opens the store read-only only after deleting it, as a concurrent removal would.
func removingOpener(t *testing.T) duckstore.Option {
	t.Helper()
	return duckstore.WithOpenReadOnly(func(ctx context.Context, path string) (duckstore.ReadDB, error) {
		require.NoError(t, os.Remove(path))
		db, err := duckdb.OpenReadOnly(ctx, path)
		if err != nil {
			return nil, err
		}
		return db, nil
	})
}

func Test_run_read_commands_refuse_when_there_is_no_store(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status"}},
		{name: "accounts", args: []string{"accounts"}},
		{name: "spend", args: []string{"spend"}},
		{name: "cashflow", args: []string{"cashflow"}},
		{name: "findings", args: []string{"findings"}},
		{name: "sql with a query", args: []string{"sql", "SELECT 1"}},
		{name: "sql with a query only DuckDB can tell is empty", args: []string{"sql", ";"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			var stdout, stderr bytes.Buffer

			exitCode := run(context.Background(), c.args, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n",
				stderr.String())
			entries, err := os.ReadDir(home)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func Test_run_status_refuses_a_store_built_by_another_version(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStoreFixture(t, home, phaseOneImportRunsDDL)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: the store at "+abbreviated(t, storePathUnder(home), home)+
		" was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it\n",
		stderr.String())
}

func Test_run_spend_refuses_a_store_built_by_an_older_quarry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStoreFixture(t, home, phaseOneImportRunsDDL+
		"CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);"+
		"INSERT INTO store_info VALUES (2, '0.2.0', TIMESTAMP '2026-09-27 14:30:05');")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"spend"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: the store at "+abbreviated(t, storePathUnder(home), home)+
		" was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it\n",
		stderr.String())
}

func Test_run_status_refuses_a_store_whose_store_info_has_no_row(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeStoreFixture(t, home, phaseOneImportRunsDDL+
		"CREATE TABLE store_info (format_version INTEGER, quarry_version VARCHAR, built_at TIMESTAMP);")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: the store at "+abbreviated(t, storePathUnder(home), home)+
		" was built by another version of quarry; run quarry sync --from 20260927T143005Z to rebuild it\n",
		stderr.String())
}

func Test_run_accounts_refuses_a_store_that_is_not_a_duckdb_file(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	require.NoError(t, os.MkdirAll(storeDirUnder(home), 0o700))
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("not a database\n"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read the store at "+abbreviated(t, storePathUnder(home), home)+
		": the file is not a DuckDB database; run quarry sync to rebuild it\n",
		stderr.String())
}

func Test_run_status_refuses_a_store_removed_while_it_opens(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	var stdout, stderr bytes.Buffer
	env := defaultEnv(&stdout, &stderr)
	env.NewReport = newReportFactory(removingOpener(t))

	exitCode := runWith(context.Background(), []string{"status"}, env)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: no store at "+abbreviated(t, storePathUnder(home), home)+" yet; run quarry sync to build it\n",
		stderr.String())
}

func Test_run_read_commands_report_an_interrupt_during_the_open(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "status", args: []string{"status"}, wantStderr: "quarry: status interrupted\n"},
		{name: "accounts", args: []string{"accounts"}, wantStderr: "quarry: accounts interrupted\n"},
		{name: "spend", args: []string{"spend"}, wantStderr: "quarry: spend interrupted\n"},
		{name: "cashflow", args: []string{"cashflow"}, wantStderr: "quarry: cashflow interrupted\n"},
		{name: "findings", args: []string{"findings"}, wantStderr: "quarry: findings interrupted\n"},
		{name: "sql", args: []string{"sql", "SELECT 1"}, wantStderr: "quarry: query interrupted\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			syncAccountsFixture(t, home)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var stdout, stderr bytes.Buffer

			exitCode := run(ctx, c.args, &stdout, &stderr)

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// editStore runs stmt against the synced store through a writable connection of its own, closed before any read.
func editStore(t *testing.T, home, stmt string) {
	t.Helper()
	conn, err := sql.Open("duckdb", storePathUnder(home))
	require.NoError(t, err)
	_, err = conn.ExecContext(t.Context(), stmt)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func Test_run_accounts_refuses_a_store_that_cannot_be_read(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	editStore(t, home, "DROP VIEW v_account_balances")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"accounts"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read the store at "+abbreviated(t, storePathUnder(home), home)+
		": Table with name v_account_balances does not exist!; run quarry sync to rebuild it\n",
		stderr.String())
}

func Test_run_status_refuses_a_store_without_an_import_run(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncAccountsFixture(t, home)
	editStore(t, home, "DELETE FROM import_runs")
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"status"}, &stdout, &stderr)

	assert.Equal(t, 1, exitCode)
	assert.Empty(t, stdout.String())
	assert.Equal(t, "quarry: cannot read the store at "+abbreviated(t, storePathUnder(home), home)+
		": the store has no import history; run quarry sync to rebuild it\n",
		stderr.String())
}
