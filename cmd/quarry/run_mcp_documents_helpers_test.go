package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/cli"
	"github.com/koblas/quarry/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// toolClock is the instant both the CLI and the MCP server take as "now".
var toolClock = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// warningsMember is the key every document ends with, so the bytes before it are the document minus its warnings.
const warningsMember = `,"warnings":`

// toolDocumentRun is one CLI --json command and the tool call that must return its document.
type toolDocumentRun struct {
	store     func(t *testing.T, home string)
	config    string
	cliArgs   []string
	tool      string
	arguments map[string]any
}

// toolDocuments are both surfaces' answers to one run: the compact document minus its warnings, and the warnings.
type toolDocuments struct {
	cliBody, toolBody         string
	cliWarnings, toolWarnings []string
}

// runBothSurfaces runs c's CLI command with --json and c's tool call over one store, the clock fixed at toolClock.
func runBothSurfaces(t *testing.T, c toolDocumentRun) toolDocuments {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	c.store(t, home)
	if c.config != "" {
		writeConfig(t, home, c.config)
	}
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, runWith(ctx, slices.Concat(c.cliArgs, []string{"--json"}), spendEnvAt(&stdout, &stderr, toolClock)), stderr.String())
	peer := startClockedMCP(ctx, t)
	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: c.tool, Arguments: c.arguments})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)
	require.False(t, result.IsError, textOf(result))
	require.Empty(t, peer.stderr.String())

	cliBody, cliWarnings := splitWarnings(t, compactJSON(t, stdout.String()))
	toolBody, toolWarnings := splitWarnings(t, textOf(result))
	return toolDocuments{cliBody: cliBody, toolBody: toolBody, cliWarnings: cliWarnings, toolWarnings: toolWarnings}
}

// startClockedMCP connects a client to quarry mcp over the HOME the test set, its clock fixed at toolClock.
func startClockedMCP(ctx context.Context, t *testing.T) *mcpPeer {
	t.Helper()
	return startMCP(ctx, t, func(env *cli.Env) {
		env.ServeMCP = newMCPServe(nil, mcp.WithClock(func() time.Time { return toolClock }))
	})
}

// splitWarnings cuts a compact document into its bytes before the warnings member, closed again, and the warnings.
func splitWarnings(t *testing.T, compact string) (string, []string) {
	t.Helper()
	at := strings.LastIndex(compact, warningsMember)
	require.Positive(t, at, compact)
	var warnings []string
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSuffix(compact[at+len(warningsMember):], "}")), &warnings))
	return compact[:at] + "}", warnings
}

// inToolWords is the CLI warnings with cliWord replaced by tool where the lines say "so <cliWord> leaves it out".
func inToolWords(cliWarnings []string, cliWord, tool string) []string {
	mapped := make([]string, len(cliWarnings))
	for i, line := range cliWarnings {
		mapped[i] = strings.ReplaceAll(line, ", so "+cliWord+" leaves it out", ", so "+tool+" leaves it out")
	}
	return mapped
}
