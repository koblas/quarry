package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const holdingsNoPriceLine = "1 holding has no price on or before 2026-03-12, so it has no value and is left out of the total; " +
	"enter a price for it in Quicken, then run quarry sync"

// holdingsNoPriceTableLine is one table line wide enough for the "no price" cell; the table ends a line
// at its last non-blank cell.
func holdingsNoPriceTableLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return strings.TrimRight(fmt.Sprintf("%-17s  %-26s  %6s  %8s  %-10s  %-8s  %9s  %9s",
		account, security, shares, price, pricedOn, currency, value, in), " ") + "\n"
}

// seedHoldingsStoreWithUnpricedHolding is seedHoldingsStore plus 40 shares of Bare Fund in the brokerage,
// whose only price is dated the day after the test clock's day.
func seedHoldingsStoreWithUnpricedHolding(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-bare", SourceID: 4, Name: "Bare Fund", Ticker: new("BARE"), Currency: new("CAD")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-bare", 4, "acct-cad", "sec-bare", "CAD", 40_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-bare", SourceID: 4, Date: holdingsDay(13), Price: 7_000_000})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_lists_a_holding_with_no_price_without_value(t *testing.T) {
	seedHoldingsStoreWithUnpricedHolding(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsNoPriceLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNoPriceTableLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNoPriceTableLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNoPriceTableLine("Brokerage", "Bare Fund (BARE)", "40", "no price", "", "CAD", "", "")+
		holdingsNoPriceTableLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsNoPriceTableLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNoPriceTableLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}

// holdingsJSONRow is the fields of one holdings entry the no-price tests read back.
type holdingsJSONRow struct {
	Security       *string `json:"security"`
	Shares         string  `json:"shares"`
	Price          *string `json:"price"`
	PriceDate      *string `json:"price_date"`
	Currency       *string `json:"currency"`
	Value          *string `json:"value"`
	ConvertedValue *string `json:"converted_value"`
}

type holdingsJSONDoc struct {
	Holdings []holdingsJSONRow `json:"holdings"`
	Totals   []struct {
		Currency string `json:"currency"`
		Value    string `json:"value"`
	} `json:"totals"`
	AccountFilter json.RawMessage `json:"account_filter"`
	Warnings      []string        `json:"warnings"`
}

func Test_run_holdings_json_lists_the_unpriced_holding_with_nulls_and_a_warning(t *testing.T) {
	seedHoldingsStoreWithUnpricedHolding(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Holdings, 4)
	assert.Equal(t, holdingsJSONRow{
		Security: new("Acme Corp"), Shares: "1200.000000", Price: new("31.420000"), PriceDate: new("2026-03-09"),
		Currency: new("CAD"), Value: new("37704.00"), ConvertedValue: new("37704.00"),
	}, doc.Holdings[0])
	assert.Equal(t, holdingsJSONRow{
		Security: new("Bare Fund"), Shares: "40.000000", Currency: new("CAD"),
	}, doc.Holdings[1])
	assert.Equal(t, "71290.72", doc.Totals[0].Value)
	assert.JSONEq(t, "[]", string(doc.AccountFilter))
	assert.Equal(t, []string{holdingsNoPriceLine}, doc.Warnings)
}
