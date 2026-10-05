package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbSaleLine is one --year table line, each cell as wide as acbRows' widest.
func acbSaleLine(date, security, shares, proceeds, outlays, acb, gain string) string {
	return strings.TrimRight(fmt.Sprintf("%-10s  %-8s  %6s  %8s  %7s  %6s  %12s", date, security, shares, proceeds, outlays, acb, gain), " ") + "\n"
}

const acbNothingToShowWarning = "no non-registered account has bought or sold a security; quarry acb has nothing to show"

func Test_run_acb_year_lists_that_years_sales_one_by_one_with_a_total(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n")
	replaceStoreWithRates(t, home, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--year", "2025"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleLine("Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbSaleLine("2025-06-02", "ACME", "60", "910.00", "10.00", "640.00", "260.00")+
		acbSaleLine("Total", "", "", "910.00", "10.00", "640.00", "260.00"),
		stdout.String())
}

func Test_run_acb_warns_when_no_non_registered_account_has_traded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	replaceStoreWithRates(t, home, rows)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout.String())
	assert.Equal(t, "quarry: warning: "+acbNothingToShowWarning+"\n", stderr.String())
}
