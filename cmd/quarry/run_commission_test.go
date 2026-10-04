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

func Test_run_sync_keeps_a_commission_with_fractions_of_a_cent(t *testing.T) {
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
	buyPK := invest(3, v9fixture.TransactionRow{Units: "10", Amount: "-1000.50", Commission: "9.99"})
	sellPK := invest(19, v9fixture.TransactionRow{Units: "-4", Amount: "400.25", Commission: "8.4998"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "6"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode)
	require.Empty(t, stderr.String())
	db, err := duckdb.OpenReadOnly(t.Context(), storePathUnder(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.Equal(t, map[string]string{
		fmt.Sprintf("itxn-%d", buyPK):  "9.9900",
		fmt.Sprintf("itxn-%d", sellPK): "8.4998",
	}, stringMap(t, db, `SELECT id, CAST(commission AS VARCHAR) FROM investment_transactions`))
}
