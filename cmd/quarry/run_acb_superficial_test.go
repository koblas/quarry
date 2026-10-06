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

const acbSuperficialConfig = "[accounts]\nnon-registered = [\"acct-cad\"]\nregistered = [\"acct-rrsp\"]\n"

const superficialLossWarning = "1 possible superficial loss in 2025: the same security was acquired within 30 days before or after the sale, " +
	"in any account, and still held 30 days after; quarry does not deny or adjust these losses; review them with your accountant"

// acbSuperficialRows is 20 Acme shares bought for 2,000.00 in a non-registered account on 2025-01-10, 10 sold there
// for 600.00 on 2025-03-01, and 5 bought for 300.00 in a registered account on 2025-03-15. Every date is long past holdingsClock.
func acbSuperficialRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		{ID: "acct-rrsp", SourceID: 2, Name: "RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2025, time.January, 10), 20_000_000, -200_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 1), -10_000_000, 60_000),
		acbTrade("inv-rebuy", 3, "acct-rrsp", "sec-acme", store.ActionBuy, "CAD", day(2025, time.March, 15), 5_000_000, -30_000),
	}
	return rows
}

func Test_run_acb_marks_a_loss_sale_rebought_in_a_registered_account_within_30_days(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, acbSuperficialConfig)
	replaceStore(t, home, acbSuperficialRows())
	var stdout, stderr, jsonOut, jsonErr bytes.Buffer

	textExit := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, holdingsClock()))
	jsonExit := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&jsonOut, &jsonErr, holdingsClock()))

	require.Equal(t, 0, textExit, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %8s  %12s\n", "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %8s  %12s  %s\n", "2025", "1", "600.00", "0.00", "1,000.00", "-400.00", "1 possible superficial loss")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-9s  %-6s  %6s  %8s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		fmt.Sprintf("%-9s  %-6s  %6s  %8s  %13s\n", "Acme Corp", "ACME", "10", "1,000.00", "100.0000"),
		stdout.String())
	assert.Equal(t, stderrWarnings(superficialLossWarning), stderr.String())
	require.Equal(t, 0, jsonExit, jsonErr.String())
	var doc struct {
		Years []struct {
			Year                      int    `json:"year"`
			Gain                      string `json:"gain"`
			PossibleSuperficialLosses int    `json:"possible_superficial_losses"`
			Sales                     []struct {
				Gain                    string `json:"gain"`
				PossibleSuperficialLoss bool   `json:"possible_superficial_loss"`
			} `json:"sales"`
		} `json:"years"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].PossibleSuperficialLosses)
	assert.Equal(t, "-400.00", doc.Years[0].Gain)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.True(t, doc.Years[0].Sales[0].PossibleSuperficialLoss)
	assert.Equal(t, "-400.00", doc.Years[0].Sales[0].Gain)
	assert.Equal(t, []string{superficialLossWarning}, doc.Warnings)
	assert.Equal(t, stderr.String(), jsonErr.String())
}
