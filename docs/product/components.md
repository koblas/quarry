# quarry — Components and requirements

Part of the quarry product spec; index in [README.md](README.md).

## Components and requirements

### Sync: backup snapshot and import

- `quarry sync` finds the `.quicken` package (configured path, or the single bundle in `~/Documents`), checks that Quicken is open, and copies `<file>.quicken/data` with SQLite's online backup API, read-only, to a timestamped snapshot. Quicken encrypts the database when closed, so a closed file is reported, not read.
- It rejects a snapshot with no accounts, runs SQLite's integrity check, records the hash, and keeps the snapshot read-only.
- It then rebuilds the store from that snapshot in one transaction; a failed build leaves the previous store untouched. `quarry sync --from <snapshot>` rebuilds from an existing snapshot without touching Quicken.
- Snapshots are capped by count: `snapshots.keep` in config, default 12, minimum 1. After a sync swaps in the new store, the oldest snapshots beyond the cap are deleted. The snapshot the current store was built from is never deleted, and a failed sync deletes nothing.
- A rejected snapshot (no accounts, failed integrity check) is deleted at once and never counts toward the cap. `import_runs` keeps one row per successful build, carried across rebuilds, so the hash of every snapshot a store was built from outlives the snapshot. A snapshot that was never built into a store keeps its hash only in its manifest, which is deleted with it.
- Each sync writes a new store file and swaps it in atomically; the previous `quarry.duckdb` is deleted after the swap, so only one store exists at a time. Scheduled jobs need no extra step: every successful sync prunes.
- `quarry snapshots` lists snapshots and `quarry snapshots prune` applies the cap by hand (see CLI).
- Validation runs on every build (see Testing) and fails the sync if balances don't reconcile.
- Mapping from Quicken Classic v9's Core Data tables (`Z`-prefixed entities; dates as seconds since 2001-01-01), seeded from dweekly's schema reference, lives in one versioned package, with a detected-schema fingerprint recorded in `import_runs`.
- Data-quality detection runs after validation and refreshes `findings`, marking earlier findings fixed when they no longer appear.
- Sync also refreshes `fx_rates` from the Bank of Canada: it fetches only the dates missing since the last stored rate (the first sync back-fills from the earliest transaction date, using the Bank's legacy noon-rate series for dates before its current daily series begins). The request carries only the currency series and date range, never user data. This runs on `sync --from <snapshot>` too.
- A failed rate fetch does not fail the sync: the Quicken data is still correct, so the new store is swapped in, a one-line warning goes to stderr (and `warnings` in `--json`), and FX coverage is recorded in `import_runs`. Conversions for dates past the last stored rate use the latest prior rate, and `quarry status` shows how far rate coverage lags.
- Target: a full sync of all history, closed accounts included, in under 60 seconds.

### CLI

Every command supports `--json` for machine consumers and a readable table by default.

| Command | Purpose |
| --- | --- |
| `quarry sync` | Backup snapshot from the open Quicken file + build + validate + detect findings + refresh Bank of Canada exchange rates; `--from <snapshot>` to rebuild from an existing one |
| `quarry snapshots` | List snapshots: ID, taken at, size, and which one the store was built from. `prune` deletes all but the newest `--keep N` (default `snapshots.keep`), never the store's own; `--dry-run` to preview |
| `quarry status` | Last sync, row counts, validation results, staleness, FX rate coverage |
| `quarry accounts` | Accounts with current balances, closed ones on request |
| `quarry holdings` | Securities held in each investment account on one day (`--as-of`, default today) with share count, latest price and its date, and value; `--account` to narrow, cash in investment accounts not included |
| `quarry spend` | Spending by category / payee / tag / month, with `--since`, `--until`, `--account` |
| `quarry cashflow` | Income, expense, savings rate by period |
| `quarry networth` | Net worth on one day (`--as-of`, default today) or at each month end (`--since`/`--until`), by account type and currency |
| `quarry recurring` | Detected recurring charges, start date, price changes |
| `quarry anomalies` | Unusually large transactions (duplicates are `findings`) |
| `quarry summary` | One month (default last month): freshness, findings counts, unusually large charges, recurring charges new in it, net worth at its end and the change; read-only, for a monthly job after sync |
| `quarry search` | Find transactions by payee, memo, amount, date, account or category; transfers and report-excluded transactions included and flagged |
| `quarry acb` | Adjusted cost base per security and realized capital gains by tax year, in CAD |
| `quarry findings` | The cleanup worklist to apply in Quicken; `--csv` to export; ignore a finding by listing its id under `findings.ignore` in the config file |
| `quarry sql` | Read-only SQL against the store |
| `quarry mcp` | Start the MCP server (stdio) |

Every reporting command takes `--currency CAD|USD|native` (default from config, CAD out of the box); `native` lists each account's own currency separately.

`quarry findings` was named `cleanup` in earlier drafts; it was renamed so the worklist can't be mistaken for deleting old data. `quarry snapshots prune` with nothing to delete exits `0`; `--keep 0` is a usage error (exit `2`).

**Exit codes** are a contract scripts and scheduled jobs branch on, the same for every command:

| Code | Meaning |
| --- | --- |
| `0` | Success, including success with warnings (for example a failed exchange-rate refresh) |
| `1` | Failure: reconciliation or validation failed, Quicken closed, snapshot rejected, unknown schema fingerprint, not found, query error |
| `2` | Usage error: unknown command, bad flag or argument |

Data goes to stdout; diagnostics and errors go to stderr as one line naming what failed and what to do next.

### MCP server

A thin wrapper over the core library, launched by the Claude desktop app as a local stdio server.

| Tool | Purpose |
| --- | --- |
| `describe_schema` | Tables, views, columns, category tree, account list, data date range |
| `query` | Read-only SQL with a row cap (default 500) and timeout |
| `spending`, `cash_flow`, `net_worth` | Correct, transfer-aware aggregates with period, filter and `currency` (`CAD`, `USD` or `native`) parameters; an absent `currency` uses the config default, as the CLI does |
| `recurring_charges`, `anomalies` | The same detections the CLI uses |
| `acb` | Adjusted cost base and realized gains by security and tax year (CAD) |
| `search_transactions` | Find transactions by payee, memo, amount, date, account or category; transfers and report-excluded transactions included and flagged |
| `holdings` | Securities held on one day (as_of, default today) with share count, latest price and its date, and value, with accounts and currency parameters; cash in investment accounts not included |
| `monthly_summary` | One month (month, default last month): freshness, findings counts, unusually large charges, recurring charges new in it, and net worth at its end beside the month before with the change; the document `quarry summary --json` prints |
| `sync_status` | Freshness of the data, last validation result |

- `query` is the escape hatch for questions no tool anticipates; the named tools exist so common questions don't depend on Claude re-deriving transfer rules in SQL.
- The schema description includes the conventions (sign, transfers, currency) so ad-hoc SQL gets them right.
- A `data_quality` tool returns the open findings, so Claude can help work through the cleanup list.
- The server never runs `sync` and never prunes snapshots; data freshness is reported, not changed.

### Claude skill

`quarry` ships a Claude skill as the primary way Claude uses the data, with the MCP server for clients that can't load skills; this follows dweekly's finding that the skill is the better default path.

- **`SKILL.md`**: when to use it, checking freshness with `quarry status` first, the conventions (sign, transfers, CAD reporting, cross-currency transfers), and the rule that every number comes from `quarry` output or SQL, never estimation.
- **`references/`**: `schema.md` for `quarry`'s own tables and views (generated from the store, so it can't drift; it lists tables, views and conventions only, never accounts or categories), plus `spending.md`, `cash-flow.md`, `recurring-and-anomalies.md`, `search.md`, `findings.md` (walking David through the findings worklist) and `monthly-summary.md` (the launchd job that runs sync and summary each month), with `.sql` recipes where no command answers the question; `net-worth.md` and `investments.md` arrive with Phase 4.
- The skill calls `quarry … --json` and `quarry sql`; it contains no Quicken schema knowledge, so a Quicken change never breaks it.
- Packaged as a Claude Code plugin that bundles the skill and the MCP server config (in `plugin.json`), in `plugin/`, listed by `.claude-plugin/marketplace.json` at the repo root, adapted from dweekly's plugin layout.

### Data-quality feedback to Quicken

`quarry` never writes to Quicken; instead each import produces a cleanup worklist David applies in Quicken, and the next import confirms which items are fixed.

| Finding | Suggested fix in Quicken |
| --- | --- |
| Uncategorized or `Uncategorized`-category splits | Recategorize; list grouped by payee so one rule fixes many |
| Payee variants of one merchant (`AMZN MKTP CA*2K4`, `Amazon.ca`) | Rename to one payee and add a renaming rule |
| Same payee split across several categories without a pattern | Pick one category, or confirm the split is intended |
| Transfer booked as income or expense (`unlinked-transfer`), or a one-sided transfer | Convert to a transfer between the two accounts |
| Likely duplicates (same account, amount, date within 3 days), unless both are reconciled | Delete one, or mark as not a duplicate |
| Near-duplicate categories (`similar-categories`) or unused categories (`unused-category`) | Merge the near-duplicates; for an unused category, check that no scheduled transaction or budget uses it, then delete it |

- Each item names the exact transactions (date, account, payee, amount) so they can be found in Quicken's register.
- Safety rule for `unused-category`: a category counts as used when it, or any subcategory, is referenced anywhere `quarry` can see a reference: an imported split in any account (closed and excluded included), an investment or scheduled-transaction entry the importer counts but does not import, a budget line, a loan split or loan interest category, or a memorized-payee rule. Hidden categories, anything under a hidden category, and a category with a hidden subcategory are never reported. Where `quarry` cannot see a reference, the fix text tells the user to check first and then delete in Quicken; `quarry` never deletes anything.
- Findings have a status: open, fixed (gone on re-import), or ignored (id listed in `findings.ignore`; remove it to list the finding again).

### Reporting currency and ACB

Every report can be produced in CAD or USD; when reporting in CAD, `quarry` also computes adjusted cost base (ACB) and realized capital gains the way the CRA defines them.

**Reporting currency**

- Amounts stay stored in each account's native currency; conversion happens only in the views.
- Each amount converts at the Bank of Canada rate for its transaction date (the latest prior business day on weekends and holidays, via an ASOF join); balances and holdings convert at the rate on the valuation date.
- Cross-currency transfers between own accounts keep both legs, so they never show up as a gain, loss or expense.
- CAD is the default (config `reporting.currency` overrides it); `--currency USD` (CLI) or `currency: "USD"` (MCP) switches any report, and `native` lists each account's own currency without converting.

**ACB (CAD reporting only)**

- Computed per security, pooled across all of David's non-registered accounts, since identical shares held in different accounts share one ACB.
- Registered accounts (RRSP, RRIF, TFSA, RESP, FHSA) are excluded; an account-classification setting marks each account, because Quicken's account types don't say which are registered. The skill asks for the classification in session the first time ACB is requested, and unclassified investment accounts appear as findings.
- Buys add cost plus commissions; sells remove a pro-rata share of ACB and produce a realized gain or loss; reinvested dividends add to ACB; splits and consolidations change units, not ACB.
- USD purchases and sales convert to CAD at the rate on each trade date, as the CRA requires, so ACB is correct even when the report is otherwise in USD.
- Return of capital and reinvested (phantom) distributions reduce or increase ACB but come from T3 slips, not Quicken; `quarry` accepts them as a small manual adjustments file per security and year.
- Possible superficial losses (a loss with the same security bought within 30 days before or after) are marked in `acb` with a warning, not a finding, and not adjusted automatically. Positions whose history starts without a purchase (a transfer-in or opening balance) are flagged as findings, since their ACB can't be derived from Quicken alone.
- Output is a per-security ACB history and a realized-gains summary per tax year, as a worksheet to support tax filing and review with an accountant, not a filing.
