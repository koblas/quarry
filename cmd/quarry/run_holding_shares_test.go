package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const holdingSpansQuery = `SELECT concat_ws(' ', CAST(from_date AS VARCHAR), COALESCE(CAST(to_date AS VARCHAR), 'NULL'), CAST(shares AS VARCHAR))
FROM holding_shares ORDER BY from_date`

func Test_run_sync_records_each_holdings_share_count_over_time(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	buyDay := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	twoBuysDay := buyDay.AddDate(0, 0, 1)
	splitDay := buyDay.AddDate(0, 0, 2)
	sellDay := buyDay.AddDate(0, 0, 3)
	futureDay := time.Now().UTC().Truncate(24*time.Hour).AddDate(1, 0, 0)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Brokerage", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	acmePK := b.Security(v9fixture.SecurityRow{Name: "Acme Corp", Ticker: "ACME", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: brokeragePK, Security: acmePK})
	invest := func(code int64, day time.Time, row v9fixture.TransactionRow) {
		row.Account, row.Position, row.PostedDate, row.Type = brokeragePK, positionPK, &day, &code
		pk := b.InvestmentTransaction(row)
		b.Entry(v9fixture.EntryRow{Parent: pk, Amount: row.Amount})
	}
	invest(3, buyDay, v9fixture.TransactionRow{Units: "10", Amount: "-100.00"})
	invest(3, twoBuysDay, v9fixture.TransactionRow{Units: "5", Amount: "-50.00"})
	invest(3, twoBuysDay, v9fixture.TransactionRow{Units: "5", Amount: "-50.00"})
	invest(23, splitDay, v9fixture.TransactionRow{Units: "0", Amount: "0", Numerator: "1", Denominator: "2"})
	invest(19, sellDay, v9fixture.TransactionRow{Units: "-4", Amount: "40.00"})
	invest(3, futureDay, v9fixture.TransactionRow{Units: "3", Amount: "-30.00"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "9"})
	bundle := b.WriteBundle(t, filepath.Join(home, "Documents"))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"sync", "--quicken", bundle.Dir}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "Shares    1 holding matches Quicken's share count\n")
	assert.Equal(t, []string{
		"2026-03-01 2026-03-01 10.000000",
		"2026-03-02 2026-03-02 20.000000",
		"2026-03-03 2026-03-03 10.000000",
		"2026-03-04 " + futureDay.AddDate(0, 0, -1).Format(time.DateOnly) + " 6.000000",
		futureDay.Format(time.DateOnly) + " NULL 9.000000",
	}, storeTextRows(t, home, holdingSpansQuery))
	assert.Equal(t, []string{"7"}, storeTextRows(t, home, "SELECT CAST(format_version AS VARCHAR) FROM store_info"))
}
