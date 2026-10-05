package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acmeNoCostWarnings is what the fixture prints: the no-cost add's line, then the removal's.
var acmeNoCostWarnings = []string{acmeAddedNoCostWarning, acmeRemovalWarning}

const acmeRemovalWarning = `"Acme Corp": 4 shares left "CAD Brokerage" on 2025-03-03 without a sale; ` +
	"quarry took their share of the ACB out and reports no gain; if they went to a registered account or to someone else, " +
	"that is a disposition at market value; check it with your accountant"

// acbSharesPositionLine is one position table line, each cell as wide as this fixture's widest.
func acbSharesPositionLine(security, ticker, shares, acb, perShare, suffix string) string {
	return strings.TrimRight(fmt.Sprintf("%-9s  %-6s  %6s  %8s  %13s  %s", security, ticker, shares, acb, perShare, suffix), " ") + "\n"
}

// acbSharesRows is one non-registered CAD brokerage whose Acme Corp holding gains 10 bought, 5 added with a
// cost and 5 added without one, then loses 4 removed with no sale.
func acbSharesRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	withCost := acbTrade("inv-add-cost", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.January, 2), 5_000_000, 0)
	withCost.CostBasis = new(int64(60_000))
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		withCost,
		acbTrade("inv-add-free", 3, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.February, 3), 5_000_000, 0),
		acbTrade("inv-remove", 4, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.March, 3), -4_000_000, 0),
	}
	return rows
}

func Test_run_acb_takes_removed_shares_out_of_the_acb_and_adds_added_shares_at_their_cost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, acbSharesRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbSharesPositionLine("Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbSharesPositionLine("Acme Corp", "ACME", "16", "1,280.00", "80.0000", "incomplete"),
		stdout.String())
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr.String())
}

func Test_run_acb_lists_a_removal_warning_in_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, acbSharesRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, acmeNoCostWarnings, doc.Warnings)
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr.String())
}

func Test_run_acb_leaves_a_zero_unit_removal_out_of_the_warnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	rows := acbSharesRows()
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		acbTrade("inv-remove-zero", 5, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.March, 10), 0, 0))
	replaceStore(t, home, rows)

	warnings, stderr := jsonWarnings(t, "acb", "--json")

	assert.Equal(t, acmeNoCostWarnings, warnings)
	assert.Equal(t, stderrWarnings(acmeNoCostWarnings...), stderr)
}

func Test_run_acb_lists_the_configs_warnings_before_a_removal_warning_in_both_forms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "colour = \"red\"\n[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, acbSharesRows())
	var textOut, textErr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"acb"}, spendEnvAt(&textOut, &textErr, holdingsClock())), textErr.String())
	warnings, machineErr := jsonWarnings(t, "acb", "--json")

	wantStderr := stderrWarnings(configShown+orderUnknownKey, acmeAddedNoCostWarning, acmeRemovalWarning)
	assert.Equal(t, []string{configPath(home) + orderUnknownKey, acmeAddedNoCostWarning, acmeRemovalWarning}, warnings)
	assert.Equal(t, wantStderr, textErr.String())
	assert.Equal(t, wantStderr, machineErr)
}
