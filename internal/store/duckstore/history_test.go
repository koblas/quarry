package duckstore_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importRunTexts is every import_runs row of st's store as text, ordered by id.
func importRunTexts(t *testing.T, st *duckstore.Store) []string {
	t.Helper()
	db := openReadOnly(t, st.Path())
	var texts []string
	require.NoError(t, db.QueryRows(t.Context(), "SELECT CAST(r AS VARCHAR) FROM import_runs r ORDER BY r.id", nil,
		func(scan func(dest ...any) error) error {
			var text string
			if err := scan(&text); err != nil {
				return err
			}
			texts = append(texts, text)
			return nil
		}))
	require.NoError(t, db.Close())
	return texts
}

var errNoStoreToOpen = errors.New("no store to open")

// eventReadDB records its Close in events.
type eventReadDB struct {
	duckstore.ReadDB

	events *[]string
}

func (e eventReadDB) Close() error {
	*e.events = append(*e.events, "closed")
	return e.ReadDB.Close()
}

func Test_replace_carries_the_previous_import_runs_unchanged(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	addImportRun(t, st, 3, "/snapshots/run-3.sqlite", 33)
	before := importRunTexts(t, st)
	next := minimalRows()
	next.ImportRuns[0].Snapshot.Path = "/snapshots/next.sqlite"

	_, err = st.Replace(t.Context(), next)

	require.NoError(t, err)
	after := importRunTexts(t, st)
	require.Len(t, before, 2)
	require.Len(t, after, 3)
	assert.Equal(t, before, after[:2])
}

func Test_replace_carries_null_for_columns_an_older_store_lacks(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, phase1ImportRunsDDL)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM import_runs WHERE id = 4
		AND snapshot_path = '/snapshots/phase1.sqlite' AND accounts_rows = 7
		AND snapshot_taken_at IS NULL AND source_path IS NULL AND balances_never_reconciled IS NULL
		AND investment_accounts IS NULL AND transfers_paired IS NULL AND transfers_cross_currency IS NULL
		AND rates_checked_from IS NULL AND rates_last IS NULL AND rates_fetch_error IS NULL
		AND securities_rows IS NULL AND prices_rows IS NULL AND investment_transactions_rows IS NULL AND shares_checked IS NULL`, "1")
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM duckdb_columns()
		WHERE table_name = 'import_runs' AND column_name = 'investment_transactions_not_imported'`, "0")
}

func Test_replace_carries_the_rates_columns_of_a_run_into_the_next_store(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	execOnStore(t, st, `UPDATE import_runs SET rates_checked_from = DATE '2026-01-02', rates_last = DATE '2026-01-05',
		rates_fetch_error = 'rates host unreachable' WHERE id = 1`)

	_, err = st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM import_runs WHERE id = 1 AND rates_checked_from = DATE '2026-01-02'
		AND rates_last = DATE '2026-01-05' AND rates_fetch_error = 'rates host unreachable'`, "1")
}

func Test_replace_carries_the_shares_checked_count_of_a_run_into_the_next_store(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)

	next := minimalRows()
	next.ImportRuns[0].SharesChecked = 3
	_, err = st.Replace(t.Context(), next)

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT concat_ws(' ', id, shares_checked) FROM import_runs WHERE id = 1`, "1 21")
	assertScalar(t, db, `SELECT concat_ws(' ', id, shares_checked) FROM import_runs WHERE id = 2`, "2 3")
}

func Test_replace_records_no_rates_columns_for_the_new_run(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM import_runs WHERE id = 1
		AND rates_checked_from IS NULL AND rates_last IS NULL AND rates_fetch_error IS NULL`, "1")
}

func Test_replace_numbers_the_new_run_after_the_highest_carried_id(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	_, err := st.Replace(t.Context(), minimalRows())
	require.NoError(t, err)
	addImportRun(t, st, 5, "/snapshots/run-5.sqlite", 55)
	addImportRun(t, st, 2, "/snapshots/run-2.sqlite", 22)

	_, err = st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, []int64{1, 2, 5, 6}, importRunIDs(t, st))
}

func Test_replace_numbers_the_first_run_1_when_import_runs_is_empty(t *testing.T) {
	t.Parallel()
	st := duckstore.New(t.TempDir())
	rows := minimalRows()
	rows.ImportRuns = nil
	_, err := st.Replace(t.Context(), rows)
	require.NoError(t, err)

	_, err = st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

func Test_replace_reads_the_previous_history_before_it_creates_the_build_file(t *testing.T) {
	t.Parallel()
	var events []string
	st := newBuiltStore(t,
		duckstore.WithOpenReadOnly(func(ctx context.Context, path string) (duckstore.ReadDB, error) {
			db, err := duckdb.OpenReadOnly(ctx, path)
			if err != nil {
				return nil, err
			}
			events = append(events, "opened")
			return eventReadDB{ReadDB: db, events: &events}, nil
		}),
		duckstore.WithCreate(func(ctx context.Context, path string) (duckstore.DB, error) {
			events = append(events, "created")
			db, err := duckdb.Create(ctx, path)
			if err != nil {
				return nil, err
			}
			return db, nil
		}))

	_, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, []string{"opened", "closed", "created"}, events)
}

func Test_replace_starts_history_silently_when_no_store_exists(t *testing.T) {
	t.Parallel()
	opens := 0
	st := duckstore.New(t.TempDir(), duckstore.WithOpenReadOnly(func(context.Context, string) (duckstore.ReadDB, error) {
		opens++
		return nil, errNoStoreToOpen
	}))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Zero(t, opens)
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

func Test_replace_starts_history_silently_when_the_store_vanishes_before_it_is_opened(t *testing.T) {
	t.Parallel()
	st := newBuiltStore(t, duckstore.WithOpenReadOnly(func(_ context.Context, path string) (duckstore.ReadDB, error) {
		require.NoError(t, os.Remove(path))
		return nil, driverIOError(`IO Error: Cannot open database "x.duckdb" in read-only mode: database does not exist`)
	}))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.False(t, replaced.StoreUnreadable)
	assert.Equal(t, []int64{1}, importRunIDs(t, st))
}

// findingsTableDDL is the v4 findings table holding one open finding of a payee no build detects.
const findingsTableDDL = `CREATE TABLE findings (id VARCHAR PRIMARY KEY, type VARCHAR NOT NULL, first_found_at TIMESTAMP NOT NULL, fixed_at TIMESTAMP);
INSERT INTO findings VALUES ('uncategorized:payee-9', 'uncategorized', '2026-06-01 10:00:00', NULL);`

func Test_replace_reports_the_findings_carried_when_the_previous_store_has_a_findings_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rows func() store.Rows
	}{
		{name: "holding findings", rows: minimalRows},
		{name: "holding none", rows: func() store.Rows {
			rows := minimalRows()
			rows.Transfers = rows.Transfers[:1]
			return rows
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			_, err := duckstore.New(dir).Replace(t.Context(), c.rows())
			require.NoError(t, err)

			replaced, err := duckstore.New(dir).Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.True(t, replaced.FindingsCarried)
		})
	}
}

func Test_replace_reports_no_findings_carried_when_the_previous_store_has_no_findings_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		newStore func(t *testing.T) *duckstore.Store
	}{
		{name: "no store", newStore: func(t *testing.T) *duckstore.Store {
			t.Helper()
			return duckstore.New(t.TempDir())
		}},
		{name: "an earlier format", newStore: func(t *testing.T) *duckstore.Store {
			t.Helper()
			return newStoreFile(t, phase1ImportRunsDDL)
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			replaced, err := c.newStore(t).Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Nil(t, replaced.HistoryFault)
			assert.Nil(t, replaced.FindingsFault)
			assert.False(t, replaced.StoreUnreadable)
			assert.False(t, replaced.FindingsCarried)
		})
	}
}

func Test_replace_carries_the_findings_beside_an_import_runs_table_it_cannot_read(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 2)+findingsTableDDL)

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Equal(t, reasonRepeatedID, historyReason(t, st, replaced))
	assert.True(t, replaced.FindingsCarried)
	assert.Nil(t, replaced.FindingsFault)
	assert.False(t, replaced.StoreUnreadable)
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE id = 'uncategorized:payee-9'
		AND first_found_at = TIMESTAMP '2026-06-01 10:00:00' AND fixed_at IS NOT NULL`, "1")
}

func Test_replace_names_a_failed_findings_read_as_incomplete_and_closes_the_connection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		spy  *spyReadDB
	}{
		{name: "the columns query", spy: &spyReadDB{passQueries: 2, queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a column name scan", spy: &spyReadDB{passQueries: 2, scanFault: ioFault(`scan column`)}},
		{name: "the rows query", spy: &spyReadDB{passQueries: 3, queryFault: ioFault(`query rows "SELECT"`)}},
		{name: "a row scan", spy: &spyReadDB{passQueries: 3, scanFault: ioFault(`scan row`)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newBuiltStore(t, spyOpener(c.spy))

			replaced, err := st.Replace(t.Context(), minimalRows())

			require.NoError(t, err)
			assert.Nil(t, replaced.HistoryFault)
			assert.False(t, replaced.FindingsCarried)
			assert.False(t, replaced.StoreUnreadable)
			require.NotNil(t, replaced.FindingsFault)
			assert.Equal(t, store.OpenFaultOther, replaced.FindingsFault.Fault)
			assert.Equal(t, "its findings table is incomplete", findingsReason(t, st, replaced))
			assert.Equal(t, 1, c.spy.closes)
			assert.Equal(t, []int64{1, 2}, importRunIDs(t, st))
		})
	}
}

func Test_replace_reads_the_history_in_four_queries_and_asks_for_the_rates_columns_in_a_fifth(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{passQueries: 4, queryFault: ioFault(`query rows "SELECT"`)}
	st := newBuiltStore(t, spyOpener(spy))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.True(t, replaced.FindingsCarried)
	assert.Equal(t, 5, spy.queries)
}

// importRunsColumns are the 18 columns every store format has, each with the cell of a valid run.
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

func Test_replace_carries_the_import_runs_of_a_store_whose_table_lacks_the_not_imported_column(t *testing.T) {
	t.Parallel()
	st := newStoreFile(t, importRunsTable("", "", 1))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.Nil(t, replaced.HistoryFault)
	assert.Equal(t, []int64{1, 2}, importRunIDs(t, st))
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
		{name: "no transfers_one_sided column", ddl: importRunsTable("transfers_one_sided", "", 1)},
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
	db, err := duckdb.OpenReadWrite(t.Context(), path)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "SET checkpoint_threshold = '1GB'")
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "INSERT INTO import_runs SELECT * REPLACE (7 AS id) FROM import_runs") //nolint:unqueryvet // a copy of the row is the point
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
