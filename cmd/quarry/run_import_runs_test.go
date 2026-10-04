// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	buyTxn := b.InvestmentTransaction(v9fixture.TransactionRow{
		Type: new(int64(3)), Account: brokeragePK, Amount: "-40.00", PostedDate: &day,
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
	assert.Equal(t, map[string]string{"1": "2 0 0 0 3 3 0 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', accounts_rows, categories_rows, payees_rows, tags_rows, "+
			"transactions_rows, splits_rows, split_tags_rows, transfers_rows) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "1 0 0 1"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), concat_ws(' ', balances_checked, balances_mismatched, splits_mismatched, "+
			"transfers_one_sided) FROM import_runs"))
	assert.Equal(t, map[string]string{"1": "true"},
		stringMap(t, db, "SELECT CAST(id AS VARCHAR), CAST(started_at <= finished_at AS VARCHAR) FROM import_runs"))
}

func Test_run_sync_from_keeps_the_earlier_build_in_import_runs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	snapshotsDir := filepath.Join(storeDirUnder(home), "snapshots")
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "DocumentsA"), "Chequing"))
	earlierManifest := onlyFileWithSuffix(t, snapshotsDir, ".json")
	earlierStore := filepath.Join(home, "earlier.duckdb")
	require.NoError(t, os.Rename(storePathUnder(home), earlierStore))
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "DocumentsB"), "Savings"))
	laterManifest := manifestOtherThan(t, snapshotsDir, earlierManifest)
	require.NoError(t, os.Rename(earlierStore, storePathUnder(home)))
	rowsBefore := importRunRowsAsText(t, home)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", strings.TrimSuffix(filepath.Base(laterManifest), ".json")}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	rowsAfter := importRunRowsAsText(t, home)
	require.Len(t, rowsAfter, 2)
	assert.Equal(t, rowsBefore["1"], rowsAfter["1"])
	assert.Equal(t, manifestSHA256(t, earlierManifest), importRunSHA256(t, home, "1"))
	assert.Equal(t, manifestSHA256(t, laterManifest), importRunSHA256(t, home, "2"))
}

func Test_run_sync_from_warns_and_restarts_history_when_the_previous_store_is_not_a_duckdb_database(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	manifest := onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".json")
	require.NoError(t, os.WriteFile(storePathUnder(home), []byte("this is not a database"), 0o600))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--from", strings.TrimSuffix(filepath.Base(manifest), ".json")}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: cannot carry import history, findings or exchange rates forward from the previous store (the file is not a DuckDB database); "+
		"all three start again with this sync\n", stderr.String())
	assert.Equal(t, map[string]string{"1": manifestSHA256(t, manifest)}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_sha256 FROM import_runs"))
}

func Test_run_sync_from_warns_and_restarts_history_when_the_previous_run_has_the_largest_id(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncBundle(t, writeNamedAccountBundle(t, filepath.Join(home, "Documents"), "Chequing"))
	id := snapshotID(onlyFileWithSuffix(t, filepath.Join(storeDirUnder(home), "snapshots"), ".sqlite"))
	editStore(t, home, "UPDATE import_runs SET id = 9223372036854775807")

	exitCode, _, stderr := runSyncFrom(t, id)

	require.Equal(t, 0, exitCode, stderr)
	assert.Equal(t, "quarry: warning: cannot carry import history forward from the previous store (its import_runs table has an id too large to follow); "+
		"import_runs starts again with this sync\n", stderr)
	assert.Equal(t, map[string]string{"1": id + ".sqlite"}, importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), regexp_extract(snapshot_path, '[^/]+$') FROM import_runs"))
}

// writeNamedAccountBundle writes a bundle whose one account carries name, so two bundles hash differently.
func writeNamedAccountBundle(t *testing.T, dir, name string) v9fixture.Bundle {
	t.Helper()
	b := v9fixture.NewBuilder()
	account := b.Account(v9fixture.AccountRow{Name: name, Type: "CHECKING", Currency: "CAD", Active: true})
	addTransaction(b, account, "10.00", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	return b.WriteBundle(t, dir)
}

// manifestOtherThan returns the one snapshot manifest in dir that is not excluded.
func manifestOtherThan(t *testing.T, dir, excluded string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	var others []string
	for _, m := range matches {
		if m != excluded {
			others = append(others, m)
		}
	}
	require.Len(t, others, 1)
	return others[0]
}

// manifestSHA256 reads the snapshot sha256 a manifest records.
func manifestSHA256(t *testing.T, manifestPath string) string {
	t.Helper()
	raw, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			SHA256 string `json:"sha256"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	return manifest.Snapshot.SHA256
}

// importRunRowsAsText maps each import_runs id to its whole row rendered as text, so every column takes part in a comparison.
func importRunRowsAsText(t *testing.T, home string) map[string]string {
	t.Helper()
	return importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), CAST(r AS VARCHAR) FROM import_runs r")
}

// importRunSHA256 reads the snapshot_sha256 of import run id.
func importRunSHA256(t *testing.T, home, id string) string {
	t.Helper()
	return importRunQuery(t, home, "SELECT CAST(id AS VARCHAR), snapshot_sha256 FROM import_runs")[id]
}

func importRunQuery(t *testing.T, home, query string) map[string]string {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	return stringMap(t, db, query)
}

// storeTextRows runs query (one string-shaped column) against the store under home and returns each row in query order.
func storeTextRows(t *testing.T, home, query string) []string {
	t.Helper()
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	var got []string
	err = db.QueryRows(t.Context(), query, nil, func(scan func(dest ...any) error) error {
		var row string
		if err := scan(&row); err != nil {
			return err
		}
		got = append(got, row)
		return nil
	})
	require.NoError(t, err)
	return got
}
