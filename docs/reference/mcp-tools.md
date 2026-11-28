# MCP tools

`quarry mcp` runs a local MCP server over stdin and stdout and serves the 13 tools below. Each reads the store as it is at that request and returns the same numbers as the command it matches. No tool syncs, prunes snapshots, reads Quicken or changes the store.

## Conventions

| Convention | Value |
|---|---|
| Dates | `YYYY`, `YYYY-MM` or `YYYY-MM-DD`. For `since`, a year or month starts on its first day; for `until` and `as_of`, it ends on its last day. |
| `currency` | `CAD`, `USD`, or `native` to list each account's own currency separately. Default: `reporting.currency` in the config file, else CAD. |
| `accounts` | A list of accounts, each given by id or by name in any letter case. Omitted: every account. |
| List limit | 500 entries at most in any list. Totals and counts cover every entry. |
| Time limit | A call stops after 30 seconds. |

## Freshness and schema

### `sync_status`

Reports the snapshot the store was built from and when it was taken, the dates its transactions cover, the checks sync ran, the open findings, and Bank of Canada rate coverage. Matches `quarry status`. No parameters.

### `describe_schema`

Describes every table and view with its columns and types, the conventions for amounts, signs, transfers and currencies, the accounts, the category tree, and the first and last transaction dates. No parameters.

### `query`

Runs one read-only SQL statement in DuckDB's dialect and returns its columns and rows. Other files, databases and extensions are off. Of several statements, only the last one's rows come back. Matches `quarry sql`.

| Parameter | Description |
|---|---|
| `sql` | The statement. Required. |
| `limit` | Most rows to return, 1 to 500. Default: 500. |

## Spending and income

### `spending`

Totals spending for a period by category, payee, tag or month, with a total per currency. Matches `quarry spend`.

| Parameter | Description |
|---|---|
| `since` | First day to count. Default: January 1 of this year. |
| `until` | Last day to count. Default: today. Future-dated transactions count only when `until` is later than today. |
| `accounts` | Count only these accounts. |
| `currency` | See Conventions. |
| `by` | `category` (default), `payee`, `tag` or `month`. A split with several tags counts under each tag. |

### `cash_flow`

Reports income, spending, net and savings rate for each month or year of a period, with totals per currency. Savings rate is null when income is zero or less. Matches `quarry cashflow`.

| Parameter | Description |
|---|---|
| `since`, `until`, `accounts`, `currency` | As `spending`. |
| `by` | `month` (default) or `year`. |

### `recurring_charges`

Lists charges that repeat every week, month, quarter or year at a steady amount, with each series' cadence, latest amount, cost per year while active, price changes and accounts. Matches `quarry recurring`.

| Parameter | Description |
|---|---|
| `since` | List series still running on or after this date. Default: January 1 of this year. |
| `until` | List series that started on or before this date. Default: today. |
| `accounts` | List only series with a charge in one of these accounts. |
| `currency` | See Conventions. |

### `anomalies`

Lists charges in the period that are unusually large for their payee or category, newest first. Matches `quarry anomalies`.

| Parameter | Description |
|---|---|
| `since` | List charges dated on or after this date. Default: January 1 of this year. |
| `until` | List charges dated on or before this date. Default: today. |
| `accounts` | List only charges in these accounts; the payee's charges in other accounts still count as history. |
| `currency` | See Conventions. |

### `monthly_summary`

Summarizes one month: data freshness, findings counts, unusually large charges, recurring charges new in the month, and net worth at the month end beside the month before, with the change. Returns the document `quarry summary --json` prints.

| Parameter | Description |
|---|---|
| `month` | `YYYY-MM`, a month that has ended. Default: last month. |
| `currency` | See Conventions. |

## Finding transactions

### `search_transactions`

Finds transactions by text, date, account, category or amount, newest first. Transfers between the user's own accounts are included and flagged `transfer`; transactions Quicken's reports leave out are included and flagged `excluded`. Amounts are in each account's own currency. Matches `quarry search`.

| Parameter | Description |
|---|---|
| `text` | Words to find in payee names, transaction memos and split memos, in any letter case. Every character is literal. |
| `since` | Earliest date to list. Omitted: from the first transaction. |
| `until` | Latest date to list. Omitted: every later date, future-dated transactions included. |
| `accounts` | Search only these accounts. |
| `category` | Only transactions with a split in this category or one under it, by full path such as `Food:Groceries`, in any letter case. |
| `min` | Smallest amount to list, as a string such as `"19.99"`, compared without its sign. |
| `max` | Largest amount to list, as a string, compared without its sign. `min` and `max` with the same value find one amount. |
| `limit` | Most transactions to return, 1 to 500. Default: 500. `matched` counts every match. |

## Accounts and investments

### `holdings`

Lists the securities held on one day with share count, latest price and its date, and value. Cash in investment accounts is not included. Matches `quarry holdings`.

| Parameter | Description |
|---|---|
| `as_of` | Day to value holdings on. Default: today. |
| `accounts` | List only these accounts. |
| `currency` | See Conventions. |

### `net_worth`

Reports net worth on one day, or at each month end of a period, by account type and currency. Brokerage and retirement accounts count their cash plus their holdings' value. Matches `quarry networth`.

| Parameter | Description |
|---|---|
| `as_of` | Day to value net worth on. Default: today. Cannot be combined with `since` or `until`. |
| `since` | List net worth at each month end on or after this date. Default: January 1 of this year when `until` is given. |
| `until` | List net worth at each month end on or before this date. Default: today; a later date means today. |
| `currency` | See Conventions. |

### `acb`

Reports adjusted cost base and realized capital gains per tax year, in CAD: average cost per security pooled across non-registered accounts, with possible superficial losses marked, not adjusted. Matches `quarry acb`. It takes no `currency`.

| Parameter | Description |
|---|---|
| `year` | Tax year to report, up to this year. Omitted: every year. |
| `security` | A list of securities, each by id, ticker or name in any letter case. Omitted: every security. A security held only in registered accounts has no ACB and is left out, with a warning. |

## Data quality

### `data_quality`

Lists the findings the last sync recorded, each with its id, the suggested fix, and the transactions, accounts, payees or categories it is about. Matches `quarry findings`.

| Parameter | Description |
|---|---|
| `status` | `open` (default), `ignored`, `fixed` or `all`. |
| `type` | Only findings of this type; the types are on [The CLI](cli.md#quarry-findings). Omitted: every type. |
| `limit` | Most findings to return, 1 to 500. Default: 50. Each finding lists at most 25 items; `counts` covers every finding. |
