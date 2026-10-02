package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uncategorizedPayees = 60
	heavyPayeeEntries   = 26
	defaultFindingLimit = 50
	itemsPerFindingCap  = 25
)

// dataQualityDocument is the findings list document, with findings left as decoded maps for comparison.
type dataQualityDocument struct {
	Status   string           `json:"status"`
	Type     *string          `json:"type"`
	Counts   map[string]any   `json:"counts"`
	Findings []map[string]any `json:"findings"`
	Warnings []string         `json:"warnings"`
}

func Test_run_mcp_data_quality_returns_the_first_50_open_findings_with_the_ruled_warnings(t *testing.T) {
	ctx, peer := newStatusPeer(t, syncManyUncategorizedPayees)
	oracle := findingsJSON(ctx, t)
	require.EqualValues(t, uncategorizedPayees+1, oracle.Counts["open"])
	require.Len(t, oracle.Findings, uncategorizedPayees+1)
	heavyID := oracle.Findings[0]["id"]
	require.Len(t, oracle.Findings[0]["items"], heavyPayeeEntries)

	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, "open", doc.Status)
	assert.Nil(t, doc.Type)
	assert.Equal(t, oracle.Counts, doc.Counts)
	assert.Equal(t, cutFindings(oracle.Findings, defaultFindingLimit, itemsPerFindingCap), doc.Findings)
	assert.Equal(t, []string{
		"listed the first 50 of 61 open findings; pass type to narrow the list, or a larger limit (at most 500)",
		fmt.Sprintf("finding %[1]v lists the first 25 of 26 items; query finding_items WHERE finding_id = '%[1]v' for the rest", heavyID),
	}, doc.Warnings)
	assert.Empty(t, peer.stderr.String())
}

func Test_run_mcp_data_quality_refuses_an_unreadable_config(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncStatusFindingsFixture(t, h)
	})
	writeConfig(t, home, "snapshots.keep = 0\n")
	var cliStdout, cliStderr bytes.Buffer
	require.Equal(t, 1, run(ctx, []string{"findings"}, &cliStdout, &cliStderr))
	refusal := strings.TrimSuffix(strings.TrimPrefix(cliStderr.String(), "quarry: "), "\n")

	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, refusal, textOf(result))
	assert.Contains(t, refusal, configShown)
	assert.Equal(t, "quarry: mcp: data_quality: cannot read quarry's config file; run quarry findings to see why\n", peer.stderr.String())
	assert.NotContains(t, peer.stderr.String(), home)
}

// syncManyUncategorizedPayees syncs a file of 60 payees with one uncategorized split each, plus a payee whose
// one transaction splits 26 ways, so the only findings are uncategorized and the heavy payee sorts first.
func syncManyUncategorizedPayees(t *testing.T, home string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	chequing := b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	for i := range uncategorizedPayees {
		amount := fmt.Sprintf("-%d.00", i+1)
		day := time.Date(2026, 3, 1+i, 0, 0, 0, 0, time.UTC)
		txn := b.Transaction(v9fixture.TransactionRow{Account: chequing, Amount: amount, PostedDate: &day, Payee: b.Payee(v9fixture.PayeeRow{Name: lettersOnlyPayee(i)})})
		b.Entry(v9fixture.EntryRow{Parent: txn, Amount: amount})
	}
	heavyDay := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	heavy := b.Transaction(v9fixture.TransactionRow{
		Account: chequing, Amount: fmt.Sprintf("-%d0.00", heavyPayeeEntries), PostedDate: &heavyDay,
		Payee: b.Payee(v9fixture.PayeeRow{Name: "Heavy Hardware"}),
	})
	for range heavyPayeeEntries {
		b.Entry(v9fixture.EntryRow{Parent: heavy, Amount: "-10.00"})
	}
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
}

// lettersOnlyPayee names payee i by two letters, so no two payee keys agree and none contains a digit.
func lettersOnlyPayee(i int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	return "Vendor " + letters[i/26:i/26+1] + letters[i%26:i%26+1]
}

// cutFindings is the first limit of findings with each one's items cut to itemCap, leaving findings untouched.
func cutFindings(findings []map[string]any, limit, itemCap int) []map[string]any {
	kept := make([]map[string]any, 0, limit)
	for _, f := range findings[:limit] {
		cut := maps.Clone(f)
		if items, ok := f["items"].([]any); ok && len(items) > itemCap {
			cut["items"] = items[:itemCap]
		}
		kept = append(kept, cut)
	}
	return kept
}

// findingsJSON is quarry findings --json's document over the HOME the test set.
func findingsJSON(ctx context.Context, t *testing.T) dataQualityDocument {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := run(ctx, []string{"findings", "--json"}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, strings.TrimSpace(stderr.String()))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	return doc
}

func callDataQuality(ctx context.Context, t *testing.T, peer *mcpPeer, arguments map[string]any) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "data_quality", Arguments: arguments})
	require.NoError(t, err)
	return result
}
