package main

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/duckdb"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_sync_keeps_quickens_cost_basis_and_stores_null_for_none(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	invest := func(code int64, row v9fixture.TransactionRow) int64 {
		row.Account, row.Position, row.PostedDate, row.Type = brokeragePK, positionPK, &day, &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
		return pk
	}
	buyPK := invest(3, v9fixture.TransactionRow{Units: "10", Amount: "-1000.50", CostBasis: "1000.50"})
	reinvestPK := invest(15, v9fixture.TransactionRow{Units: "2", Amount: "0", CostBasis: "50.25"})
	addWithCostPK := invest(2, v9fixture.TransactionRow{Units: "5", Amount: "0", CostBasis: "300"})
	addWithoutCostPK := invest(2, v9fixture.TransactionRow{Units: "3", Amount: "0"})
	sellPK := invest(19, v9fixture.TransactionRow{Units: "-4", Amount: "400.25", CostBasis: "0"})
	dividendPK := invest(10, v9fixture.TransactionRow{Amount: "12"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "16"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	require.Empty(t, stderr.String())
	assert.Equal(t, []string{"9"}, storeTextRows(t, home, "SELECT CAST(format_version AS VARCHAR) FROM store_info"))
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		fmt.Sprintf("itxn-%d", buyPK):            "1000.50",
		fmt.Sprintf("itxn-%d", reinvestPK):       "50.25",
		fmt.Sprintf("itxn-%d", addWithCostPK):    "300.00",
		fmt.Sprintf("itxn-%d", addWithoutCostPK): "NULL",
		fmt.Sprintf("itxn-%d", sellPK):           "NULL",
		fmt.Sprintf("itxn-%d", dividendPK):       "NULL",
	}, stringMap(t, db, `SELECT id, COALESCE(CAST(cost_basis AS VARCHAR), 'NULL') FROM investment_transactions`))
}
