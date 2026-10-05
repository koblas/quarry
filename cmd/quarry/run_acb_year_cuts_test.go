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

const (
	acbRegisteredConfig = "[accounts]\nnon-registered = [\"acct-cad\"]\nregistered = [\"acct-rrsp\"]\n"
	acbNonRegistered    = "[accounts]\nnon-registered = [\"acct-cad\"]\n"
)

// acbSaleRowOf is one --year table line with the columns w wide, the last cell unheaded and left-aligned.
func acbSaleRowOf(w [7]int, c [8]string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %*s  %*s  %*s  %*s  %*s  %s",
		w[0], c[0], w[1], c[1], w[2], c[2], w[3], c[3], w[4], c[4], w[5], c[5], w[6], c[6], c[7]), " ") + "\n"
}

// acbJSONYear is the part of acb's --json document the --year cut changes.
type acbJSONYear struct {
	Year  *int `json:"year"`
	Years []struct {
		Year                int    `json:"year"`
		SaleCount           int    `json:"sale_count"`
		Proceeds            string `json:"proceeds"`
		Gain                string `json:"gain"`
		ReturnOfCapitalGain string `json:"return_of_capital_gain"`
		Sales               []struct {
			PossibleSuperficialLoss bool `json:"possible_superficial_loss"`
			UnknownCost             bool `json:"unknown_cost"`
		} `json:"sales"`
	} `json:"years"`
	Securities []struct {
		ID string `json:"security_id"`
	} `json:"securities"`
	Warnings []string `json:"warnings"`
}

// acbFixture stores rows under a fresh HOME with config, and rates when any are given.
func acbFixture(t *testing.T, config string, rows store.Rows, rates ...store.Rate) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeConfig(t, home, config)
	replaceStoreWithRates(t, home, rows, rates...)
}

// runACB runs acb with args at the holdings clock; it must exit 0.
func runACB(t *testing.T, args ...string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer

	exitCode := runWith(context.Background(), append([]string{"acb"}, args...), spendEnvAt(&stdout, &stderr, holdingsClock()))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// decodeACBYear is stdout read as acb's --json document.
func decodeACBYear(t *testing.T, stdout string) acbJSONYear {
	t.Helper()
	var doc acbJSONYear
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
	return doc
}

// acbStackedRows is one 2025 loss sale that is both a possible superficial loss (rebought in a registered account)
// and a sale of shares with unknown cost (10 of 20 held were added free).
func acbStackedRows() store.Rows {
	rows := acbSuperficialRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2025, time.January, 10), 10_000_000, -100_000),
		acbTrade("inv-add", 2, "acct-cad", "sec-acme", store.ActionAddShares, "CAD", day(2025, time.January, 20), 10_000_000, 0),
		acbTrade("inv-sell", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 1), -10_000_000, 30_000),
		acbTrade("inv-rebuy", 4, "acct-rrsp", "sec-acme", store.ActionBuy, "CAD", day(2025, time.March, 15), 5_000_000, -30_000),
	}
	return rows
}

func Test_run_acb_year_marks_a_sale_that_is_both_a_possible_superficial_loss_and_of_unknown_cost(t *testing.T) {
	acbFixture(t, acbRegisteredConfig, acbStackedRows())
	w := [7]int{10, 8, 6, 8, 7, 6, 12}

	textOut, textErr := runACB(t, "--year", "2025")
	docOut, docErr := runACB(t, "--year", "2025", "--json")

	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"2025-03-01", "ACME", "10", "300.00", "0.00", "500.00", "-200.00", "possible superficial loss, unknown cost"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "300.00", "0.00", "500.00", "-200.00"}), textOut)
	doc := decodeACBYear(t, docOut)
	require.Len(t, doc.Years, 1)
	require.Len(t, doc.Years[0].Sales, 1)
	assert.True(t, doc.Years[0].Sales[0].PossibleSuperficialLoss)
	assert.True(t, doc.Years[0].Sales[0].UnknownCost)
	assert.Equal(t, stderrWarnings(superficialLossWarning, acmeAddedNoCostWarning), textErr)
	assert.Equal(t, textErr, docErr)
}

// acbGapRows is acbRows with Acme's sale moved to 2024, so sales fall in 2024 and 2026 and 2025 has none.
func acbGapRows() store.Rows {
	rows := acbRows()
	for i := range rows.InvestmentTransactions {
		if rows.InvestmentTransactions[i].ID == "inv-acme-sell" {
			rows.InvestmentTransactions[i].Date = day(2024, time.June, 2)
		}
	}
	return rows
}

const (
	acbNonRegisteredPair = "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n"
	acbGapSpanWarning    = "no sales in %d in non-registered accounts; the sales are in 2024–2026"
)

func acbGapFixture(t *testing.T) {
	t.Helper()
	acbFixture(t, acbNonRegisteredPair, acbGapRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
}

func Test_run_acb_year_prints_a_year_between_two_with_sales_as_a_zero_total_and_names_their_span(t *testing.T) {
	acbGapFixture(t)
	w := [7]int{5, 8, 6, 8, 7, 4, 12}

	textOut, textErr := runACB(t, "--year", "2025")
	docOut, docErr := runACB(t, "--year", "2025", "--json")

	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}), textOut)
	assert.Equal(t, stderrWarnings(fmt.Sprintf(acbGapSpanWarning, 2025)), textErr)
	doc := decodeACBYear(t, docOut)
	assert.Equal(t, new(2025), doc.Year)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 2025, doc.Years[0].Year)
	assert.Equal(t, 0, doc.Years[0].SaleCount)
	assert.Equal(t, "0.00", doc.Years[0].ReturnOfCapitalGain)
	assert.Empty(t, doc.Years[0].Sales)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{fmt.Sprintf(acbGapSpanWarning, 2025)}, doc.Warnings)
	assert.Equal(t, textErr, docErr)
}

func Test_run_acb_year_before_the_first_sale_names_the_same_span(t *testing.T) {
	acbGapFixture(t)

	_, stderr := runACB(t, "--year", "2023")

	assert.Equal(t, stderrWarnings(fmt.Sprintf(acbGapSpanWarning, 2023)), stderr)
}

func Test_run_acb_year_lists_a_security_whose_only_event_that_year_is_a_return_of_capital_above_its_acb(t *testing.T) {
	acbFixture(t, acbAdjustmentsConfig, acbAdjustmentsRows())
	w := [7]int{5, 27, 6, 8, 7, 4, 12}

	textOut, textErr := runACB(t, "--year", "2025")
	docOut, docErr := runACB(t, "--year", "2025", "--json")

	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"})+
		acbSaleRowOf(w, [8]string{"", "Return of capital above ACB", "", "", "", "", "1,250.00"}), textOut)
	assert.Equal(t, stderrWarnings(acmeReturnOfCapitalWarning), textErr)
	doc := decodeACBYear(t, docOut)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, "1250.00", doc.Years[0].ReturnOfCapitalGain)
	assert.Equal(t, 0, doc.Years[0].SaleCount)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "sec-acme", doc.Securities[0].ID)
	assert.Equal(t, []string{acmeReturnOfCapitalWarning}, doc.Warnings)
	assert.Equal(t, textErr, docErr)
}

// acbNoRateOnlyRows is noRateSharedTickerRows with the USD security's sale moved to 2026, so 2026's only sale is one
// quarry cannot value and 2025's only sale is the CAD security's.
func acbNoRateOnlyRows() store.Rows {
	rows := noRateSharedTickerRows()
	for i := range rows.InvestmentTransactions {
		if rows.InvestmentTransactions[i].ID == "inv-vti-sell" {
			rows.InvestmentTransactions[i].Date = day(2026, time.January, 15)
		}
	}
	return rows
}

func Test_run_acb_year_with_only_a_sale_quarry_cannot_value_warns_of_it_and_of_the_empty_year(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-usd\", \"acct-cad\"]\n", acbNoRateOnlyRows(),
		usdRate(day(2024, time.January, 2), 1_250_000))
	w := [7]int{5, 8, 6, 8, 7, 4, 12}
	empty := "no sales in 2026 in non-registered accounts; the sales are in 2025"

	textOut, textErr := runACB(t, "--year", "2026")
	docOut, docErr := runACB(t, "--year", "2026", "--json")

	assert.Equal(t, "Sales in 2026, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}), textOut)
	assert.Equal(t, stderrWarnings(empty, noRateWarningVTI, sharedTickerWarningVTI, decemberSaleWarning2025), textErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{empty, noRateWarningVTI, sharedTickerWarningVTI, decemberSaleWarning2025}, doc.Warnings)
	assert.Equal(t, textErr, docErr)
}

func Test_run_acb_year_still_warns_of_a_security_it_did_not_sell_that_year(t *testing.T) {
	acbFixture(t, acbNonRegistered, unknownCostRows())
	empty := "no sales in 2024 in non-registered accounts; the sales are in 2025"

	_, defaultErr := runACB(t)
	docOut, yearErr := runACB(t, "--year", "2024", "--json")

	assert.Equal(t, stderrWarnings(acmeAddedNoCostWarning), defaultErr)
	assert.Equal(t, stderrWarnings(empty, acmeAddedNoCostWarning), yearErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{empty, acmeAddedNoCostWarning}, doc.Warnings)
}

func Test_run_acb_leaves_a_pool_that_never_sold_with_its_positions_and_no_empty_warning(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())

	textOut, textErr := runACB(t)
	docOut, _ := runACB(t, "--json")

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %3s  %12s\n", "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-9s  %-6s  %6s  %8s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		fmt.Sprintf("%-9s  %-6s  %6s  %8s  %13s\n", "Acme Corp", "ACME", "10", "1,000.00", "100.0000"), textOut)
	assert.Empty(t, textErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Years)
	assert.Len(t, doc.Securities, 1)
	assert.Empty(t, doc.Warnings)
}

func Test_run_acb_year_of_a_pool_that_never_sold_says_no_year_has_a_sale(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())

	_, stderr := runACB(t, "--year", "2025")

	assert.Equal(t, stderrWarnings("no sales in 2025 in non-registered accounts, nor in any other year"), stderr)
}

// acbAllBeforeRatesRows is one USD security bought and part-sold on dates before the first exchange rate.
func acbAllBeforeRatesRows() store.Rows {
	rows := noRateSharedTickerRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-vti-buy", 1, "acct-usd", "sec-vti", store.ActionBuy, "USD", day(2023, time.December, 1), 10_000_000, -100_000),
		acbTrade("inv-vti-sell", 2, "acct-usd", "sec-vti", store.ActionSell, "USD", day(2023, time.December, 15), -4_000_000, 50_000),
	}
	return rows
}

func Test_run_acb_leaves_a_pool_whose_trades_all_precede_the_rates_with_its_position_and_no_empty_warning(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-usd\"]\nregistered = [\"acct-cad\"]\n", acbAllBeforeRatesRows(), usdRate(day(2024, time.January, 2), 1_250_000))

	textOut, textErr := runACB(t)
	docOut, _ := runACB(t, "--json")

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		fmt.Sprintf("%-4s  %5s  %8s  %7s  %3s  %12s\n", "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		fmt.Sprintf("%-20s  %-6s  %6s  %4s  %13s\n", "Security", "Ticker", "Shares", "ACB", "ACB per share")+
		fmt.Sprintf("%-20s  %-6s  %6s  %4s  %13s  %s\n", "Vanguard Total Stock", "VTI", "6", "0.00", "0.0000", "incomplete"), textOut)
	assert.Equal(t, stderrWarnings(noRateWarningVTI), textErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Years)
	assert.Len(t, doc.Securities, 1)
	assert.Equal(t, []string{noRateWarningVTI}, doc.Warnings)
}

// acbNoPoolEventsRows is one non-registered brokerage that never traded.
func acbNoPoolEventsRows() store.Rows {
	return spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
}

func Test_run_acb_year_with_no_pool_events_prints_a_zero_total_and_says_there_is_nothing_to_show(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbNoPoolEventsRows())
	w := [7]int{5, 8, 6, 8, 7, 4, 12}

	textOut, textErr := runACB(t, "--year", "2025")
	docOut, docErr := runACB(t, "--year", "2025", "--json")

	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}), textOut)
	assert.Equal(t, stderrWarnings(acbNothingToShowWarning), textErr)
	doc := decodeACBYear(t, docOut)
	assert.Equal(t, new(2025), doc.Year)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{acbNothingToShowWarning}, doc.Warnings)
	assert.Equal(t, textErr, docErr)
}

func Test_run_acb_json_with_no_pool_events_has_a_null_year_and_empty_arrays(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbNoPoolEventsRows())

	docOut, _ := runACB(t, "--json")

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(docOut), &doc), docOut)
	assert.JSONEq(t, `null`, string(doc["year"]))
	assert.JSONEq(t, `[]`, string(doc["years"]))
	assert.JSONEq(t, `[]`, string(doc["securities"]))
}

func Test_run_acb_year_json_of_a_pool_that_never_sold_has_the_year_and_no_sales(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())
	empty := "no sales in 2025 in non-registered accounts, nor in any other year"

	docOut, docErr := runACB(t, "--year", "2025", "--json")

	doc := decodeACBYear(t, docOut)
	assert.Equal(t, new(2025), doc.Year)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{empty}, doc.Warnings)
	assert.Equal(t, stderrWarnings(empty), docErr)
}
