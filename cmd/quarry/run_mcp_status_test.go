package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncStatusDocument is the part of sync_status's result these tests read.
type syncStatusDocument struct {
	statusFindingsJSON

	Store struct {
		Rows struct {
			Accounts int `json:"accounts"`
		} `json:"rows"`
	} `json:"store"`
	Snapshot struct {
		ID string `json:"id"`
	} `json:"snapshot"`
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
		home := t.TempDir()
		t.Setenv("HOME", home)
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
		home := t.TempDir()
		t.Setenv("HOME", home)
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
	home := t.TempDir()
	t.Setenv("HOME", home)
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

// newStatusPeer sets a fresh HOME, lets seed build the store under it, and connects a client to quarry mcp.
func newStatusPeer(t *testing.T, seed func(t *testing.T, home string)) (context.Context, *mcpPeer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	seed(t, home)
	return startStatusPeer(t)
}

// startStatusPeer connects a client to quarry mcp over the HOME the test already set.
func startStatusPeer(t *testing.T) (context.Context, *mcpPeer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	t.Cleanup(cancel)
	return ctx, startMCP(ctx, t, func(*cli.Env) {})
}

// statusJSON is quarry status --json's stdout over the HOME the test set.
func statusJSON(ctx context.Context, t *testing.T) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := run(ctx, []string{"status", "--json"}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, strings.TrimSpace(stderr.String()))
	return stdout.String()
}

func callSyncStatus(ctx context.Context, t *testing.T, peer *mcpPeer) *sdk.CallToolResult {
	t.Helper()
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "sync_status", Arguments: map[string]any{}})
	require.NoError(t, err)
	return result
}

// readSyncStatus calls sync_status and decodes the document it returns.
func readSyncStatus(ctx context.Context, t *testing.T, peer *mcpPeer) syncStatusDocument {
	t.Helper()
	result := callSyncStatus(ctx, t, peer)
	require.False(t, result.IsError, textOf(result))
	var doc syncStatusDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	return doc
}
