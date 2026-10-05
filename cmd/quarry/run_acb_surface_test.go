package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbDoc is the part of acb's --json document these tests read.
type acbDoc struct {
	AsOf     string `json:"as_of"`
	Currency string `json:"currency"`
	Year     *int   `json:"year"`
	Years    []struct {
		Year                int    `json:"year"`
		SaleCount           int    `json:"sale_count"`
		Proceeds            string `json:"proceeds"`
		Outlays             string `json:"outlays"`
		ACB                 string `json:"acb"`
		Gain                string `json:"gain"`
		ReturnOfCapitalGain string `json:"return_of_capital_gain"`
		UnknownCostSales    int    `json:"unknown_cost_sales"`
		Sales               []struct {
			UnknownCost bool `json:"unknown_cost"`
		} `json:"sales"`
	} `json:"years"`
	Securities []struct {
		ID          string  `json:"security_id"`
		Shares      string  `json:"shares"`
		ACB         string  `json:"acb"`
		ACBPerShare *string `json:"acb_per_share"`
		Incomplete  bool    `json:"incomplete"`
		Events      []struct {
			Action  string  `json:"action"`
			CAD     *string `json:"cad"`
			Outlays *string `json:"outlays"`
			Gain    *string `json:"gain"`
		} `json:"events"`
	} `json:"securities"`
	Warnings []string `json:"warnings"`
}

func Test_run_acb_prints_the_same_result_as_json(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n")
	replaceStoreWithRates(t, home, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	assert.Equal(t, "2026-03-12", doc.AsOf)
	assert.Equal(t, "CAD", doc.Currency)
	assert.Nil(t, doc.Year)
	require.Len(t, doc.Years, 2)
	assert.Equal(t, 2025, doc.Years[0].Year)
	assert.Equal(t, 1, doc.Years[0].SaleCount)
	assert.Equal(t, []string{"910.00", "10.00", "640.00", "260.00"},
		[]string{doc.Years[0].Proceeds, doc.Years[0].Outlays, doc.Years[0].ACB, doc.Years[0].Gain})
	assert.Equal(t, 2026, doc.Years[1].Year)
	assert.Equal(t, []string{"707.00", "7.00", "500.00", "200.00"},
		[]string{doc.Years[1].Proceeds, doc.Years[1].Outlays, doc.Years[1].ACB, doc.Years[1].Gain})
	require.Len(t, doc.Securities, 2)
	assert.Equal(t, "sec-acme", doc.Securities[0].ID)
	assert.Equal(t, "90.000000", doc.Securities[0].Shares)
	assert.Equal(t, "960.00", doc.Securities[0].ACB)
	assert.Equal(t, "10.6667", *doc.Securities[0].ACBPerShare)
	assert.Empty(t, doc.Warnings)
}

func Test_run_acb_leads_with_the_configs_warnings_in_both_forms(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "snapshot.keep = 3\n[accounts]\nnon-registered = [\"acct-cad\"]\nregistered = [\"acct-usd\", \"acct-rrsp\"]\n")
	replaceStore(t, home, acbRows())
	var textOut, textErr, jsonOut, jsonErr bytes.Buffer

	require.Equal(t, 0, runWith(context.Background(), []string{"acb"}, spendEnvAt(&textOut, &textErr, holdingsClock())), textErr.String())
	require.Equal(t, 0, runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&jsonOut, &jsonErr, holdingsClock())), jsonErr.String())

	assert.Equal(t, "quarry: warning: "+configShown+": unknown key snapshot.keep; quarry ignores it\n", textErr.String())
	assert.Equal(t, textErr.String(), jsonErr.String())
	var doc acbDoc
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &doc), jsonOut.String())
	assert.Equal(t, []string{configPath(home) + ": unknown key snapshot.keep; quarry ignores it"}, doc.Warnings)
}

func Test_run_acb_counts_a_sale_dated_today_in_a_zone_ahead_of_utc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	rows := spendRows([]store.Account{{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true}})
	rows.Securities = []store.Security{{ID: "sec-vanguard", SourceID: 1, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-vanguard", store.ActionBuy, "CAD", day(2026, time.September, 1), 10_000_000, -100_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-vanguard", store.ActionSell, "CAD", day(2026, time.September, 30), -4_000_000, 64_000),
	}
	replaceStore(t, home, rows)
	var stdout, stderr bytes.Buffer
	utcPlus5 := time.FixedZone("UTC+5", 5*60*60)

	exitCode := runWith(context.Background(), []string{"acb"}, spendEnvAt(&stdout, &stderr, time.Date(2026, time.September, 30, 1, 0, 0, 0, utcPlus5)))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearLine("2026", "1", "640.00", "0.00", "400.00", "240.00")+
		"\nACB on 2026-09-30, in CAD\n\n"+
		acbPositionLine("Security", "Ticker", "Shares", "ACB", "ACB per share")+
		acbPositionLine("Vanguard Total Stock", "VTI", "6", "600.00", "100.0000"),
		stdout.String())
}
