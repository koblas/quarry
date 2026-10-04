# Spending

Use this for "how much did we spend" questions. `quarry spend` answers by group and period; one recipe answers how a single category's or payee's spending changed over time. SKILL.md sections 2 and 3 set the rules for every number you quote.

## Groupings

Pick the grouping with `--by`. The default is `category`.

- `quarry spend --by category --json`: one row per category path such as `Food:Groceries`. Splits with no category that take money out sit in one row whose `category` is null; say so rather than guessing a category.
- `quarry spend --by payee --json`: one row per payee. `payee` is null for splits with no payee.
- `quarry spend --by tag --json`: one row per tag. A split with more than one tag counts under each of them, so the rows can add up to more than `totals`. Quote the total from `totals`, not from the rows. `tag` is null for splits with no tag.
- `quarry spend --by month --json`: one row per month as `YYYY-MM`, with months that had no spending listed as zero. `partial` is true for a month that `--since` or `--until` cuts short; say so when you quote that month.
- `quarry spend --account Chequing --json`: count only the account with this name or id. Repeat `--account` for more. `account_filter` in the result names the accounts the report was limited to.

## Periods

- `--since` and `--until` take `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, and both ends are included. A bare year or month covers all of it: `quarry spend --since 2024 --until 2024 --json` is the whole of 2024.
- Without them the period is January 1 this year up to today. Future-dated transactions are left out unless `--until` is later than today.
- The result's `since` and `until` are the dates quarry used. Name them in the answer.

## What counts

- Spending is every split in an expense category, plus uncategorized splits that take money out.
- Transfers between the user's own accounts, Quicken's system categories, transactions marked "exclude from reports" and accounts that Quicken leaves out of reports are not counted. Closed accounts are counted.
- Refunds in an expense category are netted against it, so a `spent` value can be negative. That means refunds were larger than charges in the period; it is not an error.
- `quarry search` lists transfers and excluded transactions that `quarry spend` leaves out; never add those to a spending total.

## Currencies

- Amounts are in CAD unless the user asks for another currency or has set `reporting.currency` in the config file. The result's `currency` says which one; state it with every total.
- `quarry spend --currency USD --json` shows USD. Each split converts at the Bank of Canada rate for its date, or the latest earlier rate on weekends, holidays and dates after the last stored rate.
- `quarry spend --currency native --json` lists CAD and USD separately in `rows` and `totals`. Never add them together.
- Amounts dated before the first stored rate stay in their own currency on rows of their own, and `warnings` says so. Report those rows separately and relay the warning.

## How spending changed over time

For one category or one payee over years or months, use `references/sql/spending-trend.sql` instead of running `quarry spend` once per period. Read the file, change only the values on its `params` row, and run the whole text with `quarry sql --json -` on stdin as SKILL.md section 5 shows. It reads `v_spending`, so it follows the same rules as `quarry spend`.

| Param | Value |
| --- | --- |
| `category` | Full category path. Matches that category and every category under it, ignoring case: `Food:Groceries` includes `Food:Groceries:Organic` but not `Foodies`. NULL means any category. |
| `payee` | Exact payee name, ignoring case. NULL means any payee. |
| `grain` | `'year'` or `'month'`. |
| `since`, `until` | Dates written `DATE 'YYYY-MM-DD'`; both ends are included. |
| `currency` | `'CAD'` or `'USD'`. Use the `currency` that `quarry spend --json` reports, so the trend matches the user's other totals; `'native'` lists each currency unconverted, never added together. |

- The shipped values are `Food:Groceries`, no payee, by year, from 2022-01-01 to today, in CAD. Change them to match the question and say in the answer what you asked for.
- Each result row has `period`, `currency` and `spent`. `period` is the first day of the year or month.
- A split dated before the first stored rate has no converted amount. It appears on a row of its own with its native currency in `currency` and `spent` in that currency. Report it separately.
- The first and last periods can be partial, because `since` and `until` cut them short. Say so when you quote one, and do not compare a partial period with a whole one as if they were equal.
- To compare two years, quote both rows and show the difference as arithmetic on them.
