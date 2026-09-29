// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_records_an_import_runs_row_for_the_build(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	reconciled := int64(2)
	depositTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "100.00", PostedDate: &day, Status: &reconciled})
	b.Entry(v9fixture.EntryRow{Parent: depositTxn, Amount: "100.00"})
	b.Reconcile(v9fixture.ReconcileRow{Account: chequingPK, EndDate: &day, EndingBalance: "100.00"})
	legTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: legTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})
	buyTxn := b.Transaction(v9fixture.TransactionRow{
		Entity: v9fixture.EntInvestmentTransaction, Account: brokeragePK, Amount: "-40.00", PostedDate: &day,
	})
	b.Entry(v9fixture.EntryRow{Parent: buyTxn, Amount: "-40.00"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"snapshot"`
		Schema struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"schema"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{"1": manifest.Snapshot.Path + " " + manifest.Snapshot.SHA256 + " " + manifest.Schema.Fingerprint},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), snapshot_path || ' ' || snapshot_sha256 || ' ' || schema_fingerprint FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "2 0 0 0 2 2 0 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, "+
			"transactions_rows, splits_rows, split_tags_rows, transfers_rows) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "1 0 0 1 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', balances_checked, balances_mismatched, splits_mismatched, "+
			"transfers_one_sided, investment_transactions_not_imported) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "true"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), CAST(started_at <= finished_at AS VARCHAR) FROM import_runs"))
}
