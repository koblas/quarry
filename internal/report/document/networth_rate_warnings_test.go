package document_test

import (
	"math/big"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/report"
	"github.com/koblas/quarry/internal/report/document"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
)

const (
	rateTail = "the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; " +
		"pass --currency native to list them"
	rateTailUSD = "the first exchange rate in the store, are not converted to USD and are left out of the USD total; " +
		"pass --currency native to list them"
	rateTailParameter = "the first exchange rate in the store, are not converted to CAD and are left out of the CAD total; " +
		"pass currency native to list them"
	noRatesLine = "the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
		"pass --currency native to list them, or run quarry sync to fetch rates"
	noRatesLineU = "the store has no exchange rates, so CAD balances are not converted to USD and are left out of the USD total; " +
		"pass --currency native to list them, or run quarry sync to fetch rates"
	noRatesLineParameter = "the store has no exchange rates, so USD balances are not converted to CAD and are left out of the CAD total; " +
		"pass currency native to list them, or run quarry sync to fetch rates"
)

func rateDay(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 0, 0, 0, 0, time.UTC)
}

// usdRow is a USD balance of cents with no CAD value, in an account of accountType.
func usdRow(accountType string, cents int64) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: "USD", Balance: big.NewInt(cents), BalanceUSD: big.NewInt(cents)}
}

// cadRow is a CAD balance of cents with no USD value, in an account of accountType.
func cadRow(accountType string, cents int64) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: "CAD", Balance: big.NewInt(cents), BalanceCAD: big.NewInt(cents)}
}

// convertedRow is a USD balance of cents that a rate converts to CAD.
func convertedRow(accountType string, cents int64) store.NetWorthRow {
	return store.NetWorthRow{Type: accountType, Currency: "USD", Balance: big.NewInt(cents), BalanceCAD: big.NewInt(cents), BalanceUSD: big.NewInt(cents)}
}

func rateSnapshot(currency money.Currency, first time.Time, rows ...store.NetWorthRow) report.NetWorth {
	asOf := rateDay(time.March, 5)
	return report.NetWorth{
		Dates: []report.NetWorthDate{{Date: asOf, Rows: rows}}, AsOf: asOf, Currency: currency, FirstRate: first,
	}
}

func rateHistory(currency money.Currency, first time.Time, dates ...report.NetWorthDate) report.NetWorth {
	window := store.Window{Since: dates[0].Date, Until: dates[len(dates)-1].Date}
	return report.NetWorth{Dates: dates, AsOf: window.Until, Window: &window, Currency: currency, FirstRate: first}
}

func Test_net_worth_warnings_say_which_day_is_before_the_first_rate(t *testing.T) {
	n := rateSnapshot(money.CAD, rateDay(time.March, 10), usdRow("chequing", 80_000))

	got := document.NetWorthWarnings(n, document.NativeFlag)

	assert.Equal(t, []string{"USD balances on 2026-03-05, before 2026-03-10, " + rateTail}, got)
}

func Test_net_worth_warnings_swap_the_currencies_in_a_usd_report(t *testing.T) {
	n := rateSnapshot(money.USD, rateDay(time.March, 10), cadRow("chequing", 80_000))

	got := document.NetWorthWarnings(n, document.NativeFlag)

	assert.Equal(t, []string{"CAD balances on 2026-03-05, before 2026-03-10, " + rateTailUSD}, got)
}

func Test_net_worth_warnings_say_the_store_has_no_rates_when_it_holds_none(t *testing.T) {
	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{"snapshot in CAD", rateSnapshot(money.CAD, time.Time{}, usdRow("chequing", 80_000)), noRatesLine},
		{"snapshot in USD", rateSnapshot(money.USD, time.Time{}, cadRow("chequing", 80_000)), noRatesLineU},
		{
			"history in CAD",
			rateHistory(money.CAD, time.Time{},
				report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}},
				report.NetWorthDate{Date: rateDay(time.February, 28), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}}),
			noRatesLine,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.NetWorthWarnings(c.n, document.NativeFlag))
		})
	}
}

func Test_net_worth_warnings_advise_the_surface_s_own_way_to_list_balances_natively(t *testing.T) {
	jan := report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}}
	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{"snapshot", rateSnapshot(money.CAD, rateDay(time.March, 10), usdRow("chequing", 80_000)), "USD balances on 2026-03-05, before 2026-03-10, " + rateTailParameter},
		{"history", rateHistory(money.CAD, rateDay(time.March, 10), jan), "USD balances on 1 month end before 2026-03-10, " + rateTailParameter},
		{"no rates in the store", rateSnapshot(money.CAD, time.Time{}, usdRow("chequing", 80_000)), noRatesLineParameter},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.NetWorthWarnings(c.n, document.NativeParameter))
		})
	}
}

func Test_net_worth_history_warnings_count_the_month_ends_before_the_first_rate(t *testing.T) {
	jan := report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}}
	feb := report.NetWorthDate{Date: rateDay(time.February, 28), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}}
	mar := report.NetWorthDate{Date: rateDay(time.March, 31), Rows: []store.NetWorthRow{convertedRow("chequing", 80_000)}}
	first := rateDay(time.March, 10)

	cases := []struct {
		name string
		n    report.NetWorth
		want string
	}{
		{"one month end", rateHistory(money.CAD, first, jan, mar), "USD balances on 1 month end before 2026-03-10, " + rateTail},
		{"several month ends", rateHistory(money.CAD, first, jan, feb, mar), "USD balances on 2 month ends before 2026-03-10, " + rateTail},
		{
			"CAD balances in a USD report",
			rateHistory(money.USD, first,
				report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{cadRow("chequing", 80_000)}},
				report.NetWorthDate{Date: rateDay(time.February, 28), Rows: []store.NetWorthRow{cadRow("chequing", 80_000)}}),
			"CAD balances on 2 month ends before 2026-03-10, " + rateTailUSD,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, []string{c.want}, document.NetWorthWarnings(c.n, document.NativeFlag))
		})
	}
}

func Test_net_worth_history_warnings_count_a_month_end_once_however_many_types_need_a_rate(t *testing.T) {
	jan := report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000), usdRow("savings", 5_000)}}

	got := document.NetWorthWarnings(rateHistory(money.CAD, rateDay(time.March, 10), jan), document.NativeFlag)

	assert.Equal(t, []string{"USD balances on 1 month end before 2026-03-10, " + rateTail}, got)
}

func Test_net_worth_warnings_leave_out_a_month_end_whose_rows_all_convert(t *testing.T) {
	jan := report.NetWorthDate{Date: rateDay(time.January, 31), Rows: []store.NetWorthRow{usdRow("chequing", 80_000)}}
	feb := report.NetWorthDate{Date: rateDay(time.February, 28), Rows: []store.NetWorthRow{convertedRow("chequing", 80_000)}}

	got := document.NetWorthWarnings(rateHistory(money.CAD, rateDay(time.February, 1), jan, feb), document.NativeFlag)

	assert.Equal(t, []string{"USD balances on 1 month end before 2026-02-01, " + rateTail}, got)
}

func Test_net_worth_warnings_say_nothing_about_rates_when_no_balance_needs_one(t *testing.T) {
	cases := []struct {
		name string
		n    report.NetWorth
	}{
		{"every balance converts", rateSnapshot(money.CAD, rateDay(time.March, 1), convertedRow("chequing", 80_000))},
		{"the only unconverted balance is zero", rateSnapshot(money.CAD, rateDay(time.March, 10), usdRow("chequing", 0))},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.NetWorthWarnings(c.n, document.NativeFlag)

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func Test_net_worth_native_listing_has_no_rate_warning(t *testing.T) {
	cases := []struct {
		name  string
		first time.Time
	}{
		{"before the first rate", rateDay(time.March, 10)},
		{"no rates in the store", time.Time{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := document.NetWorthWarnings(rateSnapshot(money.Native, c.first, usdRow("chequing", 80_000)), document.NativeFlag)

			assert.NotNil(t, got)
			assert.Empty(t, got)
		})
	}
}

func Test_net_worth_warnings_put_the_rate_line_after_the_no_price_line(t *testing.T) {
	n := rateSnapshot(money.CAD, rateDay(time.March, 10), usdRow("chequing", 80_000))
	n.Unvalued = []store.UnvaluedHolding{unpriced("a-1", "Brokerage", "s-1", "Acme", 5)}
	n.AsOf = leftOutDay(5)

	got := document.NetWorthWarnings(n, document.NativeFlag)

	assert.Equal(t, []string{
		`"Brokerage" holds 1 security with no price on or before 2026-03-05, so its balance leaves it out; enter a price in Quicken, then run quarry sync`,
		"USD balances on 2026-03-05, before 2026-03-10, " + rateTail,
	}, got)
}

func Test_NewNetWorth_writes_a_null_converted_balance_and_a_total_for_the_rows_no_rate_converts(t *testing.T) {
	n := rateSnapshot(money.CAD, rateDay(time.March, 10), usdRow("chequing", 80_000), cadRow("savings", 20_000))
	n.Dates[0].Totals = []report.NetWorthTotal{
		{Currency: "CAD", Value: big.NewInt(20_000)},
		{Currency: "USD", Value: big.NewInt(80_000)},
	}

	got := netWorthJSON(t, n, nil)

	assert.Equal(t, []any{map[string]any{
		"date": "2026-03-05",
		"balances": []any{
			map[string]any{"type": "chequing", "currency": "USD", "balance": "800.00", "converted_balance": nil},
			map[string]any{"type": "savings", "currency": "CAD", "balance": "200.00", "converted_balance": "200.00"},
		},
		"totals": []any{
			map[string]any{"currency": "CAD", "value": "200.00"},
			map[string]any{"currency": "USD", "value": "800.00"},
		},
	}}, got["dates"])
}
