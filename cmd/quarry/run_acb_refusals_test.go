package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	acbRefusalOneUnclassified = "quarry: acb needs every brokerage and retirement account classified; " +
		"1 account is in neither accounts.registered nor accounts.non-registered in " + configShown +
		"; quarry findings --type unclassified-account --status all lists it\n"
	acbRefusalTwoUnclassified = "quarry: acb needs every brokerage and retirement account classified; " +
		"2 accounts are in neither accounts.registered nor accounts.non-registered in " + configShown +
		"; quarry findings --type unclassified-account --status all lists them\n"
	acbRefusalCADOnly   = "quarry: acb is in CAD only, as the CRA requires; run it without --currency\n"
	acbRefusalBadYear24 = "quarry: --year \"24\" is not a year; use YYYY, such as 2024\n"

	acbPooledConfig = "[accounts]\nnon-registered = [\"acct-pool\"]\n"
)

// acbUnclassifiedRows is a classified pool account that bought ACME, plus each of unclassified, the first of which bought HELD.
func acbUnclassifiedRows(unclassified ...store.Account) store.Rows {
	rows := spendRows(append([]store.Account{
		{ID: "acct-pool", SourceID: 1, Name: "Pool", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	}, unclassified...))
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-held", SourceID: 2, Name: "Held Only", Ticker: new("HELD"), Currency: new("CAD")},
	}
	buy := store.ActionBuy
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-acme-buy", 1, "acct-pool", "sec-acme", buy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
	}
	if len(unclassified) > 0 {
		rows.InvestmentTransactions = append(rows.InvestmentTransactions,
			acbTrade("inv-held-buy", 2, unclassified[0].ID, "sec-held", buy, "CAD", day(2024, time.March, 1), 5_000_000, -50_000))
	}
	return rows
}

func acbOpenBrokerage(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Open " + id, Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true}
}

func acbClosedRetirement(id string, sourceID int64) store.Account {
	return store.Account{ID: id, SourceID: sourceID, Name: "Closed " + id, Type: store.AccountTypeRetirement, Currency: "CAD", Active: false}
}

func Test_run_acb_refuses_an_unclassified_account_and_a_currency_other_than_cad(t *testing.T) {
	one := []store.Account{acbOpenBrokerage("acct-unc", 2)}
	tests := []struct {
		name       string
		config     string
		accounts   []store.Account
		args       []string
		wantExit   int
		wantStderr string
	}{
		{"one open unclassified brokerage account", acbPooledConfig, one, []string{"acb"}, 1, acbRefusalOneUnclassified},
		{
			"an open and a closed unclassified account", acbPooledConfig,
			[]store.Account{acbOpenBrokerage("acct-unc", 2), acbClosedRetirement("acct-old", 3)},
			[]string{"acb"},
			1, acbRefusalTwoUnclassified,
		},
		{
			"an unclassified account whose finding is ignored", "findings.ignore = [\"unclassified-account:acct-unc\"]\n" + acbPooledConfig,
			one,
			[]string{"acb"},
			1, acbRefusalOneUnclassified,
		},
		{"json mode", acbPooledConfig, one, []string{"acb", "--json"}, 1, acbRefusalOneUnclassified},
		{"a security the pool holds", acbPooledConfig, one, []string{"acb", "--security", "ACME"}, 1, acbRefusalOneUnclassified},
		{"a security held only by the unclassified account", acbPooledConfig, one, []string{"acb", "--security", "HELD"}, 1, acbRefusalOneUnclassified},
		{"currency USD", acbPooledConfig, one, []string{"acb", "--currency", "USD"}, 2, acbRefusalCADOnly},
		{"currency native", acbPooledConfig, one, []string{"acb", "--currency", "native"}, 2, acbRefusalCADOnly},
		{"currency in lower case", acbPooledConfig, one, []string{"acb", "--currency", "usd"}, 2, acbRefusalCADOnly},
		{"currency before year", acbPooledConfig, one, []string{"acb", "--currency", "USD", "--year", "24"}, 2, acbRefusalCADOnly},
		{"a bad year", acbPooledConfig, one, []string{"acb", "--year", "24"}, 2, acbRefusalBadYear24},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			writeConfig(t, home, tc.config)
			replaceStoreWithRates(t, home, acbUnclassifiedRows(tc.accounts...))
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), tc.args, spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, tc.wantExit, exitCode)
			assert.Equal(t, tc.wantStderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_acb_counts_the_accounts_that_findings_lists_with_status_all(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "findings.ignore = [\"unclassified-account:acct-ign\"]\n"+acbPooledConfig)
	replaceStoreWithRates(t, home, acbUnclassifiedRows(
		acbOpenBrokerage("acct-unc", 2), acbOpenBrokerage("acct-ign", 3), acbClosedRetirement("acct-old", 4)))
	var findingsOut, findingsErr, acbOut, acbErr bytes.Buffer

	findingsExit := runWith(context.Background(), []string{"findings", "--type", "unclassified-account", "--status", "all", "--json"},
		spendEnvAt(&findingsOut, &findingsErr, holdingsClock()))
	acbExit := runWith(context.Background(), []string{"acb"}, spendEnvAt(&acbOut, &acbErr, holdingsClock()))

	require.Equal(t, 0, findingsExit, findingsErr.String())
	var doc struct {
		Findings []json.RawMessage `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(findingsOut.Bytes(), &doc))
	require.Len(t, doc.Findings, 3)
	assert.Equal(t, 1, acbExit)
	assert.Equal(t, fmt.Sprintf("quarry: acb needs every brokerage and retirement account classified; "+
		"%d accounts are in neither accounts.registered nor accounts.non-registered in %s"+
		"; quarry findings --type unclassified-account --status all lists them\n", len(doc.Findings), configShown), acbErr.String())
}
