package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anomaliesFXStore holds, under a fresh HOME, a USD Hardware payee with five earlier charges (median 40.00),
// a 250.00 charge on 2026-03-02 and a 90.00 charge on 2026-05-04. The rates are 1.30 for the earlier charges,
// 1.40 from the 250.00 charge's date and, after both, 1.50.
func anomaliesFXStore(t *testing.T) {
	t.Helper()
	home := newHome(t)
	charges := make([]chargeTxn, 0, 7)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		charges = append(charges, groceryCharge("Hardware", day(2025, time.March, 3+7*i), cents))
	}
	charges = append(charges,
		groceryCharge("Hardware", day(2026, time.March, 2), 25000),
		groceryCharge("Hardware", day(2026, time.May, 4), 9000),
	)
	replaceStoreWithRates(t, home,
		chargeRows([]store.Account{usdChequingAccount("acct-usd", 1)}, inUSD(charges)...),
		store.Rate{Date: day(2025, time.January, 2), USDCAD: money.Rate(1_300_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.March, 2), USDCAD: money.Rate(1_400_000), Series: "FXUSDCAD"},
		store.Rate{Date: day(2026, time.September, 20), USDCAD: money.Rate(1_500_000), Series: "FXUSDCAD"},
	)
}

func Test_run_anomalies_judges_in_native_currency_and_shows_converted_amounts(t *testing.T) {
	anomaliesFXStore(t)

	t.Run("text converts Amount and Usual at the charge's rate and keeps Times and the footer native", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Empty(t, stderr.String())
		assert.Equal(t, anomaliesTable("Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD", "2 charges checked",
			[]string{"2026-03-02", "US Chequing (USD)", "Hardware", "Food:Groceries", "350.00", "56.00", "6.3x", "payee, 5 earlier"}),
			stdout.String())
	})

	t.Run("json carries the native currency, amount and usual beside the converted ones", func(t *testing.T) {
		exitCode, stdout, stderr := runSpendCapture(context.Background(), []string{"anomalies", "--json"})

		require.Equal(t, 0, exitCode, stderr.String())
		assert.Equal(t, anomaliesJSONDoc{
			Since:         "2026-01-01",
			Until:         "2026-09-29",
			Currency:      "CAD",
			AccountFilter: []recurringIDName{},
			Anomalies: []anomalyJSON{{
				TransactionID:  "txn-usd-Hardware2026-03-02",
				Date:           "2026-03-02",
				AccountID:      "acct-usd",
				Account:        "US Chequing",
				Currency:       "CAD",
				Payee:          new("Hardware"),
				Category:       new("Food:Groceries"),
				Amount:         "350.00",
				Baseline:       "payee",
				Usual:          "56.00",
				NativeCurrency: "USD",
				NativeAmount:   "250.00",
				NativeUsual:    "40.00",
				Earlier:        5,
				Times:          6.3,
			}},
			Checked:   2,
			NotJudged: 0,
			Warnings:  []string{},
		}, decodeAnomaliesJSON(t, stdout.String()))
	})
}
