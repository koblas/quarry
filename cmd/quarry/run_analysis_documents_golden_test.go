// The bytes quarry spend, cashflow, recurring and anomalies print, stdout and stderr, as
// run_analysis_documents_test.go names them.
package main

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
