// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cashFlowLine is one cash-flow table line with every column but the last as wide as the fixtures' widest cells.
func cashFlowLine(periodWidth int, period, currency, income, spent, net, rate, status string) string {
	line := fmt.Sprintf("%-*s  %-8s  %9s  %9s  %9s  %12s", periodWidth, period, currency, income, spent, net, rate)
	if status != "" {
		line += "  " + status
	}
	return line + "\n"
}

func Test_run_cashflow_shows_income_spending_and_savings_rate_by_month(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 910000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -300000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 1, 20), cents: -320000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 2, 28), cents: 500000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 4, 15), cents: -100000},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 9, 15), cents: 602000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 9, 20), cents: -511040},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"cashflow"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const w = 7
	assert.Equal(t, "Cash flow 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
		cashFlowLine(w, "Month", "Currency", "Income", "Spent", "Net", "Savings rate", "Status")+
		cashFlowLine(w, "2026-01", "CAD", "9,100.00", "6,200.00", "2,900.00", "31.9%", "")+
		cashFlowLine(w, "2026-02", "CAD", "5,000.00", "0.00", "5,000.00", "100.0%", "")+
		cashFlowLine(w, "2026-03", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-04", "CAD", "0.00", "1,000.00", "-1,000.00", "n/a", "")+
		cashFlowLine(w, "2026-05", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-06", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-07", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-08", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2026-09", "CAD", "6,020.00", "5,110.40", "909.60", "15.1%", "partial")+
		cashFlowLine(w, "Total", "CAD", "20,120.00", "12,310.40", "7,809.60", "38.8%", ""),
		stdout.String())
}

func Test_run_cashflow_by_year_shows_one_row_per_year_and_na_without_income(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2020, 6, 1), cents: 1000000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2020, 7, 1), cents: -400000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2021, 3, 1), cents: 300000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2022, 5, 1), cents: -50000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2024, 2, 1), cents: 100000},
		spendSplit{id: "s06", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2024, 8, 1), cents: -200000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2025, 4, 1), cents: 800000},
		spendSplit{id: "s08", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 11, 1), cents: -600000},
		spendSplit{id: "s09", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 5), cents: 999900},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(),
		[]string{"cashflow", "--by", "year", "--since", "2020", "--until", "2025"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	const w = 5
	assert.Equal(t, "Cash flow 2020-01-01 to 2025-12-31 in all accounts, amounts in CAD\n\n"+
		cashFlowLine(w, "Year", "Currency", "Income", "Spent", "Net", "Savings rate", "Status")+
		cashFlowLine(w, "2020", "CAD", "10,000.00", "4,000.00", "6,000.00", "60.0%", "")+
		cashFlowLine(w, "2021", "CAD", "3,000.00", "0.00", "3,000.00", "100.0%", "")+
		cashFlowLine(w, "2022", "CAD", "0.00", "500.00", "-500.00", "n/a", "")+
		cashFlowLine(w, "2023", "CAD", "0.00", "0.00", "0.00", "n/a", "")+
		cashFlowLine(w, "2024", "CAD", "1,000.00", "2,000.00", "-1,000.00", "-100.0%", "")+
		cashFlowLine(w, "2025", "CAD", "8,000.00", "6,000.00", "2,000.00", "25.0%", "")+
		cashFlowLine(w, "Total", "CAD", "22,000.00", "12,500.00", "9,500.00", "43.2%", ""),
		stdout.String())
}

// cashFlowReport is the part of cashflow's --json document the currency tests read.
type cashFlowReport struct {
	Currency string         `json:"currency"`
	Periods  []cashFlowMark `json:"periods"`
	Totals   []cashFlowMark `json:"totals"`
	Warnings []string       `json:"warnings"`
}

// cashFlowMark is one amount of a cashflow document: a period's row and a total alike.
type cashFlowMark struct {
	Period         string   `json:"period"`
	Currency       string   `json:"currency"`
	Income         string   `json:"income"`
	Spent          string   `json:"spent"`
	Net            string   `json:"net"`
	SavingsRatePct *float64 `json:"savings_rate_pct"`
}

func Test_run_cashflow_converts_each_period_to_cad_by_default(t *testing.T) {
	// Two 0.10 USD splits at 1.25 make 0.26 rounded each, 0.25 summed first.
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 100000},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 1, 30), cents: 8000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -20000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 11), cents: -10},
		spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 12), cents: -10},
		spendSplit{id: "s06", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 2, 5), cents: -8000},
		spendSplit{id: "s07", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 2, 6), cents: -1000},
	), rateOnJan2)
	period := []string{"--since", "2026-01", "--until", "2026-02"}

	t.Run("text", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, period...))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, `Cash flow 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD

Month    Currency    Income   Spent      Net  Savings rate  Status
2026-01  CAD       1,100.00  200.26   899.74         81.8%
2026-02  CAD           0.00  110.00  -110.00           n/a
Total    CAD       1,100.00  310.26   789.74         71.8%
`, stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow", "--json"}, period...))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		var doc cashFlowReport
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
		assert.Equal(t, cashFlowReport{
			Currency: "CAD",
			Periods: []cashFlowMark{
				{Period: "2026-01", Currency: "CAD", Income: "1100.00", Spent: "200.26", Net: "899.74", SavingsRatePct: new(81.8)},
				{Period: "2026-02", Currency: "CAD", Income: "0.00", Spent: "110.00", Net: "-110.00"},
			},
			Totals:   []cashFlowMark{{Currency: "CAD", Income: "1100.00", Spent: "310.26", Net: "789.74", SavingsRatePct: new(71.8)}},
			Warnings: []string{},
		}, doc)
	})
}

// runCashFlowJSON runs cashflow with args plus --json and decodes its document.
func runCashFlowJSON(t *testing.T, args ...string) (cashFlowReport, string) {
	t.Helper()
	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow", "--json"}, args...))
	require.Equal(t, 0, exitCode, stderr.String())
	var doc cashFlowReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc, stderr.String()
}

// cashFlowTextView is the first line of a cashflow table and the cells of each of its Total rows.
func cashFlowTextView(out string) (string, [][]string) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var totals [][]string
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "Total") {
			totals = append(totals, strings.Fields(line))
		}
	}
	return lines[0], totals
}

// totalMarkCells are the Total rows of a text table for totals, as cashFlowTextView reads them.
func totalMarkCells(totals []cashFlowMark) [][]string {
	cells := make([][]string, 0, len(totals))
	for _, t := range totals {
		cells = append(cells, []string{"Total", t.Currency, t.Income, t.Spent, t.Net, fmt.Sprintf("%.1f%%", *t.SavingsRatePct)})
	}
	return cells
}

func Test_run_cashflow_converts_the_edge_cases_in_each_reporting_currency(t *testing.T) {
	const thisYear = "Cash flow 2026-01-01 to 2026-09-29 in "
	cad := []cashFlowMark{{Currency: "CAD", Income: "100.00", Spent: "50.00", Net: "50.00", SavingsRatePct: new(50.0)}}
	usd := []cashFlowMark{{Currency: "USD", Income: "80.00", Spent: "40.00", Net: "40.00", SavingsRatePct: new(50.0)}}
	fridayRate := store.Rate{Date: day(2026, 3, 13), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}
	closedUSD := store.Account{ID: "acct-usd", SourceID: 2, Name: "US Chequing", Type: "chequing", Currency: "USD", Closed: true}
	usdSplits := func(when time.Time) []spendSplit {
		return []spendSplit{
			{id: "s1", account: "acct-usd", category: "cat-salary", currency: "USD", day: when, cents: 8000},
			{id: "s2", account: "acct-usd", category: "cat-groceries", currency: "USD", day: when, cents: -4000},
		}
	}
	cadSplit := spendSplit{id: "s3", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 10000}
	cases := []struct {
		name       string
		accounts   []store.Account
		splits     []spendSplit
		rate       store.Rate
		args       []string
		caption    string
		currencies []string
		want       map[string][]cashFlowMark
	}{
		{
			name:     "a closed USD account converts",
			accounts: []store.Account{closedUSD}, splits: usdSplits(day(2026, 3, 11)), rate: rateOnJan2,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]cashFlowMark{"CAD": cad, "USD": usd, "native": usd},
		},
		{
			name:     "one USD account converts to a CAD total",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			splits:   append(usdSplits(day(2026, 3, 11)), cadSplit), rate: rateOnJan2,
			args: []string{"--account", "US Chequing"}, caption: thisYear + "US Chequing", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]cashFlowMark{"CAD": cad, "USD": usd, "native": usd},
		},
		{
			name:     "a split dated after the last rate converts at the latest rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: usdSplits(day(2099, 6, 1)), rate: rateOnJan2,
			args: []string{"--until", "2099-12-31"}, caption: "Cash flow 2026-01-01 to 2099-12-31 in all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]cashFlowMark{"CAD": cad, "USD": usd},
		},
		{
			name:     "a weekend split converts at the Friday rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: usdSplits(day(2026, 3, 14)), rate: fridayRate,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]cashFlowMark{"CAD": cad, "USD": usd},
		},
		{
			name:     "by year converts the same total",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: usdSplits(day(2026, 3, 11)), rate: rateOnJan2,
			args: []string{"--by", "year"}, caption: thisYear + "all accounts", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]cashFlowMark{"CAD": cad, "USD": usd, "native": usd},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, cashFlowRows(c.accounts, c.splits...), c.rate)

			for _, currency := range c.currencies {
				args := append([]string{"--currency", currency}, c.args...)
				wantCaption := c.caption + map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}[currency]

				exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, args...))
				require.Equal(t, 0, exitCode, stderr.String())
				gotCaption, gotTotals := cashFlowTextView(stdout.String())
				doc, echoedStderr := runCashFlowJSON(t, args...)

				assert.Empty(t, stderr.String(), currency)
				assert.Empty(t, echoedStderr, currency)
				assert.Equal(t, wantCaption, gotCaption, currency)
				assert.Equal(t, totalMarkCells(c.want[currency]), gotTotals, currency)
				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, c.want[currency], doc.Totals, currency)
			}
		})
	}
}

func Test_run_cashflow_of_an_empty_window_names_the_currency_and_lists_no_rows_beside_the_empty_window_note(t *testing.T) {
	const emptyWindowWarning = "quarry: warning: no income or spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-11 to 2026-03-11\n"
	const caption = "Cash flow 2020-01-01 to 2020-12-31 in all accounts"
	const emptyHeader = "Month  Currency  Income  Spent  Net  Savings rate  Status\n"
	suffixes := map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}
	for currency, suffix := range suffixes {
		t.Run(currency, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, cashFlowRows(
				[]store.Account{usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s1", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 11), cents: 8000},
			), rateOnJan2)
			window := []string{"--currency", currency, "--since", "2020-01-01", "--until", "2020-12-31"}

			t.Run("text", func(t *testing.T) {
				exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, window...))

				require.Equal(t, 0, exitCode, stderr.String())
				assert.Equal(t, caption+suffix+"\n\n"+emptyHeader, stdout.String())
				assert.Equal(t, emptyWindowWarning, stderr.String())
			})

			t.Run("json", func(t *testing.T) {
				doc, stderr := runCashFlowJSON(t, window...)

				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, []cashFlowMark{}, doc.Periods)
				assert.Equal(t, []cashFlowMark{}, doc.Totals)
				assert.Equal(t, emptyWindowWarning, stderr)
			})
		})
	}
}

func Test_run_cashflow_by_month_of_a_window_holding_only_unconverted_rows_still_lists_the_report_currency_zero_rows(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s1", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2025, 12, 20), cents: 8000},
	), rateOnJan2)
	window := []string{"--since", "2025-11", "--until", "2025-12"}

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, window...))
	doc, _ := runCashFlowJSON(t, window...)

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Equal(t, `Cash flow 2025-11-01 to 2025-12-31 in all accounts, amounts in CAD

Month    Currency  Income  Spent    Net  Savings rate  Status
2025-11  CAD         0.00   0.00   0.00           n/a
2025-12  CAD         0.00   0.00   0.00           n/a
2025-12  USD        80.00   0.00  80.00        100.0%
Total    USD        80.00   0.00  80.00        100.0%
`, stdout.String())
	assert.Equal(t, []string{"2025-11 CAD", "2025-12 CAD", "2025-12 USD"}, periodKeys(doc.Periods))
}

func Test_run_cashflow_json_reads_back_with_every_amount_in_the_reporting_currency(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s1", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 123456},
		spendSplit{id: "s2", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 4, 11), cents: 8001},
		spendSplit{id: "s3", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 4, 12), cents: -3333},
		spendSplit{id: "s4", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 5, 13), cents: -777},
	), rateOnJan2)

	doc, _ := runCashFlowJSON(t, "--since", "2026-03", "--until", "2026-05")

	require.Len(t, doc.Totals, 1)
	var income, spent int64
	for _, r := range append(append([]cashFlowMark{}, doc.Periods...), doc.Totals...) {
		assert.Equal(t, doc.Currency, r.Currency)
	}
	for _, r := range doc.Periods {
		income += centsOf(t, r.Income)
		spent += centsOf(t, r.Spent)
	}
	assert.Equal(t, centsOf(t, doc.Totals[0].Income), income)
	assert.Equal(t, centsOf(t, doc.Totals[0].Spent), spent)
}

const oneUSDBeforeFirstRateLine = "1 transaction dated before 2026-01-02, the first exchange rate in the store, " +
	"is listed in USD, not converted to CAD"

func Test_run_cashflow_lists_splits_before_the_first_rate_in_their_own_currency_and_warns(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s1", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2025, 12, 20), cents: 8000},
		spendSplit{id: "s2", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 15), cents: -4000},
		spendSplit{id: "s3", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 15), cents: 10000},
	), rateOnJan2)
	cases := []struct {
		currency string
		want     string
		totals   []string
		warnings []string
	}{
		{currency: "CAD", warnings: []string{oneUSDBeforeFirstRateLine}, totals: []string{"CAD", "USD"}, want: `Cash flow 2025-12-01 to 2026-02-28 in all accounts, amounts in CAD

Month    Currency  Income  Spent    Net  Savings rate  Status
2025-12  CAD         0.00   0.00   0.00           n/a
2025-12  USD        80.00   0.00  80.00        100.0%
2026-01  CAD       100.00  50.00  50.00         50.0%
2026-02  CAD         0.00   0.00   0.00           n/a
Total    CAD       100.00  50.00  50.00         50.0%
Total    USD        80.00   0.00  80.00        100.0%
`},
		{currency: "USD", totals: []string{"USD"}, want: `Cash flow 2025-12-01 to 2026-02-28 in all accounts, amounts in USD

Month    Currency  Income  Spent     Net  Savings rate  Status
2025-12  USD        80.00   0.00   80.00        100.0%
2026-01  USD        80.00  40.00   40.00         50.0%
2026-02  USD         0.00   0.00    0.00           n/a
Total    USD       160.00  40.00  120.00         75.0%
`},
		{currency: "native", totals: []string{"CAD", "USD"}, want: `Cash flow 2025-12-01 to 2026-02-28 in all accounts

Month    Currency  Income  Spent     Net  Savings rate  Status
2025-12  CAD         0.00   0.00    0.00           n/a
2025-12  USD        80.00   0.00   80.00        100.0%
2026-01  CAD       100.00   0.00  100.00        100.0%
2026-01  USD         0.00  40.00  -40.00           n/a
2026-02  CAD         0.00   0.00    0.00           n/a
2026-02  USD         0.00   0.00    0.00           n/a
Total    CAD       100.00   0.00  100.00        100.0%
Total    USD        80.00  40.00   40.00         50.0%
`},
	}

	for _, c := range cases {
		t.Run(c.currency, func(t *testing.T) {
			window := []string{"--currency", c.currency, "--since", "2025-12", "--until", "2026-02"}

			exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, window...))
			doc, echoedStderr := runCashFlowJSON(t, window...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.want, stdout.String())
			assert.Equal(t, c.currency, doc.Currency)
			assert.Equal(t, c.totals, markCurrencies(doc.Totals))
			assert.Equal(t, warningLines(c.warnings), stderr.String())
			assert.Equal(t, warningLines(c.warnings), echoedStderr)
			assert.ElementsMatch(t, c.warnings, doc.Warnings)
		})
	}
}

// markCurrencies is the currency of each mark, in order.
func markCurrencies(marks []cashFlowMark) []string {
	currencies := make([]string, len(marks))
	for i, m := range marks {
		currencies[i] = m.Currency
	}
	return currencies
}

// periodKeys is each mark as "<period> <currency>", in order.
func periodKeys(marks []cashFlowMark) []string {
	keys := make([]string, len(marks))
	for i, m := range marks {
		keys[i] = m.Period + " " + m.Currency
	}
	return keys
}

// totalsColumn is the field at index column of each "Total <currency> ..." line of out, by currency.
func totalsColumn(out string, column int) map[string]string {
	totals := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > column && fields[0] == "Total" {
			totals[fields[1]] = fields[column]
		}
	}
	return totals
}

// mustRunSpendAndCashFlow runs spend, then cashflow, with args, requiring both to exit 0, and returns their
// stdout and the stderr they shared.
func mustRunSpendAndCashFlow(tb testing.TB, args ...string) (spendOut, cashFlowOut, stderr string) {
	tb.Helper()
	var spendBuf, cashFlowBuf, errBuf bytes.Buffer
	spendExit := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&spendBuf, &errBuf))
	cashFlowExit := runWith(context.Background(), append([]string{"cashflow"}, args...), spendEnv(&cashFlowBuf, &errBuf))
	require.Equal(tb, 0, spendExit, errBuf.String())
	require.Equal(tb, 0, cashFlowExit, errBuf.String())
	return spendBuf.String(), cashFlowBuf.String(), errBuf.String()
}

func Test_run_cashflow_total_spent_equals_spend_total_per_currency(t *testing.T) {
	cases := []struct {
		name      string
		accounts  []string
		wantSpent map[string]string
	}{
		{
			name:      "every account in reports",
			wantSpent: map[string]string{"CAD": "155.00", "USD": "55.00"},
		},
		{
			name:      "one account in reports and one not in reports",
			accounts:  []string{"Chequing", "Old Card"},
			wantSpent: map[string]string{"CAD": "155.00"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			rows := cashFlowRows(
				[]store.Account{
					chequingAccount("acct-cad", 1),
					{ID: "acct-sav", SourceID: 2, Name: "Savings", Type: "savings", Currency: "CAD", Active: true},
					{ID: "acct-out", SourceID: 3, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
					usdChequingAccount("acct-usd", 4),
				},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000},
				spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 12), cents: 3000},
				spendSplit{id: "s03", account: "acct-cad", currency: "CAD", day: day(2026, 4, 1), cents: -2500},
				spendSplit{id: "s04", account: "acct-cad", currency: "CAD", day: day(2026, 4, 2), cents: 4000},
				spendSplit{id: "s05", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 2), cents: -6000},
				spendSplit{id: "s06", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 100000},
				spendSplit{id: "s07", account: "acct-cad", currency: "CAD", day: day(2026, 6, 1), cents: -20000},
				spendSplit{id: "s08", account: "acct-sav", currency: "CAD", day: day(2026, 6, 1), cents: 20000},
				spendSplit{id: "s09", account: "acct-out", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -90000},
				spendSplit{id: "s10", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 2, 1), cents: -5000},
				spendSplit{id: "s11", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 2, 3), cents: 500},
				spendSplit{id: "s12", account: "acct-usd", currency: "USD", day: day(2026, 2, 5), cents: -1000},
			)
			rows.Transfers = []store.Transfer{{ID: "xfer-1", FromSplitID: "split-s07", ToSplitID: new("split-s08")}}
			replaceStore(t, home, rows)
			period := []string{"--since", "2026-01", "--until", "2026-09"}
			for _, a := range c.accounts {
				period = append(period, "--account", a)
			}

			spendOut, cashFlowOut, _ := mustRunSpendAndCashFlow(t, period...)

			assert.Equal(t, c.wantSpent, totalsColumn(spendOut, 2))
			assert.Equal(t, c.wantSpent, totalsColumn(cashFlowOut, 3))
		})
	}
}

func Test_run_cashflow_spent_equals_spend_total_in_every_reporting_currency(t *testing.T) {
	// s07 (USD) and s08 (CAD) precede the first rate, so one of them has no converted cell in CAD or USD mode.
	// s04 and s05 are 0.10 USD each, s09 and s10 0.03 CAD each: the half-cent pairs of each direction.
	cases := []struct {
		name      string
		currency  string
		accounts  []string
		wantSpent map[string]string
	}{
		{name: "CAD, every account", currency: "CAD", wantSpent: map[string]string{"CAD": "220.32", "USD": "40.00"}},
		{name: "USD, every account", currency: "USD", wantSpent: map[string]string{"CAD": "20.00", "USD": "200.24"}},
		{name: "native, every account", currency: "native", wantSpent: map[string]string{"CAD": "120.06", "USD": "120.20"}},
		{name: "CAD, one USD account", currency: "CAD", accounts: []string{"US Chequing"}, wantSpent: map[string]string{"CAD": "100.26", "USD": "40.00"}},
		{name: "USD, one USD account", currency: "USD", accounts: []string{"US Chequing"}, wantSpent: map[string]string{"USD": "120.20"}},
		{name: "native, one USD account", currency: "native", accounts: []string{"US Chequing"}, wantSpent: map[string]string{"USD": "120.20"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStoreWithRates(t, home, cashFlowRows(
				[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000},
				spendSplit{id: "s02", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 11), cents: 50000},
				spendSplit{id: "s03", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 4, 4), cents: 1000},
				spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -10},
				spendSplit{id: "s05", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 2), cents: -10},
				spendSplit{id: "s06", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 3), cents: -8000},
				spendSplit{id: "s07", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2025, 12, 15), cents: -4000},
				spendSplit{id: "s08", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2025, 12, 16), cents: -2000},
				spendSplit{id: "s09", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 12), cents: -3},
				spendSplit{id: "s10", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 13), cents: -3},
			), rateOnJan2)
			args := []string{"--since", "2025-12", "--until", "2026-04", "--currency", c.currency}
			for _, a := range c.accounts {
				args = append(args, "--account", a)
			}

			spendOut, cashFlowOut, _ := mustRunSpendAndCashFlow(t, args...)

			assert.Equal(t, c.wantSpent, totalsColumn(spendOut, 2))
			assert.Equal(t, c.wantSpent, totalsColumn(cashFlowOut, 3))
		})
	}
}

func Test_run_cashflow_json_returns_cash_flow_as_a_document(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 1, 31), cents: 910000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 10), cents: -300000},
		spendSplit{id: "s03", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 1, 20), cents: -320000},
		spendSplit{id: "s04", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 2, 15), cents: -10000},
	))

	exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"cashflow", "--json", "--since", "2026-01", "--until", "2026-02"})

	require.Equal(t, 0, exitCode, stderr.String())
	assert.Empty(t, stderr.String())
	//nolint:testifylint // bytes are the contract
	assert.Equal(t, `{
  "since": "2026-01-01",
  "until": "2026-02-28",
  "by": "month",
  "currency": "CAD",
  "account_filter": [],
  "periods": [
    {
      "period": "2026-01",
      "currency": "CAD",
      "income": "9100.00",
      "spent": "6200.00",
      "net": "2900.00",
      "savings_rate_pct": 31.9,
      "partial": false
    },
    {
      "period": "2026-02",
      "currency": "CAD",
      "income": "0.00",
      "spent": "100.00",
      "net": "-100.00",
      "savings_rate_pct": null,
      "partial": false
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "income": "9100.00",
      "spent": "6300.00",
      "net": "2800.00",
      "savings_rate_pct": 30.8
    }
  ],
  "warnings": []
}
`, stdout.String())
}

func Test_run_cashflow_leaves_out_accounts_that_use_linked_account_tracking(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{
			chequingAccount("acct-chq", 1),
			{ID: "acct-401k", SourceID: 2, Name: "Netskope 401(k)", Type: "retirement", Currency: "CAD", Active: true, LinkedTracking: true},
		},
		spendSplit{id: "s01", account: "acct-chq", category: "cat-salary", currency: "CAD", day: day(2026, 3, 1), cents: 50000},
		spendSplit{id: "s02", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12000},
		spendSplit{id: "s03", account: "acct-401k", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 11), cents: -90000},
		spendSplit{id: "s04", account: "acct-401k", currency: "CAD", day: day(2026, 3, 12), cents: 40000},
	))

	spendOut, cashFlowOut, stderr := mustRunSpendAndCashFlow(t, "--since", "2026-01", "--until", "2026-09")

	assert.Equal(t, map[string]string{"CAD": "500.00"}, totalsColumn(cashFlowOut, 2))
	assert.Equal(t, map[string]string{"CAD": "120.00"}, totalsColumn(cashFlowOut, 3))
	assert.Equal(t, map[string]string{"CAD": "120.00"}, totalsColumn(spendOut, 2))
	assert.Empty(t, stderr)
}

func Test_run_cashflow_refuses_and_reports_empty_periods_like_spend(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		accounts   []store.Account
		splits     []spendSplit
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "a since that is not a date",
			args:       []string{"cashflow", "--since", "2024-13"},
			wantExit:   2,
			wantStderr: "quarry: --since \"2024-13\" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD\n",
		},
		{
			name:       "a period that is neither month nor year",
			args:       []string{"cashflow", "--by", "week"},
			wantExit:   2,
			wantStderr: "quarry: --by must be month or year\n",
		},
		{
			name: "an account name two accounts share",
			args: []string{"cashflow", "--account", "Visa"},
			accounts: []store.Account{
				{ID: "acct-812", SourceID: 1, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
				{ID: "acct-977", SourceID: 2, Name: "Visa", Type: "credit_card", Currency: "CAD", Active: true},
			},
			wantExit:   1,
			wantStderr: "quarry: 2 accounts are named \"Visa\"; pass one of their ids instead: acct-812, acct-977\n",
		},
		{
			name: "a store with transactions, none in the period",
			args: []string{"cashflow", "--since", "2026-01", "--until", "2026-02"},
			accounts: []store.Account{
				chequingAccount("acct-chq", 1),
			},
			splits: []spendSplit{
				{id: "s01", account: "acct-chq", category: "cat-groceries", currency: "CAD", day: day(2003, 1, 4), cents: -1000},
				{id: "s02", account: "acct-chq", category: "cat-salary", currency: "CAD", day: day(2025, 12, 31), cents: 5000},
			},
			wantExit: 0,
			wantStdout: "Cash flow 2026-01-01 to 2026-02-28 in all accounts, amounts in CAD\n\n" +
				"Month  Currency  Income  Spent  Net  Savings rate  Status\n",
			wantStderr: "quarry: warning: no income or spending from 2026-01-01 to 2026-02-28; " +
				"the store's transactions run 2003-01-04 to 2025-12-31\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows(c.accounts, c.splits...))

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, c.wantExit, exitCode)
			assert.Equal(t, c.wantStdout, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

// HOME holds no store: exit 2 (not the missing-store 1) shows each check runs first.
func Test_run_cashflow_rejects_a_period_it_cannot_use(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "a since after until",
			args:       []string{"cashflow", "--since", "2025", "--until", "2024"},
			wantStderr: "quarry: --since 2025 is after --until 2024\n",
		},
		{
			name:       "an until before the default since",
			args:       []string{"cashflow", "--until", "2024"},
			wantStderr: "quarry: --until 2024 is before the default --since 2026-01-01; pass --since too\n",
		},
		{
			name:       "a since after today",
			args:       []string{"cashflow", "--since", "2099"},
			wantStderr: "quarry: --since 2099 is after today; pass --until to include future-dated transactions\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			assert.Equal(t, 2, exitCode)
			assert.Empty(t, stdout.String())
			assert.Equal(t, c.wantStderr, stderr.String())
		})
	}
}

func Test_run_cashflow_warns_with_the_count_of_its_own_accounts_and_currency(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 1, 1), cents: -500},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 1, 1), cents: 8000},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 1, 1), cents: -2000},
	), rateOnJan2)
	cells := []unconvertedCell{
		{name: "CAD mode counts income and expense in USD", want: []string{usdPreRateLine}},
		{name: "USD mode counts the CAD transaction", args: []string{"--currency", "USD"}, want: []string{cadPreRateLine}},
		{name: "the USD account alone warns with its own count", args: []string{"--account", "acct-usd"}, want: []string{usdPreRateLine}},
		{name: "the CAD account alone stays silent in CAD", args: []string{"--account", "acct-cad"}},
		{name: "by year warns once", args: []string{"--by", "year"}, want: []string{usdPreRateLine}},
		{name: "native converts nothing", args: []string{"--currency", "native"}},
	}

	for _, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, c.args...))
			doc, echoedStderr := runCashFlowJSON(t, c.args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_without_rates_warns_only_when_a_conversion_is_needed(t *testing.T) {
	cases := []struct {
		name     string
		accounts []store.Account
		splits   []spendSplit
		want     []string
	}{
		{
			name:     "CAD and USD data in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			splits: []spendSplit{
				{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
				{id: "s02", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 11), cents: 8000},
			},
			want: []string{noRatesLine},
		},
		{
			name:     "all-CAD data in CAD",
			accounts: []store.Account{chequingAccount("acct-cad", 1)},
			splits:   []spendSplit{{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows(c.accounts, c.splits...))

			exitCode, _, stderr := runSpendCapture(context.Background(), []string{"cashflow", "--since", "2026-03", "--until", "2026-03"})
			doc, echoedStderr := runCashFlowJSON(t, "--since", "2026-03", "--until", "2026-03")

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_in_usd_without_rates_warns_only_when_a_conversion_is_needed(t *testing.T) {
	cases := []struct {
		name    string
		account store.Account
		split   spendSplit
		want    []string
	}{
		{
			name:    "all-USD data in USD",
			account: usdChequingAccount("acct-usd", 2),
			split:   spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 10), cents: 8000},
		},
		{
			name:    "all-CAD data in USD",
			account: chequingAccount("acct-cad", 1),
			split:   spendSplit{id: "s01", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 3, 10), cents: 100000},
			want:    []string{noRatesLine},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			replaceStore(t, home, cashFlowRows([]store.Account{c.account}, c.split))
			args := []string{"--currency", "USD", "--since", "2026-03", "--until", "2026-03"}

			exitCode, _, stderr := runSpendCapture(context.Background(), append([]string{"cashflow"}, args...))
			doc, echoedStderr := runCashFlowJSON(t, args...)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, warningLines(c.want), stderr.String())
			assert.Equal(t, warningLines(c.want), echoedStderr)
			assert.ElementsMatch(t, c.want, doc.Warnings)
		})
	}
}

func Test_run_cashflow_json_gives_a_period_in_the_other_currency_only_where_the_store_found_one(t *testing.T) {
	home := newHome(t)
	replaceStoreWithRates(t, home, cashFlowRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2025, 12, 20), cents: 8000},
		spendSplit{id: "s02", account: "acct-cad", category: "cat-salary", currency: "CAD", day: day(2026, 2, 10), cents: 10000},
	), rateOnJan2)

	doc, _ := runCashFlowJSON(t, "--since", "2025-12", "--until", "2026-02")

	assert.Equal(t, []string{"2025-12 CAD", "2025-12 USD", "2026-01 CAD", "2026-02 CAD"}, periodKeys(doc.Periods))
}

func Test_run_cashflow_of_an_unrated_empty_window_gives_only_the_empty_window_note(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, cashFlowRows(
		[]store.Account{usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-usd", category: "cat-salary", currency: "USD", day: day(2026, 3, 10), cents: 8000},
	))

	exitCode, _, stderr := runSpendCapture(context.Background(), []string{"cashflow", "--since", "2020-01-01", "--until", "2020-12-31"})

	require.Equal(t, 0, exitCode, stderr.String())
	const note = "no income or spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-10 to 2026-03-10"
	assert.Equal(t, warningLines([]string{note}), stderr.String())
	doc, echoedStderr := runCashFlowJSON(t, "--since", "2020-01-01", "--until", "2020-12-31")
	assert.Equal(t, stderr.String(), echoedStderr)
	assert.Equal(t, []string{note}, doc.Warnings)
}
