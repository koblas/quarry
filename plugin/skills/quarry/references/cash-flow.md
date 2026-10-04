# Cash flow

Use this for income, spending, what was left over, and the savings rate. `quarry cashflow` answers by month or year; one recipe splits income by category. SKILL.md sections 2 and 3 set the rules for every number you quote.

## Reading the result

- `quarry cashflow --by month --json` gives one entry per month in `periods`, and `quarry cashflow --by year --json` one per year. The default is `month`.
- Each period has `income`, `spent`, `net` and `savings_rate_pct`. `net` is income less spending. `totals` has the same four for the whole window, one entry per currency.
- Income and spending follow the rules of `quarry spend`: transfers between the user's own accounts, Quicken's system categories, transactions marked "exclude from reports" and accounts that Quicken leaves out of reports are not counted, and refunds are netted. `spent` equals the total of `quarry spend` for the same period, accounts and currency.
- Uncategorized splits count as income when they bring money in and as spending when they take money out. If the question is about data quality, `quarry findings` lists the payees behind them.

## Savings rate

- The savings rate is `net` divided by `income`, as a percentage in `savings_rate_pct`.
- When income is zero or less the rate is `n/a`: `savings_rate_pct` is null in the JSON and the table prints `n/a`. Say the rate is `n/a` because income was zero or less. Do not compute one yourself, and do not call it zero.

## Partial periods

- `partial` is true for a period that `--since` or `--until` cuts short. Say so when you quote it, and do not compare a partial period with a whole one as if they were equal.
- Dates are `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, and both ends are included. Without them the period is January 1 this year up to today.

## Currencies and accounts

- Amounts are in CAD unless the user asks for another currency or has set `reporting.currency` in the config file. State the currency with every total.
- `quarry cashflow --currency USD --json` shows USD. `quarry cashflow --currency native --json` lists CAD and USD separately, never added together.
- Amounts dated before the first stored rate stay in their own currency on rows of their own, and `warnings` says so. Report those rows separately and relay the warning.
- `quarry cashflow --account Chequing --json` counts only that account's name or id. Repeat `--account` for more.

## Income by category

For "where did the income come from", use `references/sql/income-by-category.sql`. Read the file, change only the values on its `params` row, and run the whole text with `quarry sql --json -` on stdin as SKILL.md section 5 shows. It reads `v_cash_flow`, so it follows the same rules as `quarry cashflow`.

| Param | Value |
| --- | --- |
| `since`, `until` | Dates; both ends are included. |
| `currency` | `'CAD'` or `'USD'`. |

- The shipped values are the start of this year to today, in CAD. Change them to match the question and say in the answer what you asked for.
- Each result row has `category`, `currency` and `income`. A split with no category shows as `(uncategorized)`.
- The recipe's total for a period equals `income` from `quarry cashflow --by year` for the same dates and currency. If you quote both, they should agree.
- A split dated before the first stored rate appears on a row of its own with its native currency in `currency` and `income` in that currency. Report it separately.
- For tax questions, give the totals for the categories the user names and say they are figures to review. quarry has no tax-line data.
