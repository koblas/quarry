# Quarry: Quicken Data Platform — PRD

Sep 27, 2026 · @david · Source: https://claude.ai/artifact/R4yVRk6NYX2U9aaYL6suE7 (rev 47)

## Overview

`quarry` turns a Quicken for Mac data file into a clean, local, queryable financial database, exposed through a CLI and an MCP server so both David and Claude can run analysis over decades of history.

Quicken holds years of categorized transactions, balances and investment activity, but that data is locked in an undocumented Core Data SQLite store. Quicken's own reports are fixed and hard to combine, and exporting CSVs by hand for every question is slow and lossy (splits, transfers and investment lots flatten badly). The goal is to make that data a first-class source for any further analysis, without changing how Quicken is used day to day.

## Prior art

Two open-source projects already read Quicken for Mac's SQLite store for AI agents; `quarry` reuses their schema knowledge but adds an owned, reconciled store and the cleanup loop.

| Project | What it does | What `quarry` takes from it |
| --- | --- | --- |
| [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills) (shell + SQL, MIT) | Agent skills over a backup-API snapshot with normalized `q_*` views: net worth, spending, investments, data hygiene | Closest design to `quarry`; test schema captured from a real 9.x file; imports daily FX because Quicken keeps only the current rate; hygiene check list |
| [dweekly/quicken-mac-mcp](https://github.com/dweekly/quicken-mac-mcp) (TypeScript, MIT) | Skill + 8-tool MCP server + `qmac` CLI over the live database, read-only, `raw_query` capped at 500 rows; CSV exporter | [Schema reference](https://github.com/dweekly/quicken-mac-mcp/blob/main/docs/schema.md) (84 entities; splits in `ZCASHFLOWTRANSACTIONENTRY`; dates in Core Data epoch, seconds since 2001-01-01) |
| [HarryDolan/qquery](https://github.com/HarryDolan/qquery) (Python) | Early CLI over a copied Quicken for Mac database | Confirms the approach has held across Quicken versions |

What neither does: reconcile against Quicken's balances on every import, keep stable IDs so findings are tracked from open to fixed across snapshots, or ship as a single binary. Both confirm a key constraint: Quicken encrypts the database when the app is closed, so a snapshot must be taken while Quicken is open.

### Reusing MIT-licensed code

`quarry` starts from dweekly/quicken-mac-mcp's code and schema knowledge instead of rediscovering it; its MIT license permits copying, porting to Go and modifying, provided the copyright and license notice travel with the code.

| Asset | Source | Use in `quarry` |
| --- | --- | --- |
| Schema reference (84 entities, Core Data semantics) | dweekly [`docs/schema.md`](https://github.com/dweekly/quicken-mac-mcp/blob/main/docs/schema.md) | Basis for the importer mapping; Phase 0 becomes verification, not discovery |
| SQL recipes: spending with transfer exclusion, `(Uncategorized)` bucket, two-level notes, portfolio with lots | dweekly MCP tools and skill references | Ported into the importer and views; their results double as expected values in tests |
| Skill layout (`SKILL.md` + `references/`) | dweekly plugin | Template for the `quarry` skill |
| Database auto-detection (one `.quicken` bundle in `~/Documents`, else explicit path) | dweekly server | `quarry sync` file discovery |
| CSV exporter | dweekly `scripts/export_sovereign_csv.py` | Independent cross-check of importer output |
| Backup-API snapshot, daily FX download, hygiene checks, 9.x test schema | [hardkoded/quicken-skills](https://github.com/hardkoded/quicken-skills) (also MIT) | `quarry sync`, `fx_rates`, `findings`, golden fixture |

- Both projects' copyright notices and MIT text go in `THIRD_PARTY_NOTICES`, and ported files carry a header naming their source.
- The README credits both projects; improvements that are not `quarry`-specific (schema corrections, v9 quirks) are offered upstream.

## Goals and non-goals

**Goals**

1. Extract the full history, including closed accounts, from Quicken Classic for Mac (v9): accounts, transactions, splits, transfers, categories, tags, payees, securities, investment activity. `quarry` takes its own snapshot with SQLite's backup API while Quicken is open; no manual export or copy step.
2. Normalize it into a documented, stable schema that is independent of Quicken's internal schema and its version changes.
3. Make the data usable four ways: SQL directly, a CLI with `--json` output, a Claude skill, and a local MCP server.
4. Provide correct building blocks for common analysis: transfer-aware spending, recurring charges, cash flow, net worth over time (CAD and USD accounts), investment performance.
5. Feed data-quality findings back as a cleanup worklist applied in Quicken, so Quicken itself, and every later snapshot or export, gets cleaner over time.
6. Build on the existing MIT-licensed work (see Prior art) rather than rediscovering Quicken's schema.
7. Keep all data local by default; nothing leaves the Mac except query results a user or Claude explicitly requests.

**Non-goals**

- Writing to Quicken programmatically. Fixes flow back as a worklist David applies in Quicken, which stays the system of record.
- Querying the live Quicken database. The live file is touched only by the backup API during `quarry sync`; every query runs on a snapshot.
- Bank aggregation or live account syncing. Quicken remains the ingestion point.
- Supporting Quicken for Windows (`.QDF`). The ingest layer is pluggable so a CSV/QIF importer can be added later.
- Dashboards or any UI in v1.
- Financial advice or automated decisions (trades, payments).

## Users and use cases

Three consumers share one database: David at the terminal, Claude through MCP or a shell, and unattended scheduled jobs.

| Consumer | Interface | Example |
| --- | --- | --- |
| David | CLI, SQL | `quarry spend --by category --since 2024`; ad-hoc DuckDB queries; export to a spreadsheet |
| Claude (desktop / chat) | MCP server | "How did our grocery spend change since 2022?" "Which subscriptions started this year?" |
| Claude (Claude Code) | Skill + CLI | Scripted analyses, building dashboards, one-off notebooks |
| Scheduled job | CLI | Monthly sync + summary of anomalies and new recurring charges |

Representative questions the system must answer correctly:

- Spending by category and payee over any period, excluding transfers between own accounts.
- Recurring charges: what they are, when they started, how their price changed.
- Anomalies: unusually large transactions, duplicates, uncategorized items.
- Cash flow and savings rate by month and year.
- Net worth history, by account and account type.
- Investment activity: contributions, dividends, realized gains, holdings over time.
- Tax-relevant totals by category for a given year.

## Architecture

A one-way pipeline copies the Quicken file, maps it into a normalized DuckDB store, and serves that store through one library through a CLI and an MCP server, with a skill that teaches Claude to use them.

```
Quicken for Mac  ──►  Snapshot            ──►  Importer             ──►  quarry.duckdb
live .quicken file    SQLite backup API        maps v9 schema            normalized schema
never written         while Quicken open       seeded from dweekly       stable, documented
                                                                              │
                                                                              ▼
┌──────────────────────────────────────────────────────────────────────────────────────┐
│ Core library — transfer-aware queries: spending, recurring charges, anomalies,       │
│ cash flow, net worth, investments                                                    │
└──────────────────────────────────────────────────────────────────────────────────────┘
        │                                                          │
        ▼                                                          ▼
  quarry CLI  ◄── Claude skill                              MCP server (stdio)
  sync, spend, findings, sql    SKILL.md + references/      schema, query, analysis tools
  David, scheduled jobs         drives the CLI; Claude Code Claude desktop and chat
```

One normalized store feeds every interface; Quicken is only ever copied.

The normalized store is the product: Quicken's quirks are resolved once in the importer, so the CLI and MCP server stay simple. Everything is Go, shipped as one binary: the duckdb-go driver (cgo, DuckDB linked in) and the official MCP Go SDK.

- **Snapshot** isolates `quarry` from Quicken: the live database is copied with SQLite's online backup API while Quicken is open (it is encrypted when Quicken is closed), then checksummed and kept, so every import is reproducible.
- **Importer** is the only code that knows Quicken's schema. Each run rebuilds the store from a snapshot, so imports are idempotent and a Quicken schema change breaks one module.
- **Core library** owns every analysis rule (transfer handling, split allocation, sign conventions) so the CLI and MCP never disagree.

## Data model

The store has a small set of normalized tables plus derived views; every analysis reads the views, never Quicken's raw structure.

| Table | Holds | Key rules |
| --- | --- | --- |
| `accounts` | Name, type (chequing, savings, credit card, investment, retirement, loan, asset), currency (CAD or USD), institution, open/closed, hidden | Closed accounts included; type drives net-worth sign and grouping |
| `categories` | Hierarchy (`parent_id`, `full_path`), kind (income / expense / system), tax line | Full history kept, including retired categories |
| `payees` | Payee names as recorded in Quicken | Variants of one merchant are flagged for cleanup, not silently merged |
| `transactions` | Account, date, payee, memo, amount, currency, cleared/reconciled status, cheque number | Negative = money leaving the account |
| `splits` | Transaction, category, amount, memo, transfer target account | Every transaction has at least one split; splits sum to the transaction amount |
| `transfers` | Pairs of splits that move money between own accounts, including CAD to USD | Excluded from spending and income by default; cross-currency pairs keep both amounts |
| `tags`, `split_tags` | Quicken tags | Many-to-many on splits |
| `securities`, `prices` | Symbol, name, type, currency; historical prices | Prices as recorded in Quicken, no external feed in v1 |
| `fx_rates` | CAD/USD rate by date | Quicken keeps only the current rate per pair, so history is a daily series (Bank of Canada) fetched by `quarry sync` |
| `investment_txns` | Action (buy, sell, dividend, reinvest, share transfer, split), security, shares, price, fees, amount | Cash side also appears in `transactions` |
| `findings`, `finding_items` | What each sync found to clean up: id, type, when first found, when fixed; `finding_items` names the transactions, splits, payees or categories | Status (open, fixed, ignored) and the suggested fix come from `quarry findings`; ignore decisions live in the config file |
| `import_runs` | Snapshot hash, row counts, validation results | One row per successful build, kept across rebuilds; audit trail |

**Derived views:** `v_spending` (expense splits, transfers removed, refunds netted), `v_cash_flow` (monthly income vs expense), `v_balances_daily` and `v_net_worth` (per account and total), `v_holdings` (shares and value by date). Recurring series and anomalies are computed by the core library; `quarry recurring --json` / `anomalies --json` are the shapes the Phase 3 `recurring_charges` / `anomalies` tools return.

**Conventions**

- Money is `DECIMAL(18,2)` in the account's native currency; no floats. Reporting-currency views convert with the rate in effect on the transaction date.
- Every row keeps `source_id` (Quicken's primary key) for traceability back to the file.
- `quarry` IDs are derived deterministically from source IDs, so a re-import keeps IDs stable and a finding can be tracked from open to fixed across snapshots.
- Real fixes happen in Quicken. `quarry` stores only decisions about findings: the ids the user lists under `findings.ignore` in `config.toml`, which quarry reads and never writes, so no second, divergent copy of the truth builds up and rebuilding the store loses no decision.

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
| `quarry spend` | Spending by category / payee / tag / month, with `--since`, `--until`, `--account` |
| `quarry cashflow` | Income, expense, savings rate by period |
| `quarry networth` | Net worth history, by account type and currency (Phase 4: needs investment holdings) |
| `quarry recurring` | Detected recurring charges, start date, price changes |
| `quarry anomalies` | Unusually large transactions (duplicates are `findings`) |
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
| `search_transactions` | Payee / memo / amount / date search |
| `sync_status` | Freshness of the data, last validation result |

- `query` is the escape hatch for questions no tool anticipates; the named tools exist so common questions don't depend on Claude re-deriving transfer rules in SQL.
- The schema description includes the conventions (sign, transfers, currency) so ad-hoc SQL gets them right.
- A `data_quality` tool returns the open findings, so Claude can help work through the cleanup list.
- The server never runs `sync` and never prunes snapshots; data freshness is reported, not changed.

### Claude skill

`quarry` ships a Claude skill as the primary way Claude uses the data, with the MCP server for clients that can't load skills; this follows dweekly's finding that the skill is the better default path.

- **`SKILL.md`**: when to use it, checking freshness with `quarry status` first, the conventions (sign, transfers, CAD reporting, cross-currency transfers), and the rule that every number comes from `quarry` output or SQL, never estimation.
- **`references/`**: `schema.md` for `quarry`'s own tables and views (generated from the store, so it can't drift), plus `spending.md`, `net-worth.md`, `investments.md` and `findings.md` (walking David through the findings worklist), each with `.sql` recipes.
- The skill calls `quarry … --json` and `quarry sql`; it contains no Quicken schema knowledge, so a Quicken change never breaks it.
- Packaged as a Claude Code plugin that bundles the skill and the MCP server config, adapted from dweekly's plugin layout.

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
- Possible superficial losses (a loss with the same security bought within 30 days before or after) are flagged as findings, not adjusted automatically. So are positions whose history starts without a purchase (a transfer-in or opening balance), since their ACB can't be derived from Quicken alone.
- Output is a per-security ACB history and a realized-gains summary per tax year, as a worksheet to support tax filing and review with an accountant, not a filing.

## Security and privacy

All data stays on the Mac; the only data that leaves is the result of a query David or Claude explicitly runs.

- **Read-only against Quicken.** `quarry` touches the live database only through SQLite's read-only backup API during sync; every query runs on a snapshot, and neither is ever written.
- **Encrypted at rest.** Snapshots and `quarry.duckdb` live under `~/Library/Application Support/quarry/` on a FileVault volume; files are `0600`. Optional DuckDB encryption with the key in the macOS Keychain.
- **Account numbers in configuration.** `quarry` imports no account-number field from Quicken. Where the user writes an account number into `quarry`'s config (for example to name an account in the account-classification setting), `quarry` shows it masked to the last four digits in everything it prints or serves, CLI and MCP alike. Free text (payees, memos, notes) is not scanned: card and account numbers there are already masked by the institution's download or by Quicken, so `quarry` passes it through as Quicken holds it.
- **MCP boundary.** The MCP server is local stdio only (no network listener), SQL is read-only (enforced by a read-only connection), and results are row-capped so a stray `SELECT *` can't dump the whole history into a conversation.
- **Snapshot retention.** Snapshots are capped by count (default 12) and the oldest are deleted after each successful sync; the rules live under Sync.
- **No telemetry.** The only outbound network call in v1 is `quarry sync` fetching exchange rates from the Bank of Canada; it sends no user data.

## Testing and validation

Correctness is defined as matching Quicken: every account's balance in `quarry` must equal Quicken's to the cent, or the sync fails.

**Validation on every import**

- Per-account balance reconciliation against the balances Quicken itself stores, at the latest date and at each reconciled statement date.
- Splits sum to their transaction; every transfer has a matching counterpart (or is flagged as one-sided).
- Row counts and date ranges compared with the previous import; a large unexplained drop fails the sync.
- Investment accounts: share counts per security match Quicken's holdings.

**Development tests**

- A synthetic Quicken file built in Quicken with known accounts and edge cases (splits, transfers, split-with-transfer, voided, foreign currency, stock split, reinvested dividend) used as a golden fixture.
- Unit tests for each view against hand-computed expectations.
- A one-time manual check: `quarry` reports for two recent years compared with the same reports run in Quicken.

**Non-functional**

| Requirement | Target |
| --- | --- |
| Full sync time | Under 60 s for 20+ years |
| Typical CLI / MCP query | Under 1 s |
| Runtime | macOS 14+, single Go binary (cgo, DuckDB linked in), installed with `go install` or a Homebrew tap |
| Quicken upgrade | Schema fingerprint change is detected and reported, never silently mis-imported |

## Milestones

The work ships in five phases; Phase 1 is the hard part, because every later phase trusts its reconciliation.

Each phase ships only when its check passes.

| Phase | Scope | Gate |
| --- | --- | --- |
| 0 — Port prior art | dweekly schema, recipes, skill; backup snapshot | Schema verified on your v9 file |
| **1 — Import + store** | Banking, credit, splits, transfers, categories | Balances reconcile for all cash accounts |
| 2 — CLI + analysis | Views, CLI, cleanup worklist, anomaly detection (slices 2a–2f; `export` dropped for `sql --csv`; 2d covers every finding type) | Matches Quicken reports, 2 years |
| 3 — Skill + MCP | `SKILL.md`, named tools, read-only query | Claude answers the use-case questions correctly |
| 4 — Investments | Holdings, ACB, prices, net worth; monthly scheduled summary | Share counts match Quicken |

Phases 0–2 alone deliver the core goal (the data as a queryable source); the MCP server and investments build on a store that is already proven correct. No dates are set yet.

## Risks and open questions

The main risk is Quicken's undocumented schema; reconciliation on every sync is the mitigation.

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Quicken's Core Data schema is undocumented and changes between versions | Wrong or failed imports | Schema fingerprint per import; mapping isolated in one module; sync fails loudly on reconciliation errors |
| Semantics hidden in the schema (split transfers, voided items, pending, scheduled vs posted) | Subtly wrong totals | Golden fixture file covering each case; balance reconciliation catches drift |
| Investment lots and cost basis may be computed by Quicken rather than stored | Phase 4 cannot match Quicken's gains | Import transactions first; reproduce cost basis only if validation shows it is needed |
| A snapshot taken while Quicken is closed (encrypted, no accounts) or mid-write | Corrupt snapshot | Backup API while Quicken is open; import rejects empty snapshots and runs an integrity check |
| Sensitive data reaching a conversation | Privacy exposure | Row caps, account numbers from config masked to the last four digits, local-only MCP; card and account numbers in free text are masked upstream by the institution or Quicken |

**Decisions**

- Source: Quicken Classic for Mac, version 9.
- Accounts: CAD and USD accounts both in scope.
- History: everything, including closed accounts.
- Input: `quarry sync` takes its own snapshot with SQLite's backup API while Quicken is open; no hand-made snapshot or import step.
- Interfaces: CLI, MCP server and a Claude skill, the skill as Claude's primary path.
- Reuse: build on dweekly/quicken-mac-mcp (and hardkoded/quicken-skills), both MIT, with notices preserved.
- Reporting currency: selectable, CAD, USD or native (each account's own currency, unconverted); default from config `reporting.currency`, else CAD. CLI and MCP accept the same values; MCP's are exact-case.
- Search: defaults to all dates; --min/--max compare absolute native amounts; no --currency; search is a lookup, not a report.
- ACB: computed when reporting in CAD, per security across non-registered accounts, with trade-date FX.
- FX history: daily series from the Bank of Canada, fetched incrementally by `quarry sync`, since Quicken stores only the current rate. A failed fetch warns but does not fail the sync.
- Exit codes: `0` success (warnings included), `1` failure, `2` usage error, for every command.
- Feedback: findings go back into Quicken as a cleanup worklist so later snapshots and exports are clean.
- Command names: the worklist is `quarry findings`, snapshot housekeeping is `quarry snapshots` / `quarry snapshots prune`; no command is called `cleanup`, since it reads as either.
- Snapshot retention: a count cap (`snapshots.keep`, default 12), applied after every successful sync and by `snapshots prune`; the store's own snapshot is always kept. No size or age caps in v1.
- Redaction: only account numbers held in `quarry`'s config are masked (last four digits). Free text is passed through as Quicken holds it, since institutions and Quicken already mask card and account numbers there.
- Dashboards: out of scope for v1.
- Language: Go, shipped as a single binary (duckdb-go, official MCP Go SDK).
- Store engine: DuckDB, for exact `DECIMAL` money, ASOF joins against FX rates and prices, and analytical SQL. Tables use standard types only, so a move to SQLite stays a port. Syncs write a new file and swap it in atomically, since DuckDB allows one writer.

**Open questions**

None block the design. Facts about a particular user's data (which accounts are registered, return-of-capital adjustments, whether trade history is complete) are gathered in session, not designed in: the skill asks for them when a report first needs them, stores the answers in `quarry`'s config, and `quarry` reports what is missing as findings.

Pinning a snapshot (for example a tax year-end) so `prune` skips it is left out of v1; add it if the count cap proves too blunt.
