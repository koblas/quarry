// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
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
