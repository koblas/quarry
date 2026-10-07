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
	"github.com/koblas/quarry/internal/store/duckstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_stamps_the_store_with_its_format_and_the_build(t *testing.T) {
	home := newHome(t)

	b := v9fixture.NewBuilder()
	chequingPK := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	savingsPK := b.Account(v9fixture.AccountRow{Name: "US Savings", Type: "SAVINGS", Currency: "USD", Active: true})
	b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	sentTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-100.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: sentTxn, Amount: "-100.00", QuickenID: 1001, Transfer: "2002"})
	receivedTxn := b.Transaction(v9fixture.TransactionRow{Account: savingsPK, Amount: "75.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: receivedTxn, Amount: "75.00", QuickenID: 2002, Transfer: "1001"})
	strayTxn := b.Transaction(v9fixture.TransactionRow{Account: chequingPK, Amount: "-5.00", PostedDate: &day})
	b.Entry(v9fixture.EntryRow{Parent: strayTxn, Amount: "-5.00", QuickenID: 3001, Transfer: "Old Visa"})

	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer
	before := time.Now().UTC().Truncate(time.Microsecond)

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	after := time.Now().UTC()
	require.Equal(t, 0, exitCode, stderr.String())
	snapshotsDir := filepath.Join(home, "Library", "Application Support", "quarry", "snapshots")
	raw, err := os.ReadFile(onlyFileWithSuffix(t, snapshotsDir, ".json"))
	require.NoError(t, err)
	var manifest struct {
		Snapshot struct {
			Source  string `json:"source"`
			TakenAt string `json:"taken_at"`
		} `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	takenAt, err := time.Parse(time.RFC3339, manifest.Snapshot.TakenAt)
	require.NoError(t, err)

	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var infoRows, formatVersion int
	var quarryVersion string
	var builtAt, finishedAt time.Time
	require.NoError(t, db.QueryRows(t.Context(), "SELECT count(*) FROM store_info", nil, func(scan func(dest ...any) error) error {
		return scan(&infoRows)
	}))
	require.Equal(t, 1, infoRows)
	require.NoError(t, db.QueryRows(t.Context(), "SELECT format_version, quarry_version, built_at FROM store_info", nil,
		func(scan func(dest ...any) error) error { return scan(&formatVersion, &quarryVersion, &builtAt) }))
	require.NoError(t, db.QueryRows(t.Context(), "SELECT finished_at FROM import_runs", nil,
		func(scan func(dest ...any) error) error { return scan(&finishedAt) }))
	assert.Equal(t, duckstore.FormatVersion, formatVersion)
	assert.NotEmpty(t, quarryVersion)
	assert.False(t, builtAt.Before(before))
	assert.False(t, builtAt.After(after))
	assert.False(t, builtAt.Before(finishedAt))

	var snapshotTakenAt time.Time
	var sourcePath string
	require.NoError(t, db.QueryRows(t.Context(), "SELECT snapshot_taken_at, source_path FROM import_runs", nil,
		func(scan func(dest ...any) error) error { return scan(&snapshotTakenAt, &sourcePath) }))
	assert.Equal(t, takenAt.UTC(), snapshotTakenAt.UTC())
	assert.Equal(t, manifest.Snapshot.Source, sourcePath)
	assert.Equal(t, map[string]string{"1": "2 1 1 1 2"}, stringMap(t, db,
		"SELECT CAST(id AS VARCHAR), concat_ws(' ', balances_never_reconciled, investment_accounts, transfers_paired, "+
			"transfers_cross_currency, transfers_rows) FROM import_runs"))
}
