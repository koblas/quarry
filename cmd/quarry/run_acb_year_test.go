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
	home := newHome(t)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n")
	replaceStoreWithRates(t, home, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"acb", "--year", "2025"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleLine("Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbSaleLine("2025-06-02", "ACME", "60", "910.00", "10.00", "640.00", "260.00")+
		acbSaleLine("Total", "", "", "910.00", "10.00", "640.00", "260.00"),
		stdout.String())
}

func Test_run_acb_refuses_a_year_it_cannot_use_before_looking_for_a_store(t *testing.T) {
	tests := []struct {
		name   string
		year   string
		stderr string
	}{
		{"not a year", "2024-03", `quarry: --year "2024-03" is not a year; use YYYY, such as 2024` + "\n"},
		{"empty", "", `quarry: --year "" is not a year; use YYYY, such as 2024` + "\n"},
		{"year zero", "0000", `quarry: --year "0000" is not a year; use YYYY, such as 2024` + "\n"},
		{"after this year", "2027", "quarry: --year 2027 is after this year; pass this year or an earlier one\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			var stdout, stderr bytes.Buffer

			exitCode := runWith(context.Background(), []string{"acb", "--year", tc.year},
				spendEnvAt(&stdout, &stderr, holdingsClock()))

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, tc.stderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_acb_year_is_bounded_by_the_injected_clocks_year(t *testing.T) {
	clock := time.Date(2024, time.June, 1, 12, 0, 0, 0, time.UTC)
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())
	var thisOut, thisErr, nextOut, nextErr bytes.Buffer

	thisExit := runWith(context.Background(), []string{"acb", "--year", "2024"}, spendEnvAt(&thisOut, &thisErr, clock))
	nextExit := runWith(context.Background(), []string{"acb", "--year", "2025"}, spendEnvAt(&nextOut, &nextErr, clock))

	require.Equal(t, 0, thisExit, thisErr.String())
	assert.True(t, strings.HasPrefix(thisOut.String(), "Sales in 2024, in CAD\n"), thisOut.String())
	assert.Equal(t, 2, nextExit)
	assert.Equal(t, "quarry: --year 2025 is after this year; pass this year or an earlier one\n", nextErr.String())
	assert.Empty(t, nextOut.String())
}

func Test_run_acb_warns_when_no_non_registered_account_has_traded(t *testing.T) {
	home := newHome(t)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	replaceStoreWithRates(t, home, rows)

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"acb"}, holdingsClock())

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout.String())
	assert.Equal(t, "quarry: warning: "+acbNothingToShowWarning+"\n", stderr.String())
}
