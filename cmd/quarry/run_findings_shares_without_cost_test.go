package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sharesWithoutCostClause = "enter what each one cost on its Add Shares transaction in Quicken, " +
	"then run quarry sync; until then quarry acb counts those shares at no cost"

func Test_run_findings_lists_shares_added_with_no_cost_as_status_sync_and_mcp_count_them(t *testing.T) {
	const addShares = int64(2)
	var syncLine, emptyCostID, zeroCostID string
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		b := v9fixture.NewBuilder()
		marginPK := b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		tfsaPK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		xeqtPK := b.Security(v9fixture.SecurityRow{Name: "XEQT", Ticker: "XEQT", Currency: "CAD"})
		marginPosition := b.Position(v9fixture.PositionRow{Account: marginPK, Security: xeqtPK})
		tfsaPosition := b.Position(v9fixture.PositionRow{Account: tfsaPK, Security: xeqtPK})
		add := func(account, position int64, day time.Time, row v9fixture.TransactionRow) string {
			row.Account, row.Position, row.PostedDate, row.Type, row.Amount = account, position, &day, new(addShares), "0"
			pk := b.InvestmentTransaction(row)
			b.Entry(v9fixture.EntryRow{Parent: pk, Amount: "0"})
			return fmt.Sprintf("itxn-%d", pk)
		}
		emptyCostID = add(marginPK, marginPosition, time.Date(2016, 3, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "100"})
		zeroCostID = add(marginPK, marginPosition, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "1", CostBasis: "0"})
		add(marginPK, marginPosition, time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "5", CostBasis: "300"})
		add(tfsaPK, tfsaPosition, time.Date(2021, 5, 1, 0, 0, 0, 0, time.UTC), v9fixture.TransactionRow{Units: "7"})
		b.Lot(v9fixture.LotRow{Position: marginPosition, LatestUnits: "106"})
		b.Lot(v9fixture.LotRow{Position: tfsaPosition, LatestUnits: "7"})
		writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [\"acct-%d\"]\nregistered = [\"acct-%d\"]\n", marginPK, tfsaPK))
		syncLine = syncFindingsBundleIn(t, home, "Documents", b)
	})
	var findingsOut, statusOut, typeOut, stderr bytes.Buffer

	require.Equal(t, 0, run(ctx, []string{"findings"}, &findingsOut, &stderr), stderr.String())
	require.Equal(t, 0, run(ctx, []string{"status"}, &statusOut, &stderr), stderr.String())
	typeExit := run(ctx, []string{"findings", "--type", "shares-without-cost"}, &typeOut, &stderr)
	syncStatus := readSyncStatus(ctx, t, peer)
	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.False(t, result.IsError, textOf(result))
	var listing dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &listing))

	assert.Equal(t, fmt.Sprintf(`Shares added with no cost (2): %s
  shares-without-cost:%s  2026-02-01  Questrade Margin  XEQT  1 share
  shares-without-cost:%s  2016-03-01  Questrade Margin  XEQT  100 shares

2 open findings
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, sharesWithoutCostClause, zeroCostID, emptyCostID, configShown), findingsOut.String())
	assert.Equal(t, "Findings  2 open; run quarry findings to list them", syncLine)
	assert.Contains(t, statusOut.String(), "\nFindings  2 open; run quarry findings to list them\n")
	assert.Equal(t, 2, syncStatus.Findings.Open)
	assert.InDelta(t, 2, listing.Counts["open"], 0)
	assert.Equal(t, 0, typeExit, stderr.String())
	assert.Empty(t, stderr.String())
}

func Test_run_status_json_counts_an_ignored_shares_without_cost_as_ignored(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	marginPK := b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	xeqtPK := b.Security(v9fixture.SecurityRow{Name: "XEQT", Ticker: "XEQT", Currency: "CAD"})
	positionPK := b.Position(v9fixture.PositionRow{Account: marginPK, Security: xeqtPK})
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	addPK := b.InvestmentTransaction(v9fixture.TransactionRow{Account: marginPK, Position: positionPK, PostedDate: &day, Type: new(int64(2)), Amount: "0", Units: "5"})
	b.Entry(v9fixture.EntryRow{Parent: addPK, Amount: "0"})
	b.Lot(v9fixture.LotRow{Position: positionPK, LatestUnits: "5"})
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [\"acct-%d\"]\n[findings]\nignore = [\"shares-without-cost:itxn-%d\"]\n", marginPK, addPK))
	syncFindingsBundleIn(t, home, "Documents", b)

	got := statusFindings(t)

	require.NotNil(t, got.Findings.Ignored)
	assert.Equal(t, []int{0, 1}, []int{got.Findings.Open, *got.Findings.Ignored})
}
