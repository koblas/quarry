package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_run_status_sync_and_mcp_agree_on_an_unclassified_account_count(t *testing.T) {
	var syncLine string
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		b := v9fixture.NewBuilder()
		listedPK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		b.Account(v9fixture.AccountRow{Name: "Old RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Active: true})
		writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", listedPK))
		syncLine = syncFindingsBundleIn(t, home, "Documents", b)
	})
	var statusOut, stderr bytes.Buffer
	require.Equal(t, 0, run(ctx, []string{"status"}, &statusOut, &stderr), stderr.String())

	syncStatus := readSyncStatus(ctx, t, peer)
	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.False(t, result.IsError, textOf(result))
	var listing dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &listing))

	assert.Equal(t, "Findings  2 open; run quarry findings to list them", syncLine)
	assert.Contains(t, statusOut.String(), "\nFindings  2 open; run quarry findings to list them\n")
	assert.Equal(t, 2, syncStatus.Findings.Open)
	assert.InDelta(t, 2, listing.Counts["open"], 0)
}
