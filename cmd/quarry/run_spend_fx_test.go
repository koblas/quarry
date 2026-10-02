// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spendReport is the part of spend's --json document the currency tests read.
type spendReport struct {
	Currency string       `json:"currency"`
	Rows     []spendMoney `json:"rows"`
	Totals   []spendMoney `json:"totals"`
}

// spendMoney is one amount of a spend document: a row's category and a total alike.
type spendMoney struct {
	Category *string `json:"category"`
	Currency string  `json:"currency"`
	Spent    string  `json:"spent"`
}

// rateOnJan2 is 1.25 CAD per USD, dated before every split the fixtures below hold.
var rateOnJan2 = store.Rate{Date: day(2026, 1, 2), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}

// runSpendJSON runs spend with args plus --json and decodes its document.
func runSpendJSON(t *testing.T, args ...string) (spendReport, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), append([]string{"spend", "--json"}, args...), spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	var doc spendReport
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), stdout.String())
	return doc, stderr.String()
}

func Test_run_spend_converts_every_split_to_cad_by_default(t *testing.T) {
	// Two 0.10 USD splits at 1.25 make 0.26 rounded each, 0.25 summed first.
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 1), cents: -10},
		spendSplit{id: "s03", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 4, 2), cents: -10},
		spendSplit{id: "s04", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 5, 2), cents: -8000},
		spendSplit{id: "s05", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, 5, 3), cents: -1000},
	), rateOnJan2)

	t.Run("text", func(t *testing.T) {
		var stdout, stderr bytes.Buffer

		exitCode := runWith(context.Background(), []string{"spend"}, spendEnv(&stdout, &stderr))

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		const row = "%-14s  %-8s  %6s\n"
		assert.Equal(t, "Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD\n\n"+
			fmt.Sprintf(row, "Category", "Currency", "Spent")+
			fmt.Sprintf(row, "Auto:Fuel", "CAD", "110.00")+
			fmt.Sprintf(row, "Food:Groceries", "CAD", "123.71")+
			fmt.Sprintf(row, "Total", "CAD", "233.71"),
			stdout.String())
	})

	t.Run("json", func(t *testing.T) {
		doc, stderr := runSpendJSON(t)

		assert.Empty(t, stderr)
		assert.Equal(t, spendReport{
			Currency: "CAD",
			Rows: []spendMoney{
				{Category: new("Auto:Fuel"), Currency: "CAD", Spent: "110.00"},
				{Category: new("Food:Groceries"), Currency: "CAD", Spent: "123.71"},
			},
			Totals: []spendMoney{{Currency: "CAD", Spent: "233.71"}},
		}, doc)
	})
}

func Test_run_spend_takes_its_currency_from_the_config_unless_the_flag_names_one(t *testing.T) {
	const (
		unset     = ""
		malformed = "[snapshots\nkeep = 24\n"
		caption   = "Spending 2026-01-01 to 2026-09-29 in all accounts"
		row       = "%-14s  %-8s  %6s\n"
	)
	cadText := caption + ", amounts in CAD\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "CAD", "200.00") +
		fmt.Sprintf(row, "Total", "CAD", "200.00")
	usdText := caption + ", amounts in USD\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "USD", "160.00") +
		fmt.Sprintf(row, "Total", "USD", "160.00")
	nativeText := caption + "\n\n" +
		fmt.Sprintf(row, "Category", "Currency", "Spent") +
		fmt.Sprintf(row, "Food:Groceries", "CAD", "100.00") +
		fmt.Sprintf(row, "Food:Groceries", "USD", "80.00") +
		fmt.Sprintf(row, "Total", "CAD", "100.00") +
		fmt.Sprintf(row, "Total", "USD", "80.00")
	groceries := new("Food:Groceries")
	cadDoc := spendReport{
		Currency: "CAD",
		Rows:     []spendMoney{{Category: groceries, Currency: "CAD", Spent: "200.00"}},
		Totals:   []spendMoney{{Currency: "CAD", Spent: "200.00"}},
	}
	usdDoc := spendReport{
		Currency: "USD",
		Rows:     []spendMoney{{Category: groceries, Currency: "USD", Spent: "160.00"}},
		Totals:   []spendMoney{{Currency: "USD", Spent: "160.00"}},
	}
	nativeDoc := spendReport{
		Currency: "native",
		Rows: []spendMoney{
			{Category: groceries, Currency: "CAD", Spent: "100.00"},
			{Category: groceries, Currency: "USD", Spent: "80.00"},
		},
		Totals: []spendMoney{{Currency: "CAD", Spent: "100.00"}, {Currency: "USD", Spent: "80.00"}},
	}
	cases := []struct {
		name   string
		config string
		flag   []string
		text   string
		doc    spendReport
	}{
		{name: "unset gives CAD", config: unset, text: cadText, doc: cadDoc},
		{name: "USD in the config", config: "reporting.currency = \"USD\"\n", text: usdText, doc: usdDoc},
		{name: "native in the config", config: "reporting.currency = \"native\"\n", text: nativeText, doc: nativeDoc},
		{name: "lower case in the config", config: "reporting.currency = \"usd\"\n", text: usdText, doc: usdDoc},
		{name: "the flag beats the config", config: "reporting.currency = \"USD\"\n", flag: []string{"--currency", "CAD"}, text: cadText, doc: cadDoc},
		{name: "the flag excuses a malformed config", config: malformed, flag: []string{"--currency", "CAD"}, text: cadText, doc: cadDoc},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStoreWithRates(t, home, spendRows(
				[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000},
				spendSplit{id: "s02", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8000},
			), rateOnJan2)
			if c.config != unset {
				writeConfig(t, home, c.config)
			}

			t.Run("text", func(t *testing.T) {
				var stdout, stderr bytes.Buffer

				exitCode := runWith(context.Background(), append([]string{"spend"}, c.flag...), spendEnv(&stdout, &stderr))

				require.Equal(t, 0, exitCode, stderr.String())
				assert.Empty(t, stderr.String())
				assert.Equal(t, c.text, stdout.String())
			})

			t.Run("json", func(t *testing.T) {
				doc, stderr := runSpendJSON(t, c.flag...)

				assert.Empty(t, stderr)
				assert.Equal(t, c.doc, doc)
			})
		})
	}
}

// spendTextView is the first line of a spend table and the cells of each of its Total rows.
func spendTextView(out string) (string, [][]string) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var totals [][]string
	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "Total") {
			totals = append(totals, strings.Fields(line))
		}
	}
	return lines[0], totals
}

// totalCells are the Total rows of a text table for totals, as spendTextView reads them.
func totalCells(totals []spendMoney) [][]string {
	cells := make([][]string, 0, len(totals))
	for _, t := range totals {
		cells = append(cells, []string{"Total", t.Currency, t.Spent})
	}
	return cells
}

func Test_run_spend_converts_the_edge_cases_in_each_reporting_currency(t *testing.T) {
	const thisYear = "Spending 2026-01-01 to 2026-09-29 in "
	cad100 := []spendMoney{{Currency: "CAD", Spent: "100.00"}}
	usd80 := []spendMoney{{Currency: "USD", Spent: "80.00"}}
	fridayRate := store.Rate{Date: day(2026, 3, 13), USDCAD: money.Rate(1_250_000), Series: "FXUSDCAD"}
	closedUSD := store.Account{ID: "acct-usd", SourceID: 2, Name: "US Chequing", Type: "chequing", Currency: "USD", Closed: true}
	usdSplit := func(when time.Time) spendSplit {
		return spendSplit{id: "s1", account: "acct-usd", category: "cat-groceries", currency: "USD", day: when, cents: -8000}
	}
	cadSplit := spendSplit{id: "s2", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -10000}
	cases := []struct {
		name       string
		accounts   []store.Account
		splits     []spendSplit
		rate       store.Rate
		args       []string
		caption    string
		currencies []string
		want       map[string][]spendMoney
	}{
		{
			name:     "a closed USD account converts",
			accounts: []store.Account{closedUSD}, splits: []spendSplit{usdSplit(day(2026, 3, 11))}, rate: rateOnJan2,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80, "native": usd80},
		},
		{
			name:     "one USD account converts to a CAD total",
			accounts: []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			splits:   []spendSplit{usdSplit(day(2026, 3, 11)), cadSplit}, rate: rateOnJan2,
			args: []string{"--account", "US Chequing"}, caption: thisYear + "US Chequing", currencies: []string{"CAD", "USD", "native"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80, "native": usd80},
		},
		{
			name:     "a split dated after the last rate converts at the latest rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: []spendSplit{usdSplit(day(2099, 6, 1))}, rate: rateOnJan2,
			args: []string{"--until", "2099-12-31"}, caption: "Spending 2026-01-01 to 2099-12-31 in all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80},
		},
		{
			name:     "a weekend split converts at the Friday rate",
			accounts: []store.Account{usdChequingAccount("acct-usd", 2)}, splits: []spendSplit{usdSplit(day(2026, 3, 14))}, rate: fridayRate,
			caption: thisYear + "all accounts", currencies: []string{"CAD", "USD"},
			want: map[string][]spendMoney{"CAD": cad100, "USD": usd80},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStoreWithRates(t, home, spendRows(c.accounts, c.splits...), c.rate)

			for _, currency := range c.currencies {
				args := append([]string{"--currency", currency}, c.args...)
				wantCaption := c.caption + map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}[currency]

				var stdout, stderr bytes.Buffer
				exitCode := runWith(context.Background(), append([]string{"spend"}, args...), spendEnv(&stdout, &stderr))
				require.Equal(t, 0, exitCode, stderr.String())
				gotCaption, gotTotals := spendTextView(stdout.String())
				doc, jsonStderr := runSpendJSON(t, args...)

				assert.Empty(t, stderr.String(), currency)
				assert.Empty(t, jsonStderr, currency)
				assert.Equal(t, wantCaption, gotCaption, currency)
				assert.Equal(t, totalCells(c.want[currency]), gotTotals, currency)
				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, c.want[currency], doc.Totals, currency)
			}
		})
	}
}

func Test_run_spend_of_an_empty_window_names_the_currency_and_lists_no_rows_beside_the_empty_window_note(t *testing.T) {
	const emptyWindowWarning = "quarry: warning: no spending from 2020-01-01 to 2020-12-31; the store's transactions run 2026-03-11 to 2026-03-11\n"
	const caption = "Spending 2020-01-01 to 2020-12-31 in all accounts"
	suffixes := map[string]string{"CAD": ", amounts in CAD", "USD": ", amounts in USD", "native": ""}
	for currency, suffix := range suffixes {
		t.Run(currency, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			replaceStoreWithRates(t, home, spendRows(
				[]store.Account{usdChequingAccount("acct-usd", 2)},
				spendSplit{id: "s1", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8000},
			), rateOnJan2)
			window := []string{"--currency", currency, "--since", "2020-01-01", "--until", "2020-12-31"}

			t.Run("text", func(t *testing.T) {
				var stdout, stderr bytes.Buffer

				exitCode := runWith(context.Background(), append([]string{"spend"}, window...), spendEnv(&stdout, &stderr))

				require.Equal(t, 0, exitCode, stderr.String())
				assert.Equal(t, caption+suffix+"\n\nCategory  Currency  Spent\n", stdout.String())
				assert.Equal(t, emptyWindowWarning, stderr.String())
			})

			t.Run("json", func(t *testing.T) {
				doc, stderr := runSpendJSON(t, window...)

				assert.Equal(t, currency, doc.Currency)
				assert.Equal(t, []spendMoney{}, doc.Rows)
				assert.Equal(t, []spendMoney{}, doc.Totals)
				assert.Equal(t, emptyWindowWarning, stderr)
			})
		})
	}
}

func Test_run_spend_json_reads_back_with_every_amount_in_the_reporting_currency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	replaceStoreWithRates(t, home, spendRows(
		[]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		spendSplit{id: "s1", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, 3, 10), cents: -12345},
		spendSplit{id: "s2", account: "acct-usd", category: "cat-groceries", currency: "USD", day: day(2026, 3, 11), cents: -8001},
		spendSplit{id: "s3", account: "acct-usd", category: "cat-fuel", currency: "USD", day: day(2026, 3, 12), cents: -3333},
	), rateOnJan2)

	doc, _ := runSpendJSON(t, "--by", "category")

	require.Len(t, doc.Totals, 1)
	var rowCents int64
	for _, r := range append(append([]spendMoney{}, doc.Rows...), doc.Totals...) {
		assert.Equal(t, doc.Currency, r.Currency)
	}
	for _, r := range doc.Rows {
		rowCents += centsOf(t, r.Spent)
	}
	assert.Equal(t, centsOf(t, doc.Totals[0].Spent), rowCents)
}

// centsOf is a spend document's amount, "123.71", in cents.
func centsOf(t *testing.T, amount string) int64 {
	t.Helper()
	cents, err := strconv.ParseInt(strings.Replace(amount, ".", "", 1), 10, 64)
	require.NoError(t, err)
	return cents
}
