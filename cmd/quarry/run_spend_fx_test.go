// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

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
	// Each 0.10 USD split is 0.125 CAD at 1.25, so it rounds to 0.13 before it is summed:
	// the two make 0.26, where summing first would give 0.25.
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
