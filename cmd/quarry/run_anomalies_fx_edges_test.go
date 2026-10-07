package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	beforeAprilUSDInCAD = "1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in USD, not converted to CAD"
	beforeAprilCADInUSD = "1 charge dated before 2026-04-01, the first exchange rate in the store, is listed in CAD, not converted to USD"
)

// runAnomaliesOK runs quarry anomalies with args and returns stdout and stderr; it must exit 0.
func runAnomaliesOK(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"anomalies"}, args...))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// jsonCells is the currency, amount and usual of an entry, then its native twins.
func jsonCells(a anomalyJSON) []string {
	return []string{a.Currency, a.Amount, a.Usual, a.NativeCurrency, a.NativeAmount, a.NativeUsual}
}

// hardwareHistory is five Hardware charges of 38.00 to 42.00 (median 40.00), weekly from 2025-03-03.
func hardwareHistory() []chargeTxn {
	history := make([]chargeTxn, 0, 5)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		history = append(history, groceryCharge("Hardware", day(2025, time.March, 3+7*i), cents))
	}
	return history
}

// usdRate is a FXUSDCAD rate of rate millionths from date.
func usdRate(date time.Time, rate int64) store.Rate {
	return store.Rate{Date: date, USDCAD: money.Rate(rate), Series: "FXUSDCAD"}
}

// anomaliesStore stores charges on accounts under a fresh HOME, with rates when any are given.
func anomaliesStore(t *testing.T, accounts []store.Account, charges []chargeTxn, rates ...store.Rate) {
	t.Helper()
	home := newHome(t)
	if len(rates) == 0 {
		replaceStore(t, home, chargeRows(accounts, charges...))
		return
	}
	replaceStoreWithRates(t, home, chargeRows(accounts, charges...), rates...)
}

// bigHardware is the 250.00 Hardware charge of 2026-03-02 that history makes unusual.
func bigHardware() chargeTxn { return groceryCharge("Hardware", day(2026, time.March, 2), 25000) }

// hardwareRow is the row bigHardware makes in account, with Amount and Usual cells as given.
func hardwareRow(account, amount, usual string) []string {
	return []string{"2026-03-02", account, "Hardware", "Food:Groceries", amount, usual, "6.3x", "payee, 5 earlier"}
}

func Test_run_anomalies_in_usd_lists_a_usd_charge_as_it_was_charged_and_says_nothing(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2025, time.January, 2), 1_300_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "250.00", "40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_lists_a_usd_charge_before_the_first_rate_in_usd_with_a_warning(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text prefixes both cells with USD", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{beforeAprilUSDInCAD}), stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json echoes the warning", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{beforeAprilUSDInCAD}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{beforeAprilUSDInCAD}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_in_usd_lists_a_cad_charge_before_the_first_rate_in_cad_with_a_warning(t *testing.T) {
	anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text prefixes both cells with CAD", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD")

		assert.Equal(t, warningLines([]string{beforeAprilCADInUSD}), stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in USD", "1 charge checked",
			hardwareRow("Chequing (CAD)", "CAD 250.00", "CAD 40.00")), stdout)
	})

	t.Run("json echoes the warning", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "USD", "--json")

		assert.Equal(t, warningLines([]string{beforeAprilCADInUSD}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{beforeAprilCADInUSD}, doc.Warnings)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"CAD", "250.00", "40.00", "CAD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_counts_each_unconverted_listed_charge_and_says_are(t *testing.T) {
	const want = "2 charges dated before 2026-04-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
	other := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		other = append(other, groceryCharge("Lumber", day(2025, time.March, 3+7*i), cents))
	}
	other = append(other, groceryCharge("Lumber", day(2026, time.March, 9), 25000))
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(append(hardwareHistory(), bigHardware()), other...)),
		usdRate(day(2026, time.April, 1), 1_300_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{want}), stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "2 charges checked",
			[]string{"2026-03-09", "US Chequing (USD)", "Lumber", "Food:Groceries", "USD 250.00", "USD 40.00", "6.3x", "payee, 5 earlier"},
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{want}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{want}, doc.Warnings)
		require.Len(t, doc.Anomalies, 2)
		for _, entry := range doc.Anomalies {
			assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(entry))
		}
	})
}

func Test_run_anomalies_on_a_store_without_rates_warns_only_when_a_usd_charge_needs_converting(t *testing.T) {
	t.Run("an unrated USD charge in CAD draws the no-rates line and stays USD", func(t *testing.T) {
		anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))

		stdout, stderr := runAnomaliesOK(t)

		assert.Equal(t, warningLines([]string{noRatesLine}), stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "USD 250.00", "USD 40.00")), stdout)
	})

	t.Run("json carries the line", func(t *testing.T) {
		anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))

		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Equal(t, warningLines([]string{noRatesLine}), stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{noRatesLine}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})

	t.Run("an all-CAD store in CAD is silent and plain", func(t *testing.T) {
		anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()))

		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
			hardwareRow("Chequing (CAD)", "250.00", "40.00")), stdout)
	})

	t.Run("an all-CAD store in CAD is silent in json too", func(t *testing.T) {
		anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1)}, append(hardwareHistory(), bigHardware()))

		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "CAD", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"CAD", "250.00", "40.00", "CAD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_native_on_a_rated_usd_store_prints_what_it_printed_before_conversion_existed(t *testing.T) {
	anomaliesFXStore(t)

	t.Run("text has no amounts clause and the USD cells", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "native")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts", "2 charges checked",
			hardwareRow("US Chequing (USD)", "250.00", "40.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--currency", "native", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, "native", doc.Currency)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, []string{"USD", "250.00", "40.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_converts_a_category_baseline_anomaly_with_no_payee(t *testing.T) {
	history := make([]chargeTxn, 0, 11)
	for i := range 10 {
		history = append(history, groceryCharge("", day(2025, time.March, 3+7*i), 4000))
	}
	history = append(history, groceryCharge("", day(2026, time.March, 2), 60000))
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(history),
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))
	want := []string{"2026-03-02", "US Chequing (USD)", "(no payee)", "Food:Groceries", "840.00", "56.00", "15.0x", "category, 10 earlier"}

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked", want), stdout)
	})

	t.Run("json lists one entry without a payee", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		require.Len(t, doc.Anomalies, 1)
		assert.Nil(t, doc.Anomalies[0].Payee)
		assert.Equal(t, "category", doc.Anomalies[0].Baseline)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "840.00", "56.00", "USD", "600.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_keeps_the_not_judged_count_in_every_reporting_currency(t *testing.T) {
	const footer = "2 charges checked; 1 had too little history to judge"
	lone := groceryCharge("Lone", day(2026, time.April, 6), 20000)
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(append(hardwareHistory(), bigHardware()), lone)),
		usdRate(day(2025, time.January, 2), 1_300_000))

	for _, currency := range []string{"CAD", "USD", "native"} {
		t.Run(currency, func(t *testing.T) {
			text, textStderr := runAnomaliesOK(t, "--currency", currency)
			js, docStderr := runAnomaliesOK(t, "--currency", currency, "--json")

			assert.Empty(t, textStderr)
			assert.Empty(t, docStderr)
			assert.True(t, strings.HasSuffix(text, "\n"+footer+"\n"), text)
			doc := decodeAnomaliesJSON(t, js)
			assert.Equal(t, []int{2, 1}, []int{doc.Checked, doc.NotJudged})
			assert.Equal(t, currency, doc.Currency)
		})
	}
}

func Test_run_anomalies_narrows_to_one_usd_account_and_converts_it_to_cad(t *testing.T) {
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}
	anomaliesStore(t, accounts, inUSD(append(hardwareHistory(), bigHardware())),
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))

	t.Run("text names the account and the currency", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--account", "US Chequing")

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in US Chequing, amounts in CAD", "1 charge checked",
			hardwareRow("US Chequing (USD)", "350.00", "56.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--account", "US Chequing", "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}}, doc.AccountFilter)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "350.00", "56.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_converts_a_charge_in_a_closed_usd_account(t *testing.T) {
	closed := usdChequingAccount("acct-old", 2)
	closed.Name, closed.Active, closed.Closed = "Old USD", false, true
	charges := inUSD(append(hardwareHistory(), bigHardware()))
	for i := range charges {
		charges[i].account = "acct-old"
	}
	anomaliesStore(t, []store.Account{chequingAccount("acct-cad", 1), closed}, charges,
		usdRate(day(2025, time.January, 2), 1_300_000), usdRate(day(2026, time.March, 2), 1_400_000))

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t)

		assert.Empty(t, stderr)
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "1 charge checked",
			hardwareRow("Old USD (USD, closed)", "350.00", "56.00")), stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runAnomaliesOK(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeAnomaliesJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		require.Len(t, doc.Anomalies, 1)
		assert.Equal(t, "Old USD", doc.Anomalies[0].Account)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{"CAD", "350.00", "56.00", "USD", "250.00", "40.00"}, jsonCells(doc.Anomalies[0]))
	})
}

func Test_run_anomalies_on_an_unrated_usd_store_says_only_that_the_period_is_empty(t *testing.T) {
	anomaliesStore(t, []store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(append(hardwareHistory(), bigHardware())))
	const empty = "no unusually large charges from 2026-09-01 to 2026-09-29; the store's transactions run 2025-03-03 to 2026-03-02"

	for _, c := range []struct{ currency, caption string }{
		{currency: "CAD", caption: ", amounts in CAD"},
		{currency: "USD", caption: ", amounts in USD"},
		{currency: "native", caption: ""},
	} {
		t.Run(c.currency, func(t *testing.T) {
			text, textStderr := runAnomaliesOK(t, "--since", "2026-09", "--currency", c.currency)
			js, docStderr := runAnomaliesOK(t, "--since", "2026-09", "--currency", c.currency, "--json")

			assert.Equal(t, warningLines([]string{empty}), textStderr)
			assert.Equal(t, warningLines([]string{empty}), docStderr)
			assert.Equal(t, anomaliesTable("Unusually large charges 2026-09-01 to 2026-09-29 in all accounts"+c.caption, "0 charges checked"), text)
			doc := decodeAnomaliesJSON(t, js)
			assert.Equal(t, []string{empty}, doc.Warnings)
			assert.Equal(t, c.currency, doc.Currency)
			assert.Equal(t, []anomalyJSON{}, doc.Anomalies)
		})
	}
}
