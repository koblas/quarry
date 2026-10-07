// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
)

// versionFiveStoreDDL is a store as format 5 left it: the 19 Phase 1 import_runs columns, the nine optional ones
// and store_info, none of the four investment count columns, and one run (id 7) that skipped 13 investment transactions.
const versionFiveStoreDDL = `CREATE TABLE store_info (format_version INTEGER NOT NULL, quarry_version VARCHAR NOT NULL, built_at TIMESTAMP NOT NULL);
INSERT INTO store_info VALUES (5, 'v0.5.0', '2026-06-01 10:00:02');
CREATE TABLE import_runs (
	id BIGINT PRIMARY KEY, started_at TIMESTAMP NOT NULL, finished_at TIMESTAMP NOT NULL,
	snapshot_path VARCHAR NOT NULL, snapshot_sha256 VARCHAR NOT NULL, schema_fingerprint VARCHAR NOT NULL,
	accounts_rows BIGINT NOT NULL, categories_rows BIGINT NOT NULL, payees_rows BIGINT NOT NULL, tags_rows BIGINT NOT NULL,
	transactions_rows BIGINT NOT NULL, splits_rows BIGINT NOT NULL, split_tags_rows BIGINT NOT NULL, transfers_rows BIGINT NOT NULL,
	balances_checked BIGINT NOT NULL, balances_mismatched BIGINT NOT NULL, splits_mismatched BIGINT NOT NULL,
	transfers_one_sided BIGINT NOT NULL, investment_transactions_not_imported BIGINT NOT NULL,
	snapshot_taken_at TIMESTAMP, source_path VARCHAR, balances_never_reconciled BIGINT, investment_accounts BIGINT,
	transfers_paired BIGINT, transfers_cross_currency BIGINT, rates_checked_from DATE, rates_last DATE, rates_fetch_error VARCHAR);
INSERT INTO import_runs (id, started_at, finished_at, snapshot_path, snapshot_sha256, schema_fingerprint,
	accounts_rows, categories_rows, payees_rows, tags_rows, transactions_rows, splits_rows, split_tags_rows, transfers_rows,
	balances_checked, balances_mismatched, splits_mismatched, transfers_one_sided, investment_transactions_not_imported)
VALUES (7, '2026-06-01 10:00:00', '2026-06-01 10:00:02', '/snapshots/version-five.sqlite', 'abc', 'sha256:fp',
	3, 2, 1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13);`

func Test_run_sync_twice_over_a_version_5_store_carries_import_history_forward(t *testing.T) {
	home := newHome(t)
	writeStoreFixture(t, home, versionFiveStoreDDL)
	bundle := writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing")
	var firstOut, firstErr, secondOut, secondErr bytes.Buffer

	first := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &firstOut, &firstErr)
	second := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &secondOut, &secondErr)

	assert.Equal(t, 0, first, firstErr.String())
	assert.Equal(t, 0, second, secondErr.String())
	assert.Empty(t, firstErr.String())
	assert.Empty(t, secondErr.String())
	assert.Equal(t, map[string]string{"7": "/snapshots/version-five.sqlite", "8": "new run", "9": "new run"},
		importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), CASE WHEN id = 7 THEN snapshot_path ELSE 'new run' END FROM import_runs"))
	assert.Equal(t, map[string]string{"7": "3 2 1 4 5 6 7 8 9 10 11 12 true"},
		importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, "+
			"transactions_rows, splits_rows, split_tags_rows, transfers_rows, balances_checked, balances_mismatched, splits_mismatched, "+
			"transfers_one_sided, CAST(securities_rows IS NULL AND prices_rows IS NULL AND investment_transactions_rows IS NULL "+
			"AND shares_checked IS NULL AS VARCHAR)) FROM import_runs WHERE id = 7"))
	assert.Equal(t, map[string]string{"format_version": strconv.Itoa(duckstore.FormatVersion)},
		importRunQuery(t, home, "SELECT 'format_version', CAST(format_version AS VARCHAR) FROM store_info"))
	assert.Empty(t, importRunQuery(t, home, "SELECT column_name, '' FROM duckdb_columns() "+
		"WHERE table_name = 'import_runs' AND column_name = 'investment_transactions_not_imported'"))
}
