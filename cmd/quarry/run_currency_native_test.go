package main

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nativeStoreRows is a CAD and a USD chequing account and a USD brokerage account, with monthly Gym and Netflix,
// a Hardware anomaly and a salary, all dated in the past.
func nativeStoreRows() store.Rows {
	accounts := []store.Account{
		chequingAccount("acct-cad", 1),
		usdChequingAccount("acct-usd", 2),
		{ID: "acct-brk", SourceID: 3, Name: "Brokerage", Type: store.AccountTypeBrokerage, Currency: "USD", Active: true},
	}
	hardware := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		hardware = append(hardware, groceryCharge("Hardware", day(2025, time.March, 3+7*i), cents))
	}
	hardware = append(hardware, groceryCharge("Hardware", day(2026, time.March, 2), 25000))
	salary := chargeTxn{
		id: "salary", account: "acct-cad", currency: "CAD", day: day(2026, time.March, 1),
		splits: []chargeSplit{{category: "cat-salary", cents: 300000}},
	}
	rows := chargeRows(accounts, slices.Concat(
		monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{2000}, 8)...),
		inUSD(monthlySeries("Netflix.com", 2026, time.February, slices.Repeat([]int64{1000}, 8)...)),
		inUSD(hardware),
		[]chargeTxn{salary},
	)...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	rows.ReferencedCategoryIDs = append(rows.ReferencedCategoryIDs, "cat-salary")
	return rows
}

// nativeCommands are the five reports with a currency, with arguments that make them independent of today.
var nativeCommands = []struct {
	name string
	args []string
	want string
}{
	{name: "spend", args: []string{"spend", "--since", "2025-01-01", "--until", "2026-09-29"}, want: "" +
		"Spending 2025-01-01 to 2026-09-29 in all accounts\n" +
		"\n" +
		"Category        Currency   Spent\n" +
		"Food:Groceries  CAD       160.00\n" +
		"Food:Groceries  USD       530.00\n" +
		"Total           CAD       160.00\n" +
		"Total           USD       530.00\n"},
	{name: "cashflow", args: []string{"cashflow", "--since", "2026-01", "--until", "2026-04"}, want: "" +
		"Cash flow 2026-01-01 to 2026-04-30 in all accounts\n" +
		"\n" +
		"Month    Currency    Income   Spent       Net  Savings rate  Status\n" +
		"2026-01  CAD           0.00    0.00      0.00           n/a\n" +
		"2026-01  USD           0.00    0.00      0.00           n/a\n" +
		"2026-02  CAD           0.00   20.00    -20.00           n/a\n" +
		"2026-02  USD           0.00   10.00    -10.00           n/a\n" +
		"2026-03  CAD       3,000.00   20.00  2,980.00         99.3%\n" +
		"2026-03  USD           0.00  260.00   -260.00           n/a\n" +
		"2026-04  CAD           0.00   20.00    -20.00           n/a\n" +
		"2026-04  USD           0.00   10.00    -10.00           n/a\n" +
		"Total    CAD       3,000.00   60.00  2,940.00         98.0%\n" +
		"Total    USD           0.00  280.00   -280.00           n/a\n"},
	{name: "recurring", args: []string{"recurring", "--since", "2000"}, want: "" +
		"Recurring charges 2000-01-01 to 2026-09-29 in all accounts\n" +
		"\n" +
		"Payee        Currency  Every  Amount  Per year  First       Last        Status       Price changes\n" +
		"Gym          CAD       month   20.00    240.00  2026-02-12  2026-09-12  active, new\n" +
		"Netflix.com  USD       month   10.00    120.00  2026-02-12  2026-09-12  active, new\n" +
		"Total        CAD                        240.00\n" +
		"Total        USD                        120.00\n"},
	{name: "anomalies", args: []string{"anomalies"}, want: "" +
		"Unusually large charges 2026-01-01 to 2026-09-29 in all accounts\n" +
		"\n" +
		"Date        Account            Payee     Category        Amount  Usual  Times  Compared with\n" +
		"2026-03-02  US Chequing (USD)  Hardware  Food:Groceries  250.00  40.00   6.3x  payee, 5 earlier\n" +
		"\n" +
		"17 charges checked\n"},
	{name: "accounts", args: []string{"accounts"}, want: "" +
		"Account      Type       Currency   Balance  Status\n" +
		"Brokerage    brokerage  USD           0.00\n" +
		"Chequing     chequing   CAD       2,840.00\n" +
		"US Chequing  chequing   USD        -530.00\n"},
}

// runNative runs args with --currency native and fails t unless it exits 0 and prints nothing on stderr.
func runNative(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exitCode := runWith(context.Background(), append(slices.Clone(args), "--currency", "native"), spendEnv(&stdout, &stderr))
	require.Equal(t, 0, exitCode, stderr.String())
	require.Empty(t, stderr.String())
	return stdout.String()
}

func Test_run_currency_native_reproduces_the_pre_fx_output(t *testing.T) {
	rates := []store.Rate{
		{Date: day(2025, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		{Date: day(2026, time.March, 2), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"},
		{Date: day(2026, time.September, 20), USDCAD: money.Rate(1_500_000), Series: "FXUSDCAD"},
	}
	for _, withRates := range []bool{true, false} {
		name := "a store with rates"
		if !withRates {
			name = "a store without rates"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if withRates {
				replaceStoreWithRates(t, home, nativeStoreRows(), rates...)
			} else {
				replaceStore(t, home, nativeStoreRows())
			}

			for _, c := range nativeCommands {
				t.Run(c.name+" text", func(t *testing.T) {
					assert.Equal(t, c.want, runNative(t, c.args...))
				})
			}

			t.Run("spend json names the currency native", func(t *testing.T) {
				var doc spendReport
				require.NoError(t, json.Unmarshal([]byte(runNative(t, "spend", "--json")), &doc))
				assert.Equal(t, "native", doc.Currency)
			})

			t.Run("cashflow json names the currency native", func(t *testing.T) {
				var doc cashFlowReport
				require.NoError(t, json.Unmarshal([]byte(runNative(t, "cashflow", "--json")), &doc))
				assert.Equal(t, "native", doc.Currency)
			})

			t.Run("recurring json repeats each series' own currency and amounts as its native ones", func(t *testing.T) {
				doc := decodeRecurringJSON(t, runNative(t, "recurring", "--since", "2000", "--json"))

				assert.Equal(t, "native", doc.Currency)
				require.Len(t, doc.Series, 2)
				for _, s := range doc.Series {
					assert.Equal(t, s.Currency, s.NativeCurrency, s.Payee)
					assert.Equal(t, s.Amount, s.NativeAmount, s.Payee)
					assert.Equal(t, s.FirstAmount, s.NativeFirstAmount, s.Payee)
				}
			})

			t.Run("anomalies json repeats each charge's own currency and amounts as its native ones", func(t *testing.T) {
				doc := decodeAnomaliesJSON(t, runNative(t, "anomalies", "--json"))

				assert.Equal(t, "native", doc.Currency)
				require.Len(t, doc.Anomalies, 1)
				a := doc.Anomalies[0]
				assert.Equal(t, a.Currency, a.NativeCurrency)
				assert.Equal(t, a.Amount, a.NativeAmount)
				assert.Equal(t, a.Usual, a.NativeUsual)
			})

			t.Run("accounts json names the currency native and converts no balance", func(t *testing.T) {
				var doc struct {
					Currency string           `json:"currency"`
					Accounts []map[string]any `json:"accounts"`
				}
				require.NoError(t, json.Unmarshal([]byte(runNative(t, "accounts", "--json")), &doc))

				assert.Equal(t, "native", doc.Currency)
				require.Len(t, doc.Accounts, 3)
				for _, row := range doc.Accounts {
					converted, present := row["converted_balance"]
					assert.True(t, present, "converted_balance is missing from %v", row["name"])
					assert.Nil(t, converted, row["name"])
				}
			})
		})
	}
}
