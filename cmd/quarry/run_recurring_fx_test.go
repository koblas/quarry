package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recurringFXStore holds, under a fresh HOME, a steady USD Netflix and a Gym in CAD and in USD (12.00 to 15.00),
// with rates 1.30, 1.40 from June and, after the last charge, 1.50.
func recurringFXStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	usdNetflix := inUSD(monthlySeries("Netflix.com", 2026, time.February, slices.Repeat([]int64{1000}, 8)...))
	cadGym := monthlySeries("Gym", 2026, time.February, slices.Repeat([]int64{2000}, 8)...)
	usdGym := inUSD(monthlySeries("Gym", 2026, time.February, slices.Concat(slices.Repeat([]int64{1200}, 4), slices.Repeat([]int64{1500}, 4))...))
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{chequingAccount("acct-cad", 1), usdChequingAccount("acct-usd", 2)},
			slices.Concat(usdNetflix, cadGym, usdGym)...),
		store.Rate{Date: day(2026, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.June, 1), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.September, 20), USDCAD: money.Rate(1_500_000), Series: "FXUSDCAD"},
	)
}

func Test_run_recurring_detects_in_native_currency_and_converts_at_the_latest_charges_rate(t *testing.T) {
	recurringFXStore(t)

	t.Run("text lists converted amounts and finds no price change in a steady USD price", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, recurringTable("Recurring charges 2000-01-01 to 2026-09-29 in all accounts, amounts in CAD",
			[]string{"Gym", "CAD (USD)", "month", "21.00", "252.00", "2026-02-12", "2026-09-12", "active, new", "1: USD 12.00 -> USD 15.00 (+25.0%)"},
			[]string{"Gym", "CAD", "month", "20.00", "240.00", "2026-02-12", "2026-09-12", "active, new", ""},
			[]string{"Netflix.com", "CAD (USD)", "month", "14.00", "168.00", "2026-02-12", "2026-09-12", "active, new", ""},
			[]string{"Total", "CAD", "", "", "660.00", "", "", "", ""}),
			stdout.String())
	})

	t.Run("json carries the native currency and amounts beside the converted ones", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"recurring", "--since", "2000", "--json"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, recurringJSONDoc{
			Since:         "2000-01-01",
			Until:         "2026-09-29",
			Currency:      "CAD",
			AccountFilter: []recurringIDName{},
			Series: []recurringSeriesJSON{
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Gym", PayeeKey: new("gym"), Currency: "CAD", Amount: "21.00", FirstAmount: "15.60", PerYear: new("252.00"),
					NativeCurrency: "USD", NativeAmount: "15.00", NativeFirstAmount: "12.00",
					Accounts: []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}},
					PriceChanges: []recurringPriceChangeJSON{
						{Date: "2026-06-12", Currency: "USD", From: "12.00", To: "15.00", ChangePct: 25.0},
					},
				}),
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Gym", PayeeKey: new("gym"), Currency: "CAD", Amount: "20.00", FirstAmount: "20.00", PerYear: new("240.00"),
					NativeCurrency: "CAD", NativeAmount: "20.00", NativeFirstAmount: "20.00",
					Accounts: []recurringIDName{{ID: "acct-cad", Name: "Chequing"}},
				}),
				monthlyFXSeries(recurringSeriesJSON{
					Payee: "Netflix.com", PayeeKey: new("netflix-com"), Currency: "CAD", Amount: "14.00", FirstAmount: "13.00", PerYear: new("168.00"),
					NativeCurrency: "USD", NativeAmount: "10.00", NativeFirstAmount: "10.00",
					Accounts: []recurringIDName{{ID: "acct-usd", Name: "US Chequing"}},
				}),
			},
			Totals:   []recurringTotalJSON{{Currency: "CAD", PerYear: "660.00"}},
			Warnings: []string{},
		}, decodeRecurringJSON(t, stdout.String()))
	})
}

// monthlyFXSeries is s with the fields every series of the store shares: monthly, eight charges from 2026-02-12
// to 2026-09-12, one payee named by s.Payee, and an empty price-change list unless s names some.
func monthlyFXSeries(s recurringSeriesJSON) recurringSeriesJSON {
	s.Payees = []recurringIDName{{ID: "payee-" + s.Payee, Name: s.Payee}}
	s.Cadence, s.State, s.New, s.ChargeCount = "monthly", "active", true, 8
	s.FirstCharge, s.LastCharge = "2026-02-12", "2026-09-12"
	if s.PriceChanges == nil {
		s.PriceChanges = []recurringPriceChangeJSON{}
	}
	return s
}
