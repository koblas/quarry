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

// unclassifiedAccountBundle is a chequing control and one brokerage account, with the brokerage account's id.
func unclassifiedAccountBundle() (*v9fixture.Builder, string) {
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	return b, fmt.Sprintf("acct-%d", brokeragePK)
}

// syncUnclassifiedFixture syncs unclassifiedAccountBundle under home, returning the brokerage account's id.
func syncUnclassifiedFixture(t *testing.T, home string) string {
	t.Helper()
	b, id := unclassifiedAccountBundle()
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	return id
}

// statusFindings is quarry status --json over the HOME the test set, decoded.
func statusFindings(t *testing.T) statusFindingsJSON {
	t.Helper()
	var stdout, stderr bytes.Buffer
	require.Equal(t, 0, run(context.Background(), []string{"status", "--json"}, &stdout, &stderr), stderr.String())
	var got statusFindingsJSON
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	return got
}

func Test_run_status_json_counts_an_unclassified_account_open_and_never_new_until_the_config_lists_it(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)

	before := statusFindings(t)
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [%q]\n", id))
	after := statusFindings(t)

	assert.Equal(t, []int{1, 0}, []int{before.Findings.Open, before.Findings.New})
	assert.Equal(t, []int{0, 0}, []int{after.Findings.Open, after.Findings.New})
}

func Test_run_status_json_counts_an_ignored_unclassified_account_as_ignored(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[findings]\nignore = [\"unclassified-account:%s\"]\n", id))

	got := statusFindings(t)

	require.NotNil(t, got.Findings.Ignored)
	assert.Equal(t, []int{0, 1}, []int{got.Findings.Open, *got.Findings.Ignored})
}

func Test_run_status_counts_every_investment_account_open_when_the_config_is_unreadable(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[accounts]\nnon-registered = [%q]\n", id))

	exitCode, stdout, stderr := runCapture(context.Background(), []string{"status"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Contains(t, stdout.String(), "\nFindings  1 open; run quarry findings to list them\n")
	assert.Equal(t, "quarry: warning: "+statusIgnoreWarningLead+configShown+
		": snapshots.keep must be a whole number of 1 or more, got 0"+statusIgnoreWarningTail+"\n", stderr.String())
}

func Test_run_status_json_counts_every_investment_account_open_when_the_config_is_unreadable(t *testing.T) {
	home := newHome(t)
	id := syncUnclassifiedFixture(t, home)
	writeConfig(t, home, fmt.Sprintf("[snapshots]\nkeep = 0\n[accounts]\nnon-registered = [%q]\n", id))

	got := statusFindings(t)

	assert.Equal(t, 1, got.Findings.Open)
	assert.Nil(t, got.Findings.Ignored)
	assert.Equal(t, []string{statusIgnoreWarningLead + configPath(home) + ": snapshots.keep must be a whole number of 1 or more, got 0" + statusIgnoreWarningTail}, got.Warnings)
}

func Test_run_mcp_sync_status_counts_an_unclassified_account_open_until_the_config_lists_it(t *testing.T) {
	var home, id string
	ctx, peer := newStatusPeer(t, func(t *testing.T, h string) {
		t.Helper()
		home = h
		id = syncUnclassifiedFixture(t, h)
	})

	before := readSyncStatus(ctx, t, peer)
	writeConfig(t, home, fmt.Sprintf("[accounts]\nnon-registered = [%q]\n", id))
	after := readSyncStatus(ctx, t, peer)

	assert.Equal(t, []int{1, 0}, []int{before.Findings.Open, after.Findings.Open})
}
