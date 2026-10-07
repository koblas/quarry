package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/koblas/quarry/internal/quicken/v9/v9fixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	unmatchedConfig = "[accounts]\nregistered = [\"acct-99\"]\nnon-registered = [\"RBC 12345678\"]\n"
	unmatchedTail   = ", which is not an account in quarry's store; quarry skips it"
)

// unmatchedWarningsAt is the two warnings unmatchedConfig draws, naming the config file as shown.
func unmatchedWarningsAt(shown string) []string {
	return []string{
		shown + `: accounts.registered lists "acct-99"` + unmatchedTail,
		shown + `: accounts.non-registered lists "RBC ****5678"` + unmatchedTail,
	}
}

// unmatchedStderr is the stderr lines unmatchedConfig draws from accounts and findings.
const unmatchedStderr = "quarry: warning: " + configShown + `: accounts.registered lists "acct-99"` + unmatchedTail + "\n" +
	"quarry: warning: " + configShown + `: accounts.non-registered lists "RBC ****5678"` + unmatchedTail + "\n"

// syncUnmatchedFixture syncs one chequing account under home and writes unmatchedConfig.
func syncUnmatchedFixture(t *testing.T, home string) {
	t.Helper()
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, unmatchedConfig)
}

// jsonWarnings runs args and returns the warnings array of its --json document and its stderr.
func jsonWarnings(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), args, &stdout, &stderr), stderr.String())
	var doc struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	return doc.Warnings, stderr.String()
}

func Test_run_accounts_json_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)

	warnings, stderr := jsonWarnings(t, "accounts", "--json")

	assert.Equal(t, unmatchedWarningsAt(configPath(home)), warnings)
	assert.Equal(t, unmatchedStderr, stderr)
}

func Test_run_findings_json_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)

	warnings, stderr := jsonWarnings(t, "findings", "--json")

	assert.Equal(t, unmatchedWarningsAt(configPath(home)), warnings)
	assert.Equal(t, unmatchedStderr, stderr)
}

func Test_run_accounts_warns_nothing_about_a_listed_closed_account_without_all(t *testing.T) {
	home := newHome(t)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	closed := b.Account(v9fixture.AccountRow{Name: "Old RRSP", Type: "RETIREMENTIRA", Currency: "CAD", Closed: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [\"acct-%d\"]\n", closed))

	warnings, stderr := jsonWarnings(t, "accounts", "--json")

	assert.Empty(t, warnings)
	assert.NotContains(t, stderr, "which is not an account")
}

func Test_run_mcp_data_quality_names_the_config_by_absolute_path_in_a_no_account_warning(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncUnmatchedFixture(t, h)
	})

	result := callDataQuality(ctx, t, peer, map[string]any{})
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	require.False(t, result.IsError, textOf(result))
	var doc dataQualityDocument
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &doc))
	assert.Equal(t, unmatchedWarningsAt(configPath(home)), doc.Warnings)
}

func Test_run_status_and_sync_stay_silent_about_a_listed_id_that_names_no_account(t *testing.T) {
	home := newHome(t)
	syncUnmatchedFixture(t, home)
	control, _ := jsonWarnings(t, "accounts", "--json")
	require.Equal(t, unmatchedWarningsAt(configPath(home)), control)

	statusWarnings, statusStderr := jsonWarnings(t, "status", "--json")
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	exitCode, _, syncStderr := syncNewBundle(t, home, "DocumentsB", b)

	assert.Empty(t, statusWarnings)
	assert.Empty(t, statusStderr)
	assert.Equal(t, 0, exitCode)
	assert.Empty(t, syncStderr)
}

func Test_run_mcp_sync_status_stays_silent_about_a_listed_id_that_names_no_account(t *testing.T) {
	var home string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		syncUnmatchedFixture(t, h)
	})
	control, _ := jsonWarnings(t, "accounts", "--json")
	require.Equal(t, unmatchedWarningsAt(configPath(home)), control)

	doc := readSyncStatus(ctx, t, peer)
	require.NoError(t, peer.session.Close())
	peer.waitForExit(ctx, t)

	assert.Empty(t, doc.Warnings)
	assert.Empty(t, peer.stderr.String())
}
