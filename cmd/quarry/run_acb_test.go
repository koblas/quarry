package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// acbYearRow is one realized-gains table line with the ACB column acbW wide, the last cell unheaded.
func acbYearRow(acbW int, year, sales, proceeds, outlays, acb, gain, suffix string) string {
	return strings.TrimRight(fmt.Sprintf("%-4s  %5s  %8s  %7s  %*s  %12s  %s", year, sales, proceeds, outlays, acbW, acb, gain, suffix), " ") + "\n"
}

// acbPositionRow is one ACB position table line with the security column secW wide and the ACB column acbW wide,
// the last cell unheaded.
func acbPositionRow(secW, acbW int, security, ticker, shares, acb, perShare, suffix string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-6s  %6s  %*s  %13s  %s", secW, security, ticker, shares, acbW, acb, perShare, suffix), " ") + "\n"
}

// acbYearLine is one realized-gains table line, each cell as wide as the fixture's widest.
func acbYearLine(year, sales, proceeds, outlays, acb, gain string) string {
	return acbYearRow(6, year, sales, proceeds, outlays, acb, gain, "")
}

// acbPositionLine is one ACB position table line, each cell as wide as the fixture's widest.
func acbPositionLine(security, ticker, shares, acb, perShare string) string {
	return acbPositionRow(20, 6, security, ticker, shares, acb, perShare, "")
}

// acbBrokerageRows is spendRows over one active CAD brokerage, "CAD Brokerage" (acct-cad), holding nothing.
func acbBrokerageRows() store.Rows {
	return spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
}

// acbTrade is a buy or sell of millionths shares of security, in account on date, with amount in cents of currency.
// A sell's shares are stored negative.
func acbTrade(id string, sourceID int64, account, security, action, currency string, date time.Time, millionths, amount int64) store.InvestmentTransaction {
	return store.InvestmentTransaction{
		ID: id, SourceID: sourceID, AccountID: account, SecurityID: &security, Date: date,
		Action: action, Shares: &millionths, Amount: amount, Currency: currency,
	}
}

// acbRows is a CAD and a USD non-registered brokerage and a registered one, trading Acme (CAD), Vanguard (USD)
// and Maple (CAD, registered only) across 2024 to 2026.
func acbRows() store.Rows {
	rows := spendRows([]store.Account{
		{ID: "acct-cad", SourceID: 1, Name: "CAD Brokerage", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
		{ID: "acct-usd", SourceID: 2, Name: "USD Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
		{ID: "acct-rrsp", SourceID: 3, Name: "RRSP", Type: store.AccountTypeBrokerage, Currency: "CAD", Active: true},
	})
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-vti", SourceID: 2, Name: "Vanguard Total Stock", Ticker: new("VTI"), Currency: new("USD")},
		{ID: "sec-maple", SourceID: 3, Name: "Maple Fund", Ticker: new("MPL"), Currency: new("CAD")},
	}
	buy, sell := store.ActionBuy, store.ActionSell
	acmeSale := acbTrade("inv-acme-sell", 4, "acct-cad", "sec-acme", sell, "CAD", day(2025, time.June, 2), -60_000_000, 90_000)
	acmeSale.Commission = new(int64(100_000))
	vtiSale := acbTrade("inv-vti-sell", 6, "acct-usd", "sec-vti", sell, "USD", day(2026, time.February, 2), -4_000_000, 50_000)
	vtiSale.Commission = new(int64(50_000))
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-acme-buy-cad", 1, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.February, 1), 100_000_000, -100_000),
		acbTrade("inv-acme-buy-usd", 2, "acct-usd", "sec-acme", buy, "CAD", day(2024, time.March, 1), 50_000_000, -60_000),
		acbTrade("inv-vti-buy", 3, "acct-usd", "sec-vti", buy, "USD", day(2024, time.May, 1), 10_000_000, -100_000),
		acmeSale,
		acbTrade("inv-maple-buy", 5, "acct-rrsp", "sec-maple", buy, "CAD", day(2024, time.April, 1), 20_000_000, -20_000),
		vtiSale,
		acbTrade("inv-maple-sell", 7, "acct-rrsp", "sec-maple", sell, "CAD", day(2025, time.July, 1), -10_000_000, 12_000),
	}
	return rows
}

func Test_run_acb_prints_gains_per_tax_year_and_todays_acb_pooled_across_the_accounts(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n", acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))

	stdout, stderr := mustRunACB(t)

	assert.Empty(t, stderr)
	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearLine("Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbYearLine("2025", "1", "910.00", "10.00", "640.00", "260.00")+
		acbYearLine("2026", "1", "707.00", "7.00", "500.00", "200.00")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionLine("Security", "Ticker", "Shares", "ACB", "ACB per share")+
		acbPositionLine("Acme Corp", "ACME", "90", "960.00", "10.6667")+
		acbPositionLine("Vanguard Total Stock", "VTI", "6", "750.00", "125.0000"),
		stdout)
}

// acbSaleLine is one --year table line, each cell as wide as acbRows' widest.
func acbSaleLine(date, security, shares, proceeds, outlays, acb, gain string) string {
	return strings.TrimRight(fmt.Sprintf("%-10s  %-8s  %6s  %8s  %7s  %6s  %12s", date, security, shares, proceeds, outlays, acb, gain), " ") + "\n"
}

const acbNothingToShowWarning = "no non-registered account has bought or sold a security; quarry acb has nothing to show"

func Test_run_acb_year_lists_that_years_sales_one_by_one_with_a_total(t *testing.T) {
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n", acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))

	stdout, stderr := mustRunACB(t, "--year", "2025")

	assert.Empty(t, stderr)
	assert.Equal(t, "Sales in 2025, in CAD\n\n"+
		acbSaleLine("Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss")+
		acbSaleLine("2025-06-02", "ACME", "60", "910.00", "10.00", "640.00", "260.00")+
		acbSaleLine("Total", "", "", "910.00", "10.00", "640.00", "260.00"),
		stdout)
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

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"acb", "--year", tc.year}, holdingsClock())

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
	writeConfig(t, home, acbNonRegistered)
	rows := acbBrokerageRows()
	replaceStoreWithRates(t, home, rows)

	stdout, stderr := mustRunACB(t)

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		"Year  Sales  Proceeds  Outlays  ACB  Gain or loss\n"+
		"\nACB on 2026-03-12, in CAD\n\n"+
		"Security  Ticker  Shares  ACB  ACB per share\n", //nolint:dupword // the ACB column sits beside the ACB per share column
		stdout)
	assert.Equal(t, "quarry: warning: "+acbNothingToShowWarning+"\n", stderr)
}

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
	home := newHome(t)
	writeConfig(t, home, config)
	replaceStoreWithRates(t, home, rows, rates...)
}

// mustRunACB runs acb with args at the holdings clock; it must exit 0.
func mustRunACB(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"acb"}, args...), holdingsClock())

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

	textOut, textErr := mustRunACB(t, "--year", "2025")
	docOut, docErr := mustRunACB(t, "--year", "2025", "--json")

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

	textOut, textErr := mustRunACB(t, "--year", "2025")
	docOut, docErr := mustRunACB(t, "--year", "2025", "--json")

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

	_, stderr := mustRunACB(t, "--year", "2023")

	assert.Equal(t, stderrWarnings(fmt.Sprintf(acbGapSpanWarning, 2023)), stderr)
}

func Test_run_acb_year_lists_a_security_whose_only_event_that_year_is_a_return_of_capital_above_its_acb(t *testing.T) {
	acbFixture(t, acbAdjustmentsConfig, acbAdjustmentsRows())
	w := [7]int{5, 27, 6, 8, 7, 4, 12}

	textOut, textErr := mustRunACB(t, "--year", "2025")
	docOut, docErr := mustRunACB(t, "--year", "2025", "--json")

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

	textOut, textErr := mustRunACB(t, "--year", "2026")
	docOut, docErr := mustRunACB(t, "--year", "2026", "--json")

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

	_, defaultErr := mustRunACB(t)
	docOut, yearErr := mustRunACB(t, "--year", "2024", "--json")

	assert.Equal(t, stderrWarnings(acmeAddedNoCostWarning), defaultErr)
	assert.Equal(t, stderrWarnings(empty, acmeAddedNoCostWarning), yearErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{empty, acmeAddedNoCostWarning}, doc.Warnings)
}

func Test_run_acb_leaves_a_pool_that_never_sold_with_its_positions_and_no_empty_warning(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())

	textOut, textErr := mustRunACB(t)
	docOut, _ := mustRunACB(t, "--json")

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearRow(3, "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss", "")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(9, 8, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(9, 8, "Acme Corp", "ACME", "10", "1,000.00", "100.0000", ""), textOut)
	assert.Empty(t, textErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Years)
	assert.Len(t, doc.Securities, 1)
	assert.Empty(t, doc.Warnings)
}

func Test_run_acb_year_of_a_pool_that_never_sold_says_no_year_has_a_sale(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())

	_, stderr := mustRunACB(t, "--year", "2025")

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

	textOut, textErr := mustRunACB(t)
	docOut, _ := mustRunACB(t, "--json")

	assert.Equal(t, "Realized capital gains by tax year, in CAD\n\n"+
		acbYearRow(3, "Year", "Sales", "Proceeds", "Outlays", "ACB", "Gain or loss", "")+
		"\nACB on 2026-03-12, in CAD\n\n"+
		acbPositionRow(20, 4, "Security", "Ticker", "Shares", "ACB", "ACB per share", "")+
		acbPositionRow(20, 4, "Vanguard Total Stock", "VTI", "6", "0.00", "0.0000", "incomplete"), textOut)
	assert.Equal(t, stderrWarnings(noRateWarningVTI), textErr)
	doc := decodeACBYear(t, docOut)
	assert.Empty(t, doc.Years)
	assert.Len(t, doc.Securities, 1)
	assert.Equal(t, []string{noRateWarningVTI}, doc.Warnings)
}

// acbNoPoolEventsRows is one non-registered brokerage that never traded.
func acbNoPoolEventsRows() store.Rows { return acbBrokerageRows() }

func Test_run_acb_year_with_no_pool_events_prints_a_zero_total_and_says_there_is_nothing_to_show(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbNoPoolEventsRows())
	w := [7]int{5, 8, 6, 8, 7, 4, 12}

	textOut, textErr := mustRunACB(t, "--year", "2025")
	docOut, docErr := mustRunACB(t, "--year", "2025", "--json")

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

	docOut, _ := mustRunACB(t, "--json")

	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(docOut), &doc), docOut)
	assert.JSONEq(t, `null`, string(doc["year"]))
	assert.JSONEq(t, `[]`, string(doc["years"]))
	assert.JSONEq(t, `[]`, string(doc["securities"]))
}

func Test_run_acb_year_json_of_a_pool_that_never_sold_has_the_year_and_no_sales(t *testing.T) {
	acbFixture(t, acbNonRegistered, acbAdjustmentsRows())
	empty := "no sales in 2025 in non-registered accounts, nor in any other year"

	docOut, docErr := mustRunACB(t, "--year", "2025", "--json")

	doc := decodeACBYear(t, docOut)
	assert.Equal(t, new(2025), doc.Year)
	assert.Empty(t, doc.Securities)
	assert.Equal(t, []string{empty}, doc.Warnings)
	assert.Equal(t, stderrWarnings(empty), docErr)
}

const acbSecurityConfig = `[accounts]
non-registered = ["acct-cad"]

[[acb.adjustment]]
security = "sec-acme"
date = 2024-06-30
return-of-capital = 150.00
`

// acbHistoryRows is one non-registered CAD brokerage that bought Acme twice and sold part of it, and bought Beta once.
func acbHistoryRows() store.Rows {
	rows := acbBrokerageRows()
	rows.Securities = []store.Security{
		{ID: "sec-acme", SourceID: 1, Name: "Acme Corp", Ticker: new("ACME"), Currency: new("CAD")},
		{ID: "sec-beta", SourceID: 2, Name: "Beta Inc", Ticker: new("BETA"), Currency: new("CAD")},
	}
	buy, sell := store.ActionBuy, store.ActionSell
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-acme-buy-1", 1, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-acme-buy-2", 2, "acct-cad", "sec-acme", buy, "CAD", day(2024, time.March, 1), 5_000_000, -60_000),
		acbTrade("inv-beta-buy", 3, "acct-cad", "sec-beta", buy, "CAD", day(2024, time.April, 1), 1_000_000, -5_000),
		acbTrade("inv-acme-sell", 4, "acct-cad", "sec-acme", sell, "CAD", day(2025, time.June, 2), -6_000_000, 90_000),
	}
	return rows
}

// acbHistoryLine is one history table line, each cell as wide as acbHistoryRows' widest.
func acbHistoryLine(date, account, action, shares, amount, rate, cad, held, acb, gain string) string {
	return strings.TrimRight(fmt.Sprintf("%-10s  %-13s  %-17s  %6s  %13s  %4s  %9s  %11s  %8s  %12s",
		date, account, action, shares, amount, rate, cad, held, acb, gain), " ") + "\n"
}

func Test_run_acb_security_prints_every_event_of_the_named_security_with_shares_held_acb_and_gain(t *testing.T) {
	acbFixture(t, acbSecurityConfig, acbHistoryRows())

	stdout, stderr := mustRunACB(t, "--security", "ACME")

	assert.Empty(t, stderr)
	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryLine("Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss")+
		acbHistoryLine("2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", "")+
		acbHistoryLine("2024-03-01", "CAD Brokerage", "buy", "5", "-600.00 CAD", "", "-600.00", "15", "1,600.00", "")+
		acbHistoryLine("2024-06-30", "", "return of capital", "", "", "", "150.00", "15", "1,450.00", "")+
		acbHistoryLine("2025-06-02", "CAD Brokerage", "sell", "6", "900.00 CAD", "", "900.00", "9", "870.00", "320.00"),
		stdout)
}

// acbHistoryRowOf is one history table line with the first ten columns w wide, the last cell unheaded.
func acbHistoryRowOf(w [10]int, c [11]string) string {
	return strings.TrimRight(fmt.Sprintf("%-*s  %-*s  %-*s  %*s  %*s  %*s  %*s  %*s  %*s  %*s  %s",
		w[0], c[0], w[1], c[1], w[2], c[2], w[3], c[3], w[4], c[4], w[5], c[5], w[6], c[6], w[7], c[7], w[8], c[8], w[9], c[9], c[10]), " ") + "\n"
}

var acbHistoryHeaderCells = [11]string{"Date", "Account", "Action", "Shares", "Amount", "Rate", "CAD", "Shares held", "ACB", "Gain or loss"}

// acbPairFixture stores acbRows with USD rates, Acme and Vanguard pooled and Maple in the registered account.
func acbPairFixture(t *testing.T) {
	t.Helper()
	acbFixture(t, acbNonRegisteredPair, acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))
}

func Test_run_acb_security_prints_a_block_for_each_named_security_in_walk_order_not_argument_order(t *testing.T) {
	acbPairFixture(t)
	acme := [10]int{10, 13, 6, 6, 13, 4, 9, 11, 8, 12}
	vanguard := [10]int{10, 13, 6, 6, 13, 6, 9, 11, 8, 12}

	stdout, stderr := mustRunACB(t, "--security", "vti", "--security", "ACME")

	assert.Empty(t, stderr)
	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryRowOf(acme, acbHistoryHeaderCells)+
		acbHistoryRowOf(acme, [11]string{"2024-02-01", "CAD Brokerage", "buy", "100", "-1,000.00 CAD", "", "-1,000.00", "100", "1,000.00", ""})+
		acbHistoryRowOf(acme, [11]string{"2024-03-01", "USD Brokerage", "buy", "50", "-600.00 CAD", "", "-600.00", "150", "1,600.00", ""})+
		acbHistoryRowOf(acme, [11]string{"2025-06-02", "CAD Brokerage", "sell", "60", "900.00 CAD", "", "900.00", "90", "960.00", "260.00"})+
		"\nACB history of \"Vanguard Total Stock\" (VTI), in CAD\n\n"+
		acbHistoryRowOf(vanguard, acbHistoryHeaderCells)+
		acbHistoryRowOf(vanguard, [11]string{"2024-05-01", "USD Brokerage", "buy", "10", "-1,000.00 USD", "1.2500", "-1,250.00", "10", "1,250.00", ""})+
		acbHistoryRowOf(vanguard, [11]string{"2026-02-02", "USD Brokerage", "sell", "4", "500.00 USD", "1.4000", "700.00", "6", "750.00", "200.00"}),
		stdout)
}

func Test_run_acb_security_names_a_security_by_its_id(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := mustRunACB(t, "--security", "sec-vti")

	assert.True(t, strings.HasPrefix(stdout, "ACB history of \"Vanguard Total Stock\" (VTI), in CAD\n\n"), stdout)
	assert.NotContains(t, stdout, "Acme Corp")
}

func Test_run_acb_security_prints_a_block_for_each_security_sharing_the_ticker(t *testing.T) {
	rows := acbSuperficialRows()
	rows.Securities = []store.Security{
		{ID: "sec-a", SourceID: 1, Name: "Alpha Fund", Ticker: new("DUP"), Currency: new("CAD")},
		{ID: "sec-b", SourceID: 2, Name: "Beta Fund", Ticker: new("DUP"), Currency: new("CAD")},
	}
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-a", 1, "acct-cad", "sec-a", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-b", 2, "acct-cad", "sec-b", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
	}
	acbFixture(t, acbRegisteredConfig, rows)
	w := [10]int{10, 13, 6, 6, 13, 4, 9, 11, 8, 12}
	buy := [11]string{"2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", ""}

	stdout, stderr := mustRunACB(t, "--security", "dup")

	assert.Equal(t, "ACB history of \"Alpha Fund\" (DUP), in CAD\n\n"+acbHistoryRowOf(w, acbHistoryHeaderCells)+acbHistoryRowOf(w, buy)+
		"\nACB history of \"Beta Fund\" (DUP), in CAD\n\n"+acbHistoryRowOf(w, acbHistoryHeaderCells)+acbHistoryRowOf(w, buy), stdout)
	assert.Equal(t, stderrWarnings(`"DUP" is 2 securities in Quicken ("Alpha Fund", "Beta Fund"); `+
		"quarry keeps a separate ACB for each; if they are the same, merge them in Quicken"), stderr)
}

func Test_run_acb_security_prints_only_the_caption_and_header_for_a_security_held_only_in_registered_accounts(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := mustRunACB(t, "--security", "MPL")

	assert.Equal(t, "ACB history of \"Maple Fund\" (MPL), in CAD\n\n"+
		"Date  Account  Action  Shares  Amount  Rate  CAD  Shares held  ACB  Gain or loss\n", stdout)
}

func Test_run_acb_security_warns_a_security_held_only_in_registered_accounts(t *testing.T) {
	acbPairFixture(t)
	want := `"Maple Fund" is held only in registered accounts, so it has no ACB`

	_, stderr := mustRunACB(t, "--security", "Maple Fund")

	doc, docStderr := mustRunACB(t, "--security", "Maple Fund", "--json")

	assert.Equal(t, stderrWarnings(want), stderr)
	assert.Equal(t, stderrWarnings(want), docStderr)
	got := decodeACBYear(t, doc)
	assert.Empty(t, got.Securities)
	assert.Equal(t, []string{want}, got.Warnings)
}

func Test_run_acb_security_gives_a_registered_only_block_beside_a_pooled_one_and_warns_for_the_first_only(t *testing.T) {
	acbPairFixture(t)

	stdout, stderr := mustRunACB(t, "--security", "MPL", "--security", "ACME")

	assert.Equal(t, stderrWarnings(`"Maple Fund" is held only in registered accounts, so it has no ACB`), stderr)
	assert.Equal(t, 1, strings.Count(stdout, "ACB history of \"Acme Corp\" (ACME), in CAD\n"))
	assert.Equal(t, 1, strings.Count(stdout, "ACB history of \"Maple Fund\" (MPL), in CAD\n"))
	assert.Less(t, strings.Index(stdout, "Acme Corp"), strings.Index(stdout, "Maple Fund"))
}

func Test_run_acb_security_writes_only_the_named_securities_and_their_years_in_json(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := mustRunACB(t, "--security", "ACME", "--json")

	doc := decodeACBYear(t, stdout)
	assert.Nil(t, doc.Year)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "sec-acme", doc.Securities[0].ID)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, 2025, doc.Years[0].Year)
	assert.Equal(t, "260.00", doc.Years[0].Gain)
	assert.Equal(t, "910.00", doc.Years[0].Proceeds)
}

func Test_run_acb_security_with_year_prints_the_years_sales_of_the_named_security_only(t *testing.T) {
	sold := [7]int{10, 8, 6, 8, 7, 6, 12}
	none := [7]int{5, 8, 6, 8, 7, 4, 12}
	header := [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"}
	cases := []struct {
		name     string
		security string
		want     string
	}{
		{
			name: "a security sold that year", security: "VTI",
			want: "Sales in 2026, in CAD\n\n" +
				acbSaleRowOf(sold, header) +
				acbSaleRowOf(sold, [8]string{"2026-02-02", "VTI", "4", "707.00", "7.00", "500.00", "200.00"}) +
				acbSaleRowOf(sold, [8]string{"Total", "", "", "707.00", "7.00", "500.00", "200.00"}),
		},
		{
			name: "a security that sold nothing that year", security: "ACME",
			want: "Sales in 2026, in CAD\n\n" +
				acbSaleRowOf(none, header) +
				acbSaleRowOf(none, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acbPairFixture(t)

			stdout, stderr := mustRunACB(t, "--year", "2026", "--security", c.security)

			assert.Empty(t, stderr)
			assert.Equal(t, c.want, stdout)
		})
	}
}

func Test_run_acb_security_with_year_for_a_security_held_only_in_registered_accounts(t *testing.T) {
	acbPairFixture(t)
	w := [7]int{5, 8, 6, 8, 7, 4, 12}
	want := `"Maple Fund" is held only in registered accounts, so it has no ACB`

	stdout, stderr := mustRunACB(t, "--year", "2026", "--security", "MPL")

	doc, docStderr := mustRunACB(t, "--year", "2026", "--security", "MPL", "--json")

	assert.Equal(t, "Sales in 2026, in CAD\n\n"+
		acbSaleRowOf(w, [8]string{"Date", "Security", "Shares", "Proceeds", "Outlays", "ACB", "Gain or loss"})+
		acbSaleRowOf(w, [8]string{"Total", "", "", "0.00", "0.00", "0.00", "0.00"}), stdout)
	assert.Equal(t, stderrWarnings(want), stderr)
	assert.Equal(t, stderr, docStderr)
	got := decodeACBYear(t, doc)
	assert.NotNil(t, got.Securities)
	assert.Empty(t, got.Securities)
	assert.Equal(t, []string{want}, got.Warnings)
}

func Test_run_acb_security_with_year_writes_that_year_re_summed_over_the_named_security_in_json(t *testing.T) {
	acbPairFixture(t)

	stdout, _ := mustRunACB(t, "--year", "2026", "--security", "VTI", "--json")

	doc := decodeACBYear(t, stdout)
	require.NotNil(t, doc.Year)
	assert.Equal(t, 2026, *doc.Year)
	require.Len(t, doc.Years, 1)
	assert.Equal(t, "707.00", doc.Years[0].Proceeds)
	require.Len(t, doc.Securities, 1)
	assert.Equal(t, "sec-vti", doc.Securities[0].ID)
}

func Test_run_acb_security_refuses_a_name_that_matches_no_security(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "an unknown name", args: []string{"--security", "XYZ"}, want: "XYZ"},
		{name: "an empty name", args: []string{"--security", ""}, want: ""},
		{name: "one unknown name beside a known one", args: []string{"--security", "ACME", "--security", "XYZ"}, want: "XYZ"},
		{name: "the first unknown name in argument order", args: []string{"--security", "NOPE-2", "--security", "ACME", "--security", "NOPE-1"}, want: "NOPE-2"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			acbPairFixture(t)

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), append([]string{"acb"}, c.args...), holdingsClock())

			assert.Equal(t, 1, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, fmt.Sprintf("quarry: acb covers no security named %q; quarry acb --json lists every security it covers\n", c.want), stderr.String())
		})
	}
}

// acbReinvestRows is one non-registered CAD brokerage whose Acme holding buys 10 for 1,000.00, reinvests a dividend
// into 1 share with no cost, then sells 2 for 300.00 and removes 1.
func acbReinvestRows() store.Rows {
	rows := acbSuperficialRows()
	rows.InvestmentTransactions = []store.InvestmentTransaction{
		acbTrade("inv-buy", 1, "acct-cad", "sec-acme", store.ActionBuy, "CAD", day(2024, time.February, 1), 10_000_000, -100_000),
		acbTrade("inv-reinvest", 2, "acct-cad", "sec-acme", store.ActionReinvestDividend, "CAD", day(2024, time.March, 1), 1_000_000, 0),
		acbTrade("inv-sell", 3, "acct-cad", "sec-acme", store.ActionSell, "CAD", day(2025, time.March, 3), -2_000_000, 30_000),
		acbTrade("inv-remove", 4, "acct-cad", "sec-acme", store.ActionRemoveShares, "CAD", day(2025, time.June, 2), -1_000_000, 0),
	}
	return rows
}

func Test_run_acb_security_lists_the_reinvested_dividends_the_no_cost_warning_names_and_marks_each_unknown_cost(t *testing.T) {
	acbFixture(t, acbRegisteredConfig, acbReinvestRows())
	_, warned := mustRunACB(t)
	advice := regexp.MustCompile(`quarry (acb --security \S+) lists them`).FindStringSubmatch(warned)
	require.Len(t, advice, 2, warned)
	args := strings.Fields(advice[1])
	w := [10]int{10, 13, 17, 6, 13, 4, 9, 11, 8, 12}

	stdout, _ := mustRunACB(t, args[1:]...)

	assert.Equal(t, "ACB history of \"Acme Corp\" (ACME), in CAD\n\n"+
		acbHistoryRowOf(w, acbHistoryHeaderCells)+
		acbHistoryRowOf(w, [11]string{"2024-02-01", "CAD Brokerage", "buy", "10", "-1,000.00 CAD", "", "-1,000.00", "10", "1,000.00", ""})+
		acbHistoryRowOf(w, [11]string{"2024-03-01", "CAD Brokerage", "reinvest_dividend", "1", "0.00 CAD", "", "0.00", "11", "1,000.00", "", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2025-03-03", "CAD Brokerage", "sell", "2", "300.00 CAD", "", "300.00", "9", "818.18", "118.18", "unknown cost"})+
		acbHistoryRowOf(w, [11]string{"2025-06-02", "CAD Brokerage", "remove_shares", "1", "0.00 CAD", "", "0.00", "8", "727.27", "", "unknown cost"}),
		stdout)
}

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
	acbFixture(t, "[accounts]\nnon-registered = [\"acct-cad\", \"acct-usd\"]\nregistered = [\"acct-rrsp\"]\n", acbRows(),
		usdRate(day(2024, time.January, 2), 1_250_000), usdRate(day(2026, time.January, 2), 1_400_000))

	stdout, stderr := mustRunACB(t, "--json")

	assert.Empty(t, stderr)
	var doc acbDoc
	require.NoError(t, json.Unmarshal([]byte(stdout), &doc), stdout)
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
	home := newHome(t)
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
	home := newHome(t)
	writeConfig(t, home, acbNonRegistered)
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
		{"currency CAD", acbPooledConfig, one, []string{"acb", "--currency", "CAD"}, 1, acbRefusalOneUnclassified},
		{"a year", acbPooledConfig, one, []string{"acb", "--year", "2024"}, 1, acbRefusalOneUnclassified},
		{"currency USD", acbPooledConfig, one, []string{"acb", "--currency", "USD"}, 2, acbRefusalCADOnly},
		{"currency USD in json mode", acbPooledConfig, one, []string{"acb", "--currency", "USD", "--json"}, 2, acbRefusalCADOnly},
		{"currency native", acbPooledConfig, one, []string{"acb", "--currency", "native"}, 2, acbRefusalCADOnly},
		{"currency in lower case", acbPooledConfig, one, []string{"acb", "--currency", "usd"}, 2, acbRefusalCADOnly},
		{"currency before year", acbPooledConfig, one, []string{"acb", "--currency", "USD", "--year", "24"}, 2, acbRefusalCADOnly},
		{"a bad year", acbPooledConfig, one, []string{"acb", "--year", "24"}, 2, acbRefusalBadYear24},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := newHome(t)
			writeConfig(t, home, tc.config)
			replaceStoreWithRates(t, home, acbUnclassifiedRows(tc.accounts...))

			exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), tc.args, holdingsClock())

			assert.Equal(t, tc.wantExit, exitCode)
			assert.Equal(t, tc.wantStderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_acb_refuses_a_year_and_a_currency_before_reading_a_malformed_config(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"a bad year", []string{"acb", "--year", "24"}, acbRefusalBadYear24},
		{"currency USD", []string{"acb", "--currency", "USD"}, acbRefusalCADOnly},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			malformedConfigFixture(t)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), tc.args)

			assert.Equal(t, 2, exitCode)
			assert.Equal(t, tc.wantStderr, stderr.String())
			assert.Empty(t, stdout.String())
		})
	}
}

func Test_run_acb_refuses_the_one_account_a_missing_config_leaves_unclassified(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, acbUnclassifiedRows())

	exitCode, stdout, stderr := runSpendCaptureAt(context.Background(), []string{"acb"}, holdingsClock())

	assert.Equal(t, 1, exitCode)
	assert.Equal(t, acbRefusalOneUnclassified, stderr.String())
	assert.Empty(t, stdout.String())
}

func Test_run_acb_counts_the_accounts_that_findings_lists_with_status_all(t *testing.T) {
	acbFixture(t, "findings.ignore = [\"unclassified-account:acct-ign\"]\n"+acbPooledConfig, acbUnclassifiedRows(
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
