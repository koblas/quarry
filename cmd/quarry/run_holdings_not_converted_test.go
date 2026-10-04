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

const (
	holdingsNoCurrencyLine = `"Mystery Fund" has no currency in Quicken, so quarry leaves its value out of the total; ` +
		`set its currency in Quicken, then run quarry sync`
	holdingsOtherCurrencyLine = `"Euro Fund" is priced in EUR, which quarry does not convert, so its value is left out of the total`
)

// holdingsNotConvertedLine is one table line wide enough for the "not converted" cell.
func holdingsNotConvertedLine(account, security, shares, price, pricedOn, currency, value, in string) string {
	return strings.TrimRight(fmt.Sprintf("%-17s  %-26s  %6s  %6s  %-10s  %-8s  %9s  %13s",
		account, security, shares, price, pricedOn, currency, value, in), " ") + "\n"
}

// seedHoldingsStoreWithUnconvertible is seedHoldingsStore plus, in the brokerage, 5 shares of Mystery Fund
// (no currency) at 10.00 and 20 shares of Euro Fund (EUR) at 12.50, both priced on day 9.
func seedHoldingsStoreWithUnconvertible(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rows := holdingsRows()
	rows.Securities = append(rows.Securities,
		store.Security{ID: "sec-null", SourceID: 4, Name: "Mystery Fund", Ticker: new("MYST")},
		store.Security{ID: "sec-eur", SourceID: 5, Name: "Euro Fund", Ticker: new("EURO"), Currency: new("EUR")})
	rows.InvestmentTransactions = append(rows.InvestmentTransactions,
		holdingsBuy("inv-null", 4, "acct-cad", "sec-null", "CAD", 5_000_000),
		holdingsBuy("inv-eur", 5, "acct-cad", "sec-eur", "CAD", 20_000_000))
	rows.Prices = append(rows.Prices,
		store.Price{SecurityID: "sec-null", SourceID: 4, Date: holdingsDay(9), Price: 10_000_000},
		store.Price{SecurityID: "sec-eur", SourceID: 5, Date: holdingsDay(9), Price: 12_500_000})
	replaceStoreWithRates(t, home, rows, usdRate(holdingsDay(10), 1_360_000))
}

func Test_run_holdings_leaves_a_security_it_cannot_convert_out_of_the_total(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsNoCurrencyLine+"\nquarry: warning: "+holdingsOtherCurrencyLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts, amounts in CAD; cash not included\n\n"+
		holdingsNotConvertedLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value", "In CAD")+
		holdingsNotConvertedLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00", "37,704.00")+
		holdingsNotConvertedLine("Brokerage", "Euro Fund (EURO)", "20", "12.50", "2026-03-09", "EUR", "250.00", "not converted")+
		holdingsNotConvertedLine("Brokerage", "Mystery Fund (MYST)", "5", "10.00", "2026-03-09", "none", "50.00", "not converted")+
		holdingsNotConvertedLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35", "33,536.72")+
		holdingsNotConvertedLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00", "50.00")+
		holdingsNotConvertedLine("Total", "", "", "", "", "", "", "71,290.72"),
		stdout.String())
}

func Test_run_holdings_json_lists_an_unconvertible_security_with_a_null_converted_value(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--json"}, spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	var doc holdingsJSONDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Len(t, doc.Holdings, 5)
	assert.Equal(t, holdingsJSONRow{
		Security: new("Euro Fund"), Shares: "20.000000", Price: new("12.500000"), PriceDate: new("2026-03-09"),
		Currency: new("EUR"), Value: new("250.00"),
	}, doc.Holdings[1])
	assert.Equal(t, holdingsJSONRow{
		Security: new("Mystery Fund"), Shares: "5.000000", Price: new("10.000000"), PriceDate: new("2026-03-09"),
		Value: new("50.00"),
	}, doc.Holdings[2])
	require.Len(t, doc.Totals, 1)
	assert.Equal(t, "CAD", doc.Totals[0].Currency)
	assert.Equal(t, "71290.72", doc.Totals[0].Value)
	assert.Equal(t, []string{holdingsNoCurrencyLine, holdingsOtherCurrencyLine}, doc.Warnings)
}

func Test_run_holdings_native_totals_a_security_priced_in_another_currency_and_never_one_with_none(t *testing.T) {
	seedHoldingsStoreWithUnconvertible(t)
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), []string{"holdings", "--currency", "native"},
		spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, "quarry: warning: "+holdingsNoCurrencyLine+"\n", stderr.String())
	assert.Equal(t, "Holdings on 2026-03-12 in all accounts; cash not included\n\n"+
		holdingsNativeLine("Account", "Security", "Shares", "Price", "Priced on", "Currency", "Value")+
		holdingsNativeLine("Brokerage", "Acme Corp (ACME)", "1,200", "31.42", "2026-03-09", "CAD", "37,704.00")+
		holdingsNativeLine("Brokerage", "Euro Fund (EURO)", "20", "12.50", "2026-03-09", "EUR", "250.00")+
		holdingsNativeLine("Brokerage", "Mystery Fund (MYST)", "5", "10.00", "2026-03-09", "none", "50.00")+
		holdingsNativeLine("IRA", "Vanguard Total Stock (VTI)", "85", "290.11", "2026-03-09", "USD", "24,659.35")+
		holdingsNativeLine("Old RRSP (closed)", "Maple Fund", "10", "5.00", "2026-03-05", "CAD", "50.00")+
		holdingsNativeLine("Total", "", "", "", "", "CAD", "37,754.00")+
		holdingsNativeLine("Total", "", "", "", "", "USD", "24,659.35")+
		holdingsNativeLine("Total", "", "", "", "", "EUR", "250.00"),
		stdout.String())
}
