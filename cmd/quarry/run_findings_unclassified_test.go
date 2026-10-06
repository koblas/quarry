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

func Test_run_findings_lists_an_unclassified_account_until_the_config_classifies_it(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	b.Account(v9fixture.AccountRow{Name: "Chequing", Type: "CHECKING", Currency: "CAD", Active: true})
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	accountID := fmt.Sprintf("acct-%d", brokeragePK)
	var before, after bytes.Buffer
	var stderr bytes.Buffer

	require.Equal(t, 0, run(context.Background(), []string{"findings"}, &before, &stderr), stderr.String())
	writeConfig(t, home, fmt.Sprintf("[accounts]\nregistered = [%q]\n", accountID))
	require.Equal(t, 0, run(context.Background(), []string{"findings"}, &after, &stderr), stderr.String())

	assert.Equal(t, fmt.Sprintf(`Unclassified investment accounts (1): list each account's id (acct-…) in accounts.registered or accounts.non-registered in %s; see quarry findings --help
  unclassified-account:%s  Questrade TFSA  brokerage, CAD

1 open finding
Ignore a finding by adding its id to findings.ignore in %s; see quarry findings --help
`, configShown, accountID, configShown), before.String())
	assert.NotContains(t, after.String(), "unclassified-account")
	assert.Contains(t, after.String(), "No open findings")
	assert.Empty(t, stderr.String())
}

func Test_run_findings_json_reports_an_unclassified_account_without_found_or_fixed_times(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	b := v9fixture.NewBuilder()
	brokeragePK := b.Account(v9fixture.AccountRow{Name: "Questrade TFSA", Type: "BROKERAGENORMAL", Currency: "CAD", Active: true})
	syncBundle(t, b.WriteBundle(t, filepath.Join(home, "Documents")))
	var stdout, stderr bytes.Buffer

	exitCode := run(context.Background(), []string{"findings", "--json", "--type", "unclassified-account"}, &stdout, &stderr)

	require.Equal(t, 0, exitCode, stderr.String())
	var doc struct {
		Findings []struct {
			Status       string  `json:"status"`
			FirstFoundAt *string `json:"first_found_at"`
			FixedAt      *string `json:"fixed_at"`
			Items        []struct {
				AccountID string `json:"account_id"`
				Account   string `json:"account"`
				Currency  string `json:"currency"`
			} `json:"items"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Findings, 1)
	entry := doc.Findings[0]
	assert.Equal(t, "open", entry.Status)
	assert.Nil(t, entry.FirstFoundAt)
	assert.Nil(t, entry.FixedAt)
	require.Len(t, entry.Items, 1)
	assert.Equal(t, fmt.Sprintf("acct-%d", brokeragePK), entry.Items[0].AccountID)
	assert.Equal(t, "Questrade TFSA", entry.Items[0].Account)
	assert.Equal(t, "CAD", entry.Items[0].Currency)
	assert.Empty(t, stderr.String())
}
