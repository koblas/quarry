# Specification: Phase 3b — MCP analysis tools

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: Claude can ask quarry's own transfer-, refund- and currency-aware analysis questions over MCP instead of re-deriving the rules in SQL. Four new tools — `spending`, `cash_flow`, `recurring_charges`, `anomalies` — return the same documents `quarry spend|cashflow|recurring|anomalies --json` print, built by the same constructors, now shared in `internal/report/document`.

**Secondary Goals**: no caller, config or data text on stderr for any of the eight tools (account refusals, window refusals and DuckDB read faults now log a withheld class line); every named-tool list bounded at 500 with a warning; per-parameter descriptions on all eight tools; instructions and the `query` description send spending/income questions to the named tools.

**Out of Scope**:
- `search_transactions` and a `quarry search` CLI twin — Phase 3c (user decision 2026-10-03).
- Skill, references, plugin manifest, use-case eval — Phase 3d.
- `net_worth`, `acb` — Phase 4.
- Any change to CLI stdout/stderr bytes.
- Redaction: unchanged (PRD §Security "Account numbers in configuration"; free text passes through).

**Business Rules**: below; literal surface in `## Surface & Copy` (product-vision Phase 1, binding).

## Business Rules & Invariants
- Rule 1 (amended for these four tools, replaces phase3a Rule 1 for them): each tool's structured result is the CLI `--json` document for the same inputs, built by the same constructor in `internal/report/document`. Every field other than `warnings` is byte-identical to the CLI's, except a list cut by Rule 11. In `warnings`, a line differs only where it names a CLI command word (`spend`, `cashflow`, `recurring`, `anomalies`), a flag (`--since`, `--until`, `--account`) or a command the client cannot use (`quarry accounts --all`); those use the tool name, its parameter names, or the tool that does the job (§4). "run quarry sync" is never reworded.
- Rule 2: `internal/mcp` stays a delivery peer of `internal/cli`; document imports report/store/finding/money/platform only.
- Rule 3: store opened per call, config read per call — and config is read only when `currency` is absent (CLI parity).
- Rule 4 (unchanged; new classifications): stderr never carries caller, config or data text. Window refusals → `refused the call's since or until; details went to the client only`; unknown account → `refused the call's accounts: one names no account; details went to the client only`; ambiguous account → `refused the call's accounts: one names more than one account; details went to the client only`; `OpenFaultOther` (open-time or statement-time, all eight tools) → `cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb; details went to the client only`; config refusal → `cannot read quarry's config file; run quarry <twin> to see why`. This overturns phase3a §2.1a's "OpenFaultOther … stays verbatim".
- Rule 5: 30 s per-call deadline (unchanged); timeout line `<tool> stopped after 30 seconds; try again`.
- Rule 10 (unchanged): hand-written input schemas with `additionalProperties:false`; `currency` enum `CAD|USD|native` exact-case with **no schema default**; `by` enums from the moved word tables with defaults (`category`, `month`).
- Rule 11 (new): every named-tool list (`spending.rows`, `cash_flow.periods`, `recurring_charges.series`, `anomalies.anomalies`) stops at 500 entries, cut after the document is built, in document order; totals, `checked`, `not_judged` always count the whole result; the cut is reported only by the §5 warning, last.
- Rule 12 (new): "today" is the server's local date read once at the start of each call (default window, charge-window check, recurring/anomalies `Now`).

---

## Triage Brief

#### Request
Scope Phase 3b: add MCP tools spending, cash_flow, recurring_charges, anomalies, search_transactions on top of merged 3a. (Sweeps done by me with multi-pattern grep, no investigator, no LSP daemon used: every caller row below is tagged `grep`. Positive control: the grep found `Server.Spend` callers matching the 3a triage's LSP table.) No spec under docs/specifications covers 3b; 3a spec lists it Out of Scope (`phase3a-mcp-core/specification.md:12`).

#### Affected surface (per tool)

| Tool | Library entry | Request (fields) | CLI flags | --json owner (all in cli, unexported) |
|---|---|---|---|---|
| spending | `report.Server.Spend` internal/report/spending.go:62 | `SpendRequest{Window, By store.SpendingGroup, Accounts []string, Currency money.Currency}` spending.go:14 | `--by` (category/payee/tag/month), `--since --until --account`, `--currency` (internal/cli/spend.go:66-84) | `spendDocument` + 4 row types internal/cli/json_spend.go:9-111, builder `renderSpendingJSON` |
| cash_flow | `Server.CashFlow` cashflow.go:58 | `CashFlowRequest{Window, By store.CashFlowPeriod, Accounts, Currency}` cashflow.go:12 | `--by` month/year + same | `cashFlowDocument`, `renderCashFlowJSON` json_cashflow.go:6-69 |
| recurring_charges | `Server.Recurring` recurring.go:199 | `RecurringRequest{Window, Now time.Time, Accounts, Currency}` recurring.go:119 | `--since --until --account --currency` (no --by) cli/recurring.go:25-78 | `recurringDocument`, `renderRecurringJSON`, `recurringSeriesOf` json_recurring.go:7-124 |
| anomalies | `Server.Anomalies` anomalies.go:90 | `AnomaliesRequest{Window, Now, Accounts, Currency}` anomalies.go:44 | same four | `anomaliesDocument`, `renderAnomaliesJSON`, `anomalyEntry` json_anomalies.go:7-92 |
| search_transactions | NONE | n/a | no CLI twin | n/a |

Note `Now` is a request field only for recurring and anomalies ("charges dated after today are left out, even with a later until", cli/recurring.go Long). `internal/mcp` has no clock at all (no `time.Now`/option in internal/mcp; cli gets one via `Env.Now`, cmd/quarry/run.go:192). Spend/CashFlow derive "today" only through `report.ParseWindow(…, now)`.

##### 1. What must move to `internal/report/document` (3a pattern: builders return values, caller encodes; warnings passed in, `[]` never null — STATE.md binding decisions 6-8) and its cli dependencies
Per-document types (move whole, cli keeps `marshalDocument` json.go:135 and calls `document.NewX`):
- spend: `spendDocument`+row types, `renderSpendingJSON` -> depends on cli var `spendGroupings[..].name` (spend_grouping.go:12, also used by render_spend.go:14 and flag default spend.go:82, `parseSpendGrouping`). The `name` column must become an exported report/store-level word table (cli keeps `header`/`missing`).
- cashflow: `cashFlowPeriods[..].name` (cashflow.go:20; also render_cashflow.go:21, cashflow.go:30,97). Same split.
- recurring: `recurringCadences` (json_recurring.go:66, moves with it) but `recurringStatus` lives in `render_recurring.go:24` (shared by text cell and JSON) -> needs one owner; `tenthsPerPercent` const json_recurring.go:74 (moves).
- anomalies: `anomaliesBaselineWord` lives in `render_anomalies.go:18` (shared text/JSON) -> one owner; `tenthsPerMultiple` json_anomalies.go:44 (moves).
- shared by all four: `accountFilterDocument`/`accountFilterDocuments` (declared in json_spend.go:22-33, used by all four; no other consumer in non-test code).
Warning helpers (pure functions of report types; every one is also exercised through cmd/quarry tests):
- `spendWarnings` spend.go:90, `cashFlowWarnings` cashflow.go:104, `recurringWarnings` recurring.go:92, `anomaliesWarnings` anomalies.go:87: compose the below. Each embeds the CLI command word (`spendCommand`, `cashFlowCommand`="cashflow", etc.) in text: `leftOutWarnings` empty_window.go:11 -> "…so spend leaves it out…" (empty_window.go:29-37), and the leftOut-of-reports warning tells the reader to "turn on reports … then run quarry sync". For MCP that is the tool name (cash_flow vs cashflow) = a copy question (see Open questions).
- `leftOutWarnings`, `linkedTrackingWarning`, `leftOutOfReportsWarning`, `appendEmptyWindowWarning`, `allLeftOut`, `emptyWindowWarning` empty_window.go:11-74: only the 4 `*Warnings` callers (grep). Move together.
- `unconvertedWarnings`, `beforeFirstRateWarning`, `noRatesWarning`, nouns `transactionsNoun/chargesNoun/seriesNoun` fx_warning.go:14-49: only the 4 `*Warnings` callers. `nativeOf` fx_warning.go:53 is ALSO used by `accountsFXWarnings` fx_warning.go:62 (called cli/accounts.go:46) which stays in cli (accounts is not a 3b tool) -> `nativeOf` is the one shared helper that would need a home both can import (document, or money).
- `withConfigWarnings` currency.go:71 (used by spend/cashflow/recurring/anomalies/accounts RunE): trivial; in mcp the analogue is `slices.Clone(cfg.WarningsAbsolute)` as data_quality.go:36 does.
- `currencyFlag.resolve` currency.go:50-57: flag if Changed, else `cfg.Currency` (config default CAD, config.go:30,46) + `cfg.WarningsAbsolute`, config loaded only when the flag is absent. It is cobra-bound; MCP needs its own tiny equivalent (param else `s.newConfig(commandName)`); `money.ParseCurrency` (CAD|USD|native, case-insensitive) is the shared parser. Reusable pattern: data_quality's config-first order (STATE decision 34).
Dependency rule holds: document imports report/store/finding/platform, never cli/mcp/config; the above need only `report`, `store`, `money`, `humanize`.

##### 2. Window / period / account / currency semantics an MCP schema would mirror
- `report.ParseWindow(since, until *string, now)` window.go:45; nil = not given. Inputs `YYYY`, `YYYY-MM`, `YYYY-MM-DD` (dateForms window.go:20); since = first day, until = LAST day of the period named; defaults: since = Jan 1 of now's year, until = today (`DefaultWindow` window.go:33). Refusals are `report.WindowError` (value type, NOT a RefusalError) with messages quoting flag names and the value: "`--since 2099 is after today; pass --until to include future-dated transactions`", "`--since %s is after --until %s`", "`--until %s is before the default --since %s; pass --since too`", "`--since %q is not a date; use YYYY, YYYY-MM or YYYY-MM-DD`" (window.go:47-103). For recurring/anomalies the CLI uses `ParseChargeWindow(command, …)` window.go:55 whose message names the command (future since refused even with until nil: "…%s lists charges up to today only…") — `reportFlags.chargesCommand` cli/window.go:13,45-52.
- `--account` is repeatable, each an id or a name (case-insens.); `namedAccounts` accounts.go:92 -> `resolveAccounts` :70 -> `pickAccount` :111; empty arg = unknown; dedup by id. Accounts left out of reports (NotInReports/LinkedTracking) yield warnings, not errors.
- Currency: CAD | USD | native (money.ParseCurrency); default config `reporting.currency`, else CAD. PRD:198 says "CAD or USD" parameters — whether `native` is exposed is an open question. 3a spec explicitly deferred any `currency` param on the first four tools (spec:17); no tool schema precedent beyond `limitSchema` / enum+default in internal/mcp/tools.go:56-95,126-135.
- `--by` enums: spend category|payee|tag|month; cashflow month|year (parse errors are cli UsageError with "--by must be …"; in MCP the schema enum + the SDK refusal handles it, `argumentsRefusedLog`).

##### 3. Account-refusal leak debt — CONFIRMED
- `unknownAccountRefusal(arg)` refusal.go:62-64 = `no account named %q; run quarry accounts --all to list them`; `ambiguousAccountRefusal` :67-70 = `%d accounts are named %q; pass one of their ids instead: <ids>`. Both are `report.RefusalError` (embed caller `arg` verbatim).
- `internal/mcp/result.go:51-57` `logLine`: `loggedError` first, then ANY `report.RefusalError` -> `refusal.Error()` (verbatim) -> the caller's account text and ids reach stderr, violating spec Rule 4 (stderr never copies text carrying caller/config values) — result.go:54 is the line REVIEW-02 cites (STATE.md:84, "OWNED BY 3b"). Latent today: none of the four 3a tools take accounts (`namedAccounts` only reached from Spend/CashFlow/Recurring/Anomalies — grep).
- What 3b must change (facts, not design): `logLine` cannot tell fixed-copy RefusalErrors (store refusals: `no store at ~… yet`, refusal.go:44-56, home-abbreviated path) from caller-text ones; they share the type. Either the account refusals stop being the verbatim-class or `logLine` classifies them (e.g. a distinct type/field in report, or tool wraps with `withLog`, result.go:36). Also the client text says "run quarry accounts --all", which an MCP client cannot run (copy ruling). Related, same debt cluster: `UnreadableReason` open/store.go open.go:49-57 returns `e.Reason` (DuckDB text, path-replaced) for OpenFaultOther/Permission/NotDuckDB and refusal.go:50-52 wraps it into a RefusalError -> also logged verbatim; STATE.md:84 records statement-time read faults as "needs copy ruling (unowned)" — 3b's tools add many more read paths (spending/cashflow/charges queries) that reach it. And `WindowError` is not a RefusalError -> already falls to `failedLog` (safe) but its client text quotes `--since` flag names and the caller's value.
- Test impact: CLI pins the exact refusal text (cli/*_account_test.go, `spend_account_test.go`); report/refusal_test.go.

##### 4. search_transactions
- Existing store reads: `report.Store` (store.go:11-28) has Status, Accounts, Spending, CashFlow, Charges, Findings, Schema, Query. `Charges` (store.Charge store.go:627) lists only EXPENSE transactions (sum of v_spending rows, kept if positive; duckstore/charges.go:15-37), ordered date then source_id, up to a date, NOT filterable by payee/memo/amount, no memo, no income/transfers. `Query` is raw SQL (the existing `query` tool already covers ad-hoc search). Nothing lists transactions with payee/memo/amount/date/account. Types exist: `store.Transaction` store.go:81-96 (Memo, Amount, PayeeID, AccountID, Date, Status, ChequeNumber, ExcludedFromReports).
- Tables a new read would use: `transactions(id, source_id, account_id, date, payee_id, memo, amount DECIMAL(18,2), currency, status, cheque_number, excluded_from_reports, posted_date)` duckstore/schema.go:44-57 + `splits(..., category_id, amount, memo, transfer_account_id)` :58-66, `payees`, `accounts`; `v_spending`/`v_cash_flow` (schema.go:171-208) are split-level expense/transfer-aware views (columns split_id, transaction_id, account_id, date, payee, category, spent…) — which source gives "transfer-aware" search (v_* views exclude transfers/excluded) vs "all transactions" is a decision.
- Prior art for the read pattern: `Store.Charges` charges.go:73 (`openRead`, `QueryRows`, `openFault(s.Path(), err)`, fault tests in `read_faults_test.go`), `accountFilter` filter.go (`and(column)`, `readArgs`), `Store.Query` query.go.
- A new port method means: `report.Store` method + `duckstore` implementation + `report.Server` method + `document` type + mcp tool; fakes embed `report.Store` (cli fakes_test.go, mcp fakeStore) so they compile without it (STATE trap :63). No CLI twin exists; PRD:201 lists the tool, "same detections the CLI uses" applies only to recurring/anomalies (PRD:200). A CLI twin (`quarry search`?) would be a new command; whether to add one is an open question.
- Row cap / redaction: caps live in mcp (STATE :35); `maxRows` 500 const tools.go:21, `limitSchema` tools.go:126. Redaction ruled out by user decision (spec:15): payee/memo pass through; so a search tool returning memos adds no masking obligation but is the first tool returning raw memo text per row (query already can).

##### 5. Other STATE debts owned/touched by 3b
- OWNED 3b (STATE.md:84): result.go:55 account-refusal leak (above).
- OWNED 3b (STATE.md:85): "no input-property `description`s … rule per-parameter copy in phase 3b" — all new schemas (and ideally the 4 existing: data_quality.limit counts findings, query.sql/limit, status, type) have none today (tools.go:56-95). Needs copy ruling pass (product-vision).
- Touched naturally: `loggedError`/`withLog`/`verbatim` machinery result.go:28-48 (new tool error paths with caller text must wrap); `absentNullArguments` arguments.go + test `Test_a_tool_call_with_null_arguments…`: every new schema with a `default` (by, currency?) must be covered (STATE :17); `tools/list` pin with `additionalProperties:false` for all tools (STATE :15); cmd/quarry root help pin lists subcommands (not tools; but `quarry mcp` Long says "Tools: describe_schema, query, sync_status, data_quality." cli/mcp.go and the spec Long block spec:162 — must be updated, and `instructions` tools.go:30-35 says "Call describe_schema before writing SQL"; both are copy); `maxRows`/caps (tool results unbounded for e.g. spend by payee or anomalies list? no cap exists for named-tool arrays; search needs one); `callTimeout` + `stoppedLine(name)` per tool; `WithConfig`/`WithReport` per-call (STATE :27,30); config-first-then-store order (STATE :34); inherited unowned MINORs on mcp (data_quality limit clamp, etc., STATE :82). `quarry mcp --help` showing inherited `--json` (STATE :85) is unowned, unrelated.
- Also: RETRO.md proposal 2/6: scoping should include a per-tool outcome table (outcome, client text, stderr line, exit) and cross-rule check (Rule 4 vs error copy) — directly relevant (the 3a R1 MAJOR was this class); retro change 6 says the account-refusal debt must be a 3b scenario.

#### Prior art in this repo
- internal/mcp/data_quality.go:20-47 + document.NewFindingsList (document/findings.go:52) — the exact template: handler(config first, then report), warnings composed in mcp, document built by the shared builder, caps in mcp.
- 3a SCENARIO-01 extraction (report/document status/findings/sql) — the 3b document move follows it; builders return values, cli indents (STATE :6-8).
- internal/mcp/tools.go:56-95 schema registration, `limitSchema`, `objectSchema`, enum from typed source (`finding.Types()`); same should hold for by/period enums.

#### Already exists — do not re-plan
- `report.Server.{Spend,CashFlow,Recurring,Anomalies}` and their Request/result types (above); `report.ParseWindow/DefaultWindow/ParseChargeWindow` (window.go:45,33,55); `namedAccounts`/`pickAccount`; `fillSeries` zero-filled periods and Partial flags (spending.go:79, cashflow.go:70).
- Warning policy functions (cli, moveable), `document.Money/DateLayout/NullString` (document/common.go).
- mcp wiring: `WithReport`, `WithConfig`, `handler`/`withLog`/`verbatim`, `tool()`, `objectSchema()`, `limitSchema()`, `absentNullArguments`.
- `store.Charges`, `Store.Query`, `Store.Schema`, `store.Transaction` type.

#### Must be built
- Move the four documents + warnings + shared word tables/maps into `internal/report/document` (cli re-points, text renderers import word tables); keep CLI `--json` byte-identical (pinned by cmd/quarry/run_*_json_test.go key-order tests, `run_shared_documents_test.go`).
- Four mcp tool handlers + schemas (since/until/account(s)/currency/by) + descriptions + per-parameter descriptions.
- A clock in internal/mcp (`WithClock`/`time.Now` default, wired in cmd/quarry/run.go `newMCPServe`) — nothing exists; recurring/anomalies/default-window need it, and tests need a fixed one.
- MCP-side currency resolution (param else config default) and window-error/account-refusal copy for MCP vocabulary; stderr classification for account refusals.
- search_transactions end-to-end: store read (new `report.Store` method + duckstore query + fault tests), Server method, document, tool, row cap + truncation warning, schema (query text on payee/memo, amount range, date range, account, limit).
- Update `quarry mcp` Long tool list, `instructions`, tools/list pins, cmd/quarry mcp acceptance tests.

#### Callers (grep; symbols are package-private so no LSP-exported surface changes except report API additions)
| Symbol / string | Caller | Via |
|---|---|---|
| Server.Spend/CashFlow/Recurring/Anomalies | cli/spend.go:69, cashflow.go:86, recurring.go:74, anomalies.go:67 (+3a triage table, spec:62-76) | grep |
| renderSpendingJSON / renderCashFlowJSON / renderRecurringJSON / renderAnomaliesJSON | only their cli RunE: spend.go:77, cashflow.go:93, recurring.go:81, anomalies.go:75; tests: internal/cli/json_{spend,cashflow,recurring,anomalies}_internal_test.go, spend_account_test.go, spend_empty_test.go | grep |
| spend/cashflow/recurring/anomalies Warnings | cli RunE same files; tests unconverted_test.go, spend_empty_test.go, fx_warning_internal_test.go, cmd/quarry/run_spend_fx_test.go, run_cashflow_fx_test.go | grep |
| leftOutWarnings, appendEmptyWindowWarning, emptyWindowWarning, allLeftOut | only the 4 *Warnings fns (spend.go:91,97, cashflow.go:106,108, recurring.go:93,95, anomalies.go:88,90) | grep |
| unconvertedWarnings, noRatesWarning, *Noun | only the 4 *Warnings fns; nativeOf also accountsFXWarnings (fx_warning.go:66) <- cli/accounts.go:46 | grep |
| accountFilterDocument(s) | json_{spend,cashflow,recurring,anomalies}.go only | grep |
| spendGroupings | render_spend.go:14, spend.go:82, spend_grouping.go:25, json_spend.go:89 | grep |
| cashFlowPeriods | render_cashflow.go:21, cashflow.go:30,97, json_cashflow.go:62 | grep |
| recurringStatus | render_recurring.go:35-37, json_recurring.go:121 | grep |
| anomaliesBaselineWord | render_anomalies.go:46, json_anomalies.go:84 | grep |
| withConfigWarnings | spend/cashflow/recurring/anomalies/accounts RunE (accounts.go:50) | grep |
| currencyFlag.resolve | the same 4 RunE + accounts (cli/currency.go:50) | grep |
| report.RefusalError | classified at internal/mcp/result.go:54; passed through sync_status.go:20, data_quality.go:30, describe_schema.go:19; created refusal.go:34,62,68 | grep |
| ParseWindow | cli/window.go:40-52 only (+tests) | grep |
Not LSP-verified; "only" claims rest on a repo-wide non-test grep by symbol name (would miss callers via reflection/strings, none expected).

#### Becomes dead if this ships
- Nothing becomes dead: the cli copies move, they are not orphaned (the CLI still calls them via document). `leftOutWarnings` etc. delete from cli once relocated (move, not removal).

#### Slicing and size (sketch only, no design)
One feature is too big for one spec? My read: two sequenced features, or one spec with a hard seam:
- 3b-1 "shared analysis documents": behaviour-neutral extraction of the four documents/warnings/word tables into document (pinned by existing --json tests; precedent SCENARIO-01 cost 1.6M IE, 10% of 3a). Includes making `Now`/clock available to mcp.
- 3b-2 "named tools": spending, cash_flow (same shape: window+accounts+currency+by) -> recurring_charges, anomalies (adds Now/charge-window rules) -> account-refusal stderr classification + per-parameter descriptions (can land with the first account-taking tool, since the tool makes the leak reachable) -> search_transactions (store read + tool; largest single new piece, can be its own spec/3b-3 since it is the only one with new storage code and no CLI twin).
Rough scenario list: (1) document extraction spend+cashflow, (2) extraction recurring+anomalies (likely FOLD candidates), (3) `spending` tool end-to-end incl. currency/window/account params, (4) `cash_flow` tool (LIGHT/FOLD-ish twin of 3), (5) account refusal classified, stderr line fixed copy, (6) window/currency refusal copy, (7) `recurring_charges`, (8) `anomalies`, (9) per-parameter descriptions + tools/list pin + Long/instructions update, (10-12) search store read, tool, caps. ~10-13 scenarios, comparable to 3a (17); search is ~25-30% of it.

#### Open questions (user only)
1. `currency` values: PRD:198 says CAD or USD; CLI also allows `native`. Expose `native` on MCP? (changes enum, mixed-currency warnings, and `native` rows in docs.) And the default: config `reporting.currency` (CLI behaviour) or always CAD?
2. Tool copy vocabulary for warnings/refusals that today say "spend", "cashflow", "quarry accounts --all", "--since", "run quarry sync" (empty_window.go:29-37, window.go, refusal.go:62-68): MCP-flavoured (tool names, param names, "ask the user to run quarry sync") — does Rule 1 allow differing from CLI text here (3a allowed only "next-step" lines to differ)? Changes whether warnings take a surface/vocabulary parameter in document.
3. search_transactions: MCP-only, or also a CLI `quarry search` twin (affects document ownership/Rule 1 and a new command through the full pipeline)? Source of truth: all transactions incl. transfers/excluded-from-reports, or only the transfer-aware reported set? Filters wanted (payee text, memo text, min/max amount in which currency, dates, account, category?) and row cap (500 like query, or lower)?
4. Slicing: single 3b spec, or extraction first and `search_transactions` as a separate later spec?
5. Do the new tools need a result cap/warning for large arrays (e.g. spend by payee over all history, anomalies, recurring)? 3a capped data_quality and query only.
6. Whether the unowned DuckDB-reason-via-RefusalError leak (open.go:49-57 -> refusal.go:50-52 -> result.go:54) is in 3b scope now that more read paths expose it (STATE :84).
7. Redaction: unchanged (spec:15), but search_transactions returns per-row memo text; confirm "pass through" still holds.

Files cited: /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/{internal/report/{spending,cashflow,recurring,anomalies,window,refusal,accounts,store,charges}.go, internal/cli/{spend,cashflow,recurring,anomalies,currency,window,empty_window,fx_warning,spend_grouping,json_spend,json_cashflow,json_recurring,json_anomalies,render_recurring,render_anomalies}.go, internal/mcp/{tools,result,data_quality,server}.go, internal/store/duckstore/{schema,charges,filter}.go, internal/store/{store,open}.go, docs/specifications/phase3a-mcp-core/{specification,STATE,RETRO,REVIEW-02}.md, docs/initial-prd.md:190-207}

## Product Verdict

**SHIP WITH CHANGES** (product-vision, Phase 1, 2026-10-03). Accepted changes, folded into the rules and scenarios above: (1) classify account refusals, window refusals and OpenFaultOther on stderr — own scenario, right after spending; (2) amended Rule 1 + CLI-vs-MCP equality test; (3) no schema default on currency; (4) 500 cap on every named-tool list; (5) reword only CLI command/flag/`quarry accounts --all` lines, "run quarry sync" verbatim; (6) instructions + query description send spending/income to named tools (SQLConventions untouched); (7) per-parameter descriptions on all eight tools; (8) per-call clock.

User decisions 2026-10-03: slice = analysis tools (search → 3c with a `quarry search` CLI twin; skill → 3d); currency accepts CAD, USD, native, default config `reporting.currency` else CAD (PRD §MCP says "CAD or USD" — recorded deviation, no PRD edit).

## Surface & Copy

Product-vision Phase 1 ruling, verbatim (headings demoted). Binding: developers implement these strings verbatim; §3.8 lists changes to existing surfaces.

### Product-vision Phase 1: phase3b-analysis-tools

#### What I read

- The triage brief.
- The phase3a spec: Rules 1-10, §2.1a, §2.3, §2.6, §2.7, §4. Also its STATE.md and RETRO.md.
- PRD §MCP server (lines 190-207).
- In `internal/cli`: `spend.go`, `cashflow.go`, `recurring.go`, `anomalies.go`, `currency.go`, `window.go`, `empty_window.go`, `fx_warning.go`, `spend_grouping.go` and `json_{spend,cashflow,recurring,anomalies}.go`.
- In `internal/report`: `window.go`, `refusal.go`, `accounts.go:69-140`, `spending.go`, `recurring.go:100-240` and `anomalies.go`.
- In `internal/mcp`: `result.go`, `tools.go`, `data_quality.go` and `query_refusal_test.go:80-118`.
- Also: `internal/store/open.go`, `internal/store/duckstore/{duckstore.go:196-230,spending.go,cashflow.go,charges.go}` and `internal/config/refusal.go`.

#### 1. Verdict on the slice

The slice is right.
- The four `--json` documents and their warnings get their second caller now. That is the 3a rule for moving them.
- The account-refusal leak is fixed in the same slice that makes it reachable. Today no tool takes `accounts`; `spending` is the first.
- `search_transactions` with its CLI twin goes to 3c. That is correct: it is the only piece with new storage code, and it needs its own CLI surface decision.

**Nothing dies.** The cli warning helpers (`leftOutWarnings`, `appendEmptyWindowWarning`, `unconvertedWarnings`, the nouns, `accountFilterDocument(s)`, and the `name` column of `spendGroupings`/`cashFlowPeriods`) move. They are not copied. Delete the cli copies in the same scenario that moves them. `nativeOf` is shared with `accountsFXWarnings`, so it gets one home that both can import.

#### 2. Rule changes (binding; go into the 3b spec Business Rules)

**Rule 1, amended for 3b. Replaces 3a Rule 1 for these four tools.**

> Each tool's structured result is the CLI `--json` document for the same inputs, built by the same constructor in `internal/report/document`.
> - Every field other than `warnings` is byte-identical to the CLI's, except a list cut by the 3b cap (Rule 11).
> - In `warnings`, a line differs from the CLI only where it names a CLI command word (`spend`, `cashflow`, `recurring`, `anomalies`), a flag (`--since`, `--until`, `--account`) or a command the client cannot use (`quarry accounts --all`). Those lines use the tool's name, its parameter names, or the tool that does the job, per the mapping table in §3.
> - "run quarry sync" is never reworded on any surface. The user runs it, and the instructions already say so. Store refusals already say it verbatim, so one tool never speaks in two styles.

The equality test this implies:
- For each tool, take MCP `structuredContent` minus `warnings` and the CLI `--json` minus `warnings`, for the same since/until/accounts/currency/by and the same fixed clock. They are byte-equal (compare the compact `TextContent`, not the decoded map: key order matters).
- The `warnings` arrays are equal after applying the §3 mapping row by row.
- The fixture must produce at least one left-out account warning and one before-first-rate warning, or the mapping half proves nothing.

**Rule 11 (new): caps.**
- Every list a named tool returns stops at 500 entries (`maxRows`, the existing constant):
  - `spending.rows`
  - `cash_flow.periods`
  - `recurring_charges.series`
  - `anomalies.anomalies`
- Lists are cut in document order. Totals, `checked` and `not_judged` always count the whole uncut result.
- A cut is reported only through `warnings` (copy in §5). There is no new field and no `limit` parameter (precedent: `describe_schema` lists).
- Nested arrays (`series[].price_changes`, `payees`, `accounts`) are not capped. A listed series is steady by definition, so its price changes are few, and its payees and accounts are the distinct ones in one run.

**Rule 12 (new): clock.** "Today" is the server's local date when each call starts. It is read once per call and used for the default window, the charge-window check and recurring/anomalies `Now`. It is the same as the CLI's single `at := now()`. A server left running past midnight uses the new date on its next call. Adding `WithClock` (default `time.Now`, wired in `newMCPServe`) is fine; the design is the architect's.

**Rule 4: no change to the rule, but three classifications are new** (§4): window refusals, account refusals and `OpenFaultOther`.

#### 3. Literal surface

##### 3.1 Tool names (permanent)

`spending`, `cash_flow`, `recurring_charges`, `anomalies`. These match the PRD table.

##### 3.2 Shared parameter schema

Each tool is hand-written with `additionalProperties:false`, per 3a Rule 10.

| param | JSON schema | default | tools |
|---|---|---|---|
| `since` | `{"type":"string"}` | none in schema (handler: absent → Jan 1 this year) | all four |
| `until` | `{"type":"string"}` | none in schema (handler: absent → today) | all four |
| `accounts` | `{"type":"array","items":{"type":"string"}}` | none | all four |
| `currency` | `{"type":"string","enum":["CAD","USD","native"]}` | **none in schema.** A schema `default` would override `reporting.currency`. Absent → config `reporting.currency`, else CAD. | all four |
| `by` (spending) | `{"type":"string","enum":["category","payee","tag","month"],"default":"category"}` | `category` | spending |
| `by` (cash_flow) | `{"type":"string","enum":["month","year"],"default":"month"}` | `month` | cash_flow |

- **Enum source.** The `by` enums come from the moved word tables (the `name` column), never typed by hand. The precedent is `finding.Types()`.
- **`absentNullArguments`.** Both `by` schemas carry a `default`, so `Test_a_tool_call_with_absent_null_or_empty_arguments_reaches_the_handler` must cover `spending` and `cash_flow`.
- **No `pattern` on since/until.** The handler owns the one date-refusal line. A pattern would split one refusal across SDK text and quarry text.
- **Deliberate deviation from "same as CLI":** the `currency` enum is exact-case. `cad` is an SDK argument refusal on MCP, while the CLI accepts any case. The enum documents itself to the model, which outweighs case-folding.

How absent, empty and null values behave:

| Value | Behaviour |
|---|---|
| `since`/`until` absent | default |
| `since`/`until` `""` | window refusal `since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` (CLI parity: `--since ""` refuses) |
| `since`/`until` `null` | SDK type refusal |
| `accounts` absent or `[]` | every account |
| `accounts` `[""]` | `no account named ""; call describe_schema to list the accounts` |
| `accounts` `null` | SDK type refusal |

Repeated names or ids in `accounts` are de-duplicated by id, as in the CLI. The architect needs pointer fields, or equivalent, to tell absent from `""`.

**Config is read only when `currency` is absent**, matching the CLI. With `currency` given, a broken config does not refuse, and the document carries no config warnings. Rule 1 requires this.

**Order of outcomes (matches CLI RunE):**
1. SDK argument check
2. window
3. config (only when `currency` is absent)
4. store open: the accounts read when `accounts` is non-empty, else the main read
5. account resolution
6. main read, which can hit a statement-time fault

A timeout can end any step after 1.

##### 3.3 Tool descriptions (model-facing; hard line breaks, no trailing newline)

**spending**
```
Total the user's spending for a period, grouped by category, payee, tag
or month, with a total per currency. quarry's spending rules apply:
transfers between the user's own accounts, Quicken's system categories,
transactions marked "exclude from reports" and accounts Quicken leaves out
of reports are not counted, and refunds are netted, so a category can come
out negative. Each split is converted at the Bank of Canada rate for its
date. Use this rather than query for spending totals. Returns at most 500
rows; totals always count every row.
```

**cash_flow**
```
Report income, spending, net and savings rate for each month or year of a
period, with totals per currency. The rules are spending's: transfers
between the user's own accounts are neither income nor spending, and spent
equals spending's total for the same period, accounts and currency.
Savings rate is net divided by income, null when income is zero or less.
A period that since or until cuts short is marked partial.
```

**recurring_charges**
```
List charges that repeat every week, month, quarter or year at a steady
amount (subscriptions, memberships, insurance), found in all of the
user's history and listed when they were running during the period. Each
series has its cadence, latest amount, cost per year while active, price
changes and accounts. A series is found in its account's own currency, so
an exchange-rate change is never a price change. Charges dated after today
never count. Bills whose amount changes most times, such as utilities, are
not listed; use spending with by payee for those. Returns at most 500
series; totals count every series.
```

**anomalies**
```
List charges in the period that are unusually large: more than 2 times the
median of the payee's earlier charges (when it has at least 3), else more
than 5 times the median of the category's earlier charges (at least 10).
Charges under 100.00 in their account's own currency are never listed.
Each charge is compared with all earlier history, whatever the period.
Possible duplicates are not listed here; data_quality lists them. Charges
dated after today are never listed. Returns at most 500 charges, newest
first.
```

##### 3.4 Per-parameter `description` strings (closes the STATE debt; includes the 3a tools)

**query**
- `sql`: `One read-only SQL statement in DuckDB's dialect over quarry's tables and views; describe_schema lists them.`
- `limit`: `Most rows to return, 1 to 500. Defaults to 500.`

**data_quality**
- `status`: `Which findings to list: open (the default), ignored (the user listed the id in findings.ignore), fixed (no longer found since a later sync), or all.`
- `type`: `List only findings of this type. Omit it to list every type.`
- `limit`: `Most findings to return, 1 to 500. Defaults to 50. counts always covers every finding, and each finding lists at most 25 items.`

**describe_schema**, **sync_status**: no parameters.

**spending and cash_flow**
- `since`: `First day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month starts on its first day. Defaults to January 1 of this year.`
- `until`: `Last day to count: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today; future-dated transactions count only when until is later than today.`
- `accounts`: `Count only these accounts, each given by id or by name in any letter case. Omit it to count every account.`
- `currency`: `Currency for amounts: CAD, USD, or native to list each account's own currency separately. Defaults to reporting.currency in quarry's config file, else CAD.`
- `by` (spending): `Group by category (the default), payee, tag or month. A split with several tags counts under each tag.`
- `by` (cash_flow): `One row per month (the default) or per year.`

**recurring_charges**
- `since`: `List series still running on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to January 1 of this year.`
- `until`: `List series that started on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today.`
- `accounts`: `List only series with a charge in one of these accounts, each given by id or by name in any letter case. Omit it for every account.`
- `currency`: the same string as spending.

**anomalies**
- `since`: `List charges dated on or after this date: YYYY, YYYY-MM or YYYY-MM-DD. Defaults to January 1 of this year.`
- `until`: `List charges dated on or before this date: YYYY, YYYY-MM or YYYY-MM-DD; a year or month ends on its last day. Defaults to today.`
- `accounts`: `List only charges in these accounts, each given by id or by name in any letter case; the payee's charges in other accounts still count as history.`
- `currency`: the same string as spending.

##### 3.5 Output

Each tool returns the CLI `--json` document verbatim per amended Rule 1:
- `spending` → `{since, until, by, currency, account_filter, rows, totals, warnings}`
- `cash_flow` → `{since, until, by, currency, account_filter, periods, totals, warnings}`
- `recurring_charges` → `{since, until, currency, account_filter, series, totals, warnings}`
- `anomalies` → `{since, until, currency, account_filter, anomalies, checked, not_judged, warnings}`

`currency` is the resolved value, so the model sees which default applied. `since` and `until` are the resolved window, so the model sees what "today" was.

##### 3.6 `quarry mcp` Long (full replacement)
```
Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port. Add it to your client's MCP config with the command
"quarry" and the argument "mcp".

The server reads quarry's store; it never runs quarry sync, never prunes
snapshots and never touches Quicken. Each request reads the store as it
is then, so after you run quarry sync the client sees the new data
without a restart. SQL runs read-only, and every list a tool returns
stops at 500 entries.

Payee names, memos, account names and category names reach the client
as Quicken holds them; quarry does not rewrite or mask them.

Tools: describe_schema, query, sync_status, data_quality, spending,
cash_flow, recurring_charges, anomalies.
```

##### 3.7 Server `instructions` (full replacement)
```
quarry serves David's Quicken Classic for Mac data from a local, read-only
store. Call sync_status first and tell the user how old the snapshot is
(snapshot.taken_at). For spending, income, recurring charges and unusually
large charges call spending, cash_flow, recurring_charges and anomalies:
they apply quarry's rules for transfers, refunds and currencies. For other
questions call describe_schema before writing SQL for query: its
conventions say which views already leave out transfers and how amounts,
signs and currencies work. Every number you report must come from a tool
result; never estimate. quarry cannot change data: fixes are made in
Quicken, then the user runs quarry sync.
```

The "David's" NIT stays deferred.

##### 3.8 Changes to existing surfaces

1. **`query` description**, line 3-5.
   - Old: `For spending and income use v_spending and\nv_cash_flow: they already leave out transfers between the user's own\naccounts.`
   - New, with the whole block re-wrapped:
   ```
   Run one read-only SQL query (DuckDB dialect) against quarry's store and
   return its columns and rows. Call describe_schema first for the tables,
   views and conventions. For spending and income totals call spending or
   cash_flow instead; in SQL use v_spending and v_cash_flow, which already
   leave out transfers between the user's own accounts. Returns at most
   `limit` rows (default 500, the most allowed); aggregate in SQL rather
   than paging through rows. The store cannot be changed, and other files,
   databases and extensions are off.
   Send one statement; if you send several, only the last one's rows come back.
   ```
   - Do **not** touch `report.SQLConventions`. It renders byte-identical into `sql --help`.
2. **Instructions** (§3.7) and **mcp Long** (§3.6). The tools/list and Long pins update.
3. **Overturned 3a §2.1a sentence.** "The `OpenFaultOther` reason is DuckDB's open error with the path replaced, and it stays verbatim too."
   - It now logs the withheld line in §4 for **all eight tools**.
   - Pin that changes: `internal/mcp/query_refusal_test.go:99-101`, row "a store unreadable for another reason". Its stderr assertion becomes the withheld line. Its client text is unchanged.
   - Record in STATE.md `## Open debts` so the scenario's developer re-asserts it. Grep `cmd/quarry/run_read_refusals_test.go` and `run_status_findings_test.go` for any mcp stderr assertion on an `OpenFaultOther` reason; none were visible in my grep.
4. **PRD §MCP server** says `currency (CAD or USD)`. The 2026-10-03 user decision adds `native` and defaults to `reporting.currency`. Note this in the spec's Decisions. No PRD edit is needed beyond a one-line Decisions entry if the user wants one.
5. **CLI stdout and stderr: no byte changes.** The existing `--json`, help and refusal pins are the proof.

#### 4. Vocabulary: every warning and refusal line, CLI vs MCP

##### 4.1 Warnings (in `warnings`; never on stderr)

| Warning | Tools | CLI line | MCP line |
|---|---|---|---|
| Linked tracking | all | `account "Visa" uses linked account tracking in Quicken, so spend leaves it out, as Quicken's reports do` | `account "Visa" uses linked account tracking in Quicken, so spending leaves it out, as Quicken's reports do`, with the tool name: `spending` / `cash_flow` / `recurring_charges` / `anomalies` |
| Not in reports | all | `account "Visa" is not used in reports in Quicken, so spend leaves it out; to include it, turn on reports for it in Quicken's account settings, then run quarry sync` | the same line, with the tool name replacing the command word; the rest is unchanged |
| No rates | all | `the store has no exchange rates, so amounts are listed in each account's own currency; run quarry sync to fetch them` | unchanged |
| Before first rate | all | `3 transactions dated before 2017-01-03, the first exchange rate in the store, are listed in USD, not converted to CAD` | unchanged. The noun is `transaction(s)` for spending and cash_flow, `series with a charge` for recurring_charges, and `charge(s)` for anomalies. |
| Multi-tag | spending, `by: tag` | `4 splits carry more than one tag, so the rows add up to more than the total` | unchanged |
| Empty window | all | `no spending from 2026-01-01 to 2026-10-03; the store's transactions run 2003-01-02 to 2026-09-30` (with the named-accounts and no-transactions variants) | unchanged. The subject is `spending` / `income or spending` / `recurring charges` / `unusually large charges`. |
| Config unknown key | all, when `currency` is absent | absolute form, as in `--json` | unchanged (`cfg.WarningsAbsolute`) |
| Cap | all | none (CLI has no cap) | §5 |

Warning order: config lines, then the CLI's own order (left-out, unconverted, multi-tag, empty window), then the cap line last.

##### 4.2 Refusals (isError, one line, no `quarry: `)

| Outcome | Tools | CLI line | MCP client line |
|---|---|---|---|
| Not a date | all | `--since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` | `since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD` (likewise `until`) |
| Future since, no until | spending, cash_flow | `--since 2099 is after today; pass --until to include future-dated transactions` | `since 2099 is after today; pass until to include future-dated transactions` |
| Future since, no until | recurring_charges, anomalies | `--since 2099 is after today; recurring lists charges up to today only, so pass an earlier --since` | `since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since` (or `anomalies`) |
| since after until | all | `--since 2025 is after --until 2024` | `since 2025 is after until 2024` |
| until before default since | all | `--until 2025-03 is before the default --since 2026-01-01; pass --since too` | `until 2025-03 is before the default since 2026-01-01; pass since too` |
| Unknown account | all | `no account named "Visa"; run quarry accounts --all to list them` | `no account named "Visa"; call describe_schema to list the accounts` |
| Ambiguous account | all | `2 accounts are named "Visa"; pass one of their ids instead: a1, a2` | unchanged |
| Config unreadable or bad value | all, when `currency` is absent | the config loader's line, `~` form | the same line, verbatim (data_quality precedent) |
| Store: missing, other format, not DuckDB, permission, locked, other | all | `report.storeRefusal` lines | unchanged, `~` form |
| Timeout | all | none (CLI has Ctrl-C) | `spending stopped after 30 seconds; try again` (via `stoppedLine(<tool>)`) |
| "<cmd> interrupted" | all | `spend interrupted` | not reachable: cancel is silent, and a deadline maps to the timeout line |

Mechanism constraint: `report.WindowError` and the account refusals must carry their parts (which bound, value, kind, ids), not only CLI text, so each surface words its own line. The CLI stays byte-identical. Design is the architect's.

#### 5. Caps (Rule 11), copy

Numbers use `humanize.Thousands`. The cap line is the last warning.

| Tool | Line |
|---|---|
| spending | `spending lists the first 500 rows of 1,234; totals count every row; pass a shorter period or fewer accounts, or query v_spending for the rest` |
| cash_flow | `cash_flow lists the first 500 periods of 612; totals count every period; pass a later since, or by year` |
| recurring_charges | `recurring_charges lists the first 500 series of 731; totals count every series; pass a shorter period or fewer accounts` |
| anomalies | `anomalies lists the first 500 charges of 812; pass a shorter period or fewer accounts` |

Why cap rather than "no cap":
- Q7 says results are row-capped.
- `spending` by payee over all history is thousands of rows.
- `by: month` with `currency: native` since 2000 is 2 × 12 × 26 = 624 rows. That is reachable.
- recurring and anomalies are rarely near 500. The rule costs nothing there, and keeps the tool from returning an unbounded list.

#### 6. Per-tool outcome tables (retro requirement)

In the tables:
- The stderr column is the text after `quarry: mcp: <tool>: `.
- "Values" says what the client text and the stderr line each carry from the caller, config or data.
- ✗ is an isError line, W is a warning, R is a result.

##### 6.1 Shared rows: every tool, T = tool name, twin = spend / cashflow / recurring / anomalies

| # | Outcome | Client | stderr (§2.1a class) | Values: client / stderr |
|---|---|---|---|---|
| 1 | SDK argument refusal: wrong type, `by`/`currency` not in enum (including `cad`), unknown property, `null` property | ✗ SDK text | `refused the call's arguments; details went to the client only` (withheld) | may echo argument values / none |
| 2 | Not a date, including `""` | ✗ §4.2 | `refused the call's since or until; details went to the client only` (withheld) | caller value / none |
| 3 | since after until | ✗ §4.2 | same as row 2 | caller values / none |
| 4 | until before default since | ✗ §4.2 | same as row 2 | caller value and the default date / none |
| 5 | Config unparseable or bad value (only when currency is absent) | ✗ config line, `~` | `cannot read quarry's config file; run quarry <twin> to see why` (withheld) | config text or value / none |
| 6 | No store | ✗ `no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it` | same line (verbatim) | `~` path only / `~` path only |
| 7 | Other format | ✗ existing line | verbatim | snapshot id, path / same (fixed, file-level) |
| 8 | Not DuckDB, permission | ✗ existing line | verbatim | fixed phrase / same |
| 9 | Locked | ✗ existing line | verbatim | fixed phrase / same |
| 10 | `OpenFaultOther`, open-time **or statement-time** | ✗ `cannot read the store at ~/…: <DuckDB reason>; run quarry sync to rebuild it` | `cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb; details went to the client only` (withheld, **new**) | DuckDB reason / `~` path only |
| 11 | Unknown account, including `""` | ✗ `no account named "Visa"; call describe_schema to list the accounts` | `refused the call's accounts: one names no account; details went to the client only` (withheld, **new**) | caller text / none |
| 12 | Ambiguous account | ✗ `2 accounts are named "Visa"; pass one of their ids instead: a1, a2` | `refused the call's accounts: one names more than one account; details went to the client only` (withheld, **new**) | caller text, ids / none |
| 13 | Timeout | ✗ `T stopped after 30 seconds; try again` | same (verbatim) | none / none |
| 14 | Client cancel or disconnect | silence | none | — |
| 15 | Factory or unclassified error | ✗ error text | `failed; details went to the client only` | possibly / none |
| 16 | Success, including warnings, empty period or cap hit | R | none | warnings carry account names / — |
| 17 | Named account left out (linked or not in reports) | R + W per §4.1, with T as the subject | none | account name / — |
| 18 | Every named account left out | R, no empty-window line (the left-out lines say why) | none | — |
| 19 | Empty period | R + W empty-window (subject per tool) | none | dates / — |
| 20 | No rates (currency CAD or USD) | R + W no-rates | none | — |
| 21 | Before first rate | R + W (noun per tool) | none | dates, count / — |
| 22 | `currency: native` | R, rows per currency, no FX warnings | none | — |
| 23 | Closed account named or counted | R, included, no warning | none | — |
| 24 | Store stale | R, no hint (freshness is `sync_status`'s job) | none | — |
| 25 | Quicken closed, reconciliation failed, fingerprint changed | no row: MCP never sees Quicken, and a failed sync leaves the previous store | — | — |
| 26 | Sync runs concurrently | R from the old or new store; no row | — | — |
| 27 | Store has no transactions | R + empty-window line ending `; the store has no transactions` | none | — |
| 28 | Config unknown keys (currency absent) | R + absolute config warning lines first | none | config path, key / — |
| 29 | currency given and config broken | R, no config warnings (config is not read) | none | — |

##### 6.2 Rows that differ by tool

| Row | spending | cash_flow | recurring_charges | anomalies |
|---|---|---|---|---|
| Future since, no until | ✗ `since 2099 is after today; pass until to include future-dated transactions` | same as spending | ✗ `since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since` | ✗ `since 2099 is after today; anomalies lists charges up to today only, so pass an earlier since` |
| stderr for the row above | `refused the call's since or until; details went to the client only` | same | same | same |
| `by` | enum category/payee/tag/month, default category | enum month/year, default month | none (an unknown property is an SDK refusal) | none |
| Multi-tag W | with `by: tag` and multi-tag splits | — | — | — |
| Before-first-rate noun | transaction(s) | transaction(s) | series with a charge | charge(s) |
| Empty-window subject | `spending` | `income or spending` | `recurring charges` | `unusually large charges` (when `checked == 0`) |
| Cap noun and list | rows / `rows` | periods / `periods` | series / `series` | charges / `anomalies` |
| Cap with `currency: native`, `by: payee` | payee rows sort currency first, so a cut can drop every USD row while `totals` still shows USD. The cap line covers it ("totals count every row"); no extra copy. | `by: month`, native since 2000 can cut the newest periods; the line advises a later since or by year | — | newest first, so a cut drops the oldest |
| Config stderr twin | `run quarry spend to see why` | `run quarry cashflow to see why` | `run quarry recurring to see why` | `run quarry anomalies to see why` |
| Timeout line | `spending stopped after 30 seconds; try again` | `cash_flow stopped after …` | `recurring_charges stopped after …` | `anomalies stopped after …` |
| Future-dated transactions with `until` later than today | counted | counted | never counted (`Now` cut) | never listed |

##### 6.3 Cross-rule check

1. **Contradiction found: Rule 4 vs `logLine`'s `RefusalError` arm.** Account refusals are `RefusalError`s carrying caller text, and today they would log verbatim. Resolved by rows 11 and 12: a fixed class line, with account refusals no longer in the verbatim class. This is the OWNED-BY-3b debt and must be its own scenario (retro change 6), landing with or before the first tool that takes `accounts`.
2. **Contradiction found: Rule 4 vs 3a §2.1a "OpenFaultOther stays verbatim".**
   - Statement-time faults in `duckstore` spending, cashflow and charges (`spending.go:152`, `cashflow.go:101`, `charges.go:69`) go through `openFault` → `OpenFaultOther` with DuckDB's statement text.
   - That text can quote a stored value, for example a Conversion Error.
   - 3b fixes it now, for all eight tools, by row 10. That closes the "unowned" STATE debt for `schema_read.go`/`status.go`/`findings_read.go` too, because the classification is per fault, not per call site.
   - The architect needs no open-time vs statement-time distinction: both are withheld.
3. **Rule 4 vs `WindowError`.** It is not a `RefusalError`, so today it falls to `failedLog`. That is safe but tells the operator nothing. Rows 2-4 rule a class line. No leak either way.
4. **Rule 4 vs config.** Withheld, using the data_quality precedent (`configRefusalLog`). Each tool's line names its CLI twin.
5. **Rule 4 vs warnings.** Account names, dates and payees appear only in `warnings` and the document, never on stderr, because success writes nothing to stderr. No contradiction.
6. **Rule 1 vs MCP warning rewording.** Under 3a's wording ("only next-step lines naming a CLI flag"), the left-out lines' subject word (`spend leaves it out`) would be a violation. That wording is replaced by the §2 amendment. With "run quarry sync" left verbatim, store refusals and warnings speak one style.
7. **Rule 1 vs Rule 11 caps.** A cut changes one list only. Totals, `checked` and `not_judged` are uncut, and the change is visible in `warnings`. The equality test excludes capped fixtures. No contradiction.
8. **Q4 check.** The transfer, refund and FX rules stay in `report` and the views. MCP adds only wording and the cap. `cash_flow.spent` equals `spending`'s total by the existing core rule.
9. **Rule 7.** Every refusal above is one line with no `quarry: ` prefix. ✓

#### 7. Clock

Confirmed. "Today" is the server's local date per call, read once at call start (Rule 12). `WithClock` is acceptable. Tests inject a fixed time, and the CLI-vs-MCP equality test uses the same instant on both sides.

#### Costs

- **New contract:**
  - four tool names
  - params `since`, `until`, `accounts`, `currency`, `by`
  - the 500 cap and four cap lines
  - five new stderr class lines
  - eight per-tool refusal and warning rewordings
  - per-parameter descriptions on all eight tools
- **Breaks:** no CLI break. The changed pins are tools/list (descriptions plus the new property descriptions), the mcp Long tool list, the instructions, the query description, and the stderr line of the `OpenFaultOther` row in `query_refusal_test.go:99-101`.
- **No new exit codes.** Server exits are unchanged, and tool outcomes are isError, not exit codes.
- **Data text:** account names in warnings and refusals use Go `%q`, then standard JSON escaping, exactly as in `--json`. Payee, category and tag keys are JSON strings; `null` (no key) stays distinct from `""`. MCP has no text or CSV channel.

#### Verdict: SHIP WITH CHANGES

Ranked:
1. **Classify account refusals, window refusals and `OpenFaultOther` on stderr (§6.1 rows 2-4, 10-12)** as its own scenario, landing before or with `spending`. It makes Rule 4 true for every new read path and closes the 3b-owned debt plus the unowned statement-fault debt. List the overturned §2.1a sentence and the changed pin.
2. **Amend Rule 1 as written in §2 and build the equality test it implies.** Compare non-warning fields byte-equal, and warnings through the §4.1 mapping. Without it, "the CLI and MCP never disagree" is unproven, or the scenario over-asserts.
3. **Do not give `currency` a schema default.** Config decides, and config is read only when `currency` is absent (CLI parity). The exact-case enum is a recorded deviation.
4. **Cap every named-tool list at 500 with the §5 lines.** Totals stay uncut. This is Q7.
5. **Reword only lines naming CLI commands, flags or `quarry accounts --all`; keep "run quarry sync" verbatim.** Rules out two styles inside one tool's output.
6. **Update the instructions and the query description (§3.7, §3.8)** to send spending and income questions to the named tools. This is the PRD's reason for named tools (Q4). `SQLConventions` is untouched.
7. **Add per-parameter descriptions to all eight tools (§3.4).** This closes the STATE debt.
8. **Clock per call (Rule 12).**

Key files:
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/mcp/result.go — `logLine` RefusalError arm
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/refusal.go — account refusals and the `storeRefusal` OpenFaultOther arm
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/window.go — `WindowError` text with flag names
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/empty_window.go — `leftOutWarnings` command word
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/fx_warning.go — `nativeOf` is shared with `accountsFXWarnings`
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/mcp/tools.go — instructions, query description, schemas
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/mcp/query_refusal_test.go — line 99-101, the pin that changes
- /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/store/duckstore/spending.go — line 152, same pattern at `cashflow.go:101` and `charges.go:69`: statement faults become OpenFaultOther

---

## Scenarios (Gherkin)

Approved by the user 2026-10-03. Sizing split SCENARIO-10 (document vs refusal) and added SCENARIO-11b (anomalies future-since line, already ruled in §6.2). Listed in build order.

```gherkin
Scenario Outline: SCENARIO-01 — CLI output is unchanged after the analysis documents and warnings move to shared code
  Given a built store with a left-out account and amounts dated before the first rate
  When the user runs quarry <cmd>
  Then stdout, stderr and exit code are byte-identical to before the move

  Examples:
    | cmd              |
    | spend --json     |
    | cashflow --json  |
    | recurring --json |
    | anomalies --json |
    | spend            |
    | cashflow         |
    | recurring        |
    | anomalies        |

Scenario: SCENARIO-02 — spending returns the spend --json document
  Given a built store, a fixed clock and the same since, until, accounts, currency and by
  When the client calls spending
  Then every field but warnings is byte-equal to quarry spend --json, and warnings equal the CLI's after the §4.1 wording map

Scenario Outline: SCENARIO-06 — currency follows the config only when it is absent
  Given a built store and <config>
  When the client calls spending with currency <currency>
  Then <outcome>

  Examples:
    | currency | config                     | outcome |
    | absent   | reporting.currency = "USD" | the USD document |
    | absent   | no config file             | the CAD document |
    | absent   | an unparseable config      | isError with the config line; stderr "cannot read quarry's config file; run quarry spend to see why" |
    | CAD      | an unparseable config      | the CAD document with no config warnings |

Scenario: SCENARIO-07 — spending's warnings use the tool's name and keep "run quarry sync"
  Given a store with a linked-tracking account, a not-in-reports account and amounts before the first rate
  When the client calls spending naming those accounts
  Then the warnings are the ruled MCP lines

Scenario: SCENARIO-12 — "today" is read on every call
  Given quarry mcp is serving and its clock moves past midnight between two spending calls
  When the client calls spending the second time without since or until
  Then the window ends on the new date

Scenario Outline: SCENARIO-03 — spending refuses a bad window in MCP words
  Given a built store
  When the client calls spending with <window>
  Then the result is isError with <line>
  And stderr gets only "quarry: mcp: spending: refused the call's since or until; details went to the client only"

  Examples:
    | window                         | line |
    | since "2024-13"                | since "2024-13" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD |
    | since ""                       | since "" is not a date; use YYYY, YYYY-MM or YYYY-MM-DD |
    | since 2025, until 2024         | since 2025 is after until 2024 |
    | until before the default since | until 2025-03 is before the default since 2026-01-01; pass since too |
    | since 2099, no until           | since 2099 is after today; pass until to include future-dated transactions |

Scenario Outline: SCENARIO-04 — an unknown or ambiguous account is refused without its name reaching stderr
  Given a built store
  When the client calls spending with accounts <accounts>
  Then the result is isError with <line>
  And stderr gets only <log> and carries no account name or id

  Examples:
    | accounts      | line | log |
    | ["Nope"]      | no account named "Nope"; call describe_schema to list the accounts | refused the call's accounts: one names no account; details went to the client only |
    | [""]          | no account named ""; call describe_schema to list the accounts | refused the call's accounts: one names no account; details went to the client only |
    | ["Visa"] (2)  | 2 accounts are named "Visa"; pass one of their ids instead: a1, a2 | refused the call's accounts: one names more than one account; details went to the client only |

Scenario Outline: SCENARIO-05 — a DuckDB read fault logs only the withheld store line
  Given a store whose read fails with a DuckDB reason, at open or mid-statement
  When the client calls <tool>
  Then the client gets the cannot-read line with the reason
  And stderr gets only "cannot read the store at ~/Library/Application Support/quarry/quarry.duckdb; details went to the client only"

  Examples:
    | tool              |
    | query             |
    | describe_schema   |
    | sync_status       |
    | data_quality      |
    | spending          |
    | cash_flow         |
    | recurring_charges |
    | anomalies         |

Scenario: SCENARIO-09 — cash_flow returns the cashflow --json document
  Given a built store, a fixed clock and the same since, until, accounts, currency and by
  When the client calls cash_flow
  Then every field but warnings is byte-equal to quarry cashflow --json, and warnings equal the CLI's after the §4.1 wording map

Scenario: SCENARIO-08 — a list over 500 entries is cut with the cap warning
  Given a store yielding more than 500 spending rows by payee
  When the client calls spending with by payee
  Then rows holds the first 500, totals count every row, and the last warning is the ruled cap line

Scenario: SCENARIO-14 — absent, null and empty arguments reach spending and cash_flow with the by default applied
  Given a built store
  When the client calls spending and cash_flow with arguments omitted, null or {}
  Then each call returns its document grouped by its by default

Scenario: SCENARIO-10 — recurring_charges returns the recurring --json document
  Given a built store, a fixed clock and the same since, until, accounts and currency
  When the client calls recurring_charges
  Then every field but warnings is byte-equal to quarry recurring --json, and warnings equal the CLI's after the §4.1 wording map

Scenario: SCENARIO-10b — recurring_charges refuses a future since in its own words
  Given a built store and a fixed clock
  When the client calls recurring_charges with since 2099 and no until
  Then the result is isError with "since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since"

Scenario: SCENARIO-11 — anomalies returns the anomalies --json document
  Given a built store, a fixed clock and the same since, until, accounts and currency
  When the client calls anomalies
  Then every field but warnings is byte-equal to quarry anomalies --json, and warnings equal the CLI's after the §4.1 wording map

Scenario: SCENARIO-11b — anomalies refuses a future since in its own words
  Given a built store and a fixed clock
  When the client calls anomalies with since 2099 and no until
  Then the result is isError with "since 2099 is after today; anomalies lists charges up to today only, so pass an earlier since"

Scenario: SCENARIO-13 — the server describes all eight tools
  Given an MCP client has started quarry mcp
  When the client initializes and lists tools
  Then it sees the ruled instructions and eight tools with the ruled descriptions and per-parameter descriptions, the query description sends spending and income totals to the named tools, and quarry mcp --help lists the eight tools
```

---

## Sizing

Sizing pass (architect, opus) 2026-10-03 — 16 IDs → 7 runs. Order is binding: S01 → S02 → S03 → S04 → S09 → S10 → S13. S02's Handoff lists window, account and OpenFaultOther classification under Left unbuilt (S03/S04 follow immediately). Full report in `## Sizing notes`.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN (opus) — 4 batches: shared primitives (word-table enums, accountFilterDocument, nativeOf home); warnings move with command word; spend+cashflow documents; recurring+anomalies documents. Green on arrival (literal goldens written in run A before the move) |
| SCENARIO-02 | OWNS A RUN (opus) — 3 batches + CLI-vs-MCP equality harness: WithClock; spending schema/registration; handler (window, config-only-when-currency-absent, warnings with tool word). Absorbs 06, 07, 12 |
| SCENARIO-06 | FOLD into SCENARIO-02 |
| SCENARIO-07 | FOLD into SCENARIO-02 |
| SCENARIO-12 | FOLD into SCENARIO-02 |
| SCENARIO-03 | OWNS A RUN (sonnet) — 2 batches: WindowError carries parts (CLI Error() byte-identical); mcp windowRefusal wording incl. charge-window variant + class line |
| SCENARIO-04 | OWNS A RUN (sonnet) — 3 batches: account + store refusal parts in report; logLine classification; flip query_refusal_test.go:99-101 pin + OpenFaultOther table. Absorbs 05 |
| SCENARIO-05 | FOLD into SCENARIO-04 — tick covers 5 tools at S04; S09/S10 add cash_flow, recurring_charges, anomalies rows |
| SCENARIO-09 | OWNS A RUN (sonnet) — 3 batches: cash_flow tool; mcp cap helper + spending/cash_flow cap lines; S14 rows. Absorbs 08, 14 |
| SCENARIO-08 | FOLD into SCENARIO-09 |
| SCENARIO-14 | FOLD into SCENARIO-09 |
| SCENARIO-10 | OWNS A RUN (opus) — 4 batches, Runs B1(3) | B2(1): recurring_charges (Now, ParseChargeWindow with tool word, cap); S10b refusal; anomalies (cap, checked/not_judged uncut, S11b line); all-tools table rows. Absorbs 10b, 11, 11b |
| SCENARIO-10b | FOLD into SCENARIO-10 |
| SCENARIO-11 | FOLD into SCENARIO-10 |
| SCENARIO-11b | FOLD into SCENARIO-10 |
| SCENARIO-13 | OWNS A RUN (sonnet) — 2 batches: 3a per-parameter descriptions, instructions, query description, 8-tool tools/list pin (rename test); mcp Long + pin |

## BDD Acceptance Progress
- [x] SCENARIO-01: CLI output is unchanged after the analysis documents and warnings move to shared code — `cmd/quarry/run_analysis_documents_test.go` `Test_run_prints_spend_cashflow_recurring_and_anomalies_byte_for_byte`
- [x] SCENARIO-02: spending returns the spend --json document — `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_returns_the_spend_json_document`
- [x] SCENARIO-06: currency follows the config only when it is absent — delivered by SCENARIO-02 — `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_reads_the_config_only_when_currency_is_absent`
- [x] SCENARIO-07: spending's warnings use the tool's name and keep "run quarry sync" — delivered by SCENARIO-02 — `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_words_its_warnings_with_the_tool_name`
- [x] SCENARIO-12: "today" is read on every call — delivered by SCENARIO-02 — `internal/mcp/spending_test.go` `Test_spending_reads_today_once_at_the_start_of_every_call`
- [x] SCENARIO-03: spending refuses a bad window in MCP words — `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_refuses_a_bad_window_in_mcp_words`
- [x] SCENARIO-04: an unknown or ambiguous account is refused without its name reaching stderr — `cmd/quarry/run_mcp_spending_test.go` `Test_run_mcp_spending_refuses_an_account_without_its_name_on_stderr`
- [x] SCENARIO-05: a DuckDB read fault logs only the withheld store line — delivered by SCENARIO-04, covering query, describe_schema, sync_status, data_quality and spending (cash_flow, recurring_charges and anomalies rows are added by S09/S10) — `cmd/quarry/run_mcp_store_faults_test.go` `Test_run_mcp_logs_only_the_withheld_line_for_a_store_read_fault`
- [x] SCENARIO-09: cash_flow returns the cashflow --json document — `cmd/quarry/run_mcp_cash_flow_test.go` `Test_run_mcp_cash_flow_returns_the_cashflow_json_document`
- [x] SCENARIO-08: a list over 500 entries is cut with the cap warning — delivered by SCENARIO-09 — `cmd/quarry/run_mcp_spending_cap_test.go` `Test_run_mcp_spending_cuts_a_list_over_500_rows_with_the_cap_warning`
- [x] SCENARIO-14: absent, null and empty arguments reach spending and cash_flow with the by default applied — delivered by SCENARIO-09 — `internal/mcp/server_test.go` `Test_a_by_tool_call_with_absent_null_or_empty_arguments_applies_the_by_default`
- [ ] SCENARIO-10: recurring_charges returns the recurring --json document
- [ ] SCENARIO-10b: recurring_charges refuses a future since in its own words
- [ ] SCENARIO-11: anomalies returns the anomalies --json document
- [ ] SCENARIO-11b: anomalies refuses a future since in its own words
- [ ] SCENARIO-13: the server describes all eight tools

---

## Sizing notes

There are 7 architect runs for 14 approved IDs, which become 15 once S10 is split. Every unit is code-first: nothing touches the mandatory test-first set, and there is no write guard, atomic adapter or bug fix. No unit has a new `report.Store` port method, so no fake or adapter fan-out. The feature packages touched are `report` and `report/document`. `mcp` and `cli` are delivery peers, as in 3a.

#### Sizing table (build order)

| # | Scenario | Verdict | Size |
|---|---|---|---|
| 1 | S01 extraction | OWNS A RUN, opus | 4 batches, 1 feature pkg (report/document) + cli. (1) Shared primitives: the `name` column of the word tables becomes a typed enum list in store/report (like `finding.Types()`), plus `accountFilterDocument(s)` and a single home for `nativeOf` (money or document) that `accountsFXWarnings` also imports. (2) The warnings move: empty_window, fx_warning, the nouns, and the four `*Warnings` composers taking the command word. (3) spend and cashflow documents. (4) recurring and anomalies documents, with `recurringStatus` and `anomaliesBaselineWord` exported from document for the text renderers. The cli copies are deleted in the same batch that moves them. |
| 2 | S02 spending **+ S06 FOLD** (the currency/config branch is a few lines of the same handler) **+ S07 FOLD** (verbatim pins of lines S02 already produces) **+ S12 FOLD** (a two-call test of the per-call clock S02 introduces) | OWNS A RUN, opus | 3 batches + harness. (1) `WithClock`, read once per call, plus the S12 midnight test. (2) spending schema and registration: `by` enum from the word table, `currency` enum with no default, the ruled description and parameter descriptions, the absent/null row, and rows in the all-tools tables. (3) Handler: pointer since/until, then window, then config only when currency is absent (S06 matrix, stderr `run quarry spend to see why`), then Spend, then `SpendWarnings(…, "spending")` (S07 pins). The acceptance test is the CLI-vs-MCP equality harness. Runs: A \| B1(3) \| V. |
| 3 | S03 window refusals in MCP words | OWNS A RUN, sonnet | 2 batches. (1) report: `WindowError` carries its parts, `Error()` stays byte-identical, and each kind is pinned in its own package. (2) mcp: wording for every kind, including the charge-window variant S10 needs, plus the `withLog` class line, through spending. Not LIGHT: 6 ruled lines. |
| 4 | S04 account refusals **+ S05 FOLD** (OpenFaultOther is a second arm in the same `logLine` rework) | OWNS A RUN, sonnet | 3 batches. (1) report: account refusal parts (kind, arg, ids) and `RefusalError` store parts (fault, `~` path); CLI text unchanged. (2) mcp: `logLine` classifies account refusals into two class lines, OpenFaultOther (open- or statement-time) into the withheld line, everything else verbatim; plus the MCP unknown-account line `call describe_schema to list the accounts`. (3) Flip the pin at `internal/mcp/query_refusal_test.go:99-101` and add the per-tool OpenFaultOther table. |
| 5 | S09 cash_flow **+ S08 FOLD** (cap: cut a slice, append a line) **+ S14 FOLD** (test rows only, once both `by` tools exist) | OWNS A RUN, sonnet | 3 batches. (1) cash_flow tool plus its rows in the all-tools tables. (2) mcp cap helper plus the spending and cash_flow cap lines: bounds 500/501, totals uncut, cap line last. (3) S14 absent/null/`{}` rows for spending and cash_flow at `internal/mcp/server_test.go:163`. |
| 6 | S10a recurring_charges document **+ S10b FOLD** (future since refused in recurring_charges' words) **+ S11 FOLD** (anomalies, a twin handler) | OWNS A RUN, opus | 4 batches, Runs B1(3) \| B2(1). (1) recurring_charges: `Now` from the per-call clock, `ParseChargeWindow` with the tool word, cap (`series`). (2) S10b refusal row through S03's wording. (3) anomalies: handler, cap (`anomalies`; `checked` and `not_judged` uncut), and its own future-since line pinned. (4) Rows in the all-tools tables for both tools. |
| 7 | S13 tools/list (8 tools), parameter descriptions on the 3a tools, instructions, query description, `quarry mcp` Long | OWNS A RUN, sonnet | 2 batches. (1) mcp: the 5 parameter descriptions on the 3a tools, §3.7 instructions, §3.8 query description, and the 8-tool tools/list pin, with the test renamed. (2) The cli mcp Long §3.6 and its pin. Not LIGHT: more than 3 ruled lines. Not a FOLD: it needs all 8 tools to exist, and S10 is already at 4 batches. |

**SPLIT check.** Every unit has at most 4 batches and one feature package. p90 is about 2,785k IE across the 72 past units with numeric cost (median 1,522k). I computed it from every `docs/specifications/*/METRICS.md` unit table, because `feature-metrics.py` reports one feature only. Twins, all under p90:
- 3a S01 at 1,585k for S01.
- 3a S03(+04,+05) at 1,483k and 3a S06 at 2,270k for S02 and S04.
- 2e SCENARIO-01 at 3,032k is the only recurring/anomalies twin above p90, but that was the whole store-side build. S10 has no store work.

**Fold ticks.**
- S06, S07 and S12 are delivered by S02.
- S05 by S04.
- S08 and S14 by S09.
- S10b and S11 by S10.

Each folded scenario keeps its own acceptance test on an `Acceptance test (SCENARIO-NN, folded):` line.

#### Ordering ruling (binding)

The order is S01 → S02 → S03 → S04(+S05) → S09 → S10 → S13.

S03 and S04 come straight after S02, with no other tool in between. They cannot come first, because their `When` calls `spending`. This is how "before or with spending" is met: the leak exists only on the branch for the span of S02, never on main.

S02's Handoff must list window, account and OpenFaultOther classification under *Left unbuilt*. Otherwise its checkpoint reads the verbatim stderr log as a BLOCKER and costs a fix pass. This does not change any size.

#### Seams

- **Window refusal parts.** `report.WindowError` gets exported parts: kind (not-a-date, since-after-today, charge-since-after-today, since-after-until, until-before-default), bound, value, the other value, the default since, and the command.
  - `Error()` keeps today's `--since` text byte-identical.
  - The parse code stops hard-coding `"--since"`/`"--until"` (`window.go:66,74,97`).
  - mcp owns one `windowRefusal` wording function and the class line `refused the call's since or until; details went to the client only`.
  - `ParseChargeWindow`'s command is a part, and MCP passes the tool name.
- **Account refusal parts.** `unknownAccountRefusal`/`ambiguousAccountRefusal` (`refusal.go:62-70`) carry kind, arg and ids.
  - No non-test code in cli or cmd type-switches on `report.RefusalError`; the only one is `internal/mcp/result.go:54`. So a distinct type is free for the CLI, whose RunE wraps the error in `runtimeError` either way.
  - Check `report/refusal_test.go` for type pins.
- **Store refusal parts.** `RefusalError` must carry the `~` path (and fault) as a part. `logLine` has no `home`, and the withheld line `cannot read the store at ~/…; details went to the client only` needs the `~` form.
  - The classification is per fault: OpenFaultOther from `storeRefusal` (`refusal.go:49-50`), whether open-time or statement-time.
  - NotDuckDB, Permission and Locked stay verbatim. I checked `internal/store/open.go:40-44`: their `UnreadableReason` is a fixed phrase, so product row 8 is sound.
- **CLI→MCP warning vocabulary map.** It is the existing `command` word argument of the left-out warning builders (`internal/cli/empty_window.go:11-37`), carried into the moved document composers, e.g. `document.SpendWarnings(s, command)`. CLI passes spend/cashflow/recurring/anomalies; MCP passes tool names. Nothing else in §4.1 differs, so no other table exists.
  - The cap line is composed in mcp (3a `listCutWarning` precedent).
  - The equality harness substitutes words on those two templates only.
- **Clock.**
  - `mcp.WithClock(func() time.Time)`, default `time.Now` set in `NewServer`, read once at handler start and passed to `ParseWindow`/`ParseChargeWindow` and to `Request.Now`.
  - Tests inject it through `newMCPServe(info, opts ...mcp.Option)`, which already appends options (`cmd/quarry/run.go:137`).
  - With a default in `NewServer` there is no shipped wiring line to delete, which departs from the product text "wired in newMCPServe". The mutation check is therefore: hoist the clock read from per call to construction, which reddens S12.
- **Cap placement.** Caps live in mcp (3a binding). The cut is applied to the document's exported slice **after** it is built, so totals, `checked` and `not_judged` stay uncut whatever the builders compute. The §5 line is appended last; `document` and `report` are unchanged. One generic helper is built in S09, and S10 reuses it.
- **Equality harness.** It is a cmd/quarry test helper, because that is the only package wiring both surfaces. It is built in S02 and reused by S09 and S10 (+S11). It:
  - runs `quarry <cmd> --json` via `runWith` with a fixed `Env.Now`;
  - starts `startMCP` (`cmd/quarry/run_mcp_test.go:144`) with `ServeMCP = newMCPServe(info, mcp.WithClock(fixed))`;
  - compacts the CLI stdout, removes `warnings` from the raw bytes, and compares against the tool's `TextContent` (the SDK map loses key order);
  - maps warnings word for word.

  The fixture needs a left-out account and before-first-rate amounts (Rule 1), and must stay under the cap.
- **All-tools tables.** Each tool scenario adds its row to:
  - `cmd/quarry/run_mcp_test.go:87` `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc` (the name says "four"; S13 renames it);
  - `cmd/quarry/run_mcp_no_store_test.go`;
  - `internal/mcp/timeout_test.go`;
  - `internal/mcp/query_helpers_test.go`;
  - `internal/mcp/server_test.go:163` (spending and cash_flow only);
  - the S05 OpenFaultOther table.

  `configRefusalLog` (`internal/mcp/data_quality.go:14`) is a findings-only constant today; the config stderr line becomes per tool.

#### Risks and flags

1. **S10 is a mandatory SPLIT.** Its title joins returning a document and refusing a future since, and no single call does both. Seam: S10a is the document equality, S10b is `since 2099 is after today; recurring_charges lists charges up to today only, so pass an earlier since`. Fold S10b back into the S10 run with its own acceptance test.
   - **Gap:** the anomalies future-since line (§6.2) has no scenario. Pin it as a ruled string in S10's anomalies batch, or add S11b folded into S10.
2. **The S05 tick covers 5 tools.** When S04 ticks it, the table covers the 4 3a tools plus spending; S09 and S10 add the other 3 rows. Say so on the tick line so the gate's `spec-check.py --run` is no surprise.
3. **S01's acceptance test is green on arrival** because the move is behaviour-neutral (3a S01 precedent). Its pins must be literal goldens written in run A before anything moves, covering text-mode stderr warnings too. `run_shared_documents_test.go` is the only test that checks key order; the `JSONEq` pins pass after a reorder (3a trap).
4. **`currency` enum is exact-case.** `money.ParseCurrency` is case-insensitive, so the schema enum alone enforces case. This deviation is recorded and should be pinned with a `cad` row giving the SDK refusal.
5. **since/until need `*string` fields** so that `""` gives a window refusal and absent gives the default. `null` must stay an SDK type refusal; `absentNullArguments` rewrites `null` arguments, not `null` properties, so verify it in S02.
6. **No infeasible or contradictory approved behaviour found** beyond item 1. Product row 8 is consistent with Rule 4 (see store refusal parts above), and describe_schema does list accounts, so the unknown-account advice can be followed.

Files: /Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/window.go, internal/report/refusal.go, internal/mcp/result.go, internal/cli/empty_window.go, internal/cli/fx_warning.go, internal/store/open.go, cmd/quarry/run.go, cmd/quarry/run_mcp_test.go, internal/mcp/server_test.go, internal/mcp/data_quality.go.
