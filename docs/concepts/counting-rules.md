# How quarry counts money

A total is only as good as the rule that decided what went into it. quarry decides each rule once, in its own code, and every way of asking uses it: the command line, the Claude skill, the MCP tools, and the views that free-form SQL reads. This page states the rules.

## What counts as spending

Spending is every split in an expense category, plus uncategorized splits that take money out of an account. quarry leaves out:

- **Transfers between your own accounts.** Moving money from chequing to savings is neither spending nor income.
- **Quicken's system categories.**
- **Transactions marked "exclude from reports"** in Quicken.
- **Accounts that Quicken leaves out of reports.**

Closed accounts are counted: money spent from an account you later closed was still spent.

Income follows the same rules in the other direction. An uncategorized split counts as income when it brings money in and as spending when it takes money out, and the `uncategorized` finding lists the payees behind them.

`quarry spend` and `quarry cashflow` agree by construction: the spending in `quarry cashflow` for a period equals the total of `quarry spend` for the same period, accounts and currency.

`quarry search` is the exception on purpose. It finds every transaction, transfers and excluded ones included, and flags each as `transfer` or `excluded`, so a search result is a list to read, not a total to add up.

## Refunds

A refund in an expense category is netted against that category. A return to a store lowers the spending for the category it was bought under. When refunds exceed charges in a period, the category's spending is negative.

## Signs

An amount is negative when money leaves the account. Spending totals turn that around: money spent is a positive number.

## Splits

Quicken puts the category on each split line, not on the transaction. quarry counts splits, so a transaction divided between groceries and household goods adds to each category by its own amount. Every transaction equals the sum of its splits; `quarry sync` fails when one does not.

With `--by tag`, a split with several tags counts under each of them, so the rows can add up to more than the total. The total counts each split once.

## Periods

Dates are a year, a month or a day, and both ends are included. Without dates, `quarry spend`, `quarry cashflow`, `quarry recurring` and `quarry anomalies` cover January 1 of this year to today. Future-dated transactions are left out unless the period runs past today. A month or year that the period cuts short is marked partial, because comparing it with a whole one misleads.

## Currencies

Accounts are in CAD or USD, and amounts stay stored in the account's own currency. Reports convert to the reporting currency, CAD unless you set another:

- Each split converts at the Bank of Canada rate for its own date, then is rounded to the cent before it is added. A purchase from 2019 converts at the 2019 rate, not today's.
- On weekends, holidays and dates after the last stored rate, the latest earlier rate applies.
- Amounts dated before the first stored rate stay in their own currency, on rows of their own, with a warning.
- With `--currency native`, nothing converts. CAD and USD are listed separately and never added together.
- A transfer between a CAD and a USD account keeps both legs, each in its own currency. It is a transfer, not a gain, a loss or an expense.

Adjusted cost base is always in CAD. A purchase or sale in USD converts at the rate on its trade date, as the CRA requires.

## Savings rate

The savings rate is income less spending, divided by income. When income is zero or less there is no rate, and quarry prints `n/a`, not zero.

## Recurring charges

A recurring charge is the same payee, in the same currency, charged every week, month, quarter or year at a steady amount. quarry looks for series in all of your history and lists those running in the period you ask about.

- A series is **new** when its first charge falls in the period.
- A series has **ended** when no charge has come for more than 14 days (weekly), 45 days (monthly), 120 days (quarterly) or 400 days (annual).
- A **price change** is a step of more than 5% from one charge to the next. A series is found in its account's own currency, so a move in the exchange rate is never a price change.
- A bill whose amount changes by more than 5% on more than a quarter of its steps, such as electricity, is not a series. `quarry spend --by payee` covers those.
- A charge that comes off schedule starts the series again. A payee that charges in both CAD and USD has two series.

## Unusually large charges

A charge is unusually large when it is more than 2 times the median of the payee's earlier charges, given at least 3 of them. With fewer, it is compared with the category: more than 5 times the median of the category's earlier charges, given at least 10. Charges under 100.00 in the account's own currency are never listed.

Each charge is compared with all earlier history, whatever period you ask about, and in its account's own currency. A charge with no payee history and no category history cannot be judged; quarry counts those and reports the count, so an empty list is not mistaken for a clean one.

Possible duplicates are a finding, not an unusually large charge.

## Net worth and holdings

Net worth on a day is every account's balance on that day, converted at that day's rate. A brokerage or retirement account counts its cash plus the value of its holdings. Holdings are valued at the latest price Quicken has on or before the day, and quarry shows that price's date. Prices come from Quicken; quarry fetches none.

## Where the rules are written down for SQL

The store's views apply the same rules: `v_spending` for spending and `v_cash_flow` for income and spending. [How to query the store with SQL](../guides/query-with-sql.md) says how to use them.
