# The CLI

Every `quarry` command, with its flags as `quarry <command> --help` prints them. Dates are `YYYY`, `YYYY-MM` or `YYYY-MM-DD`; a year or month given to `--as-of` means its last day. A flag marked *repeatable* takes several values, one per use.

## Global flags

| Flag | Description |
|---|---|
| `--json` | Print the result as JSON on stdout. Every command takes it. |
| `--version` | Print quarry's version, as `quarry version` does. |

## Version

### `quarry version`

Prints the version of this quarry binary: the release tag, such as `v1.0.0`, for a binary installed with `go install github.com/koblas/quarry/cmd/quarry@<version>`, or Go's pseudo-version for one built from a checkout, which names the commit and ends in `+dirty` when the checkout had uncommitted changes. It reads only the binary: no store, no config, no Quicken. With `--json` it adds the full commit, whether the checkout had uncommitted changes, and the Go version quarry was built with. No flags of its own.

## Sync and snapshots

### `quarry sync`

Snapshots the open Quicken file and rebuilds quarry's store from it.

| Flag | Description |
|---|---|
| `--quicken` *path* | The `.quicken` file to snapshot. Default: `quicken.path` in the config file, else the only one in `~/Documents` or Quicken's Documents folder. |
| `--from` *snapshot* | Rebuild the store from this snapshot instead of reading Quicken: an id such as `20260927T143005Z`, or the path to its `.sqlite` file. |

### `quarry status`

Shows which snapshot the store was built from and what it holds. No flags of its own.

### `quarry snapshots`

Lists the snapshots `quarry sync` has taken, newest first: when each was taken, its size, the Quicken file it came from, and which one the store was built from. Snapshots live in `~/Library/Application Support/quarry/snapshots`. No flags of its own.

| Status | Meaning |
|---|---|
| `store` | The snapshot the store was built from. |
| `schema differs` | A snapshot quarry cannot import. |
| `no manifest` | A snapshot that cannot be used with `--from`. |

### `quarry snapshots prune`

Deletes all but the newest snapshots, as sync does after each successful sync. The snapshot the store was built from is never deleted, even when it is older. With `--dry-run` it runs even while a sync is running.

| Flag | Description |
|---|---|
| `--keep` *n* | Keep the newest *n* snapshots. Default: `snapshots.keep` in the config file, 12 unless set. |
| `--dry-run` | List the snapshots prune would delete without deleting them. |

## Accounts and investments

### `quarry accounts`

Lists accounts with their current balances.

| Flag | Description |
|---|---|
| `--all` | Include closed accounts. |
| `--currency` *code* | Add a column with each balance in `CAD` or `USD`; `native` adds none. Default: `reporting.currency`, else CAD. |

### `quarry holdings`

Lists the securities held in each account and their value.

| Flag | Description |
|---|---|
| `--account` *name* | Only the account with this name or id. Repeatable. |
| `--as-of` *date* | Value holdings on this date. Default: today. |
| `--currency` *code* | Add a column with each value in `CAD` or `USD`; `native` adds none. Default: `reporting.currency`, else CAD. |

### `quarry acb`

Shows the adjusted cost base of each security and the capital gains and losses realized each tax year, in CAD, pooled across every account in `accounts.non-registered`. Return of capital and reinvested distributions come from `acb.adjustment` in the config file. Possible superficial losses are marked, not adjusted. The output is a worksheet to review with an accountant, not a tax filing.

| Flag | Description |
|---|---|
| `--year` *year* | List the sales in tax year `YYYY` one by one. |
| `--security` *name* | The full history of the security with this name, ticker or id. Repeatable. |
| `--currency` *currency* | ACB is in CAD only; any other currency is refused. |

### `quarry networth`

Shows net worth today or at each month end, by account type and currency.

| Flag | Description |
|---|---|
| `--as-of` *date* | Value net worth on this date. Default: today. |
| `--since` *date* | List net worth at each month end on or after this date. Default: January 1 this year when `--until` is given. |
| `--until` *date* | List net worth at each month end on or before this date. Default: today; a later date means today. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

## Spending and income

### `quarry spend`

Shows spending by category, payee, tag or month.

| Flag | Description |
|---|---|
| `--by` *group* | `category` (default), `payee`, `tag` or `month`. |
| `--since` *date* | Count transactions dated on or after this date. Default: January 1 this year. |
| `--until` *date* | Count transactions dated on or before this date. Default: today. |
| `--account` *name* | Only the account with this name or id. Repeatable. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

### `quarry cashflow`

Shows income, spending and savings rate by month or year.

| Flag | Description |
|---|---|
| `--by` *period* | `month` (default) or `year`. |
| `--since` *date* | Count transactions dated on or after this date. Default: January 1 this year. |
| `--until` *date* | Count transactions dated on or before this date. Default: today. |
| `--account` *name* | Only the account with this name or id. Repeatable. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

### `quarry recurring`

Lists charges that repeat every week, month, quarter or year.

| Flag | Description |
|---|---|
| `--since` *date* | List series running on or after this date. Default: January 1 this year. |
| `--until` *date* | List series that started on or before this date. Default: today. |
| `--account` *name* | Only series with a charge in the account with this name or id. Repeatable. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

### `quarry anomalies`

Lists charges unusually large for their payee or category.

| Flag | Description |
|---|---|
| `--since` *date* | List charges dated on or after this date. Default: January 1 this year. |
| `--until` *date* | List charges dated on or before this date. Default: today. |
| `--account` *name* | Only charges in the account with this name or id. Repeatable. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

### `quarry summary`

Summarizes a month: unusual charges, new recurring charges, net worth and findings.

| Flag | Description |
|---|---|
| `--month` *YYYY-MM* | Summarize this month instead of last month; it must have ended. |
| `--currency` *code* | `CAD`, `USD`, or `native` for each account's own. Default: `reporting.currency`, else CAD. |

## Finding transactions

### `quarry search`

Finds transactions by payee, memo, amount, date, account or category. Takes optional search text as its argument.

| Flag | Description |
|---|---|
| `--account` *name* | Search only the account with this name or id. Repeatable. |
| `--category` *path* | Only transactions with a split in this category or one under it, by full path such as `Food:Groceries`. |
| `--min` *amount* | Only transactions of at least this amount, sign ignored, in the account's own currency. |
| `--max` *amount* | Only transactions of at most this amount, sign ignored, in the account's own currency. |
| `--since` *date* | Transactions dated on or after this date. Default: the first transaction. |
| `--until` *date* | Transactions dated on or before this date. Default: no end, future-dated included. |
| `--limit` *n* | Print at most *n* transactions, newest first. 500 unless set; 0 prints every one. |

### `quarry sql`

Runs a read-only SQL query against quarry's store. The query is the argument; `-` reads it from stdin.

| Flag | Description |
|---|---|
| `--limit` *n* | Print at most *n* rows. 500 unless set, every row with `--csv`; 0 prints every row. |
| `--csv` | Print the rows as CSV, with a header line. |

## Data quality

### `quarry findings`

Lists the problems sync found in the Quicken data, as a worklist to fix in Quicken.

| Flag | Description |
|---|---|
| `--status` *status* | `open` (default), `ignored`, `fixed` or `all`. |
| `--type` *type* | Only findings of this type (below). |
| `--csv` | One row per transaction, split, payee, category, account or investment transaction, as CSV. |

| Type | What it is |
|---|---|
| `duplicate` | Two transactions in one account with the same amount, dated within 3 days of each other, unless both are reconciled. |
| `one-sided-transfer` | A transfer with no matching transaction in the other account. |
| `unlinked-transfer` | Two transactions in different accounts of the same currency that look like one transfer (opposite amounts, within 3 days) but are not linked as one. |
| `uncategorized` | Splits with no category, one finding per payee; `quarry cashflow` counts them as income or spending. |
| `mixed-categories` | A payee whose transactions go back and forth between categories. |
| `payee-variants` | Payees whose names differ only in case, punctuation, spacing, or store and reference numbers. |
| `similar-categories` | Categories whose names differ only in case, punctuation, spacing or a plural. |
| `unused-category` | A category no transaction uses. |
| `unclassified-account` | A brokerage or retirement account in neither `accounts.registered` nor `accounts.non-registered`. Fixed in the config file; it leaves the list without a sync and is never marked fixed. |
| `shares-without-cost` | Shares added to a non-registered account with no cost basis. Leaves the list on the first sync after the cost is entered in Quicken; never marked fixed. |

`duplicate` and `unlinked-transfer` compare register entries only, not investment transactions.

## Claude

### `quarry mcp`

Runs quarry as a local MCP server over stdin and stdout; it opens no network port. The server reads the store as it is at each request, never syncs, prunes or touches Quicken, runs SQL read-only, and stops every list at 500 entries. No flags of its own.

Tools: `describe_schema`, `query`, `sync_status`, `data_quality`, `spending`, `cash_flow`, `recurring_charges`, `anomalies`, `search_transactions`, `holdings`, `net_worth`, `acb`, `monthly_summary`.

### `quarry claude`

Groups `install` and `uninstall`. No flags of its own.

### `quarry claude install`

Installs quarry in Claude Code (the plugin: skill and MCP server, user scope) and Claude Desktop (the MCP server only), whichever is on this Mac. Claude Code counts as present when `claude` is on the `PATH`; Claude Desktop when `~/Library/Application Support/Claude` exists. Each step already done is skipped. No flags of its own.

### `quarry claude uninstall`

Removes what install added: the user-scope plugin and the quarry marketplace from Claude Code, and the `quarry` entry under `mcpServers` from Claude Desktop when it runs `quarry mcp`. Leaves the quarry store and snapshots alone. No flags of its own.
