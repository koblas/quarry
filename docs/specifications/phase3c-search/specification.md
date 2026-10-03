# Specification: Phase 3c — transaction search

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Find particular transactions — "that Costco charge", "the 42.17 payment", "transfers to savings" — by text, date, account, category or amount, from the terminal (`quarry search`) and from Claude (`search_transactions`), with one shared document so the two never disagree.

**Secondary Goals**: every transaction is searchable, including transfers and transactions Quicken's reports leave out, each flagged so nobody adds them into spending totals; results are bounded (newest first, limit 500 by default) without reading all history; Rule 4 holds — no caller text on MCP stderr.

**Out of Scope**:
- Converted amounts or a `--currency` flag (amounts are native, never converted).
- CSV output (`quarry sql --csv` exists).
- Wildcards, regex, or matching account/category names as text.
- Skill, references, plugin, use-case eval — Phase 3d.

**Business Rules**: below; literal surface in `## Surface & Copy` (product-vision Phase 1, binding — includes interpretations a-d, approved by the user 2026-10-03).

## Business Rules & Invariants
- Rule 1 (extended to search): `search_transactions`' structured result is the `quarry search --json` document for the same inputs, built by `document.NewSearch`; every field but `warnings` byte-equal; in `warnings` only the cut line differs by surface; the no-match line is identical.
- Rule 4 (unchanged): MCP stderr never carries the caller's text, amounts, category or account values; new class lines `refused the call's text; …`, `refused the call's min or max; …`, `refused the call's category: it names no category; …`.
- Rule S1 source: every transaction (all accounts incl. closed, left out of reports, linked tracking; transfers; "exclude from reports"); each row carries `transfer` and `excluded`, derived from the same SQL fragments `v_cash_flow` uses (extracted, shared; `v_cash_flow` semantics unchanged).
- Rule S2 grain: one row per transaction, native signed amount + currency, `splits[{category,memo,amount,transfer}]` in split source order.
- Rule S3 text: case-insensitive literal substring over payee name, transaction memo, split memos (`contains(lower(x), lower($n))`, never LIKE); account/category names not matched; blank text refused.
- Rule S4 amounts: `min`/`max` compare `abs(amount)` native, inclusive; grammar `^[0-9]{1,16}(\.[0-9]{1,2})?$`; MCP passes them as strings.
- Rule S5 category: matches a split whose category `full_path` equals the argument ignoring case, or starts with it plus `:`; hidden categories count; unknown refused; known-but-empty gives the no-match warning.
- Rule S6 window: absent `since`/`until` mean no bound (future-dated included); `report.ParseSearchWindow` (no clock) returns the existing `WindowError`.
- Rule S7 order/limit: `date DESC, source_id DESC`; limit applied in the store with the full match count (`matched`), `truncated = matched > len(transactions)`; a cut drops the oldest. Deliberate deviation from 3b Rule 11 (no `capList`).
- Rule S8: search never reads config and never reads the clock.
- Rule S9 order of outcomes: argument shape (CLI: argument count → --limit → blank text → text not UTF-8 → --category not UTF-8; MCP: SDK schema check → blank text) → min → max → min>max → window → store open → account → category → read. (Mid-feature ruling, SCENARIO-04.)
- An empty result exits 0 with the no-match warning; "not found" (exit 1) is for named resources (unknown account, unknown category).

---

## Triage Brief

#### Request
Add `search_transactions` (MCP) plus a `quarry search` CLI twin: payee/memo/amount/date search over the store, built on merged 3b. No spec covers it (3b spec lists it as 3c, `docs/specifications/phase3b-analysis-tools/specification.md:12,151,174`; STATE "Left unbuilt"). Pipeline applies.

Method: read 3b spec Rules, §4.2, §5, §6 and triage §4 (lines 20-31, 76-81, 425-516), both STATE files and the 3b RETRO. Opened the code cited below myself with Read/grep. I used grep only (no LSP, no investigator), so every caller row is tagged `grep`. Positive control: grep found the known `Charges` implementer `duckstore/charges.go:46`. Not verified: DuckDB `LIKE ... ESCAPE` behaviour (no repo prior art, and I ran nothing).

#### Affected surface
- command (new): `internal/cli/root.go:28-38` has the `AddCommand` list. `search` is not in it and collides with nothing (grep for "search" in `internal/cli`, `cmd/quarry` and the PRD finds only `docs/initial-prd.md:201`). Root help pin `cmd/quarry/run_status_test.go` `Test_run_help_prints_quarrys_description` lists every subcommand and needs a row (3a STATE:18).
- port: `internal/report/store.go:11-28` is the `Store` interface (Status, Accounts, Spending, CashFlow, Charges, Findings, Schema, Query). It needs one new read method.
- adapter: `internal/store/duckstore/` has one file per read (`charges.go`, `spending.go`, `cashflow.go`, ...). `cmd/quarry/run.go:31` has the compile-time assertion `_ report.Store = (*duckstore.Store)(nil)`.
- service: `report.Server` methods follow the `Anomalies` shape (`internal/report/anomalies.go:90-100`): `s.namedAccounts` -> store read -> `s.readRefusal(ctx, <command>, err)`.
- document: `internal/report/document` holds the builders (`NewAnomalies` etc.). A new `NewSearch` would follow them, with `[]` never null and warnings passed in (3a STATE:6-8).
- mcp: tool consts are `internal/mcp/tools.go:17-25`. Registration is `tools.go:192-223` (`sdk.AddTool(... tool(name, desc, objectSchema(...)), handler(s.timeout, stoppedLine(name), s.fn))`). The handler template is `internal/mcp/anomalies.go:12-34`: `s.now()` once -> window parse (`windowRefusal`) -> `resolveCurrency` -> `s.newReport` -> `accountRefusal` -> `document.New*` -> `capList`.
- copy that mentions the tool list and must change: `instructions` (`tools.go:36-38`, "Tools: describe_schema, query, ..."), `quarry mcp` Long (`internal/cli/mcp.go:24-36`, "every list a tool returns stops at 500"), `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_all_eight_tools` (the only pin, with its own byte copies), and the `query` description redirect.

#### Prior art in this repo
- CLI twin shape: `internal/cli/anomalies.go` (whole file). `reportFlags{}` + `flags.bind(cmd, reportFlagHelp)` (`internal/cli/window.go:14-37`) gives `--since`, `--until`, repeatable `--account`. `flags.window(cmd, at)` (`window.go:40-58`) calls `report.ParseWindow` and turns a refusal into `UsageError`. `currency.resolve` and `withConfigWarnings` handle currency. `openReport` -> `srv.X` -> `emitReport(cmd, *jsonOut, warnings, renderJSON, renderText)` (`output.go:33-40`, warnings on stderr as `quarry: warning: `). Table renderer: `renderTable(caption, aligns, rows)` (`render_table.go:23-47`), `windowCaption` (`:55`). `transactionFlagHelp` (`window.go:26-30`) is the "count transactions dated ..." help text, which fits search better than `anomaliesFlagHelp`.
- Row limit flag: `sql --limit` (`internal/cli/sql.go:16-23,49-54,62,112,116-138,171-173`). 0 means every row, negative is `UsageError` (`errSQLNegativeLimit`), and there is a truncation note on stderr.
- Row-listing store read: `duckstore/charges.go:15-77`. A SQL const, `s.openRead(ctx)`, `db.QueryRows(ctx, q, args, func(scan) error)`, a `scanX` helper, `openFault(s.Path(), err)` on any error. `accountFilter` (`duckstore/filter.go:14-39`) is the IN-clause helper, but `and()` numbers its params from `$3` because it assumes a two-date window (`filter.go:19-25`; `marks(first)` is general).
- MCP cap: `capList[T]` (`internal/mcp/cap.go:9-17`). It cuts after the document is built, so a search that returns every match reads the whole table first.
- Read-fault tests: `duckstore/read_faults_test.go` has `rowReads()` (`:21-40`, a list of Status/Accounts/Schema/Charges/Findings) and `allReads()`; the open-fault and query-fault tests loop over them. A new read adds one `readOp` row. 3a STATE:64 trap: the query-fault row fails only the first query, so further `QueryRows` calls need `passQueries`.
- Per-row ctx check: `internal/platform/duckdb/duckdb.go:150,183`, `table.go:73` (STATE trap: `rows.Err()` alone misses a cancel). It lives in the adapter's `QueryRows`, so a new read gets it free if it uses `db.QueryRows`.

#### Already exists - do not re-plan
- `store.Transaction` (`internal/store/store.go:81-96`): ID, SourceID, AccountID, Date, PayeeID, Memo, Amount (cents int64), Currency, Status, ChequeNumber, ExcludedFromReports, PostedDate. It is the table row type (the read can use it or its own row type).
- Schema (`duckstore/schema.go:44-66`): `transactions(id, source_id, account_id, date, payee_id, memo, amount DECIMAL(18,2), currency, status, cheque_number, excluded_from_reports, posted_date)`, `splits(id, source_id, transaction_id, category_id, amount, memo, transfer_account_id)`, `payees(id, source_id, name)`, `transfers`. `v_cash_flow` / `v_spending` (`schema.go:171-208`) are split-level, reported-account-only, transfer-excluded, and carry category, payee name and converted amounts.
- `reportedAccount` predicate `a.in_reports AND NOT a.linked_tracking` (`schema.go:~169`); `transactionRange` (`filter.go:57-65`).
- `report.ParseWindow`, `ParseChargeWindow`, `DefaultWindow` (`internal/report/window.go`), `WindowError` with parts, and `mcp.windowRefusal`.
- `Server.namedAccounts` (`internal/report/accounts.go:92-105`): names to accounts and ids; unknown/ambiguous become `RefusalError`; `mcp.accountRefusal` words them for the client.
- `readRefusal` (`internal/report/refusal.go:52-57`) / `storeRefusal`. Stderr classes for window, account and `OpenFaultOther` are already ruled and implemented (3b Rule 4; STATE "logLine ... refusalLine").
- `resolveCurrency`, `handler`, `capList`, `described`, `accountsSchema`, `currencySchema`, `sinceDescription` / `untilDescription` / `accountsDescription`, `limitSchema` (`tools.go:251`, shared by `query` and `data_quality`, wrap with `described(...)`), `absentNullArguments`, `windowRefusal`.
- `money.ParseCurrency` (`internal/platform/money/money.go:24`). No amount parser exists yet (grep of `internal/platform/money` finds only `ParseCurrency`).
- A "search" word is free in cli; nothing to rename.

#### Must be built
- `store.SearchParams` / result type and one `report.Store.Search` method. Row carries at least date, account (id and name), payee, memo, amount cents, currency (see Q1).
- `duckstore.Search` plus the SQL: text predicate, amount range, window, account filter, order, limit. There is no `LIKE`/`ILIKE` prior art in `internal/`, so `%`, `_` and the escape character are new ground. DuckDB parameter binding keeps the text from being SQL, but `%` and `_` inside the user's text still act as wildcards unless escaped.
- `report.Server.Search(ctx, SearchRequest)`: `namedAccounts`, store read, `readRefusal`, plus the amount-argument validation.
- `document.NewSearch` plus `SearchWarnings`; the CLI `--json` document and the MCP tool share it (Rule 1).
- `internal/cli/search.go`, `render_search.go`, `json_search.go` (or the document equivalent), registered in `root.go`.
- `internal/mcp/search.go`, `toolSearch` const, schema and description consts in `tools.go`, `registerTools` entry, cap with its copy line, and the copy updates listed under Affected surface.
- Tests, with these existing homes: a `read_faults_test.go` row; `run_mcp_*` equality test (harness: `cmd/quarry/run_mcp_documents_helpers_test.go`); the all-tools tables (no-store, timeout, store-fault, tools/list, account refusal) each gain a row.
- A capped-in-store read, or a limit parameter, if the answer to Q3 is "don't read everything".

#### Callers (grep; nothing exported changes shape, only additions)
| Symbol / string | Caller (path:line) | Via |
|---|---|---|
| `report.Store` (interface gains a method) | implemented only by `*duckstore.Store`, asserted `cmd/quarry/run.go:31` | grep |
| `report.Store` embedded in fakes (compile without the new method; panic if a test calls it) | `internal/cli/fakes_test.go:19`, `internal/cli/status_test.go:20`, `internal/mcp/log_internal_test.go:72`, `internal/mcp/log_classes_internal_test.go:26`, `internal/mcp/timeout_test.go:20-23`, `internal/mcp/query_helpers_test.go:36`, `cmd/quarry/run_mcp_timeout_test.go:122-124` | grep |
| `stallingStore` overrides `Charges` to record the deadline (`timeout_test.go:76`); a search timeout row needs a `Search` override | `internal/mcp/timeout_test.go` | grep |
| `rowReads()` / `allReads()` (add a search op) | `internal/store/duckstore/read_faults_test.go:21-48` | grep |
| `AddCommand` list | `internal/cli/root.go:28-38` | grep |
| root help subcommand list | `cmd/quarry/run_status_test.go` `Test_run_help_prints_quarrys_description` | grep |
| `registerTools` (AddTool calls) | `internal/mcp/tools.go:192-223` | grep |
| tools list strings: `instructions` | `internal/mcp/tools.go:36-38` | grep |
| `quarry mcp` Long | `internal/cli/mcp.go:24-36` | grep |
| descriptions pin | `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_all_eight_tools` | grep |

#### Becomes dead if this ships
Nothing becomes dead. `query` stays as the ad-hoc path; its description will want a "call search_transactions for payee/memo lookups" redirect (copy, not deletion).

#### Answers to the numbered questions
1. Source set. `Charges` (`charges.go:15-37`) is expense-only and v_spending-based, so it is not a base for search. Two honest choices: (a) `transactions` joined to `accounts` and `payees`, with all transactions including transfers and `excluded_from_reports` (an `excluded` boolean on each row would let the user see why Quicken's reports omit it); (b) `v_cash_flow`, which is reported-account, transfer-free, split-level and carries category and converted amounts, but hides transfers and excluded rows. A person asking "find that Costco charge" or "all transfers to savings" generally wants (a); (b) is spending's set and lets search disagree with `transactions` rows. Row level: transaction-level matches `store.Transaction` and `Charge`. Transaction-level amounts are native (`t.amount`, `t.currency`); the splits (category, per-split memo, `transfer_account_id`) are what a transaction-level row would drop. That choice (and a category column or filter) is Q1/Q2.
2. Port. Implementers: only `duckstore.Store`; all fakes embed the interface (table above). CLI shape to mirror: `anomalies.go` for flags/window/currency/emit, `sql.go` for `--limit`.
3. Semantics, evidence only. No LIKE/ILIKE use anywhere in `internal/` (grep) and no amount parsing. Window and account reuse is direct (`flags.window`, `namedAccounts`; `accountFilter.and` hard-codes `$3` so a different parameter layout needs `marks(first)`). Amount sign: `transactions.amount` is signed native with outflow negative (`v_cash_flow` flips it for `spent`); a min/max filter needs a sign rule (Q3). Currency: `transactions.currency` is per-row native; converting would need the `convertedTo` / `fx_rates` machinery (`schema.go:~180`, `convertedTo`), which search does not otherwise need. Ordering: Charges and recurring order date asc then `source_id` (`charges.go:37`); search wants date desc then `source_id` for stability, and the cap-drops-the-oldest consequence needs a ruling (anomalies precedent: "newest first, so a cut drops the oldest", 3b spec §6.2). Rule 11 caps are post-document in MCP (`cap.go`), so a store-side limit is needed to avoid reading all history.
4. Rule 4. Payee/memo are document data (allowed in the result; the `query` tool and `instructions` already pass them through, mcp Long `cli/mcp.go:35-36`) and never on stderr. New outcomes needing stderr classification: a bad amount (non-numeric, min above max) and a bad text pattern, if any pattern syntax exists. Both should be fixed-copy refusals (`withLog`/`verbatim`) with caller text only in the client line, same class as window refusals (spec §6.1 rows 2-4). Existing arms cover window, account, store faults, timeout and config. An empty or whitespace-only text query (matches everything?) is a ruling for copy, not a store question.
5. CLI conventions. Commands: sync, status, accounts, spend, cashflow, recurring, anomalies, findings, sql, snapshots, mcp (`root.go:28-38`). Long text style: paragraph + `Example:` block (see `anomalies.go`). `--json` is a persistent root flag bound to `Execute`'s `jsonOut` (3a STATE:58). Exit codes: bad flags are `UsageError` (exit 2), store/runtime faults `runtimeError`. Caps: `sql --limit` (default 500, `0` = every row, negative refused). No name collision.
6. Slicing (below).
7. Open questions below.

#### Slicing and size
One feature, one spec `phase3c-search`. It is smaller than 3b (about 20-30% of it). Rough scenarios (one `When` each):
1. Store `Search` read with text predicate, ordering, limit, and the read-fault row (largest; mandatory-test-first SQL; per-arm pins for each filter, escaping, order).
2. `report.Server.Search` + document `NewSearch` + CLI `quarry search` (text and `--json`, window, account, empty-result warning).
3. CLI refusal rows (bad amount, min>max, bad since/until, unknown/ambiguous account, store faults).
4. MCP `search_transactions` end to end (schema, description, equality with the CLI `--json`, cap line last).
5. MCP refusal rows and stderr classification, plus the all-tools table rows for the ninth tool.
6. Copy updates (`instructions`, mcp Long, `query` redirect, descriptions pin).
Likely FOLD candidates: 5 into 4, 3 into 2. Plan on roughly 4-6 developer runs. The `-` unit overhead (scoping, gate) was 24% of 3b tokens, so keep scenario count small.

Retro lessons that apply (3b RETRO.md "Late findings", "Ranked rule changes" 1, 5):
- Per-surface outcome table in the spec before scenarios (0 Rule-4 findings in 3b with it, 1 MAJOR without): rows for CLI and MCP separately, stderr column, values column.
- Pins that can go red for: each default-by-omission (limit default, ordering default, absent amount bounds), each passthrough arm (`windowRefusal`, `accountRefusal` for the new tool, new amount-refusal wrapper), and each "X is last" claim (cap line last in `warnings`; competing warning after it in the test).
- A red acceptance test must not panic (stub returns a zero document that the test dereferences kills the whole `cmd/quarry` test binary; assert/`require` first).
- Mutation sample runs alone, no load generators, no background processes.
- Equality harness rules: `Env.Now` fixed via `runWith`, never `run()`; fixtures under the 500 cap; warnings stripped and mapped separately; do not rename a ticked acceptance test without repointing the spec line (`spec-check.py`).
- Copy ruled at scoping by `product-vision` Phase 1 (flag names, help, refusal lines, exit codes, JSON keys), not at the keyboard.

#### Open questions (user only)
1. Source set: all transactions (incl. transfers and `excluded_from_reports`, with an indicator on each row) or only the transfer-aware reported set (v_cash_flow)? Changes the SQL base, whether transfers are findable, and whether search totals can ever differ from `spend`/`cashflow`.
2. Row grain and category: one row per transaction (native amount) or per split (category, per-split memo)? Is a category filter/column in scope for 3c? A per-split row changes the document shape and the cap noun.
3. Amount semantics: native only, or also convertible? Sign convention for min/max (signed, or magnitude with a direction flag?). One exact `--amount` too, or only a range?
4. Text semantics: case-insensitive substring over payee OR memo (OR also split memos?), literal `%` and `_` treated as plain characters, or wildcard/regex allowed? Is the text parameter required or may search be amount/date-only?
5. Result limit: cap at 500 like the other tools with a warning, and does the CLI get `--limit` (sql precedent) or stay uncapped (Rule 1 byte-equality holds only below the cap, as for the other four tools)? Newest-first, so a cut drops the oldest? Does the MCP tool take a `limit` parameter (`query`/`data_quality` precedent) or only the fixed cap?
6. Currency flag: does search take `--currency` at all (nothing is converted unless Q3 says so)? If not, the MCP schema has no `currency` property and an unknown-property refusal covers it.
7. CLI name: `quarry search` (the user's stated name) is free; confirm it, and the flags (`--text`, `--min`, `--max`, positional query?) go to the product-vision Phase 1 copy pass.
8. Carry-over: PRD §MCP says currency "CAD or USD" while the shipped enum adds `native` (3b STATE debts); not a 3c blocker, but the user still owes a Decisions line.

## Product Verdict

**SHIP WITH CHANGES** (product-vision, Phase 1, 2026-10-03), changes folded into the rules above. Four interpretations of the user's decisions, approved with the scenarios: (a) each row carries `splits[{category,memo,amount,transfer}]`; (b) `excluded` also covers accounts Quicken's reports leave out (not in reports, linked tracking); (c) default window = all dates; (d) MCP `min`/`max` are JSON strings (the SDK re-marshals numbers through float64).

User decisions 2026-10-03: CLI twin `quarry search`; source = all transactions, flagged; grain = per transaction with splits/categories; amounts = absolute, native, no conversion; text = case-insensitive literal substring, optional.

## Surface & Copy

Product-vision Phase 1 ruling, verbatim (headings demoted). Binding: developers implement these strings verbatim.

### Product-vision Phase 1: phase3c-search

#### What I read
- The triage brief (`scratchpad/triage-3c.md`).
- The phase3b spec: Rules 1-12, §3-§6, the outcome tables and the cross-rule check. Also its STATE.md and RETRO.md.
- PRD §CLI, §MCP server and the exit-code table.
- In `internal/cli`: `anomalies.go`, `window.go`, `sql.go`, `mcp.go`, `root.go`, `render_table.go`, `render_anomalies.go`, and `render.go` (`formatMoney`, `accountLabel`, `payeeLabel`).
- `internal/mcp/{tools.go,anomalies.go,cap.go,arguments.go}`.
- `internal/report/{window.go,sql_conventions.go}` and `internal/report/document/{anomalies.go,common.go,sql.go,warnings.go}`.
- `internal/store/duckstore/schema.go`: the tables, `v_cash_flow` and `reportedAccount`.
- `cmd/quarry/run.go` `newReportFactory`, and the root help pin in `cmd/quarry/run_status_test.go:110-136`.
- go-sdk v1.8.0 `mcp/tool.go:75-139` (`applySchema`).

Two facts I checked myself, because they decide rulings:
1. **The SDK turns tool arguments into floats.** `applySchema` unmarshals the arguments into a `map[string]any`, applies defaults, then re-marshals (`tool.go:94-139`). A JSON number therefore becomes a `float64` before quarry sees it. MCP `min`/`max` must be strings (Q5).
2. **The CLI report factory never loads config.** `newReportFactory` (`cmd/quarry/run.go:92-101`) builds only the store and home. Search has no `--currency`, so it never reads config. On both surfaces the config outcome row disappears.

#### 1. Verdict on the slice
Ship it as one feature: one `report.Store.Search` read, one `report.Server.Search`, one `document.NewSearch`, a `quarry search` command and a `search_transactions` tool. It is about a quarter the size of 3b.

**Nothing dies.** `query` remains the escape hatch, and its description gains a redirect.

**Four rulings interpret the user's 2026-10-03 decisions. Each is marked [INTERPRETATION] below.** They are not departures, but SHIP WITH CHANGES auto-continues past the user, so the orchestrator should show them at scenario approval:
- **(a)** The row lists its splits as a `splits` array (category, memo, amount and transfer per split), not a bare category list. Text matches split memos, so a hit must be explainable from the row.
- **(b)** `excluded` also covers an account that Quicken's reports leave out, not only the transaction's own "exclude from reports" mark.
- **(c)** The default window is all dates, not January 1 of this year.
- **(d)** MCP `min`/`max` are JSON strings, not numbers.

#### 2. Literal surface

##### 2.1 Rules for the spec (binding)
- **Rule 1 (extended to search).** `search_transactions`' structured result is the `quarry search --json` document for the same inputs, built by `document.NewSearch`.
  - Every field except `warnings` is byte-equal for the same text, since, until, accounts, category, min, max and limit.
  - In `warnings`, only the cut line differs (§2.5). The no-match line is the same on both surfaces.
- **Rule S1: the source.** Every transaction in the store is searched: every account (closed, left out of reports, linked tracking), transfers, and transactions marked "exclude from reports".
  - Each row carries `transfer` and `excluded`. Both come from the same SQL fragments `v_cash_flow` uses (Q4):
    - `transfer`: at least one split is named in `transfers.from_split_id` or `transfers.to_split_id`. The `NOT EXISTS` in `cashFlowViewDDL` becomes one shared fragment used by both.
    - `excluded` **[INTERPRETATION b]**: `NOT (reportedAccount AND NOT t.excluded_from_reports)`, reusing the `reportedAccount` const.
  - Of `v_cash_flow`'s five exclusion reasons:
    - a transfer leg → `transfer`
    - excluded_from_reports → `excluded`
    - an account left out of reports → `excluded`
    - a system category → not flagged. The Long and the tool description say so.
    - an uncategorized zero-amount split → not flagged. It carries no money, so no copy is needed.
- **Rule S2: the grain.** One row per transaction, with the native signed amount and currency. `splits` are in split `source_id` order. A transaction with no splits has `splits: []`.
- **Rule S3: text.**
  - Case-insensitive substring over the payee name, the transaction memo and every split memo, with both sides Unicode lower-cased.
  - Every character is literal. Recommendation to the architect: use `contains(lower(x), lower($1))` rather than `LIKE`, so `%`, `_` and `\` cannot act as wildcards and no escape clause exists.
  - Account and category names are not matched.
  - The text is matched as given, with no trimming. Blank text (empty or whitespace only) is refused (§3).
- **Rule S4: amounts.** `--min`/`--max` compare `abs(transaction amount)` in the account's native currency. Nothing is converted, there is no `--currency`, and the comparison is inclusive at both ends. min == max is allowed: it finds one amount. Split amounts are never compared.
- **Rule S5: category.**
  - A transaction matches when any of its splits has a category whose `full_path` equals the argument in any letter case, or starts with the argument plus `:`. That is the category and everything under it. Hidden categories count.
  - The argument is unknown when no `categories.full_path` equals it in any letter case. An unknown category is refused, never an empty result.
  - A category that exists but matches nothing gives an empty result with the no-match warning.
  - The category is single-valued, not repeatable.
- **Rule S6: window [INTERPRETATION c].**
  - Absent `since` means no lower bound. Absent `until` means no upper bound, so future-dated transactions are included.
  - A given bound uses the existing forms: `since` is the first day of the period named, `until` the last day.
  - Parse with a new `report.ParseSearchWindow(since, until *string)` that takes no clock and returns the existing `report.WindowError`, so `mcp.windowRefusal` passes it through unchanged.
  - Reachable kinds: `WindowNotADate` and `WindowSinceAfterUntil`. Unreachable: `SinceAfterToday`, `ChargeSinceAfterToday` and `UntilBeforeDefault`.
  - Why: search is a lookup, not a period report. Defaulting to this year silently misses "that Costco charge from 2023", and an empty result cannot be told apart from "never happened". The limit plus newest-first keeps the default cheap.
- **Rule S7: order and limit.**
  - Order is `date DESC, source_id DESC`, so a cut drops the oldest.
  - The limit is applied in the store read, with the total match count read in the same statement (for example `count(*) OVER ()`).
  - `matched` always counts every match. `truncated` is `matched > len(transactions)`.
  - This deliberately departs from 3b Rule 11's "cut after the document is built": the result is the same first N in document order, without reading all of history to throw it away.
- **Rule S8: config and clock.** Search never reads config and never reads the clock. Neither surface has a config outcome. Rule 12 does not apply.
- **Rule S9: order of outcomes.** RunE and the handler follow the same order:
  1. argument shape (CLI: argument count, then `--limit`, then blank text; MCP: the SDK schema check, then blank text)
  2. `min`
  3. `max`
  4. min > max
  5. window
  6. store open
  7. account resolution
  8. category resolution
  9. search read (statement-time fault)

  A timeout or interrupt can end any step after 1.

##### 2.2 CLI
- `Use: "search [text]"`
- `Short: "Find transactions by payee, memo, amount, date, account or category"`
- Long:
```
Find transactions by text, date, account, category or amount, newest
first. The text matches payee names, transaction memos and split memos,
ignoring letter case; every character is literal, so % and _ match only
themselves. Leave the text out to search by the flags alone, or pass
nothing at all to list the newest transactions. Text that starts with -
goes after --: quarry search -- "-50% off"

Every transaction is searched, closed accounts included. Transfers
between your own accounts and transactions Quicken's reports leave out
are listed too, flagged transfer or excluded, because quarry spend and
quarry cashflow do not count them. Excluded means the transaction is
marked "exclude from reports" in Quicken, or its account is not used in
reports or uses linked account tracking. Spend also leaves out Quicken's
system categories; those are not flagged.

Amounts are in each account's own currency and are never converted.
--min and --max compare the amount without its sign, so --min 100 finds
charges and deposits of 100.00 or more; give both the same value to find
one amount. --category matches a split in that category or in any
category under it, by full path in any letter case.

Without --since and --until every date is searched, future-dated
transactions included. At most --limit transactions are printed (500
unless set); when more match, quarry says so on stderr.
```
- Example:
```
  quarry search costco
  quarry search --min 42.17 --max 42.17
  quarry search "e-transfer" --account Chequing --since 2026-01
  quarry search --category Food --since 2026-09 --json
```
- Flags, in this order. The backticked word becomes the placeholder.

| Flag | Type | Help string |
|---|---|---|
| `--since` | string | `list transactions dated on or after `` `date` `` (YYYY, YYYY-MM or YYYY-MM-DD; default the first transaction)` |
| `--until` | string | `list transactions dated on or before `` `date` `` (YYYY, YYYY-MM or YYYY-MM-DD; default no end, future-dated included)` |
| `--account` | string array | `search only the account with this `` `name` `` or id; repeat for more` |
| `--category` | string | `list only transactions with a split in this category or one under it, by full `` `path` `` such as Food:Groceries` |
| `--min` | string | `list only transactions of at least this `` `amount` ``, sign ignored, in the account's own currency` |
| `--max` | string | `list only transactions of at most this `` `amount` ``, sign ignored, in the account's own currency` |
| `--limit` | int | `print at most `` `n` `` transactions, newest first (500 unless set; 0 prints every one)` |

  - `--limit` uses the `sql` trick: the flag's own default is 0, so help prints no "(default …)", and Changed decides. `defaultSearchLimit = 500`.
  - There is no `--currency` and no `--csv`. Both are cobra's unknown-flag line, exit 2. For CSV, use `quarry sql --csv`.
- Root help row, between `recurring` and `snapshots`: `  search      Find transactions by payee, memo, amount, date, account or category`

##### 2.3 Amount grammar (both surfaces, one parser in `report` or `platform/money`)
- Accepted: `^[0-9]{1,16}(\.[0-9]{1,2})?$`, for example `12`, `12.5`, `12.50`, `0`.
- Refused with the not-an-amount line:
  - `1,234.56`
  - `-12` and `+12`
  - `$12`
  - `12.`, `.5` and `12.345`
  - `1e2`
  - leading or trailing spaces
  - `""`
  - more than 16 integer digits, because DECIMAL(18,2) cannot hold it
- The value is echoed normalized: `12.5` → `"12.50"`.

##### 2.4 `--json` document (also the MCP result). Key order is binding.
```
{"since": "2026-01-01" | null, "until": "2026-03-31" | null,
 "account_filter": [...existing shape...],
 "text": "costco" | null, "category": "Food" | null,
 "min": "20.00" | null, "max": "50.00" | null,
 "limit": 500, "matched": 1234, "truncated": true,
 "transactions": [ {"transaction_id","date","account_id","account","payee","memo","amount","currency","transfer","excluded",
                    "splits":[{"category","memo","amount","transfer"}]} ],
 "warnings": []}
```
- `since`/`until` echo the resolved first or last day when given, and `null` when open.
- `text` and `category` are echoed exactly as given (no case-folding).
- `limit` is the value used: 0 on the CLI means every match, and it is never 0 on MCP.
- `payee` is null when there is no payee.
- `memo` and split `memo` are null for both NULL and `""` (the `NullString` precedent).
- Split `category` is null when uncategorized or a transfer leg.
- `amount` is the transaction's native signed amount in `document.Money` form. `currency` is the native currency.
- Every array is `[]`, never null.
- `--json` has no other mode. Refusals print nothing on stdout.

##### 2.5 Text output
- **Caption:** `Transactions` + ` matching %q` (if text) + ` in ` + `accountsCaption` + `, ` + dates + `, category %q` (if given) + `, amount ` + range (if given).
  - dates: `all dates` | `from 2026-01-01` | `through 2025-12-31` | `2026-01-01 to 2026-03-31`.
  - range: `20.00 to 50.00` | `at least 20.00` | `at most 50.00` | `exactly 42.17` (min == max), using `formatMoney` (grouped).
  - Example: `Transactions matching "costco" in Visa Infinite, from 2025-01-01, amount at least 100.00`
- **Columns:** `Date | Account | Payee | Category | Memo | Amount | Flags`. Amount is right-aligned, the rest left-aligned.
  - Account: `accountLabel` (shows the currency and closed/inactive).
  - Payee: `payeeLabel` (`(no payee)`).
  - Category: each split's label (path, `(transfer)` or `(uncategorized)`), distinct, in split order, joined with `, `. No splits gives `(uncategorized)`.
  - Memo: the transaction memo, then each distinct non-empty split memo that differs from it, joined with ` / `. A blank cell when there are none.
  - Amount: `formatMoney`, signed, native.
  - Flags: `transfer`, `excluded`, `transfer, excluded`, or empty.
- **Footer** (after a blank line): `humanize.Count(matched, "matching transaction", "matching transactions")`. It always counts every match.
- **Data text:** every cell taken from the user's file goes through `escapeCell` (`\n`, `\t`, `\r`; the existing single escaper). Caller strings in the caption use `%q`. In JSON: standard encoding via `marshalDocument`, with null versus `""` as in §2.4. There is no CSV channel. Paths appear only in the existing store refusals, in `~` form on stderr and never in `--json`.

##### 2.6 Warnings (doc `warnings` + CLI stderr `quarry: warning: `; at most ONE line per result)
- **No match** (identical on both surfaces):
  - `no transactions match the search; the store's transactions run 2003-01-02 to 2026-09-30`
  - store empty: `no transactions match the search; the store has no transactions`
  - accounts named: `no transactions in the named accounts match the search; their transactions run A to B` / `...; they have no transactions`
- **Cut:**
  - CLI: `showing the newest 500 of 1,234 matching transactions; pass --limit 0 to list every one`
  - MCP, limit = 500: `search_transactions lists the newest 500 of 1,234 matching transactions; narrow the search with text, since, until, accounts, category, min or max`
  - MCP, limit < 500: `search_transactions lists the newest 20 of 1,234 matching transactions; pass a higher limit, up to 500, or narrow the search with text, since, until, accounts, category, min or max`
  - Numbers use `humanize.Thousands`.
- **The two are mutually exclusive.** A cut needs at least limit + 1 matches, and the no-match line needs 0. There are no FX, config or left-out warnings: rows are flagged instead. So there is no "last" claim to pin.

##### 2.7 MCP `search_transactions`
- Tool const `toolSearch = "search_transactions"`.
- Description (hard line breaks, no trailing newline):
```
Find the user's transactions by text, date, account, category or amount,
newest first. text matches payee names, transaction memos and split
memos, ignoring letter case; % and _ are plain characters. Every
transaction is searched, including transfers between the user's own
accounts (flagged transfer) and transactions Quicken's reports leave out
(flagged excluded); spending and cash_flow do not count those, so call
them for totals rather than adding up these rows. Amounts are in each
account's own currency and are never converted. Returns at most limit
transactions (default 500); matched counts every match.
```
- Input schema: `objectSchema`, `additionalProperties:false`, nothing required. No `pattern` and no `minimum`, so the handler owns every value refusal.

| param | schema | description |
|---|---|---|
| `text` | `{"type":"string"}` | `Words to find in payee names, transaction memos and split memos, in any letter case; every character is literal. Omit it to search by the other parameters alone.` |
| `since` | `{"type":"string"}` | `Earliest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. Omit it to search from the first transaction.` |
| `until` | `{"type":"string"}` | `Latest date to list: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Omit it to search every later date, future-dated transactions included.` |
| `accounts` | `accountsSchema(...)` | `Search only these accounts, each given by id or by name in any letter case. Omit it to search every account.` |
| `category` | `{"type":"string"}` | `List only transactions with a split in this category or one under it, given by its full path (such as Food:Groceries) in any letter case.` |
| `min` | `{"type":"string"}` | `Smallest amount to list, as a string such as "25" or "19.99", compared without its sign in the account's own currency.` |
| `max` | `{"type":"string"}` | `Largest amount to list, as a string such as "100" or "250.50", compared without its sign in the account's own currency. Give min and max the same value to find one amount.` |
| `limit` | `described(..., limitSchema(maxRows))` (integer 1..500, default 500) | `Most transactions to return, newest first, 1 to 500. Defaults to 500. matched always counts every match.` |

- `limit` carries a schema default, so `absentNullArguments`' absent/null/`{}` test gains a `search_transactions` row.
- Absent, empty and null values:

| Value | Behaviour |
|---|---|
| `text` absent | no text filter |
| `text` `""` or whitespace | blank-text refusal |
| `since`/`until` `""` | not-a-date refusal |
| `category` `""` | unknown category |
| `accounts` `[""]` | unknown account |
| `min`/`max` `""` | not-an-amount refusal |
| any `null` | SDK type refusal |
| `min`/`max` as a JSON number | SDK type refusal (wants string) |

##### 2.8 Changes to existing surfaces
1. **`instructions`, full replacement.** It also closes the "David's" open debt from 3b STATE:
```
quarry serves the user's Quicken Classic for Mac data from a local,
read-only store. Call sync_status first and tell the user how old the
snapshot is (snapshot.taken_at). For spending, income, recurring charges
and unusually large charges call spending, cash_flow, recurring_charges
and anomalies: they apply quarry's rules for transfers, refunds and
currencies. To find particular transactions by payee, memo, amount or
date call search_transactions. For other questions call describe_schema
before writing SQL for query: its conventions say which views already
leave out transfers and how amounts, signs and currencies work. Every
number you report must come from a tool result; never estimate. quarry
cannot change data: fixes are made in Quicken, then the user runs quarry
sync.
```
2. **`queryDescription`, full replacement** (keep the `` `limit` `` concat):
```
Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income totals call spending or
cash_flow instead, and to find transactions by payee, memo or amount call
search_transactions; in SQL use v_spending and v_cash_flow, which already
leave out transfers between the user's own accounts. Returns at most
`limit` rows (default 500, the most allowed); aggregate in SQL rather
than paging through rows. The store cannot be changed, and other files,
databases and extensions are off.
Send one statement; if you send several, only the last one's rows come back.
```
3. **`quarry mcp` Long, last paragraph:** `Tools: describe_schema, query, sync_status, data_quality, spending,\ncash_flow, recurring_charges, anomalies, search_transactions.` The "every list a tool returns stops at 500 entries" sentence stays true.
4. **Root help pin** (`run_status_test.go:122-135`): add the §2.2 row. Also update the `newRootCommand` doc comment (`root.go`), which lists the subcommands.
5. **`Test_run_mcp_describes_all_eight_tools` now covers nine tools, so its name is false.** Rename it to `Test_run_mcp_describes_every_tool`, and repoint the phase3b spec line that cites it in the same scenario. `spec-check.py` trap, 3b STATE:48.
6. **`report.SQLConventions`: untouched.**
7. **PRD:**
   - Add a CLI table row: `quarry search` | `Find transactions by payee, memo, amount, date, account or category; transfers and report-excluded transactions included and flagged`.
   - Add a Decisions line: "search defaults to all dates; --min/--max compare absolute native amounts; no --currency; search is a lookup, not a report".
   - The "Every reporting command takes --currency" line stays true, because search is not a reporting command.
8. **Unchanged, already true:** the 3b tool descriptions, all CLI bytes of the other commands, and `describe_schema`.

#### 3. Refusal copy (one line, no `quarry: ` on MCP)
| Outcome | CLI (stderr `quarry: ` + line, exit) | MCP client line |
|---|---|---|
| more than one positional argument | `search takes one text; quote it as one argument` (2) | n/a |
| `--limit` < 0 | `--limit must be 0 or more; 0 prints every transaction` (2) | SDK (limit 1..500) |
| blank text | `search text is blank; leave it out to search by date, account, category or amount alone` (2) | `text is blank; leave it out to search by date, account, category or amount alone` |
| not an amount | `--min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99` (2; likewise `--max`) | `min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99"` (likewise `max`) |
| min > max | `--min 50 is more than --max 20` (2), with the raw values as given | `min 50 is more than max 20` |
| not a date / since after until | existing `WindowError` CLI lines (2) | existing `windowRefusal` lines |
| unknown account / ambiguous | existing lines (1) | existing `accountRefusal` lines |
| unknown category | `no category named "Fod"; list them with quarry sql "SELECT full_path FROM categories ORDER BY full_path"` (1, parity with unknown account) | `no category named "Fod"; call describe_schema to list the categories` |
| store faults | existing `storeRefusal` lines (1) | existing |
| interrupt | `search interrupted` (1) | silent on cancel; timeout `search_transactions stopped after 30 seconds; try again` |

- The not-an-amount line quotes the value with `%q`, as not-a-date does. min > max prints the raw values, as since-after-until does.
- The unknown category needs a new `report.RefusalError` kind (`RefusalUnknownCategory`, `Arg`) so each surface words it and `mcp.refusalLine` classifies it. The amount refusals need a parts-carrying error (`Bound` min/max, `Value`, `Other`), like `WindowError`. The design is the architect's.

#### 4. Outcome tables

##### 4.1 CLI `quarry search` (stdout / stderr / exit)
| # | Outcome | stdout | stderr | exit |
|---|---|---|---|---|
| 1 | 2+ positional args | — | `quarry: search takes one text; quote it as one argument` | 2 |
| 2 | `--limit -1` | — | `quarry: --limit must be 0 or more; 0 prints every transaction` | 2 |
| 3 | blank text (`""`, `"  "`) | — | `quarry: search text is blank; …` | 2 |
| 4 | unknown flag (`--currency`, `--csv`) | — | cobra's existing line | 2 |
| 5 | bad `--min` / `--max` (each §2.3 class) | — | `quarry: --min "<v>" is not an amount; …` | 2 |
| 6 | min > max | — | `quarry: --min 50 is more than --max 20` | 2 |
| 7 | bad since/until, since > until | — | existing window lines | 2 |
| 8 | future `--since`, no `--until` | rows incl. future-dated | none | 0 |
| 9 | no store / other format / not DuckDB / permission / locked / OpenFaultOther | — | existing store lines, `~` | 1 |
| 10 | unknown / ambiguous account (incl. `--account ""`) | — | existing lines | 1 |
| 11 | unknown category (incl. `--category ""`) | — | `quarry: no category named "Fod"; list them with quarry sql "…"` | 1 |
| 12 | Ctrl-C | — | `quarry: search interrupted` | 1 |
| 13 | matches ≤ limit | table or doc | none | 0 |
| 14 | matches > limit | first `limit` | `quarry: warning: showing the newest 500 of 1,234 matching transactions; pass --limit 0 to list every one` | 0 |
| 15 | `--limit 0` | every match, `truncated:false` | none | 0 |
| 16 | zero matches (incl. a known category with no transactions) | caption, header, footer `0 matching transactions`; JSON `transactions:[]`, `matched:0` | `quarry: warning: no transactions match the search; …` | 0 |
| 17 | store has no transactions | same | `…; the store has no transactions` | 0 |
| 18 | config broken or unknown key | normal result; config never read | none | 0 |
| 19 | closed account | included, label `, closed` | none | 0 |
| 20 | account left out of reports / linked | included, `excluded` | none | 0 |
| 21 | USD account | native amount, label `(USD)` | none | 0 |
| 22 | stale store | no hint (that is `status`'s job) | none | 0 |
| 23 | Quicken closed, reconciliation failed, fingerprint changed | no row: search reads only the store, and a failed sync leaves the previous one | — | — |
| 24 | `--json` + any refusal | nothing | same line | same |

- **An empty result exits 0.** The PRD's "not found" (exit 1) means a named resource (account, category, snapshot). That is why rows 10-11 exit 1 and row 16 exits 0. Put this sentence in the spec, so nobody "fixes" it toward grep's exit-1 convention.

##### 4.2 MCP `search_transactions`
The stderr column is the text after `quarry: mcp: search_transactions: `.

| # | Outcome | Client | stderr | Values: client / stderr |
|---|---|---|---|---|
| 1 | SDK refusal (type incl. number min/max, limit outside 1..500, unknown property, null) | ✗ SDK text | `refused the call's arguments; details went to the client only` | may echo / none |
| 2 | blank text | ✗ §3 | `refused the call's text; details went to the client only` **(new)** | none / none |
| 3 | bad min/max | ✗ §3 | `refused the call's min or max; details went to the client only` **(new)** | caller value / none |
| 4 | min > max | ✗ §3 | same as row 3 | caller values / none |
| 5 | not a date, since > until | ✗ `windowRefusal` | existing window class | caller value / none |
| 6-10 | store rows (3b §6.1 rows 6-10) | unchanged | unchanged (OpenFaultOther withheld) | unchanged |
| 11 | unknown account | ✗ existing MCP line | existing withheld class | caller text / none |
| 12 | ambiguous account | ✗ existing | existing withheld class | caller text, ids / none |
| 13 | unknown category | ✗ `no category named "Fod"; call describe_schema to list the categories` | `refused the call's category: it names no category; details went to the client only` **(new)** | caller text / none |
| 14 | timeout | ✗ `search_transactions stopped after 30 seconds; try again` | same | none / none |
| 15 | cancel | silence | none | — |
| 16 | factory / unclassified | ✗ text | `failed; details went to the client only` | possibly / none |
| 17 | success, cut, zero matches | R (+ one W) | none | payee/memo/names in the doc only / — |
| 18 | config broken | R (config never read) | none | — |
| 19-23 | closed, left-out, USD account, stale, Quicken-side failures | as in CLI rows 19-23 | none | — |

##### 4.3 Cross-rule check
1. **Rule 1 vs the cut line.** The cut line differs by surface because it names `--limit` or the tool and parameters, which the amended Rule 1 allows. The no-match line is identical. The equality fixture stays under the cap and must include a no-match case, or the "identical" half is unproven. No contradiction.
2. **Rule 1 vs limit.** Both defaults are 500, so `limit` is equal. CLI `--limit 0` has no MCP counterpart, so that case is outside the equality test. No contradiction.
3. **Rule 4.** The text, amounts, category and account values appear only in client lines. Every new class line (rows 2, 3, 13) carries none of them. Payee and memo text appear only in the document, and success writes nothing to stderr. CLI stderr quotes caller values exactly as the other commands' refusals do. No contradiction.
4. **Rule 11 vs Rule S7: contradiction, resolved.** Rule 11 says "cut after the document is built". Search cuts in the store with a full count instead. It is the same first N in document order, and `matched` is uncut. Record it as a deliberate deviation. `capList` is not used.
5. **Rule 3 / config: contradiction with precedent, resolved.** 3b's config-first order does not apply, because search has no currency (Rule S8). There is no `configRefusalLog` twin.
6. **Rule 12.** There is no clock read. Search has no "today", so the future-since refusal is gone.
7. **Q4.** `transfer` and `excluded` come from shared fragments. A row that re-derives either one is a defect. The absolute-amount filter is a search predicate in core (report validates, duckstore applies), not a front-end rule.
8. **Q5.** min/max are exact decimal strings parsed to cents. A JSON number would pass through a float64 in `applySchema` (go-sdk `tool.go:94-139`), so the schema type is string.
9. **Q7 and Q8.** MCP is bounded at 500. The description tells Claude not to sum rows for totals.
10. **Rule 7.** Every refusal is one line, with no `quarry: ` on MCP.

#### 5. Tests that can go red (retro list)
- **Defaults:**
  - CLI limit 500 when `--limit` is not given: 501 matches give 500 rows.
  - MCP limit 500 when absent.
  - absent/null/`{}` arguments reach the handler, which proves the schema default.
  - absent since lists a transaction dated before Jan 1 this year.
  - absent until lists a future-dated transaction.
  - the order is date DESC and the tie-break is source_id DESC: a same-date pair in a fixture whose insertion order differs.
  - a cut drops the oldest.
- **Arms:**
  - `--limit 0` lists every match.
  - `matched` counts every match when cut, and `truncated` is true or false accordingly.
  - a negative transaction is found by `--min`, which proves abs.
  - min == max matches inclusively.
  - `%`, `_` and `\` are literal: a fixture payee contains them, and the text `5%` does not match `50`.
  - non-ASCII case folding: `CAFÉ` finds `café`. Not verified in DuckDB; the architect pins it.
  - a match only in a split memo is listed, and that memo is visible in the text Memo cell and in `splits[].memo`.
  - a match in the transaction memo.
  - the payee is matched; an account name is not.
  - the category arm matches the category itself and a child, and does not match a sibling prefix (`Foo` vs `Food`).
- **Shared predicates:** one mutation per fragment (transfer leg, `reportedAccount`, `excluded_from_reports`). Each turns `v_cash_flow` and search red together.
- **Passthroughs:** `windowRefusal` on the search tool, `accountRefusal` at the Search error site, the new category refusal wrapper, the amount refusal wrapper, and `refusalLine`'s new category class. Each wrapper's identity mutant (`return err`) must fail.
- **Refusal order:** bad min plus bad since gives the min line, and a bad account plus a bad category gives the account line.
- **Config never read:** a broken config gives a normal result on both surfaces.
- **Equality:** the CLI `--json` and MCP documents are byte-equal, for one fixture with matches and one with none.
- **Pins:** the root help row, the renamed descriptions pin, instructions, the query description, the mcp Long, and the five all-tools tables (no-store, timeout, store-fault, tools/list, account refusal) each gain a `search_transactions` row. `read_faults_test.go` gains a `Search` readOp.
- **No ordering pin needed:** the cut and no-match warnings exclude each other (§2.6).

#### Costs
- **New contract:**
  - one command and seven flags
  - one tool with eight parameters
  - a document with 12 keys, a 10-key row and a 4-key split
  - three new MCP class lines
  - seven new refusal lines
  - one new `RefusalError` kind
  - one amount parser
- **Breaks:** no CLI break. Pins change: root help, the descriptions test (renamed), instructions, the query description, the mcp Long, the all-tools tables.
- **No new exit codes.**

#### Verdict: SHIP WITH CHANGES
Ranked:
1. **Default window = all dates (Rule S6) [INTERPRETATION c].** Defaulting to this year silently misses older matches, which looks the same as "never happened". Add `report.ParseSearchWindow` returning the existing `WindowError`. `since`/`until` echo `null` when open.
2. **Derive `transfer`/`excluded` from `v_cash_flow`'s own fragments (Q4).** Extract the transfer-leg `NOT EXISTS` into a shared fragment. `excluded` includes left-out accounts [INTERPRETATION b], and the copy names system categories as unflagged.
3. **Row carries `splits[{category,memo,amount,transfer}]` [INTERPRETATION a].** A split-memo match must be visible on the row, in text and in `--json`.
4. **MCP `min`/`max` are strings [INTERPRETATION d].** go-sdk `applySchema` turns numbers into float64 (Q5). One parser and one refusal line per surface. No schema `minimum`.
5. **Limit in the store, with a full count:** `limit`, `matched`, `truncated`. Newest first, ties source_id DESC. A deliberate Rule 11 deviation, recorded.
6. **Refuse an unknown category (and blank text); never return an empty result for them.** An empty search exits 0 with the no-match warning. The "not found" exit code is for named resources.
7. **Search reads no config and no clock.** That removes two outcome rows on each surface.
8. **Apply the §2.8 copy changes.** That includes closing the "David's" debt, renaming the descriptions pin and repointing the phase3b spec line, and the PRD CLI row plus Decisions line.
9. **Implementation note for the architect:** use `contains(lower(..), lower($n))`, not LIKE, so wildcards and escaping cannot exist.

Key files:
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/store/duckstore/schema.go (`reportedAccount`; the `v_cash_flow` NOT EXISTS to extract)
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/window.go (`WindowError`, `dateForms`)
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/mcp/tools.go (instructions, `queryDescription`, `limitSchema`)
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/sql.go (the `--limit` precedent)
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/mcp.go (Long tool list)
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/cmd/quarry/run_status_test.go:110-136 (root help pin)
- /Users/koblas/repos/github.com/koblas/quarry/.devenv/state/go/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/tool.go:94-139 (where arguments go through a map and floats)


### Mid-feature copy ruling (SCENARIO-04): invalid UTF-8 text or category

CLI refuses both as a usage error (exit 2) before the store opens; value quoted with Go `%q`; `--json` prints nothing on stdout, same line and exit.

| # | Outcome | stdout | stderr | exit |
|---|---|---|---|---|
| 3a | text not valid UTF-8 (`$'\xff'`, `$'caf\xc3'`) | — | `quarry: search text "\xff" is not valid UTF-8; set your terminal or script to UTF-8` | 2 |
| 3b | `--category` not valid UTF-8 | — | `quarry: --category "\xff" is not valid UTF-8; set your terminal or script to UTF-8` | 2 |
| 3c | text containing NUL (valid UTF-8) | caption, header, `0 matching transactions` | `quarry: warning: no transactions match the search; …` | 0 |

MCP: no row and no class line — go-sdk v1.8.0 decodes arguments with segmentio/encoding/json, which replaces invalid UTF-8 with U+FFFD, so the handler always sees valid UTF-8 (§4.2 row 20: falls through to row 17 no-match or row 13 unknown category). Any MCP-reachable check is `// unreachable:` with that reason; no MCP wrapper. `--account`, `since`/`until`, `min`/`max` are parsed in Go and unaffected. No new `RefusalKind` enters `mcp.refusalLine`.

---

## Scenarios (Gherkin)

Approved by the user 2026-10-03. Sizing pass reworded S06/S09/S11 as outlines, split S12 by surface (S12a CLI, S12b MCP) and reduced S13's When to one action (root help row moved into S01's build, `quarry mcp` Long and PRD text into S13's build/sweep). Listed in build order.

```gherkin
Scenario: SCENARIO-01 — quarry search lists matching transactions newest first, flagged, with their splits
  Given a store with a transfer, a report-excluded transaction, a transaction in a left-out account and a split transaction
  When the user runs quarry search --json
  Then the document lists every transaction newest first per §2.4, each with transfer and excluded flags and its splits

Scenario: SCENARIO-05 — without --since and --until every date is searched
  Given transactions dated before January 1 this year and one dated in the future
  When the user runs quarry search --json with no dates
  Then both are listed, and since and until echo null

Scenario Outline: SCENARIO-02 — text matches payee, transaction memo and split memo, ignoring case, every character literal
  Given a store with <fixture>
  When the user runs quarry search <text>
  Then <outcome>

  Examples:
    | fixture                         | text   | outcome |
    | payee "Costco"                  | costco | listed |
    | memo "birthday gift"            | GIFT   | listed |
    | only a split memo "tip"         | tip    | listed, the split memo shown in the Memo cell and splits[].memo |
    | payee "CAFÉ"                    | café   | listed |
    | payee "50 off"                  | 5%     | not listed |
    | payee "a_b"                     | a_b    | listed; "axb" not listed |
    | memo with a backslash           | \      | listed |
    | account named "Costco Visa"     | Visa   | not listed by its account name |

Scenario Outline: SCENARIO-06 — more matches than the limit print the newest ones
  Given 501 matching transactions
  When the user runs quarry search <limit args>
  Then <rows> rows print, matched is 501, and <warning>

  Examples:
    | limit args | rows | warning |
    | none       | 500  | stderr carries the CLI cut warning |
    | --limit 0  | 501  | no cut warning |

Scenario: SCENARIO-07 — no match prints an empty result and the no-match warning
  Given a built store
  When the user runs quarry search with text that matches nothing
  Then exit is 0, transactions is [] and matched is 0, and stderr carries the ruled no-match warning

Scenario: SCENARIO-12a — a broken config file never affects quarry search
  Given an unparseable config file
  When the user runs quarry search
  Then it prints a normal result with no config warning

Scenario Outline: SCENARIO-03 — --min and --max compare the amount without its sign
  Given a store with charges and deposits of several amounts
  When the user runs quarry search <bounds>
  Then <outcome>

  Examples:
    | bounds                    | outcome |
    | --min 100                 | a -150.00 charge and a 120.00 deposit are listed; a -99.99 charge is not |
    | --max 20                  | a -20.00 charge is listed; a -20.01 charge is not |
    | --min 20 --max 50         | only amounts from 20.00 to 50.00 without sign |
    | --min 42.17 --max 42.17   | only the 42.17 transactions |

Scenario Outline: SCENARIO-04 — --category matches the category and any category under it
  Given categories Food, Food:Groceries, Food:Groceries:Organic, Foo and a hidden category
  When the user runs quarry search --category <arg>
  Then <outcome>

  Examples:
    | arg            | outcome |
    | Food           | Food, Food:Groceries and Food:Groceries:Organic splits listed; Foo not |
    | food:groceries | Food:Groceries and Food:Groceries:Organic listed |
    | Foo            | only Foo listed |
    | the hidden one | its transactions listed |
    | a known category with no transactions | empty result with the no-match warning, exit 0 |

Scenario Outline: SCENARIO-08 — quarry search refuses bad input with the ruled line and exit code
  Given a built store
  When the user runs quarry search <args>
  Then stderr is <line> and exit is <code>

  Examples:
    | args                       | line | code |
    | two texts                  | quarry: search takes one text; quote it as one argument | 2 |
    | --limit -1                 | quarry: --limit must be 0 or more; 0 prints every transaction | 2 |
    | blank text                 | quarry: search text is blank; leave it out to search by date, account, category or amount alone | 2 |
    | --min "-12"                | quarry: --min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as 25 or 19.99 | 2 |
    | --min 50 --max 20          | quarry: --min 50 is more than --max 20 | 2 |
    | --since 2024-13            | the existing not-a-date line | 2 |
    | --since 2025 --until 2024  | the existing since-after-until line | 2 |
    | --account Nope             | the existing unknown-account line | 1 |
    | --account Visa (two)       | the existing ambiguous-account line | 1 |
    | --category Fod             | quarry: no category named "Fod"; list them with quarry sql "SELECT full_path FROM categories ORDER BY full_path" | 1 |
    | --category ""              | quarry: no category named ""; list them with quarry sql "SELECT full_path FROM categories ORDER BY full_path" | 1 |

Scenario Outline: SCENARIO-09 — search_transactions returns the quarry search --json document
  Given a built store, <fixture>, and the same text, since, until, accounts, category, min, max and limit
  When the client calls search_transactions
  Then the structured result is byte-equal to quarry search --json

  Examples:
    | fixture        |
    | with matches   |
    | with no match  |

Scenario Outline: SCENARIO-11 — search_transactions cuts to its limit with the MCP cut line
  Given more than 500 matching transactions
  When the client calls search_transactions with limit <limit>
  Then the newest <n> are returned, matched counts all, and the warning is <line>

  Examples:
    | limit  | n   | line |
    | 20     | 20  | search_transactions lists the newest 20 of <m> matching transactions; pass a higher limit, up to 500, or narrow the search with text, since, until, accounts, category, min or max |
    | absent | 500 | search_transactions lists the newest 500 of <m> matching transactions; narrow the search with text, since, until, accounts, category, min or max |

Scenario: SCENARIO-12b — a broken config file never affects search_transactions
  Given an unparseable config file
  When the client calls search_transactions
  Then it returns a normal result with no config warning

Scenario Outline: SCENARIO-10 — search_transactions refuses bad input without the caller's values on stderr
  Given a built store
  When the client calls search_transactions with <args>
  Then the result is isError with <line> and stderr carries only "quarry: mcp: search_transactions: <class>"

  Examples:
    | args                 | line | class |
    | text "  "            | text is blank; leave it out to search by date, account, category or amount alone | refused the call's text; details went to the client only |
    | min "-12"            | min "-12" is not an amount; use digits with up to 2 decimals and no sign, such as "25" or "19.99" | refused the call's min or max; details went to the client only |
    | min "50", max "20"   | min 50 is more than max 20 | refused the call's min or max; details went to the client only |
    | since "2024-13"      | the existing MCP not-a-date line | refused the call's since or until; details went to the client only |
    | accounts ["Nope"]    | the existing MCP unknown-account line | refused the call's accounts: one names no account; details went to the client only |
    | category "Fod"       | no category named "Fod"; call describe_schema to list the categories | refused the call's category: it names no category; details went to the client only |
    | min as a JSON number | SDK text | refused the call's arguments; details went to the client only |
    | limit 501            | SDK text | refused the call's arguments; details went to the client only |

Scenario: SCENARIO-13 — the server names search to the client
  Given an MCP client has started quarry mcp
  When the client initializes and lists tools
  Then it sees the ruled instructions, the ruled query description, and nine tools including search_transactions with its ruled description and per-parameter descriptions
```

---

## Sizing

Sizing pass (architect, opus) 2026-10-03 — 15 IDs → 6 runs, all code-first. Order binding: S01 → S02 → S03 → S04 → S09 → S10. S09's Handoff lists the MCP refusal wrappers under Left unbuilt; S10 runs immediately after. Full report in `## Sizing notes`.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (opus) — 4 batches: shared v_cash_flow fragments + store Search read (flags, splits, window, account filter, order, limit + count, span) + read-fault row; ParseSearchWindow + Server.Search + document.NewSearch; cli `search` + --json + --since/--until/--account + root help row; text renderer. Absorbs 05 |
| SCENARIO-05 | FOLD into SCENARIO-01 |
| SCENARIO-02 | OWNS A RUN (sonnet) — 3 batches: text predicate + positional text + two-texts/blank refusals; no-match SearchWarnings; --limit + CLI cut line + broken-config pin. Absorbs 06, 07, 12a |
| SCENARIO-06 | FOLD into SCENARIO-02 |
| SCENARIO-07 | FOLD into SCENARIO-02 |
| SCENARIO-12a | FOLD into SCENARIO-02 |
| SCENARIO-03 | OWNS A RUN (sonnet) — 3 batches: amount parser + amount error with parts; abs predicate + min>max; flags + caption range arms |
| SCENARIO-04 | OWNS A RUN (sonnet) — 3 batches: category resolution (RefusalUnknownCategory) + predicate; --category flag + caption + echo; S08 refusal matrix + Rule S9 order pins. Absorbs 08 |
| SCENARIO-08 | FOLD into SCENARIO-04 |
| SCENARIO-09 | OWNS A RUN (opus) — 3 batches: MCP handler + tool registration + equality acceptance; all-tools table rows + descriptions-pin rename (Test_run_mcp_describes_every_tool, repoint phase3b spec line); MCP cut lines + broken-config pin. Absorbs 11, 12b |
| SCENARIO-11 | FOLD into SCENARIO-09 |
| SCENARIO-12b | FOLD into SCENARIO-09 |
| SCENARIO-10 | OWNS A RUN (sonnet) — 3 batches: MCP refusal wrappers + 3 refusalLine classes; refusal acceptance matrix; S13 copy (instructions, query description, mcp Long, descriptions pin, PRD row + Decisions line). Absorbs 13 |
| SCENARIO-13 | FOLD into SCENARIO-10 |

## BDD Acceptance Progress
- [x] SCENARIO-01: quarry search lists matching transactions newest first, flagged, with their splits — `cmd/quarry/run_search_json_test.go` `Test_run_search_json_lists_every_transaction_newest_first_flagged_with_its_splits`
- [x] SCENARIO-05: without --since and --until every date is searched — delivered by SCENARIO-01 — `cmd/quarry/run_search_window_test.go` `Test_run_search_without_since_or_until_searches_every_date`
- [x] SCENARIO-02: text matches payee, transaction memo and split memo, ignoring case, every character literal — `cmd/quarry/run_search_text_test.go` `Test_run_search_text_lists_payee_memo_and_split_memo_matches_ignoring_case_with_every_character_literal`
- [x] SCENARIO-06: more matches than the limit print the newest ones — delivered by SCENARIO-02 — `cmd/quarry/run_search_limit_test.go` `Test_run_search_prints_the_newest_500_of_501_matches_unless_limit_0`
- [x] SCENARIO-07: no match prints an empty result and the no-match warning — delivered by SCENARIO-02 — `cmd/quarry/run_search_no_match_test.go` `Test_run_search_with_text_that_matches_nothing_prints_an_empty_result_and_the_no_match_warning`
- [x] SCENARIO-12a: a broken config file never affects quarry search — delivered by SCENARIO-02 — `cmd/quarry/run_config_test.go` `Test_run_search_ignores_a_malformed_config`
- [x] SCENARIO-03: --min and --max compare the amount without its sign — `cmd/quarry/run_search_amount_test.go` `Test_run_search_min_and_max_compare_the_amount_without_its_sign`
- [ ] SCENARIO-04: --category matches the category and any category under it
- [ ] SCENARIO-08: quarry search refuses bad input with the ruled line and exit code
- [ ] SCENARIO-09: search_transactions returns the quarry search --json document
- [ ] SCENARIO-11: search_transactions cuts to its limit with the MCP cut line
- [ ] SCENARIO-12b: a broken config file never affects search_transactions
- [ ] SCENARIO-10: search_transactions refuses bad input without the caller's values on stderr
- [ ] SCENARIO-13: the server names search to the client

---

## Sizing notes

#### Sizing pass: phase3c-search

I wrote nothing to the repo. The 13 approved IDs become 14 once S12 is split, and they come out as **6 architect runs**. Every run is **code-first**: nothing touches the mandatory test-first set, because there is no bug fix, no write-safety guard and no atomic or exclusive-create adapter. The `v_cash_flow` fragment extraction is a change to a read-only view, so it is not on that set. Its three mutants still go on S01's `Mutation checks:` line. `schema_read.go:19` exposes only `view_name`, so the DDL edit cannot change `describe_schema` bytes. The existing `internal/store/duckstore/views_test.go` and `views_fx_test.go` must stay green unchanged.

Feature packages: `report` (with its `report/document` subpackage) plus `report.Store`'s adapter `duckstore`. `cli`, `mcp` and `cmd/quarry` are delivery and wiring. No unit crosses two feature packages, so no SPLIT is mandatory.

p90 is about 2,785k IE over 72 past units; I took that from the 3b sizing notes and did not recompute it. The twins are all under it: S01 vs 3b S01 1,819k and 3a S01 1,585k; S09 vs 3b S10 1,701k and S09 1,658k.

##### Sizing table (build order is binding)
| # | Scenario | Verdict | Size and batches |
|---|---|---|---|
| 1 | **S01** + S05 FOLD | OWNS A RUN, opus | **4 batches**. Runs A \| B1(3) \| B2(1) \| V.<br>(1) store: shared fragments extracted from `cashFlowViewDDL` (`schema.go:~176-208`); `store.SearchParams`/result; `report.Store.Search`; `duckstore.Search` (transfer/excluded flags, splits in split `source_id` order, nullable window, account filter, `date DESC, source_id DESC`, limit with `count(*) OVER ()`, transaction span for the no-match warning); a `read_faults_test.go` readOp.<br>(2) `report.ParseSearchWindow` (no clock) + `Server.Search` (`namedAccounts` → read → `readRefusal`) + `document.NewSearch`.<br>(3) cli `search` command, `--json`, flags `--since`/`--until`/`--account`, `AddCommand`, root help row at `run_status_test.go:122-135` plus the `newRootCommand` doc comment. Moved here from S13 because the exact-bytes pin goes red the moment the command is registered.<br>(4) text renderer: caption base, the four date arms and `accountsCaption`; columns; footer; `escapeCell`. |
| — | S05 | FOLD into S01 | `ParseSearchWindow` and the open-window predicate are S01's code; S05 only adds pins (a transaction before Jan 1, a future-dated one, `since`/`until` null). |
| 2 | **S02** + S06 FOLD + S07 FOLD + S12a FOLD | OWNS A RUN, sonnet | **3 batches**. Runs A \| B1(3) \| V.<br>(1) text predicate in the store (`contains(lower,lower)` over payee, memo and an EXISTS on split memo), positional `[text]`, the two-texts refusal, blank-text refusal (report-level error with parts so MCP can word it), caption ` matching %q`, split memo visible in the Memo cell.<br>(2) S07: `document.SearchWarnings` no-match composer, four variants, from S01's span; CLI stderr.<br>(3) S06: `--limit` (the `sql` Changed trick, `defaultSearchLimit=500`, negative refused), CLI cut line, `truncated`; S12a broken-config pin modelled on `Test_run_sql_ignores_a_malformed_config` (`run_config_test.go:299`). |
| — | S06 | FOLD into S02 | Store limit/count already exist from S01; what is left is one flag plus one warning line. |
| — | S07 | FOLD into S02 | Only a handful of lines, because the span is built in S01. Must land before S04, whose "known category, no rows" example needs this warning. |
| — | S12a (CLI) | FOLD into S02 | A pure pin: `newReportFactory` never loads config (`cmd/quarry/run.go:92-101`). |
| 3 | **S03** | OWNS A RUN, sonnet | **3 batches**.<br>(1) amount parser for the §2.3 grammar, every refused class as its own row, 16 vs 17 integer digits, normalization (`12.5`→`"12.50"`), and an amount error with parts (`Bound`, `Value`, `Other`, kind not-an-amount / min-above-max) whose `Error()` is the CLI line.<br>(2) `abs(t.amount)` inclusive predicate + `Server.Search` min>max check + JSON echo.<br>(3) `--min`/`--max` flags + the four caption range arms (`exactly` when min == max). |
| 4 | **S04** + S08 FOLD | OWNS A RUN, sonnet | **3 batches**.<br>(1) category resolution (`RefusalUnknownCategory` + `Arg`, CLI line, exit 1) + EXISTS predicate: `full_path` equal ignoring case, or prefix + `:`; hidden categories count.<br>(2) `--category` flag, caption arm, JSON echo.<br>(3) S08 CLI refusal matrix (all §4.1 refusal rows, `--json` variants) + Rule S9 order pins. |
| — | S08 | FOLD into S04 | Pure coverage of refusals that S01-S04 build. Its acceptance test becomes S04's folded acceptance line. |
| 5 | **S09** + S11 FOLD + S12b FOLD | OWNS A RUN, opus | **3 batches**. Runs A \| B1(3) \| V.<br>(1) handler (`ParseSearchWindow`→`windowRefusal`, amounts, `newReport`, `Search`→`accountRefusal`, `NewSearch`, limit passthrough, no `resolveCurrency`) + `toolSearch`, description, 8 parameter descriptions, schema with limit `described(limitSchema(maxRows))`, registration; equality-harness acceptance (match + no-match).<br>(2) all-tools rows: no-store `run_mcp_no_store_test.go:31`, store-fault `run_mcp_store_faults_test.go:72,100`, timeout `internal/mcp/timeout_test.go:114` (+ `stallingStore.Search` override), account refusal (`refuseAccountKeepingItsNameOffStderr`), the `absentNullArguments` row, descriptions pin row 9 (see the rename under Seams).<br>(3) S11 MCP cut lines (limit 500 and limit below 500) + S12b broken-config pin. |
| — | S11 | FOLD into S09 | Limit is passed through in S09; S11 adds one warning with two variants. `capList` is not used. |
| — | S12b (MCP) | FOLD into S09 | A pin that the handler never calls `resolveCurrency`. |
| 6 | **S10** + S13 FOLD | OWNS A RUN, sonnet | **3 batches**.<br>(1) MCP wording wrappers for blank text, amount (not-an-amount and min>max) and unknown category; three new class lines in `refusalLine`/`logLine` (`internal/mcp/result.go:59-71`).<br>(2) refusal acceptance matrix (§4.2 rows 1-13, including min as a JSON number and limit 501), with values asserted absent from stderr.<br>(3) S13 copy: `instructions` (closes the "David's" debt), `queryDescription`, `quarry mcp` Long (`internal/cli/mcp.go:24-36`), each byte copy in the descriptions pin, PRD CLI row + Decisions line (Sweep). |
| — | S13 | FOLD into S10 | Three const replacements plus their pins and PRD text. The root help row moved to S01 and the test rename moved to S09. |

**Totals:** 6 runs, 19 batches, 14 IDs.

##### When rewordings (needed before `spec-check.py`)
- **S06** has a second action in its Then. Make it an outline over `<limit args>` with Examples `none` (500 rows, warning) and `--limit 0` (501 rows, no warning).
- **S09** becomes an outline with Examples `matching` and `no match`.
- **S11** becomes an outline over `<limit>` with Examples `20` and `absent` (over 500 matches).
- **S12** has two surfaces in one When, so **SPLIT**:
  - S12a: "When the user runs quarry search" → FOLD into S02.
  - S12b: "When the client calls search_transactions" → FOLD into S09.
- **S13** has three actions. Reduce it to "When a client connects and lists the tools", with Then: instructions, query description, nine tools with search_transactions' description and schema.
  - The `quarry mcp` Long becomes a Build pin; its byte copy already lives in the descriptions test (3b STATE).
  - The root help row moves to S01.
  - The PRD becomes a Sweep doc step.
- S02, S03, S04, S08 and S10 are outlines with one When each and are fine. S01, S05 and S07 are fine.

##### Unowned §4.1 / §4.2 rows, now assigned as Build rows
No scenario's Examples name these:
- **S01:**
  - `--account` happy path and `accountsCaption`
  - given-bound `--since`/`--until` and the four caption date arms
  - store faults (row 9: the existing `storeRefusal` lines through `readRefusal`)
  - `search interrupted` (row 12)
  - closed / left-out-or-linked / USD edge rows (19-21), each crossed with text and `--json`
  - `transfer, excluded` both set on one row
  - `splits: []` for a transaction with no splits
  - null payee, and null memo for both NULL and `""`
- **S04 (S08 matrix):**
  - unknown flags `--currency`/`--csv` (row 4)
  - `--json` plus any refusal prints nothing on stdout (row 24)
  - `--account ""` and `--category ""`
- **S09:** §4.2 rows 6-10 (store faults), 14 (timeout) and 15 (cancel) through the all-tools tables.

##### Seams
1. **Store read (S01 B1).** One port call, `Search(ctx, SearchParams)`. The result carries rows, splits, `matched` and the transaction span, fixed in S01.
   - Later scenarios add only WHERE predicates, never statements. A new statement shifts which query the read-fault row fails (the 3a `passQueries` trap).
   - Category resolution (S04) must not add a statement either. Otherwise S04 re-points `read_faults_test.go`, so that plan must say which query each fault row hits.
2. **Shared SQL fragments.**
   - The transfer-leg `NOT EXISTS`, parameterised by split alias, is used per split and per transaction (EXISTS over its splits).
   - `excluded` = `NOT (reportedAccount AND NOT t.excluded_from_reports)`, reusing the `reportedAccount` const (`schema.go:~167`).
   - `v_cash_flow` must keep the same semantics. A row that re-derives either flag is a defect (§4.3 item 7).
3. **`report.Server.Search`.** The Rule S9 order is built in place: blank text → min → max → min>max → window → open → account → category → read. Amount parser and amount error, `RefusalUnknownCategory`, `ParseSearchWindow` (no clock; returns the existing `WindowError`; only `WindowNotADate` and `WindowSinceAfterUntil` are reachable).
4. **`document.NewSearch` + `SearchWarnings` (no-match only).** The cut line is per surface: CLI in `internal/cli`, MCP in the handler, after the document is built, as the only warning.
5. **CLI:** `internal/cli/search.go` + the render file; the anomalies shape plus the `sql --limit` precedent.
6. **MCP:** the `internal/mcp/search.go` handler; refusal wrappers come in S10.
7. **Equality harness** `cmd/quarry/run_mcp_documents_helpers_test.go`, reused unchanged. Fixtures stay under 500 rows. Warnings are stripped; the no-match line must be byte-identical on both surfaces.
8. **Descriptions pin.** `cmd/quarry/run_mcp_descriptions_test.go:202` `require.Len(listed.Tools, len(wantTools))` goes red at S09's registration.
   - So **S09** renames it to `Test_run_mcp_describes_every_tool` and repoints `docs/specifications/phase3b-analysis-tools/specification.md:747`.
   - S13's folded acceptance line then names the renamed test.

##### Can-go-red pins per run (retro list, for each plan's `Mutation checks:`)
- **S01:**
  - one mutant per fragment (transfer leg, `reportedAccount`, `excluded_from_reports`), each red in `v_cash_flow` tests and in search
  - tie-break `source_id DESC`: a same-date pair whose insertion order differs
  - date DESC
  - `matched` uncut while `truncated` is true (at least a store-level limit test)
  - an absent since lists a pre-Jan-1 row; an absent until lists a future row
  - split order
  - each `--account` arm
- **S02:**
  - an absent `--limit` gives 500 rows from 501 (default by omission)
  - `--limit 0` gives every row
  - `--limit -1` is refused
  - `%`, `_` and `\` are literal (`5%` does not match `50`)
  - `CAFÉ` finds `café`
  - a split-memo-only match is visible in both the text Memo cell and `splits[].memo`
  - an account name is not matched
  - each of the three no-match variants (all / empty store / named accounts, with span and without)
  - broken config: normal output, no warning
- **S03:**
  - a negative amount is found by `--min` (proves `abs`)
  - min == max, and each bound tested at the value and one cent outside
  - the 16 vs 17 digit bound
  - normalization echo
  - each refused class in §2.3
- **S04:**
  - category exact match, a child, a grandchild (depth ≥ 2), other letter case, sibling prefix `Foo`/`Food` not matched, a hidden category
  - unknown is refused but known-and-empty is not
  - Rule S9 order: bad min + bad since gives the min line; bad account + bad category gives the account line; argument count before `--limit` before blank text
- **S09:**
  - absent / null / `{}` arguments reach the handler (proves the schema default of 500)
  - the `windowRefusal` passthrough on the search tool (identity mutant)
  - `accountRefusal` at the `Search` error site
  - the stallingStore `Search` deadline
  - each cut-line variant (limit 500 and limit 20)
  - broken config gives no warning
  - equality on the no-match fixture
- **S10:**
  - each wrapper's identity mutant (`return err`): blank text, amount, category
  - each new `refusalLine` class
  - client line carries the value, stderr does not (Rule 4)

##### Risks: pins to prove, not facts
- **Non-ASCII case folding.** Whether DuckDB `lower()` folds `CAFÉ` is unverified. If it does not, the S02 architect needs a product-vision copy ruling before building.
- **`--limit 0`.** It means every row, so it must never reach SQL as `LIMIT 0`. Whether `LIMIT NULL` works or the clause must be omitted is unverified in DuckDB.
- **`count(*) OVER ()`.** It must be evaluated before the LIMIT; pin it with 501 rows.
- **Splits for limited rows only.** Fetching splits for just the returned rows (aggregated, or a second statement) is S01's design choice, and the statement count is fixed there (Seam 1).
- **S09 → S10 window.** Between them, MCP amount, category and blank-text errors reach stderr verbatim with caller values (a Rule 4 breach in the intermediate state). S09's Handoff must list the wrappers under Left unbuilt, and S10 must run immediately after S09.
- **Panicking red tests.** A red acceptance test that dereferences a zero stub document panics the whole `cmd/quarry` binary. Use `require` on the error or nil first.
- **`readCommandArgs`.** Do not add search to `cmd/quarry/run_config_test.go:218-226` (the "read commands refuse a malformed config" table). Search ignores config; its pin follows `Test_run_sql_ignores_a_malformed_config`.
- **Unit-test fakes.** `fakeStore` and the other fakes embed a nil `report.Store`. A unit test that reaches `Search` or `namedAccounts` panics, so account and category rows belong in tests at the `cmd/quarry` level.
- **Rule 11 deviation.** The store-side limit is deliberate and goes in the spec; `capList` stays unused for search.
- **Unverified items.** I did not run `feature-metrics.py` or check `limitSchema`'s minimum of 1; S09 must confirm that MCP can never send 0.

Key files:
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/store/duckstore/schema.go
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/store.go
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/cmd/quarry/run_mcp_descriptions_test.go
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/cmd/quarry/run_status_test.go
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/cmd/quarry/run_config_test.go
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/docs/specifications/phase3b-analysis-tools/specification.md
