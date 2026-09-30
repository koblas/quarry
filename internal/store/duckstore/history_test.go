package duckstore_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/store"
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// phase1ImportRunsDDL is Phase 1's import_runs: 19 columns, one run (id 4), and no store_info beside it.
const phase1ImportRunsDDL = `CREATE TABLE import_runs (
	id BIGINT PRIMARY KEY, started_at TIMESTAMP NOT NULL, finished_at TIMESTAMP NOT NULL,
	snapshot_path VARCHAR NOT NULL, snapshot_sha256 VARCHAR NOT NULL, schema_fingerprint VARCHAR NOT NULL,
	accounts_rows BIGINT NOT NULL, categories_rows BIGINT NOT NULL, payees_rows BIGINT NOT NULL, tags_rows BIGINT NOT NULL,
	transactions_rows BIGINT NOT NULL, splits_rows BIGINT NOT NULL, split_tags_rows BIGINT NOT NULL, transfers_rows BIGINT NOT NULL,
	balances_checked BIGINT NOT NULL, balances_mismatched BIGINT NOT NULL, splits_mismatched BIGINT NOT NULL,
	transfers_one_sided BIGINT NOT NULL, investment_transactions_not_imported BIGINT NOT NULL);
INSERT INTO import_runs VALUES (4, '2026-06-01 10:00:00', '2026-06-01 10:00:02', '/snapshots/phase1.sqlite', 'abc', 'sha256:fp',
	7, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13);`

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

// importRunIDs is the ids of st's import_runs rows in ascending order.
func importRunIDs(t *testing.T, st *duckstore.Store) []int64 {
	t.Helper()
	db := openReadOnly(t, st.Path())
	var ids []int64
	require.NoError(t, db.QueryRows(t.Context(), "SELECT id FROM import_runs ORDER BY id", nil,
		func(scan func(dest ...any) error) error {
			var id int64
			if err := scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
			return nil
		}))
	require.NoError(t, db.Close())
	return ids
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
		AND snapshot_path = '/snapshots/phase1.sqlite' AND accounts_rows = 7 AND investment_transactions_not_imported = 13
		AND snapshot_taken_at IS NULL AND source_path IS NULL AND balances_never_reconciled IS NULL
		AND investment_accounts IS NULL AND transfers_paired IS NULL AND transfers_cross_currency IS NULL`, "1")
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
	db := openReadOnly(t, st.Path())
	assertScalar(t, db, `SELECT CAST(count(*) AS VARCHAR) FROM findings WHERE id = 'uncategorized:payee-9'
		AND first_found_at = TIMESTAMP '2026-06-01 10:00:00' AND fixed_at IS NOT NULL`, "1")
}

func Test_replace_starts_findings_silently_when_the_findings_read_fails(t *testing.T) {
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
			assert.Equal(t, 1, c.spy.closes)
			assert.Equal(t, []int64{1, 2}, importRunIDs(t, st))
		})
	}
}

func Test_replace_reads_the_history_in_four_queries_and_carries_the_findings_after_the_fourth(t *testing.T) {
	t.Parallel()
	spy := &spyReadDB{passQueries: 4, queryFault: ioFault(`query rows "SELECT"`)}
	st := newBuiltStore(t, spyOpener(spy))

	replaced, err := st.Replace(t.Context(), minimalRows())

	require.NoError(t, err)
	assert.True(t, replaced.FindingsCarried)
	assert.Equal(t, 4, spy.queries)
}
