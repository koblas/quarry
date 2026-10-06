package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	orderUnknownKey = ": unknown key colour; quarry ignores it"
	orderIgnoreID   = `: findings.ignore lists "uncategorized:payee-999", which is not a finding in quarry's store; quarry skips it`
	orderAccountID  = `: accounts.registered lists "acct-99"` + unmatchedTail
	orderConfig     = "colour = \"red\"\n[findings]\nignore = [\"uncategorized:payee-999\"]\n[accounts]\nregistered = [\"acct-99\"]\n"
)

// findingsWarningsAt is the three warnings orderConfig draws on findings, in the order they are owed, naming the config as shown.
func findingsWarningsAt(shown string) []string {
	return []string{shown + orderUnknownKey, shown + orderIgnoreID, shown + orderAccountID}
}

// stderrWarnings is the stderr text of lines printed as warnings.
func stderrWarnings(lines ...string) string {
	return "quarry: warning: " + strings.Join(lines, "\nquarry: warning: ") + "\n"
}

func Test_run_findings_prints_config_then_unmatched_ignore_then_unmatched_account_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, orderConfig)
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, stderrWarnings(findingsWarningsAt(configShown)...), stderr.String())
}

func Test_run_findings_json_orders_config_then_unmatched_ignore_then_unmatched_account_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncIgnoreFixture(t, home)
	writeConfig(t, home, orderConfig)

	warnings, stderr := jsonWarnings(t, "findings", "--json")

	assert.Equal(t, findingsWarningsAt(configPath(home)), warnings)
	assert.Equal(t, stderrWarnings(findingsWarningsAt(configShown)...), stderr)
}

func Test_run_accounts_orders_config_then_unmatched_account_then_all_closed_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	syncClosedAccountsFixture(t, home, 3)
	writeConfig(t, home, "colour = \"red\"\n[accounts]\nregistered = [\"acct-99\"]\n")
	closed := "all 3 accounts are closed; pass --all to list them"

	warnings, stderr := jsonWarnings(t, "accounts", "--json")

	assert.Equal(t, []string{configPath(home) + orderUnknownKey, configPath(home) + orderAccountID, closed}, warnings)
	assert.Equal(t, stderrWarnings(configShown+orderUnknownKey, configShown+orderAccountID, closed), stderr)
}

func Test_run_mcp_data_quality_orders_config_unmatched_ignore_unmatched_account_then_cap_warnings(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncIgnoreFixture(t, h)
		writeConfig(t, h, orderConfig)
	})

	result := callDataQuality(ctx, t, peer, map[string]any{"limit": 1})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, append(findingsWarningsAt(configPath(home)),
		"listed the first 1 of 2 open findings; pass type to narrow the list, or a larger limit (at most 500)"), doc.Warnings)
}
