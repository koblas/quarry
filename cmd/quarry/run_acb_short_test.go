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

const moneyFundOversoldWarning = `"Money Fund": the sale on 2017-01-12 in "CAD Brokerage" sold 10 more shares than the ` +
	"non-registered accounts held; quarry counts them at no cost, so the sale's gain is too high by what they cost, " +
	"and the next 10 shares acquired only bring the holding back to 0; correct the shares in Quicken if they are wrong"

// oversoldRows is one non-registered brokerage whose Money Fund holding buys 100 for 100.00, sells 110 for
// 110.00, then buys 15 for 15.00.
func oversoldRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{{ID: "sec-fund", SourceID: 1, Name: "Money Fund", Ticker: new("MNY"), Currency: new("CAD")}}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 3), 100_000_000, -10_000),
		acbTrade("inv-sell", 2, "acct-cad", "sec-fund", store.ActionSell, "CAD", day(2017, time.January, 12), -110_000_000, 11_000),
		acbTrade("inv-cover", 3, "acct-cad", "sec-fund", store.ActionBuy, "CAD", day(2017, time.January, 30), 15_000_000, -1_500),
	}

	return rows
}

// shortDoc is the part of acb's --json document the oversold sale reads.
type shortDoc struct {
	Years []struct {
		Year             int `json:"year"`
		UnknownCostSales int `json:"unknown_cost_sales"`
		Sales            []struct {
			ACB         string `json:"acb"`
			Gain        string `json:"gain"`
			UnknownCost bool   `json:"unknown_cost"`
		} `json:"sales"`
	} `json:"years"`
	Securities []struct {
		Security   string `json:"security"`
		Shares     string `json:"shares"`
		ACB        string `json:"acb"`
		Incomplete bool   `json:"incomplete"`
		Events     []struct {
			Action     string `json:"action"`
			SharesHeld string `json:"shares_held"`
			ACB        string `json:"acb"`
		} `json:"events"`
	} `json:"securities"`
}

func Test_run_acb_counts_shares_sold_beyond_the_pool_at_no_cost_and_lets_the_next_buy_cover_the_short(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, "[accounts]\nnon-registered = [\"acct-cad\"]\n")
	replaceStore(t, home, oversoldRows())
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"acb", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc shortDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 1, doc.Years[0].UnknownCostSales)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.Equal(t, "100.00", doc.Years[0].Sales[0].ACB)
	assert.Equal(t, "10.00", doc.Years[0].Sales[0].Gain)
	assert.True(t, doc.Years[0].Sales[0].UnknownCost)
	require.Len(t, doc.Securities, 1)
	events := doc.Securities[0].Events
	require.Len(t, events, 3)
	assert.Equal(t, []string{"sell", "-10.000000", "0.00"}, []string{events[1].Action, events[1].SharesHeld, events[1].ACB})
	assert.Equal(t, []string{"buy", "5.000000", "5.00"}, []string{events[2].Action, events[2].SharesHeld, events[2].ACB})
	assert.Equal(t, []string{"Money Fund", "5.000000", "5.00"},
		[]string{doc.Securities[0].Security, doc.Securities[0].Shares, doc.Securities[0].ACB})
	assert.False(t, doc.Securities[0].Incomplete)
	assert.Equal(t, stderrWarnings(moneyFundOversoldWarning), stderr.String())
}
