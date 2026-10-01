package duckstore_test

import (
	"context"
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	reasonRepeatedID = "its import_runs table repeats an id"
	reasonNoTable    = "it has no import_runs table"
	reasonIncomplete = "its import_runs table is incomplete"
	reasonIDTooLarge = "its import_runs table has an id too large to follow"

	reasonFindingsRepeatedID = "its findings table repeats an id"
	reasonFindingsIncomplete = "its findings table is incomplete"
	previousOpenFindingID    = "uncategorized:payee-9"
	previousOpenFindingType  = "uncategorized"
)

// importRunsColumns are the 19 columns every store format has, each with the cell of a valid run.
var importRunsColumns = [][3]string{
	{"id", "BIGINT", "1"},
	{"started_at", "TIMESTAMP", "'2026-06-01 10:00:00'"},
	{"finished_at", "TIMESTAMP", "'2026-06-01 10:00:02'"},
	{"snapshot_path", "VARCHAR", "'/snapshots/run-1.sqlite'"},
	{"snapshot_sha256", "VARCHAR", "'abc'"},
	{"schema_fingerprint", "VARCHAR", "'sha256:fp'"},
	{"accounts_rows", "BIGINT", "0"},
	{"categories_rows", "BIGINT", "0"},
	{"payees_rows", "BIGINT", "0"},
	{"tags_rows", "BIGINT", "0"},
	{"transactions_rows", "BIGINT", "0"},
	{"splits_rows", "BIGINT", "0"},
	{"split_tags_rows", "BIGINT", "0"},
	{"transfers_rows", "BIGINT", "0"},
	{"balances_checked", "BIGINT", "0"},
	{"balances_mismatched", "BIGINT", "0"},
	{"splits_mismatched", "BIGINT", "0"},
	{"transfers_one_sided", "BIGINT", "0"},
	{"investment_transactions_not_imported", "BIGINT", "0"},
}

// importRunsTable is DDL for an import_runs table holding runs identical rows, without the omit
// column and with a NULL in the nullCell column; it has no primary key, so runs of 2 repeat an id.
func importRunsTable(omit, nullCell string, runs int) string {
	var columns, cells []string
	for _, column := range importRunsColumns {
		name, sqlType, cell := column[0], column[1], column[2]
		if name == omit {
			continue
		}
		if name == nullCell {
			cell = "NULL"
		}
		columns = append(columns, name+" "+sqlType)
		cells = append(cells, cell)
	}
	insert := "INSERT INTO import_runs VALUES (" + strings.Join(cells, ", ") + ");"
	return "CREATE TABLE import_runs (" + strings.Join(columns, ", ") + "); " + strings.Repeat(insert, runs)
}

// findingsColumns are the four columns of a findings table, each with the cell of an open finding.
var findingsColumns = [][3]string{
	{"id", "VARCHAR", "'" + previousOpenFindingID + "'"},
	{"type", "VARCHAR", "'" + previousOpenFindingType + "'"},
	{"first_found_at", "TIMESTAMP", "'2026-06-01 10:00:00'"},
	{"fixed_at", "TIMESTAMP", "NULL"},
}

// findingsTable is DDL for a findings table with one row per id, without the omit column and with a NULL
// in the nullCell column; it has no primary key, so a repeated id is stored.
func findingsTable(omit, nullCell string, ids ...string) string {
	var columns []string
	for _, column := range findingsColumns {
		if column[0] != omit {
			columns = append(columns, column[0]+" "+column[1])
		}
	}
	var ddl strings.Builder
	ddl.WriteString("CREATE TABLE findings (" + strings.Join(columns, ", ") + ");")
	for _, id := range ids {
		cells := make([]string, 0, len(findingsColumns))
		for _, column := range findingsColumns {
			cell := column[2]
			switch column[0] {
			case omit:
				continue
			case "id":
				cell = "'" + id + "'"
			}
			if column[0] == nullCell {
				cell = "NULL"
			}
			cells = append(cells, cell)
		}
		ddl.WriteString(" INSERT INTO findings VALUES (" + strings.Join(cells, ", ") + ");")
	}
	return ddl.String()
}

// findingsReason is the phrase a sync would print for replaced's findings fault.
func findingsReason(t *testing.T, st *duckstore.Store, replaced store.Replaced) string {
	t.Helper()
	require.NotNil(t, replaced.FindingsFault)
	return replaced.FindingsFault.UnreadableReason(st.Path())
}

// historyReason is the phrase a sync would print for replaced's history fault.
func historyReason(t *testing.T, st *duckstore.Store, replaced store.Replaced) string {
	t.Helper()
	require.NotNil(t, replaced.HistoryFault)
	return replaced.HistoryFault.UnreadableReason(st.Path())
}

func Test_replace_restarts_history_when_the_previous_store_cannot_be_read(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		newStore  func(t *testing.T) *duckstore.Store
		wantFault store.OpenFault
		wantWhy   string
	}{
		{
			name: "a file that is not a database",
			newStore: func(t *testing.T) *duckstore.Store {
				t.Helper()
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, duckstore.FileName), []byte("text\n"), 0o600))
				return duckstore.New(dir)
			},
			wantFault: store.OpenFaultNotDuckDB,
			wantWhy:   "the file is not a DuckDB database",
		},
		{
			name: "a file the process may not read",
			newStore: func(t *testing.T) *duckstore.Store {
				t.Helper()
				skipAsRoot(t)
				dir := t.TempDir()
				path := filepath.Join(dir, duckstore.FileName)
				require.NoError(t, os.WriteFile(path, []byte("text\n"), 0o600))
				require.NoError(t, os.Chmod(path, 0o000))
				return duckstore.New(dir)
			},
			wantFault: store.OpenFaultPermission,
			wantWhy:   "permission denied",
		},
		{
			name: "a file another program holds open for writing",
			newStore: func(t *testing.T) *duckstore.Store {
				t.Helper()
				return newBuiltStore(t, failingOpener(driverIOError(`IO Error: Could not set lock on file "x.duckdb": `+
					`Conflicting lock is held in /usr/local/bin/quarry (PID 42) by user dave.`)))
			},
			wantFault: store.OpenFaultLocked,
			wantWhy:   "another program has it open for writing",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := c.newStore(t)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			require.NotNil(t, replaced.HistoryFault)
			assert.Equal(t, c.wantFault, replaced.HistoryFault.Fault)
			assert.Equal(t, c.wantWhy, replaced.HistoryFault.UnreadableReason(st.Path()))
			assert.True(t, replaced.StoreUnreadable)
			assert.Nil(t, replaced.FindingsFault)
			assert.Equal(t, []int64{1}, importRunIDs(t, st))
		})
	}
}

func Test_replace_names_a_repeated_findings_id_as_the_findings_fault(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{}
	st := newStoreFile(t, importRunsTable("", "", 1)+findingsTable("", "", previousOpenFindingID, previousOpenFindingID), spyOpener(spy))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonFindingsRepeatedID, findingsReason(t, st, replaced))
	assert.Equal(t, store.OpenFaultOther, replaced.FindingsFault.Fault)
	assert.False(t, replaced.FindingsCarried)
	assert.False(t, replaced.StoreUnreadable)
	assert.Nil(t, replaced.HistoryFault)
	assert.Equal(t, 1, spy.closes)
}

func Test_replace_carries_findings_whose_ids_are_all_distinct(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 1)+findingsTable("", "", previousOpenFindingID, "uncategorized:payee-10"))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.FindingsFault)
	assert.True(t, replaced.FindingsCarried)
}

func Test_replace_names_a_findings_table_without_a_required_cell_as_incomplete(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
	}{
		{name: "no id column", ddl: findingsTable("id", "", previousOpenFindingID)},
		{name: "no type column", ddl: findingsTable("type", "", previousOpenFindingID)},
		{name: "no first_found_at column", ddl: findingsTable("first_found_at", "", previousOpenFindingID)},
		{name: "no fixed_at column", ddl: findingsTable("fixed_at", "", previousOpenFindingID)},
		{name: "a NULL id", ddl: findingsTable("", "id", previousOpenFindingID)},
		{name: "a NULL type", ddl: findingsTable("", "type", previousOpenFindingID)},
		{name: "a NULL first_found_at", ddl: findingsTable("", "first_found_at", previousOpenFindingID)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, importRunsTable("", "", 1)+c.ddl)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonFindingsIncomplete, findingsReason(t, st, replaced))
			assert.False(t, replaced.FindingsCarried)
			assert.Nil(t, replaced.HistoryFault)
		})
	}
}

func Test_replace_names_both_tables_as_faulty_when_each_is_unreadable_after_the_store_opens(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 2)+findingsTable("", "", previousOpenFindingID, previousOpenFindingID))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonRepeatedID, historyReason(t, st, replaced))
	assert.Equal(t, reasonFindingsRepeatedID, findingsReason(t, st, replaced))
	assert.False(t, replaced.StoreUnreadable)
}

func Test_replace_marks_nothing_fixed_when_the_previous_findings_cannot_be_read(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 1)+findingsTable("", "", previousOpenFindingID, previousOpenFindingID))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.NotZero(t, replaced.Findings.Open)
	assert.Zero(t, replaced.Findings.NewlyFixed)
	assert.Equal(t, replaced.Findings.Open, replaced.Findings.New)
	assertScalar(t, openReadOnly(t, st.Path()), "SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE id = '"+previousOpenFindingID+"'", "0")
}

func Test_replace_names_a_repeated_import_run_id_as_the_history_fault(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{}
	st := newStoreFile(t, importRunsTable("", "", 2), spyOpener(spy))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonRepeatedID, historyReason(t, st, replaced))
	assert.Equal(t, 1, spy.closes)
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

func Test_replace_names_an_import_run_id_at_the_int64_maximum_as_the_history_fault(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 1)+" UPDATE import_runs SET id = 9223372036854775807;")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonIDTooLarge, historyReason(t, st, replaced))
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

func Test_replace_carries_an_import_run_id_one_below_the_int64_maximum_and_numbers_the_new_run_at_it(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 1)+" UPDATE import_runs SET id = 9223372036854775806;")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Equal(t, []int64{math.MaxInt64 - 1, math.MaxInt64}, importRunIDs(t, st))
}

func Test_replace_names_a_missing_import_runs_table_as_the_history_fault(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, "CREATE TABLE accounts (id VARCHAR);")

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonNoTable, historyReason(t, st, replaced))
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

func Test_replace_names_an_import_runs_table_without_a_required_cell_as_incomplete(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ddl  string
	}{
		{name: "no id column", ddl: importRunsTable("id", "", 1)},
		{name: "no snapshot_path column", ddl: importRunsTable("snapshot_path", "", 1)},
		{name: "no snapshot_sha256 column", ddl: importRunsTable("snapshot_sha256", "", 1)},
		{name: "no started_at column", ddl: importRunsTable("started_at", "", 1)},
		{name: "only id and snapshot_path", ddl: importRunsDDL},
		{name: "a NULL id", ddl: importRunsTable("", "id", 1)},
		{name: "a NULL snapshot_path", ddl: importRunsTable("", "snapshot_path", 1)},
		{name: "a NULL snapshot_sha256", ddl: importRunsTable("", "snapshot_sha256", 1)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStoreFile(t, c.ddl)

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonIncomplete, historyReason(t, st, replaced))
			assert.Equal(t, []int64{1}, importRunIDs(t, st))
		})
	}
}

func Test_replace_names_a_failed_history_read_as_incomplete_and_closes_the_connection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spy  *spyReadDB
	}{
		{name: "the columns query", spy: &spyReadDB{queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a column name scan", spy: &spyReadDB{scanFault: ioFault(`scan column`)}},
		{name: "the rows query", spy: &spyReadDB{passQueries: 1, queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a row scan", spy: &spyReadDB{passQueries: 1, scanFault: ioFault(`scan row`)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Equal(t, reasonIncomplete, historyReason(t, st, replaced))
			assert.Equal(t, 1, c.spy.closes)
			assert.Equal(t, []int64{1}, importRunIDs(t, st))
		})
	}
}

// staleWALStore is a store whose quarry.duckdb.wal holds one committed, uncheckpointed import run (id 7) beside a checkpointed run 1.
func staleWALStore(t *testing.T) *duckstore.Store {
	t.Helper()
	dir := t.TempDir()
	_, err := duckstore.New(dir).Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	path := filepath.Join(dir, duckstore.FileName)
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "SET checkpoint_threshold = '1GB'")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "INSERT INTO import_runs SELECT * REPLACE (7 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
	require.NoError(t, err)
	wal, err := os.ReadFile(path + ".wal")
	require.NoError(t, err)
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	// Closing checkpoints the wal away; put back the file and the wal a crash would have left.
	require.NoError(t, db.Close())
	require.NoError(t, os.WriteFile(path, file, 0o600))       //nolint:gosec // path is under the test's own temp dir
	require.NoError(t, os.WriteFile(path+".wal", wal, 0o600)) //nolint:gosec // path is under the test's own temp dir
	return duckstore.New(dir)
}

func Test_replace_carries_the_runs_a_stale_wal_committed(t *testing.T) {
	t.Parallel()
	st := staleWALStore(t)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Equal(t, []int64{1, 7, 8}, importRunIDs(t, st))
}

func Test_replace_carries_history_beside_a_wal_that_holds_no_transaction(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t)
	require.NoError(t, os.WriteFile(st.Path()+".wal", []byte(strings.Repeat("x", 4096)), 0o600))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Equal(t, []int64{1, 2}, importRunIDs(t, st))
}

func Test_replace_does_not_swap_when_the_context_ends_during_the_history_read(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	st := newBuiltStore(t, spyOpener(&spyReadDB{onQuery: cancel}))
	before, err := os.ReadFile(st.Path())
	require.NoError(t, err)

	_, err = st.Replace(ctx, minimalRows())

	require.ErrorIs(t, err, context.Canceled)
	after, readErr := os.ReadFile(st.Path())
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	entries, err := os.ReadDir(filepath.Dir(st.Path()))
	require.NoError(t, err)
	assert.Equal(t, []string{duckstore.FileName}, direntNames(entries))
}
