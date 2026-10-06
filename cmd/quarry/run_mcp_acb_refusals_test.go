package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbRefusals are both surfaces' answers to one refused acb call: the CLI's refusal without its "quarry: " prefix,
// and the tool's text and stderr.
type acbRefusals struct{ cli, tool, toolStderr string }

// acbRefusedPair runs the CLI acb with cliArgs and the acb tool with arguments over one store and config, each refused.
func acbRefusedPair(t *testing.T, accounts []store.Account, cliArgs []string, arguments map[string]any) acbRefusals {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbPooledConfig)
	replaceStoreWithRates(t, home, acbUnclassifiedRows(accounts...))
	ctx, cancel := context.WithTimeout(t.Context(), mcpTestDeadline)
	defer cancel()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 1, runWith(ctx, cliArgs, spendEnvAt(&stdout, &stderr, toolClock)))
	peer := startClockedMCP(ctx, t)

	result, err := peer.session.CallTool(ctx, &sdk.CallToolParams{Name: "acb", Arguments: arguments})
	require.NoError(t, err)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.True(t, result.IsError)
	return acbRefusals{
		cli:        strings.TrimSuffix(strings.TrimPrefix(stderr.String(), "quarry: "), "\n"),
		tool:       textOf(result),
		toolStderr: peer.stderr.String(),
	}
}

func Test_run_mcp_acb_refuses_unclassified_accounts_in_the_clis_words_naming_the_tool_that_lists_them(t *testing.T) {
	accounts := []store.Account{acbOpenBrokerage("acct-unc", 2), acbClosedRetirement("acct-old", 3)}

	got := acbRefusedPair(t, accounts, []string{"acb"}, map[string]any{})

	want := strings.Replace(got.cli, "quarry findings --type unclassified-account --status all", "data_quality with type unclassified-account and status all", 1)
	assert.Contains(t, got.cli, "2 accounts are in neither")
	assert.Equal(t, want, got.tool)
	assert.Equal(t, "quarry: mcp: acb: "+want+"\n", got.toolStderr)
}

func Test_run_mcp_acb_refuses_an_unknown_security_in_the_clis_words_without_the_name_on_stderr(t *testing.T) {
	got := acbRefusedPair(t, nil, []string{"acb", "--security", "XYZ"}, map[string]any{"security": []string{"XYZ"}})

	want := strings.Replace(got.cli, "quarry acb --json", "acb with no arguments", 1)
	assert.Contains(t, got.cli, `no security named "XYZ"`)
	assert.Equal(t, want, got.tool)
	assert.Equal(t, "quarry: mcp: acb: refused the call's security; details went to the client only\n", got.toolStderr)
}
