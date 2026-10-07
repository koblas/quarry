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

const anomaliesLogPrefix = "quarry: mcp: anomalies: "

func Test_run_mcp_anomalies_returns_the_anomalies_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all four given", store: populatedAnalysisStore,
			cliArgs: []string{
				"anomalies", "--since", "2026-01", "--until", "2026-08", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"anomalies"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"anomalies", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{name: "empty window", store: emptyWindowAnalysisStore, cliArgs: []string{"anomalies"}, arguments: map[string]any{}},
		{
			name: "currency absent, config with an unknown key", store: populatedAnalysisStore, config: unknownKeyConfig,
			cliArgs: []string{"anomalies"}, arguments: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "anomalies", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "anomalies", "anomalies"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_anomalies_refuses_a_future_since_in_its_own_words(t *testing.T) {
	home := newHome(t)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "anomalies", Arguments: map[string]any{"since": "2099"},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, "since 2099 is after today; anomalies lists charges up to today only, so pass an earlier since", textOf(result))
	assert.Equal(t, anomaliesLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
}

const (
	recurringChargesLogPrefix      = "quarry: mcp: recurring_charges: "
	linkedLineForRecurringCharges  = `account "Linked" uses linked account tracking in Quicken, so recurring_charges leaves it out, as Quicken's reports do`
	oldCardLineForRecurringCharges = `account "Old Card" is not used in reports in Quicken, so recurring_charges leaves it out; ` +
		`to include it, turn on reports for it in Quicken's account settings, then run quarry sync`
)

func Test_run_mcp_recurring_charges_returns_the_recurring_json_document(t *testing.T) {
	cases := []struct {
		name      string
		store     func(*testing.T, string)
		config    string
		cliArgs   []string
		arguments map[string]any
	}{
		{
			name: "all four given", store: populatedAnalysisStore,
			cliArgs: []string{
				"recurring", "--since", "2026-01", "--until", "2026-08", "--currency", "CAD",
				"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card",
			},
			arguments: map[string]any{
				"since": "2026-01", "until": "2026-08", "currency": "CAD",
				"accounts": []string{"Chequing", "US Chequing", "Linked", "Old Card"},
			},
		},
		{name: "none given", store: populatedAnalysisStore, cliArgs: []string{"recurring"}, arguments: map[string]any{}},
		{
			name: "native currency", store: populatedAnalysisStore,
			cliArgs: []string{"recurring", "--currency", "native"}, arguments: map[string]any{"currency": "native"},
		},
		{name: "empty window", store: emptyWindowAnalysisStore, cliArgs: []string{"recurring"}, arguments: map[string]any{}},
		{
			name: "currency absent, config with an unknown key", store: populatedAnalysisStore, config: unknownKeyConfig,
			cliArgs: []string{"recurring"}, arguments: map[string]any{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runBothSurfaces(t, toolDocumentRun{
				store: c.store, config: c.config, cliArgs: c.cliArgs, tool: "recurring_charges", arguments: c.arguments,
			})

			assert.Equal(t, got.cliBody, got.toolBody)
			assert.Equal(t, inToolWords(got.cliWarnings, "recurring", "recurring_charges"), got.toolWarnings)
		})
	}
}

func Test_run_mcp_recurring_charges_refuses_a_future_since_in_its_own_words(t *testing.T) {
	home := newHome(t)
	populatedAnalysisStore(t, home)
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{
		Name: "recurring_charges", Arguments: map[string]any{"since": "2099"},
	})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, "since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since", textOf(result))
	assert.Equal(t, recurringChargesLogPrefix+windowRefusedLog+"\n", peer.stderr.String())
}

func Test_run_mcp_recurring_charges_words_its_left_out_warnings_with_the_tool_name(t *testing.T) {
	got := runBothSurfaces(t, toolDocumentRun{
		store: populatedAnalysisStore, tool: "recurring_charges",
		cliArgs:   []string{"recurring", "--account", "Linked", "--account", "Old Card"},
		arguments: map[string]any{"accounts": []string{"Linked", "Old Card"}},
	})

	assert.Equal(t, []string{linkedLineForRecurringCharges, oldCardLineForRecurringCharges}, got.toolWarnings)
}

const (
	uncategorizedPayees = 60
	heavyPayeeEntries   = 26
	defaultFindingLimit = 50
	itemsPerFindingCap  = 25
)

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

func Test_run_mcp_data_quality_leaves_out_an_account_the_config_classifies(t *testing.T) {
	var siblingID string
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		b := v9fixture.NewBuilder()
		listedPK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		siblingPK := b.Account(v9fixture.AccountRow{Name: "Questrade Margin", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
		syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
		siblingID = fmt.Sprintf("acct-%d", siblingPK)
		writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", listedPK))
	})

	result := callDataQuality(ctx, t, peer, map[string]any{"type": "unclassified-account"})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	require.Len(t, doc.Findings, 1)
	assert.Equal(t, "unclassified-account:"+siblingID, doc.Findings[0]["id"])
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
	exitCode, stdout, stderr := runCapture(ctx, []string{"findings", "--json"})
	require.Equal(t, 0, exitCode, strings.TrimSpace(stderr.String()))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	return doc
}

func Test_run_mcp_sync_status_returns_the_status_json_document(t *testing.T) {
	ctx, peer := newStatusPeer(t, func(t *testing.T, home string) {
		t.Helper()
		syncStatusFindingsFixture(t, home)
	})
	cliStdout := statusJSON(ctx, t)

	result := callSyncStatus(ctx, t, peer)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	want := compactJSON(t, cliStdout)
	assert.Equal(t, want, textOf(result))
	assert.Equal(t, want, structuredFrame(t, peer.stdout.String()))
	assert.Empty(t, peer.stderr.String())
}

func Test_run_mcp_sync_status_answers_when_the_config_is_unreadable(t *testing.T) {
	const problem = "snapshots.keep must be a whole number of 1 or more, got 0"
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		duplicate := syncStatusFindingsFixture(t, h)
		writeConfig(t, h, fmt.Sprintf("[snapshots]\nkeep = 0\n[findings]\nignore = [%q]\n", duplicate))
	})
	cliStdout := statusJSON(ctx, t)

	result := callSyncStatus(ctx, t, peer)

	require.False(t, result.IsError, textOf(result))
	var doc syncStatusDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Nil(t, doc.Findings.Ignored)
	assert.Equal(t, 4, doc.Findings.Open)
	assert.Equal(t, []string{statusIgnoreWarningLead + configPath(home) + ": " + problem + statusIgnoreWarningTail}, doc.Warnings)
	assert.Equal(t, compactJSON(t, cliStdout), textOf(result))
}

func Test_run_mcp_sync_status_sees_a_sync_between_calls(t *testing.T) {
	t.Run("a re-sync shows the new store", func(t *testing.T) {
		home := newHome(t)
		snapshots := filepath.Join(storeDirUnder(home), "snapshots")
		exitCode, _, stderr := syncNewBundle(t, home, "DocumentsA", accountsBuilder("Chequing"))
		require.Equal(t, 0, exitCode, stderr)
		firstSnapshotID := snapshotID(onlyFileWithSuffix(t, snapshots, ".sqlite"))
		exitCode, _, stderr = syncNewBundle(t, home, "DocumentsB", accountsBuilder("Chequing", "Savings"))
		require.Equal(t, 0, exitCode, stderr)
		ctx, peer := startStatusPeer(t)
		before := readSyncStatus(ctx, t, peer)

		exitCode, _, stderr = runSyncFrom(t, firstSnapshotID)
		require.Equal(t, 0, exitCode, stderr)
		result := callSyncStatus(ctx, t, peer)

		require.False(t, result.IsError, textOf(result))
		var after syncStatusDocument
		require.NoError(t, json.Unmarshal([]byte(textOf(result)), &after))
		assert.Equal(t, 2, before.Store.Rows.Accounts)
		assert.Equal(t, 1, after.Store.Rows.Accounts)
		assert.NotEqual(t, before.Snapshot.ID, after.Snapshot.ID)
		assert.Equal(t, compactJSON(t, statusJSON(ctx, t)), textOf(result))
	})

	t.Run("the config is read on every call", func(t *testing.T) {
		home := newHome(t)
		duplicate := syncStatusFindingsFixture(t, home)
		ctx, peer := startStatusPeer(t)
		before := readSyncStatus(ctx, t, peer)

		writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [%q]\n", duplicate))
		after := readSyncStatus(ctx, t, peer)

		require.NotNil(t, before.Findings.Ignored)
		require.NotNil(t, after.Findings.Ignored)
		assert.Equal(t, 0, *before.Findings.Ignored)
		assert.Equal(t, 1, *after.Findings.Ignored)
		assert.Equal(t, before.Findings.Open-1, after.Findings.Open)
	})
}

func Test_run_mcp_sync_status_refuses_a_store_without_an_import_run(t *testing.T) {
	home := newHome(t)
	syncAccountsFixture(t, home)
	editStore(t, home, "DELETE FROM import_runs")
	var cliStdout, cliStderr bytes.Buffer
	require.Equal(t, 1, run(t.Context(), []string{"status"}, &cliStdout, &cliStderr))
	refusal := strings.TrimSuffix(strings.TrimPrefix(cliStderr.String(), "quarry: "), "\n")
	at := abbreviated(t, storePathUnder(home), home)
	ctx, peer := startStatusPeer(t)

	result := callSyncStatus(ctx, t, peer)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.True(t, result.IsError)
	assert.Equal(t, refusal, textOf(result))
	assert.Contains(t, refusal, "the store has no import history")
	assert.Equal(t, "quarry: mcp: sync_status: cannot read the store at "+at+"; details went to the client only\n", peer.stderr.String())
}

// accountsBuilder is a Quicken file of one CAD chequing account per name, each with one transaction.
func accountsBuilder(names ...string) *v9fixture.Builder {
	b := v9fixture.NewBuilder()
	for _, name := range names {
		account := b.Account(v9fixture.AccountRow{Name: name, Type: "CHECKING", Currency: "CAD", Active: true})
		addTransaction(b, account, "10.00", time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC))
	}
	return b
}

// statusJSON is quarry status --json's stdout over the HOME the test set.
func statusJSON(ctx context.Context, t *testing.T) string {
	t.Helper()
	exitCode, stdout, stderr := runCapture(ctx, []string{"status", "--json"})
	require.Equal(t, 0, exitCode, strings.TrimSpace(stderr.String()))
	return stdout.String()
}
