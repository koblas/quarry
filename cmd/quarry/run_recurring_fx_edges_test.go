package main

import (
	"bytes"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRecurring runs quarry recurring from 2000 with args and returns stdout and stderr; it must exit 0.
func runRecurring(t *testing.T, args ...string) (string, string) {
	t.Helper()

	exitCode, stdout, stderr := runSpendCapture(context.Background(), append([]string{"recurring", "--since", "2000"}, args...))

	require.Equal(t, 0, exitCode, stderr.String())
	return stdout.String(), stderr.String()
}

// monthlyRow is the cells of a monthly series charged 2026-02-12 to 2026-09-12 that is active and new.
func monthlyRow(payee, currency, amount, perYear, priceChanges string) []string {
	return []string{payee, currency, "month", amount, perYear, "2026-02-12", "2026-09-12", "active, new", priceChanges}
}

func Test_run_recurring_in_usd_lists_each_series_beside_its_native_currency(t *testing.T) {
	recurringFXStore(t)

	t.Run("text puts the series' own currency in the cell and keeps price changes native", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
			monthlyRow("Gym", "USD", "15.00", "180.00", "1: 12.00 -> 15.00 (+25.0%)"),
			monthlyRow("Gym", "USD (CAD)", "14.29", "171.48", ""),
			monthlyRow("Netflix.com", "USD", "10.00", "120.00", ""),
			[]string{"Total", "USD", "", "", "471.48", "", "", "", ""}),
			stdout)
	})

	t.Run("json lists the USD series with currency equal to native_currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "USD", doc.Currency)
		require.Len(t, doc.Series, 3)
		cadGym := doc.Series[1]
		assert.Equal(t, []string{"USD", "14.29", "CAD", "20.00", "20.00"},
			[]string{cadGym.Currency, cadGym.Amount, cadGym.NativeCurrency, cadGym.NativeAmount, cadGym.NativeFirstAmount})
		assert.Equal(t, []recurringTotalJSON{{Currency: "USD", PerYear: "471.48"}}, doc.Totals)
	})
}

func Test_run_recurring_prefixes_a_price_change_with_the_native_code_when_the_row_is_converted(t *testing.T) {
	home := newHome(t)
	cadHydro := monthlySeries("Hydro", 2026, time.February, slices.Concat(slices.Repeat([]int64{999}, 4), slices.Repeat([]int64{1299}, 4))...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, cadHydro...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"})

	stdout, stderr := runRecurring(t, "--currency", "USD")

	assert.Empty(t, stderr)
	assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
		monthlyRow("Hydro", "USD (CAD)", "9.99", "119.88", "1: CAD 9.99 -> CAD 12.99 (+30.0%)"),
		[]string{"Total", "USD", "", "", "119.88", "", "", "", ""}),
		stdout)
}

func Test_run_recurring_native_on_a_store_with_rates_lists_every_series_as_it_was_charged(t *testing.T) {
	recurringFXStore(t)

	t.Run("text has no amounts clause and one Total per currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "native")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts",
			monthlyRow("Gym", "CAD", "20.00", "240.00", ""),
			monthlyRow("Gym", "USD", "15.00", "180.00", "1: 12.00 -> 15.00 (+25.0%)"),
			monthlyRow("Netflix.com", "USD", "10.00", "120.00", ""),
			[]string{"Total", "CAD", "", "", "240.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "300.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json repeats each row's own values as its native ones", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "native", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "native", doc.Currency)
		assert.Equal(t, [][]string{
			{"Gym", "CAD", "20.00", "20.00", "CAD", "20.00", "20.00"},
			{"Gym", "USD", "15.00", "12.00", "USD", "15.00", "12.00"},
			{"Netflix.com", "USD", "10.00", "10.00", "USD", "10.00", "10.00"},
		}, amountsOf(doc.Series))
	})
}

// amountsOf is each series' payee, currency, amount and first amount, then its native currency, amount and first amount.
func amountsOf(series []recurringSeriesJSON) [][]string {
	rows := make([][]string, len(series))
	for i, s := range series {
		rows[i] = []string{s.Payee, s.Currency, s.Amount, s.FirstAmount, s.NativeCurrency, s.NativeAmount, s.NativeFirstAmount}
	}
	return rows
}

func Test_run_recurring_narrows_to_one_usd_account_and_converts_it_to_cad(t *testing.T) {
	recurringFXStore(t)

	t.Run("text", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--account", "US Chequing")

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in US Chequing, amounts in CAD",
			monthlyRow("Gym", "CAD (USD)", "21.00", "252.00", "1: USD 12.00 -> USD 15.00 (+25.0%)"),
			monthlyRow("Netflix.com", "CAD (USD)", "14.00", "168.00", ""),
			[]string{"Total", "CAD", "", "", "420.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--account", "US Chequing", "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}}, doc.AccountFilter)
		assert.Equal(t, []recurringTotalJSON{{Currency: "CAD", PerYear: "420.00"}}, doc.Totals)
		assert.Equal(t, []string{"USD", "USD"}, []string{doc.Series[0].NativeCurrency, doc.Series[1].NativeCurrency})
	})
}

func Test_run_recurring_converts_an_ended_series_in_a_closed_account_at_the_rate_of_its_latest_charge(t *testing.T) {
	home := newHome(t)
	closed := usdChequingAccount("acct-old", 2)
	closed.Name, closed.Active = "Old USD", false
	old := inUSD(monthlySeries("Old Sub", 2026, time.February, slices.Repeat([]int64{800}, 5)...))
	for i := range old {
		old[i].account = "acct-old"
	}
	replaceStoreWithRates(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), closed}, old...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})

	t.Run("text has a blank Per year and no Total", func(t *testing.T) {
		stdout, stderr := runRecurring(t)

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			[]string{"Old Sub", "CAD (USD)", "month", "11.20", "", "2026-02-12", "2026-06-12", "ended, new", ""}),
			stdout)
	})

	t.Run("json has a null per_year, no totals and the first amount at the first charge's rate", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		require.Len(t, doc.Series, 1)
		assert.Nil(t, doc.Series[0].PerYear)
		assert.Equal(t, []string{"11.20", "10.40", "8.00", "8.00"},
			[]string{doc.Series[0].Amount, doc.Series[0].FirstAmount, doc.Series[0].NativeAmount, doc.Series[0].NativeFirstAmount})
		assert.Equal(t, []recurringTotalJSON{}, doc.Totals)
	})
}

func Test_run_recurring_lists_a_series_charged_before_the_first_rate_in_its_own_currency_with_a_warning(t *testing.T) {
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, slices.Concat(usdGym, cadRent)...),
		store.Rate{Date: day(2026, time.April, 1), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})
	const warning = "1 series with a charge dated before 2026-04-01, the first exchange rate in the store, " +
		"is listed in USD, not converted to CAD"

	t.Run("text shows the USD row plain, a USD Total and the series line", func(t *testing.T) {
		stdout, stderr := runRecurring(t)

		assert.Equal(t, warningLines([]string{warning}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json lists the row's currency apart from the document's and echoes the warning", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--json")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{warning}, doc.Warnings)
		require.Len(t, doc.Series, 2)
		assert.Equal(t, []string{"USD", "12.00", "USD"}, []string{doc.Series[1].Currency, doc.Series[1].Amount, doc.Series[1].NativeCurrency})
	})
}

func Test_run_recurring_in_usd_lists_cad_series_charged_before_the_first_rate_in_cad_with_a_warning(t *testing.T) {
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	cadPhone := monthlySeries("Phone", 2026, time.February, slices.Repeat([]int64{3000}, 8)...)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}, slices.Concat(usdGym, cadRent, cadPhone)...),
		store.Rate{Date: day(2026, time.April, 1), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"})
	const warning = "2 series with a charge dated before 2026-04-01, the first exchange rate in the store, " +
		"are listed in CAD, not converted to USD"

	t.Run("text lists the CAD rows plain ahead of the USD one", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in USD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Phone", "CAD", "30.00", "360.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "960.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json echoes the warning and names USD as the reporting currency", func(t *testing.T) {
		stdout, stderr := runRecurring(t, "--currency", "USD", "--json")

		assert.Equal(t, warningLines([]string{warning}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "USD", doc.Currency)
		assert.Equal(t, []string{warning}, doc.Warnings)
		assert.Equal(t, [][]string{
			{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"},
			{"Phone", "CAD", "30.00", "30.00", "CAD", "30.00", "30.00"},
			{"Gym", "USD", "12.00", "12.00", "USD", "12.00", "12.00"},
		}, amountsOf(doc.Series))
	})
}

func Test_run_recurring_on_a_store_without_rates_warns_only_when_a_usd_series_needs_converting(t *testing.T) {
	const noRates = "the store has no exchange rates, so amounts are listed in each account's own currency; run quarry sync to fetch them"
	home := newHome(t)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))
	cadRent := monthlySeries("Rent", 2026, time.February, slices.Repeat([]int64{5000}, 8)...)
	accounts := []store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)}

	t.Run("an unrated USD series in CAD draws the no-rates line and stays USD", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		stdout, stderr := runRecurring(t)

		assert.Equal(t, warningLines([]string{noRates}), stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			monthlyRow("Gym", "USD", "12.00", "144.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""},
			[]string{"Total", "USD", "", "", "144.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json for an unrated USD series carries the line and the series' own values as native", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		stdout, stderr := runRecurring(t, "--json")

		assert.Equal(t, warningLines([]string{noRates}), stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, "CAD", doc.Currency)
		assert.Equal(t, []string{noRates}, doc.Warnings)
		assert.Equal(t, [][]string{
			{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"},
			{"Gym", "USD", "12.00", "12.00", "USD", "12.00", "12.00"},
		}, amountsOf(doc.Series))
	})

	t.Run("all-CAD series in CAD are silent", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, cadRent...))

		stdout, stderr := runRecurring(t)

		assert.Empty(t, stderr)
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			monthlyRow("Rent", "CAD", "50.00", "600.00", ""),
			[]string{"Total", "CAD", "", "", "600.00", "", "", "", ""}),
			stdout)
	})

	t.Run("json for all-CAD series in CAD has no warnings", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, cadRent...))

		stdout, stderr := runRecurring(t, "--json")

		assert.Empty(t, stderr)
		doc := decodeRecurringJSON(t, stdout)
		assert.Equal(t, []string{}, doc.Warnings)
		assert.Equal(t, [][]string{{"Rent", "CAD", "50.00", "50.00", "CAD", "50.00", "50.00"}}, amountsOf(doc.Series))
	})

	t.Run("native is silent whatever the store holds", func(t *testing.T) {
		replaceStore(t, home, chargeRows(accounts, slices.Concat(usdGym, cadRent)...))

		_, stderr := runRecurring(t, "--currency", "native")

		assert.Empty(t, stderr)
	})
}

func Test_run_recurring_says_only_that_the_window_is_empty_on_an_unrated_usd_store_in_every_currency(t *testing.T) {
	home := newHome(t)
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
		inUSD(monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{1200}, 8)...))...))
	cases := []struct{ name, flag, wantCaption string }{
		{name: "CAD", flag: "CAD", wantCaption: ", amounts in CAD"},
		{name: "USD", flag: "USD", wantCaption: ", amounts in USD"},
		{name: "native", flag: "native", wantCaption: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			const emptyLine = "no recurring charges from 2030-01-01 to 2031-12-31; the store's transactions run 2026-02-12 to 2026-09-12"
			args := []string{"--since", "2030", "--until", "2031", "--currency", c.flag}
			var textOut, textErr, jsonOut, jsonErr bytes.Buffer

			textExit := runWith(context.Background(), append([]string{"recurring"}, args...), spendEnv(&textOut, &textErr))
			jsonExit := runWith(context.Background(), append([]string{"recurring", "--json"}, args...), spendEnv(&jsonOut, &jsonErr))

			require.Equal(t, 0, textExit, textErr.String())
			require.Equal(t, 0, jsonExit, jsonErr.String())
			assert.Equal(t, "Recurring charges 2030-01-01 to 2031-12-31 in all accounts"+c.wantCaption+"\n\n"+recurringHeaderOnly, textOut.String())
			assert.Equal(t, warningLines([]string{emptyLine}), textErr.String())
			doc := decodeRecurringJSON(t, jsonOut.String())
			assert.Equal(t, c.flag, doc.Currency)
			assert.Equal(t, []recurringTotalJSON{}, doc.Totals)
			assert.Equal(t, []recurringSeriesJSON{}, doc.Series)
			assert.Equal(t, []string{emptyLine}, doc.Warnings)
		})
	}
}
