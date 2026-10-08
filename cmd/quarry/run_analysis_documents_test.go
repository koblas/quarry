// run is unexported, so its tests live in package main rather than
// importing main from outside.
package main

import (
	"context"
	"testing"
	"time"

	"github.com/koblas/quarry/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// analysisRun is one command line over a store and the exact bytes it prints.
type analysisRun struct {
	name   string
	store  func(t *testing.T, home string)
	args   []string
	stdout string
	stderr string
}

// Byte-for-byte on purpose: the recurring and anomalies pins decode their JSON and
// ignore key order, and these four reports are rendered by more than one surface.
func Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte(t *testing.T) {
	for _, c := range analysisRuns() {
		t.Run(c.name, func(t *testing.T) {
			home := newHome(t)
			c.store(t, home)

			exitCode, stdout, stderr := runSpendCapture(context.Background(), c.args)

			require.Equal(t, 0, exitCode, stderr.String())
			assert.Equal(t, c.stdout, stdout.String())
			assert.Equal(t, c.stderr, stderr.String())
		})
	}
}

// analysisRuns is the table of command lines over stores and the bytes each must print.
func analysisRuns() []analysisRun {
	populatedAccounts := []string{"--account", "Chequing", "--account", "US Chequing", "--account", "Linked", "--account", "Old Card"}
	args := func(command string, more ...string) []string {
		return append([]string{command}, more...)
	}
	cases := []analysisRun{
		{
			name: "spend --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("spend", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenSpendPopulatedJSONStdout, stderr: goldenSpendPopulatedJSONStderr,
		},
		{
			name: "spend text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("spend", populatedAccounts...), stdout: goldenSpendPopulatedTextStdout, stderr: goldenSpendPopulatedTextStderr,
		},
		{
			name: "cashflow --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("cashflow", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenCashflowPopulatedJSONStdout, stderr: goldenCashflowPopulatedJSONStderr,
		},
		{
			name: "cashflow text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("cashflow", populatedAccounts...), stdout: goldenCashflowPopulatedTextStdout, stderr: goldenCashflowPopulatedTextStderr,
		},
		{
			name: "recurring --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("recurring", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenRecurringPopulatedJSONStdout, stderr: goldenRecurringPopulatedJSONStderr,
		},
		{
			name: "recurring text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("recurring", populatedAccounts...), stdout: goldenRecurringPopulatedTextStdout, stderr: goldenRecurringPopulatedTextStderr,
		},
		{
			name: "anomalies --json over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("anomalies", append([]string{"--json"}, populatedAccounts...)...), stdout: goldenAnomaliesPopulatedJSONStdout, stderr: goldenAnomaliesPopulatedJSONStderr,
		},
		{
			name: "anomalies text over named accounts, a left-out pair and a USD span across the first rate", store: populatedAnalysisStore,
			args: args("anomalies", populatedAccounts...), stdout: goldenAnomaliesPopulatedTextStdout, stderr: goldenAnomaliesPopulatedTextStderr,
		},

		{
			name: "spend --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("spend", "--json"), stdout: goldenSpendEmptyJSONStdout, stderr: goldenSpendEmptyJSONStderr,
		},
		{
			name: "spend text over an empty window", store: emptyWindowAnalysisStore,
			args: args("spend"), stdout: goldenSpendEmptyTextStdout, stderr: goldenSpendEmptyTextStderr,
		},
		{
			name: "cashflow --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("cashflow", "--json"), stdout: goldenCashflowEmptyJSONStdout, stderr: goldenCashflowEmptyJSONStderr,
		},
		{
			name: "cashflow text over an empty window", store: emptyWindowAnalysisStore,
			args: args("cashflow"), stdout: goldenCashflowEmptyTextStdout, stderr: goldenCashflowEmptyTextStderr,
		},
		{
			name: "recurring --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("recurring", "--json"), stdout: goldenRecurringEmptyJSONStdout, stderr: goldenRecurringEmptyJSONStderr,
		},
		{
			name: "recurring text over an empty window", store: emptyWindowAnalysisStore,
			args: args("recurring"), stdout: goldenRecurringEmptyTextStdout, stderr: goldenRecurringEmptyTextStderr,
		},
		{
			name: "anomalies --json over an empty window", store: emptyWindowAnalysisStore,
			args: args("anomalies", "--json"), stdout: goldenAnomaliesEmptyJSONStdout, stderr: goldenAnomaliesEmptyJSONStderr,
		},
		{
			name: "anomalies text over an empty window", store: emptyWindowAnalysisStore,
			args: args("anomalies"), stdout: goldenAnomaliesEmptyTextStdout, stderr: goldenAnomaliesEmptyTextStderr,
		},

		{
			name: "spend --by tag --json with a split carrying two tags", store: multiTagAnalysisStore,
			args: args("spend", "--by", "tag", "--json"), stdout: goldenSpendMultiTagJSONStdout, stderr: goldenSpendMultiTagJSONStderr,
		},
		{
			name: "spend --by tag text with a split carrying two tags", store: multiTagAnalysisStore,
			args: args("spend", "--by", "tag"), stdout: goldenSpendMultiTagTextStdout, stderr: goldenSpendMultiTagTextStderr,
		},
	}

	return cases
}

// populatedAnalysisStore holds CAD and USD accounts plus one linked-tracking and one
// not-in-reports account; USD charges run on both sides of the first rate, 2026-03-01.
func populatedAnalysisStore(t *testing.T, home string) {
	t.Helper()
	accounts := []store.Account{
		chequingAccount("acct-cad", 1),
		usdChequingAccount("acct-usd", 2),
		{ID: "acct-linked", SourceID: 3, Name: "Linked", Type: "retirement", Currency: "CAD", Active: true, LinkedTracking: true},
		{ID: "acct-old", SourceID: 4, Name: "Old Card", Type: "credit_card", Currency: "CAD", Active: true, NotInReports: true},
	}
	charges := monthlySeries("Netflix.com", 2026, time.February, 999, 999, 999, 999, 1199, 1199, 1199, 1199)
	charges = append(charges, hardwareHistory()...)
	charges = append(charges, bigHardware())
	charges = append(charges, inUSD(lumberHistoryWithBigCharge())...)
	charges = append(charges, inUSD(monthlySeries("Spotify", 2025, time.November, 1099, 1099, 1099, 1099, 1099, 1099, 1099))...)
	charges = append(charges,
		fuelCharge("acct-cad", day(2026, time.May, 2), 4500),
		salary("pay-march", day(2026, time.March, 1), 500000),
		salary("pay-april", day(2026, time.April, 1), 500000),
		onAccount(groceryCharge("Whole Foods", day(2026, time.March, 11), 90000), "acct-linked"),
		onAccount(groceryCharge("Costco", day(2026, time.March, 12), 40000), "acct-old"),
	)
	rows := chargeRows(accounts, charges...)
	rows.Categories = append(rows.Categories,
		store.Category{ID: "cat-salary", SourceID: 3, Name: "Salary", FullPath: "Income:Salary", Kind: "income"})
	replaceStoreWithRates(t, home, rows, usdRate(day(2026, time.March, 1), 1_300_000))
}

// lumberHistoryWithBigCharge is five 2025 Lumber charges of 38.00 to 42.00, then a 250.00 one on 2026-02-02.
func lumberHistoryWithBigCharge() []chargeTxn {
	history := make([]chargeTxn, 0, 6)
	for i, cents := range []int64{3800, 3900, 4000, 4100, 4200} {
		history = append(history, groceryCharge("Lumber", day(2025, time.March, 3+7*i), cents))
	}
	return append(history, groceryCharge("Lumber", day(2026, time.February, 2), 25000))
}

// emptyWindowAnalysisStore holds charges only before 2026, so the default window is empty.
func emptyWindowAnalysisStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, chargeRows([]store.Account{chequingAccount("acct-cad", 1)}, bakeryHistory()...))
}

// multiTagAnalysisStore holds a grocery split tagged vacation and alpha beside a fuel split tagged alpha.
func multiTagAnalysisStore(t *testing.T, home string) {
	t.Helper()
	replaceStore(t, home, spendRows([]store.Account{chequingAccount("acct-cad", 1)},
		spendSplit{
			id: "s01", account: "acct-cad", category: "cat-groceries", currency: "CAD", day: day(2026, time.March, 10), cents: -12000,
			tags: []string{"tag-vacation", "tag-alpha"},
		},
		spendSplit{
			id: "s02", account: "acct-cad", category: "cat-fuel", currency: "CAD", day: day(2026, time.April, 2), cents: -3000,
			tags: []string{"tag-alpha"},
		},
	))
}

// fuelCharge is a fuel charge of cents on account, with no payee.
func fuelCharge(account string, date time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: "fuel" + date.Format(time.DateOnly), account: account, currency: "CAD", day: date,
		splits: []chargeSplit{{category: "cat-fuel", cents: -cents}},
	}
}

// salary is an income deposit of cents on the CAD chequing account.
func salary(id string, date time.Time, cents int64) chargeTxn {
	return chargeTxn{
		id: id, account: "acct-cad", payee: "Employer", currency: "CAD", day: date,
		splits: []chargeSplit{{category: "cat-salary", cents: cents}},
	}
}

// onAccount is charge moved to account.
func onAccount(charge chargeTxn, account string) chargeTxn {
	charge.account = account
	return charge
}

const (
	goldenSpendPopulatedJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "CAD",
  "account_filter": [
    {
      "id": "acct-cad",
      "name": "Chequing"
    },
    {
      "id": "acct-usd",
      "name": "US Chequing"
    },
    {
      "id": "acct-linked",
      "name": "Linked"
    },
    {
      "id": "acct-old",
      "name": "Old Card"
    }
  ],
  "rows": [
    {
      "category": "Auto:Fuel",
      "currency": "CAD",
      "spent": "45.00"
    },
    {
      "category": "Food:Groceries",
      "currency": "CAD",
      "spent": "380.79"
    },
    {
      "category": "Food:Groceries",
      "currency": "USD",
      "spent": "271.98"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "425.79"
    },
    {
      "currency": "USD",
      "spent": "271.98"
    }
  ],
  "warnings": [
    "account \"Linked\" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do",
    "account \"Old Card\" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync",
    "3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
  ]
}
`
	goldenSpendPopulatedJSONStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD
`
)

const (
	goldenSpendPopulatedTextStdout = `Spending 2026-01-01 to 2026-09-29 in Chequing, US Chequing, Linked, Old Card, amounts in CAD

Category        Currency   Spent
Auto:Fuel       CAD        45.00
Food:Groceries  CAD       380.79
Food:Groceries  USD       271.98
Total           CAD       425.79
Total           USD       271.98
`
	goldenSpendPopulatedTextStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD
`
)

const (
	goldenCashflowPopulatedJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "month",
  "currency": "CAD",
  "account_filter": [
    {
      "id": "acct-cad",
      "name": "Chequing"
    },
    {
      "id": "acct-usd",
      "name": "US Chequing"
    },
    {
      "id": "acct-linked",
      "name": "Linked"
    },
    {
      "id": "acct-old",
      "name": "Old Card"
    }
  ],
  "periods": [
    {
      "period": "2026-01",
      "currency": "CAD",
      "income": "0.00",
      "spent": "0.00",
      "net": "0.00",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-01",
      "currency": "USD",
      "income": "0.00",
      "spent": "10.99",
      "net": "-10.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-02",
      "currency": "CAD",
      "income": "0.00",
      "spent": "9.99",
      "net": "-9.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-02",
      "currency": "USD",
      "income": "0.00",
      "spent": "260.99",
      "net": "-260.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-03",
      "currency": "CAD",
      "income": "5000.00",
      "spent": "274.28",
      "net": "4725.72",
      "savings_rate_pct": 94.5,
      "partial": false
    },
    {
      "period": "2026-04",
      "currency": "CAD",
      "income": "5000.00",
      "spent": "24.28",
      "net": "4975.72",
      "savings_rate_pct": 99.5,
      "partial": false
    },
    {
      "period": "2026-05",
      "currency": "CAD",
      "income": "0.00",
      "spent": "69.28",
      "net": "-69.28",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-06",
      "currency": "CAD",
      "income": "0.00",
      "spent": "11.99",
      "net": "-11.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-07",
      "currency": "CAD",
      "income": "0.00",
      "spent": "11.99",
      "net": "-11.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-08",
      "currency": "CAD",
      "income": "0.00",
      "spent": "11.99",
      "net": "-11.99",
      "savings_rate_pct": null,
      "partial": false
    },
    {
      "period": "2026-09",
      "currency": "CAD",
      "income": "0.00",
      "spent": "11.99",
      "net": "-11.99",
      "savings_rate_pct": null,
      "partial": true
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "income": "10000.00",
      "spent": "425.79",
      "net": "9574.21",
      "savings_rate_pct": 95.7
    },
    {
      "currency": "USD",
      "income": "0.00",
      "spent": "271.98",
      "net": "-271.98",
      "savings_rate_pct": null
    }
  ],
  "warnings": [
    "account \"Linked\" uses linked account tracking in Quicken, so cashflow leaves it out, as Quicken's reports do",
    "account \"Old Card\" is not used in reports in Quicken, so cashflow leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync",
    "3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD"
  ]
}
`
	goldenCashflowPopulatedJSONStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so cashflow leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so cashflow leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD
`
)

const (
	goldenCashflowPopulatedTextStdout = `Cash flow 2026-01-01 to 2026-09-29 in Chequing, US Chequing, Linked, Old Card, amounts in CAD

Month    Currency     Income   Spent       Net  Savings rate  Status
2026-01  CAD            0.00    0.00      0.00           n/a
2026-01  USD            0.00   10.99    -10.99           n/a
2026-02  CAD            0.00    9.99     -9.99           n/a
2026-02  USD            0.00  260.99   -260.99           n/a
2026-03  CAD        5,000.00  274.28  4,725.72         94.5%
2026-04  CAD        5,000.00   24.28  4,975.72         99.5%
2026-05  CAD            0.00   69.28    -69.28           n/a
2026-06  CAD            0.00   11.99    -11.99           n/a
2026-07  CAD            0.00   11.99    -11.99           n/a
2026-08  CAD            0.00   11.99    -11.99           n/a
2026-09  CAD            0.00   11.99    -11.99           n/a  partial
Total    CAD       10,000.00  425.79  9,574.21         95.7%
Total    USD            0.00  271.98   -271.98           n/a
`
	goldenCashflowPopulatedTextStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so cashflow leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so cashflow leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 3 transactions dated before 2026-03-01, the first exchange rate in the store, are listed in USD, not converted to CAD
`
)

const (
	goldenRecurringPopulatedJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "currency": "CAD",
  "account_filter": [
    {
      "id": "acct-cad",
      "name": "Chequing"
    },
    {
      "id": "acct-usd",
      "name": "US Chequing"
    },
    {
      "id": "acct-linked",
      "name": "Linked"
    },
    {
      "id": "acct-old",
      "name": "Old Card"
    }
  ],
  "series": [
    {
      "payee": "Netflix.com",
      "payee_key": "netflix-com",
      "payees": [
        {
          "id": "payee-Netflix.com",
          "name": "Netflix.com"
        }
      ],
      "currency": "CAD",
      "cadence": "monthly",
      "amount": "11.99",
      "first_amount": "9.99",
      "per_year": "143.88",
      "native_currency": "CAD",
      "native_amount": "11.99",
      "native_first_amount": "9.99",
      "first_charge": "2026-02-12",
      "last_charge": "2026-09-12",
      "charge_count": 8,
      "state": "active",
      "new": true,
      "accounts": [
        {
          "id": "acct-cad",
          "name": "Chequing"
        }
      ],
      "price_changes": [
        {
          "date": "2026-06-12",
          "currency": "CAD",
          "from": "9.99",
          "to": "11.99",
          "change_pct": 20
        }
      ]
    },
    {
      "payee": "Spotify",
      "payee_key": "spotify",
      "payees": [
        {
          "id": "payee-Spotify",
          "name": "Spotify"
        }
      ],
      "currency": "USD",
      "cadence": "monthly",
      "amount": "10.99",
      "first_amount": "10.99",
      "per_year": null,
      "native_currency": "USD",
      "native_amount": "10.99",
      "native_first_amount": "10.99",
      "first_charge": "2025-11-12",
      "last_charge": "2026-05-12",
      "charge_count": 7,
      "state": "ended",
      "new": false,
      "accounts": [
        {
          "id": "acct-usd",
          "name": "US Chequing"
        }
      ],
      "price_changes": []
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "per_year": "143.88"
    }
  ],
  "warnings": [
    "account \"Linked\" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do",
    "account \"Old Card\" is not used in reports in Quicken, so recurring leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync",
    "1 series with a charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD"
  ]
}
`
	goldenRecurringPopulatedJSONStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so recurring leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 1 series with a charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD
`
)

const (
	goldenRecurringPopulatedTextStdout = `Recurring charges 2026-01-01 to 2026-09-29 in Chequing, US Chequing, Linked, Old Card, amounts in CAD

Payee        Currency  Every  Amount  Per year  First       Last        Status       Price changes
Netflix.com  CAD       month   11.99    143.88  2026-02-12  2026-09-12  active, new  1: 9.99 -> 11.99 (+20.0%)
Spotify      USD       month   10.99            2025-11-12  2026-05-12  ended
Total        CAD                        143.88
`
	goldenRecurringPopulatedTextStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so recurring leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so recurring leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 1 series with a charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD
`
)

const (
	goldenAnomaliesPopulatedJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "currency": "CAD",
  "account_filter": [
    {
      "id": "acct-cad",
      "name": "Chequing"
    },
    {
      "id": "acct-usd",
      "name": "US Chequing"
    },
    {
      "id": "acct-linked",
      "name": "Linked"
    },
    {
      "id": "acct-old",
      "name": "Old Card"
    }
  ],
  "anomalies": [
    {
      "transaction_id": "txn-Hardware2026-03-02",
      "date": "2026-03-02",
      "account_id": "acct-cad",
      "account": "Chequing",
      "currency": "CAD",
      "payee": "Hardware",
      "category": "Food:Groceries",
      "amount": "250.00",
      "baseline": "payee",
      "usual": "40.00",
      "native_currency": "CAD",
      "native_amount": "250.00",
      "native_usual": "40.00",
      "earlier": 5,
      "times": 6.3
    },
    {
      "transaction_id": "txn-usd-Lumber2026-02-02",
      "date": "2026-02-02",
      "account_id": "acct-usd",
      "account": "US Chequing",
      "currency": "USD",
      "payee": "Lumber",
      "category": "Food:Groceries",
      "amount": "250.00",
      "baseline": "payee",
      "usual": "40.00",
      "native_currency": "USD",
      "native_amount": "250.00",
      "native_usual": "40.00",
      "earlier": 5,
      "times": 6.3
    }
  ],
  "checked": 16,
  "not_judged": 0,
  "warnings": [
    "account \"Linked\" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do",
    "account \"Old Card\" is not used in reports in Quicken, so anomalies leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync",
    "1 charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD"
  ]
}
`
	goldenAnomaliesPopulatedJSONStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so anomalies leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 1 charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD
`
)

const (
	goldenAnomaliesPopulatedTextStdout = `Unusually large charges 2026-01-01 to 2026-09-29 in Chequing, US Chequing, Linked, Old Card, amounts in CAD

Date        Account            Payee     Category            Amount      Usual  Times  Compared with
2026-03-02  Chequing (CAD)     Hardware  Food:Groceries      250.00      40.00   6.3x  payee, 5 earlier
2026-02-02  US Chequing (USD)  Lumber    Food:Groceries  USD 250.00  USD 40.00   6.3x  payee, 5 earlier

16 charges checked
`
	goldenAnomaliesPopulatedTextStderr = `quarry: warning: account "Linked" uses linked account tracking in Quicken, so anomalies leaves it out, as Quicken's reports do
quarry: warning: account "Old Card" is not used in reports in Quicken, so anomalies leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync
quarry: warning: 1 charge dated before 2026-03-01, the first exchange rate in the store, is listed in USD, not converted to CAD
`
)

const (
	goldenSpendEmptyJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "category",
  "currency": "CAD",
  "account_filter": [],
  "rows": [],
  "totals": [],
  "warnings": [
    "no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31"
  ]
}
`
	goldenSpendEmptyJSONStderr = "quarry: warning: no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenSpendEmptyTextStdout = `Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD

Category  Currency  Spent
`
	goldenSpendEmptyTextStderr = "quarry: warning: no spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenCashflowEmptyJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "month",
  "currency": "CAD",
  "account_filter": [],
  "periods": [],
  "totals": [],
  "warnings": [
    "no income or spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31"
  ]
}
`
	goldenCashflowEmptyJSONStderr = "quarry: warning: no income or spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenCashflowEmptyTextStdout = `Cash flow 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD

Month  Currency  Income  Spent  Net  Savings rate  Status
`
	goldenCashflowEmptyTextStderr = "quarry: warning: no income or spending from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenRecurringEmptyJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "currency": "CAD",
  "account_filter": [],
  "series": [],
  "totals": [],
  "warnings": [
    "no recurring charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31"
  ]
}
`
	goldenRecurringEmptyJSONStderr = "quarry: warning: no recurring charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenRecurringEmptyTextStdout = `Recurring charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD

Payee  Currency  Every  Amount  Per year  First  Last  Status  Price changes
`
	goldenRecurringEmptyTextStderr = "quarry: warning: no recurring charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenAnomaliesEmptyJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "currency": "CAD",
  "account_filter": [],
  "anomalies": [],
  "checked": 0,
  "not_judged": 0,
  "warnings": [
    "no unusually large charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31"
  ]
}
`
	goldenAnomaliesEmptyJSONStderr = "quarry: warning: no unusually large charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenAnomaliesEmptyTextStdout = `Unusually large charges 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD

Date  Account  Payee  Category  Amount  Usual  Times  Compared with

0 charges checked
`
	goldenAnomaliesEmptyTextStderr = "quarry: warning: no unusually large charges from 2026-01-01 to 2026-09-29; the store's transactions run 2003-01-04 to 2025-12-31\n"
)

const (
	goldenSpendMultiTagJSONStdout = `{
  "since": "2026-01-01",
  "until": "2026-09-29",
  "by": "tag",
  "currency": "CAD",
  "account_filter": [],
  "rows": [
    {
      "tag": "alpha",
      "currency": "CAD",
      "spent": "150.00"
    },
    {
      "tag": "Vacation",
      "currency": "CAD",
      "spent": "120.00"
    }
  ],
  "totals": [
    {
      "currency": "CAD",
      "spent": "150.00"
    }
  ],
  "warnings": [
    "1 split carries more than one tag, so the rows add up to more than the total"
  ]
}
`
	goldenSpendMultiTagJSONStderr = "quarry: warning: 1 split carries more than one tag, so the rows add up to more than the total\n"
)

const (
	goldenSpendMultiTagTextStdout = `Spending 2026-01-01 to 2026-09-29 in all accounts, amounts in CAD

Tag       Currency   Spent
alpha     CAD       150.00
Vacation  CAD       120.00
Total     CAD       150.00
`
	goldenSpendMultiTagTextStderr = "quarry: warning: 1 split carries more than one tag, so the rows add up to more than the total\n"
)
