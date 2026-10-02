# Specification: Phase 3a — MCP server core

<!-- spec-check: v1 -->

## Intent & Goal

**Primary Goal**: `quarry mcp` runs a local stdio MCP server so Claude (desktop or any MCP client) can read quarry's store directly: `describe_schema` to learn the tables, views and conventions, `query` for read-only SQL, `sync_status` for freshness, `data_quality` for the findings worklist. Every tool returns the same JSON document the matching CLI `--json` prints (one owner per document), so the CLI and MCP never disagree (PRD:108, PRD:129).

**Secondary Goals**: read-only and bounded — SQL row cap 500 the model cannot raise, 30 s per-call timeout, findings and items capped with warnings; stdout carries only protocol traffic; the server never holds the store open between calls and reads config per call, so `quarry sync` while serving is seen on the next call.

**Out of Scope**:
- Named analysis tools `spending`, `cash_flow`, `recurring_charges`, `anomalies`, `search_transactions` (Phase 3b; their `--json` documents move to `internal/report/document` then).
- Skill, references, plugin manifest, MCP client config, use-case eval (Phase 3c).
- `net_worth`, `acb` (Phase 4). No stubs.
- **Redaction on import** — user decision 2026-10-02: skipped for now. Open security debt: payee, memo, split-note, account and category text reaches the MCP client as Quicken holds it (e.g. a payee carrying an 18-digit cheque number, `docs/specifications/phase2e-recurring-anomalies/STATE.md:59`). One honest line in `quarry mcp` Long help; nowhere else.
- Config keys `mcp.max_rows`, `mcp.query_timeout` — reserved by name only, not built.
- A `currency` parameter on any of the four tools.

**Business Rules**: see below and `## Surface & Copy` (product-vision Phase 1 ruling, binding).

## Business Rules & Invariants
- Rule 1: Each tool's structured result is the CLI `--json` document for that data, built by the same constructor in `internal/report/document`. Only encoding (compact vs indented) and next-step warning lines that name a CLI flag may differ.
- Rule 2: `internal/mcp` is a delivery peer of `internal/cli`: cli never imports mcp, mcp never imports cli. `cmd/quarry` wires `cli.Env`'s MCP hook to `mcp.NewServer` using the existing report factory and config loader.
- Rule 3: The server never holds the store open between tool calls; config is loaded per call.
- Rule 4: stdout carries MCP JSON-RPC only. stderr carries only the TTY hint and one `quarry: mcp: <tool>: <error text>` line per call that ends `isError`. SQL text, row values and finding items never reach stderr.
- Rule 5: `query` returns at most 500 rows (hard max; `limit` 1..500, default 500). Every tool call has a 30 s deadline (named constant, injectable for tests). Deadline is classified distinctly from cancel all the way up the error chain.
- Rule 6: `data_quality` returns at most `limit` findings (default 50, 1..500) and at most 25 items per finding; overflow is reported only through `warnings`, `counts` never trimmed.
- Rule 7: Warnings go in the document's `warnings` array (`[]`, never null). Refusals are `isError: true` with one text line, no "quarry: " prefix, no document.
- Rule 8: Server exit: stdin EOF, SIGINT/SIGTERM, stdout write failure (EPIPE) → exit 0 (deliberate deviation from read commands' "<cmd> interrupted" exit 1 — stopping a server is its normal end). `$HOME` unset → exit 1 at start. Positional args or `--json` → exit 2.
- Rule 9: Never use `mcp.StdioTransport` (reads `os.Stdin`/`os.Stdout` directly, bypasses `Env`); use `mcp.IOTransport` over `Env` streams. Never pass `slog.Default()` as SDK logger.
- Rule 10: Tool results set `StructuredContent: json.RawMessage(compactDoc)` with explicit permissive `OutputSchema {"type":"object"}` and `Out = any` — the SDK's typed-output path re-marshals through `any`, alphabetising keys and losing int64 precision above 2^53. Every tool has an explicit hand-written `InputSchema` (inference gives no bounds, enums, minLength or defaults); those schemas are contract.

---

## Triage Brief

### Headline findings
1. No MCP code exists. No `quarry mcp` in `internal/cli/root.go:25-34`. SDK (`github.com/modelcontextprotocol/go-sdk`) absent from go.mod/go.sum/module cache. `go.mod:3` = go 1.27.1. DuckDB is cgo. SDK Go/cgo constraints unverified offline.
2. Redaction not implemented anywhere (no mask/redact in internal/ or cmd/). USER DECISION: skip redaction for now; record as open security debt. Payee/memo/split-note text reaches Claude unmasked.
3. Every `--json` shape is an unexported type in `internal/cli`: `json_spend.go:9 spendDocument`, `json_cashflow.go:6`, `json_recurring.go:7`, `json_anomalies.go:7`, `json_findings.go:10 findingsListDocument`, `json_status.go:11 statusDocument`, `json_sql.go:12 sqlDocument`, `json_accounts.go:8 accountsDocument`. `report` returns domain structs, not documents. PRD:129 says MCP tools return the CLI shapes. To mirror, documents/renderers must move or be exported out of `cli`, or MCP duplicates them (drift risk vs PRD:108 "CLI and MCP never disagree").
4. Policy shaping tool output lives in `cli`: warnings (`internal/cli/empty_window.go:11 leftOutWarnings`, `:42 appendEmptyWindowWarning`; `internal/cli/fx_warning.go:30 unconvertedWarnings`, `:62 accountsFXWarnings`; `spendWarnings` `internal/cli/spend.go:90`), `--currency` resolution (`internal/cli/currency.go:68 resolve`). Window parsing is `report.ParseWindow` `internal/report/window.go:45`.

### Tool-to-library map
| MCP tool | Entry point | --json owner | Status |
|---|---|---|---|
| query | `report.Server.Query(ctx, query, limit)` `internal/report/query.go:21` → `report.QueryResult{Truncated}` (asks limit+1) | `sqlDocument` json_sql.go:12 | Exists |
| spending | `Server.Spend(SpendRequest{Window,By,Accounts,Currency})` spending.go:62 | spendDocument | Exists (slice 3b+) |
| cash_flow | `Server.CashFlow` cashflow.go:58 | cashFlowDocument | Exists (later slice) |
| recurring_charges | `Server.Recurring` recurring.go:199 | recurringDocument | Exists (later slice) |
| anomalies | `Server.Anomalies` anomalies.go:90 | anomaliesDocument | Exists (later slice) |
| data_quality | `Server.Findings(FindingsRequest{Ignore,Status,Type})` findings.go:53; Ignore from config | findingsListDocument | Exists as `quarry findings` |
| sync_status | `Server.Status(ctx)` report.go:53 + `report.CountFindings(st, ignore)` (status.go:41) | statusDocument | Exists as `quarry status` (validation, staleness, FX coverage) |
| describe_schema | none. DDL hard-coded `internal/store/duckstore/schema.go` (tables from :13, views :144,:171,:199). No introspection, no Store port method. Conventions text only in `internal/cli/sql.go:30-60` Long help | n/a | Must be built |
| search_transactions | none; no transaction-listing read on `report.Store` | n/a | Must be built (later slice) |
| net_worth, acb | none; Phase 4 | n/a | Out of Phase 3 |

### Wiring (reuse, don't duplicate)
- Read port `report.Store` (`internal/report/store.go`, 7 methods). `report.NewServer`, `WithStore`, `WithHome` (`internal/report/report.go:13-40`).
- `cmd/quarry/run.go:91 newReportFactory` builds `duckstore.New(storeDirUnder(home))` → `report.NewServer`; `cmd/quarry/run.go:123 newConfigLoader`; `defaultEnv` `cmd/quarry/run.go:160`. `cli.Env` has `NewReport`, `LoadConfig`, `Now` (`internal/cli/run.go:40-48`). ADR: `docs/adr/003-duckstore-owns-the-read-side.md`.
- Config warnings printed to stderr via `printConfigWarnings` `internal/cli/output.go:67`.

### Caller table
| Symbol | Caller | Via |
|---|---|---|
| Server.Spend | internal/cli/spend.go:69 | LSP |
| Server.CashFlow | internal/cli/cashflow.go:86 | LSP |
| Server.Recurring | internal/cli/recurring.go:74 | LSP |
| Server.Anomalies | internal/cli/anomalies.go:67 | LSP |
| Server.Query | internal/cli/sql.go:99 | LSP |
| Server.Findings | internal/cli/findings.go:90 | LSP |
| Server.Status | internal/cli/status.go:34 | LSP |
| Server.Accounts | internal/cli/accounts.go:37 | LSP |
| Env.NewReport / ReportFactory | internal/cli/root.go:27-33 (7 read cmds), internal/cli/output.go:11 openReport | grep |
| newReportFactory | cmd/quarry/run.go:91, wired defaultEnv cmd/quarry/run.go:160 | grep |
| --json strings | cmd/quarry/run_*_json_test.go, internal/cli/*_json_test.go | grep |
All non-test callers in internal/cli, one each.

### Read-only + row cap
- `internal/platform/duckdb/duckdb.go:72-73` DSN `access_mode=READ_ONLY&enable_external_access=false&autoload_known_extensions=false&autoinstall_known_extensions=false&lock_configuration=true`; `OpenReadOnly` (:79) `SetMaxOpenConns(1)`.
- `duckstore.Store.Query` (`internal/store/duckstore/query.go:17`) opens per call, maps failures to `ErrReadOnlyQuery`, `ErrExternalAccess`, `ErrEmptyQuery`, `ErrQueryInterrupted`, `UnprintableValueError`, `QueryError` (`queryRefusal` :36).
- Row cap `QueryTable(ctx, query, maxRows)` `internal/platform/duckdb/table.go:48`. CLI default 500 (`internal/cli/sql.go:16 defaultSQLLimit`); `--limit 0`/`--csv` = no cap. Cap bounds output, not compute.
- NO query timeout exists; only ctx cancellation (SIGINT). Timeout behaviour of ctx-cancel on DuckDB unverified.
- Refusal copy is cli's `queryFailure` (`internal/cli/sql.go:162`) "quarry sql only reads…" — MCP needs own wording or shared mapper.

### Currency
`money.ParseCurrency`: CAD, USD, native. Default config.Config.Currency then CAD (`internal/cli/currency.go:56-69`). PRD MCP currency = CAD|USD.

### Packaging
No `.claude-plugin`, `plugin.json`, `.mcp.json`, product SKILL.md outside `docs/prior-art/**`. Prior-art layouts: `docs/prior-art/dweekly/plugin/.claude-plugin/plugin.json`, `docs/prior-art/hardkoded/plugins/quicken/.claude-plugin/plugin.json`.

### Triage-proposed slicing (redaction dropped by user)
3b share contracts (extract --json docs/warnings/currency out of cli) → 3c `quarry mcp` core (SDK, stdio, query cap+timeout, sync_status, data_quality, describe_schema) → 3d named tools (spending, cash_flow, recurring_charges, anomalies, search_transactions) → 3e skill + plugin + use-case eval.

### Open questions (triage)
1. Reuse --json shapes literally vs MCP-specific output.
2. MCP warnings channel (no stderr the model sees): `warnings` array vs error.
3. query timeout value, hard max cap, model-raisable cap, multi-statement refusal.
4. net_worth/acb out of Phase 3 (PRD says Phase 4).
5. search_transactions: MCP-only or CLI twin.
6. Plugin location/name; binary path in MCP config.
7. Phase 3 gate evidence (question set + ground truth).
8. SDK min Go version / cgo — verify by `go get` at planning.

## Product Verdict

**SHIP WITH CHANGES** (product-vision, Phase 1, 2026-10-02). Accepted changes, all folded into the scenarios and rules above:
1. Shared-document extraction (sql, status, findings + warning builders, query-error classifier, conventions constant) folded in as SCENARIO-01; MCP structured output is those documents verbatim.
2. Query timeout must be proven against duckdb (SCENARIO-06); deadline kept distinct from cancel. Sizing pass probe: 1 s deadline on a huge cross join returned at 1.50 s with `DeadlineExceeded` — feasible.
3. Never-hold-the-store-open + per-call-config invariant, with the sync-while-serving scenario (SCENARIO-14).
4. `data_quality` bounded: 50 findings default (500 max), 25 items per finding, overflow via `warnings`.
5. stdout protocol-only with a test; stderr only TTY hint + per-error log line.
6. `query` cap hard 500; `mcp.max_rows` / `mcp.query_timeout` reserved by name only.
7. Exit-0-on-signal deviation and redaction doc debts recorded (Rule 8, `### Changes to existing surfaces`).

## Surface & Copy

Product-vision Phase 1 ruling, verbatim (headings demoted). Binding: developers implement these strings verbatim.

I read `docs/initial-prd.md` (MCP §190-207, Security, exit codes, phases), the triage brief, and these files: `internal/cli/{root,sql,json_sql,status,json_status,findings,json_findings,errors,output}.go`, `internal/report/refusal.go`, `internal/report/findings.go:55-75`, `internal/store/duckstore/{query,schema}.go`, `internal/platform/duckdb/table.go`, `internal/store/store.go:437-454`, `internal/config/config.go`, `cmd/quarry/run.go:80-200`.

#### 1. Slice boundary and the shared-documents question

The boundary is right. `query`, `describe_schema`, `sync_status` and `data_quality` are useful together even before the named tools exist:
- The rules that matter for spending and income already live in the views. `v_spending` and `v_cash_flow` leave out transfers and convert per split, so `query` over those views, guided by `describe_schema`, is Q4-correct.
- `sync_status` gives freshness. `data_quality` gives the cleanup worklist.
- The named tools, the skill and plugin, and `net_worth`/`acb` (Phase 4) wait, as proposed.

**Ruling on CLI vs MCP output shapes.** MCP output is the CLI `--json` document, built by the same constructor. It is not a parallel shape. PRD:108 ("never disagree") and PRD:129 ("tools return the CLI shapes") agree once each document has one owner. Two things are allowed to differ:
- **Encoding.** MCP text content is compact JSON; CLI is 2-space indented. Field names, order and null/[] rules are the same.
- **Warning lines that tell the reader what to do next.** The CLI says "pass --limit 0"; the model can't, so it gets its own line.

**Fold the extraction into this slice as Scenario 1**, a behaviour-neutral move that the existing `--json` tests pin. Don't make it a separate prior slice. Move only what this slice gives a second caller:
- `sqlDocument` with `jsonSQLCell` and `jsonFloat`
- `statusDocument` with `newStatusDocument`, the `cannotTellIgnored` warning and `statusIgnore`'s policy
- `findingsListDocument` with its item and entry builders, and `unmatchedIgnoreWarnings`
- the store-error classification behind `queryFailure`
- the conventions paragraph (see §2.6)

The spend, cashflow, recurring and anomalies documents move in 3b, when they get their second caller. Extracting them now leaves them with no consumer. The destination package is the architect's call. The constraint: `internal/cli` and the new `internal/mcp` both call it, and neither re-derives a field.

#### 2. Literal surface

##### 2.1 `quarry mcp`

- Use: `mcp`
- Short: `Serve quarry's store to Claude over MCP (stdio)`
- Long:
```
Run quarry as a local MCP server for Claude and other MCP clients. The
client starts it and talks to it over stdin and stdout; quarry opens no
network port. Add it to your client's MCP config with the command
"quarry" and the argument "mcp".

The server reads quarry's store; it never runs quarry sync, never prunes
snapshots and never touches Quicken. Each request reads the store as it
is then, so after you run quarry sync the client sees the new data
without a restart. SQL runs read-only and returns at most 500 rows.

Payee names, memos and category names reach the client as Quicken holds
them: quarry does not yet mask account or card numbers written in them.

Tools: describe_schema, query, sync_status, data_quality.
```
- Example: none.
- Flags: none.
- Args: `noArgs` → `mcp takes no arguments`, exit 2.
- `--json`: usage error, exit 2: `mcp always speaks JSON on stdout; drop --json`.

**Server identity.** `Implementation{Name: "quarry", Version: <buildVersion(info)>}`. Use the same source as `store_info.quarry_version`; when it is empty, send `(devel)`.

**Server `instructions`** (sent at initialize; this is copy):
```
quarry serves David's Quicken Classic for Mac data from a local, read-only
store. Call sync_status first and tell the user how old the snapshot is
(snapshot.taken_at). Call describe_schema before writing SQL: its
conventions say which views already leave out transfers and how amounts,
signs and currencies work. Every number you report must come from a tool
result; never estimate. quarry cannot change data: fixes are made in
Quicken, then the user runs quarry sync.
```

**Exit codes:**

| Outcome | stderr | exit |
|---|---|---|
| Client closes stdin (EOF) | nothing | 0 |
| SIGINT or SIGTERM | nothing | 0 |
| stdout write fails (client gone) | nothing | 0 |
| `$HOME` unset, checked at start | `quarry: cannot find your home directory ($HOME is not set); set HOME, then run quarry mcp again` | 1 |
| Positional args or `--json` | the usage line above | 2 |
| Store missing at start | nothing; server starts, each call refuses (§4) | — |

- **SIGINT/SIGTERM exit 0 is a deliberate deviation** from the read commands' "<cmd> interrupted", exit 1. Stopping a server is its normal end. Record this in the spec so the gate doesn't flag it.
- A broken stdout pipe means the client went away, so it is exit 0, not 1.
- `$HOME` refuses at start because nothing per-call can fix it. It reuses `resolveHome`'s copy verbatim.

**stdin is a TTY.** Print one stderr line, then keep serving:
`quarry: mcp: this is an MCP server for Claude and other MCP clients; it reads JSON-RPC on stdin. Press Ctrl-D to stop.`

**stdout carries the MCP protocol and nothing else.** No path in `internal/mcp` may reach `cmd.OutOrStdout()`, `emit`, `writeResult` or `printConfigWarnings`. Make it a test: run the server, then assert every stdout line parses as JSON-RPC.

**stderr** has exactly two kinds of line:
1. the TTY hint;
2. one line per tool call that ends `isError`: `quarry: mcp: <tool>: <error text>`. This is the after-the-fact log that Claude Desktop saves to its MCP log.

SQL text, row values and finding items never go to stderr. Successful calls write nothing.

##### 2.2 Store and config lifetime (invariant; give it a scenario)

- The server **never holds the store open between tool calls**. Each call opens, reads and closes. `duckstore` already does this per `Query`; it now becomes a rule.
- Config is **loaded per call**, so an edit to `findings.ignore` or `reporting.currency` takes effect without a restart.
- Acceptance scenario: start `quarry mcp`, call `sync_status`, run `quarry sync --from <snapshot>`, call `sync_status` again. The second call shows the new `store.built_at` with no restart.

##### 2.3 Result envelope (all four tools)

- **Success:** `structuredContent` is the document. `content` is one `TextContent` holding the same document as compact JSON. `isError` is false.
- **Warnings** go in the document's own `warnings` array, never in `isError`. An empty array is `[]`, never null. Paths in warnings are absolute, like `--json`.
- **Refusal:** `isError: true` with one `TextContent` line. There is no "quarry: " prefix and no document.
  - Store refusals reuse `report.storeRefusal`'s lines verbatim, with `~` form, as on stderr.
  - Paths inside documents are absolute.
- **Architect to verify offline:** whether the SDK checks `structuredContent` against an output schema it infers from Go types. If it does, `*string` nulls and `[][]any` rows must pass. If they can't, declare a permissive output schema. Do not drop fields.

##### 2.4 Tool: `query`

**Description:**
```
Run one read-only SQL query (DuckDB dialect) against quarry's store and
return its columns and rows. Call describe_schema first for the tables,
views and conventions. For spending and income use v_spending and
v_cash_flow: they already leave out transfers between the user's own
accounts. Returns at most `limit` rows (default 500, the most allowed);
aggregate in SQL rather than paging through rows. The store cannot be
changed, and other files, databases and extensions are off.
```

**Input:**

| param | type | required | default | bounds |
|---|---|---|---|---|
| `sql` | string | yes | — | minLength 1 |
| `limit` | integer | no | 500 | 1..500 |

The model cannot raise the cap. 500 is the hard maximum (Q7). It is a named constant, beside the timeout constant. Reserve config keys `mcp.max_rows` and `mcp.query_timeout` by name only. **Do not build them in 3a**: they add config-parse failure copy for no current ask.

**Output:** `sqlDocument` verbatim, with fields `columns[{name,type}], rows, row_count, limit, truncated, warnings`. Cells use the same rules as `jsonSQLCell`: DECIMAL as DuckDB text, DATE as `YYYY-MM-DD`, NaN as `"nan"`.

**Cap hit:** `truncated: true`, plus the warning
`returned the first 500 rows; the query has more; aggregate or filter in SQL to see the rest`
<n> via humanize.Count: "1 row" at limit 1, "<n> rows" otherwise (mid-feature ruling, SCENARIO-03).
The number shown is the effective limit.

**Multiple statements:** no refusal specific to MCP. Behaviour is whatever the driver does for `quarry sql`, and one test pins it for both surfaces. Add the sentence `Send one statement; if you send several, only the last one's rows come back.` to the description **only if** that test shows last-statement semantics. Otherwise the driver's error reaches the model as `query failed: <reason>`.

**Timeout: 30 s per call, applied by the MCP server only.** The CLI user has Ctrl-C; the model doesn't.

Two requirements **block this part of the slice**:
1. Store and duckstore classification must keep `context.DeadlineExceeded` separate from cancel. Today `queryRefusal` folds every `ctx.Err()` into `ErrQueryInterrupted`.
2. An acceptance test runs a deliberately slow query (for example a large `range()` cross join) and shows the call returns within a few seconds of the deadline.

If duckdb-go cannot interrupt on deadline, the timeout copy is false. Stop and come back to product; do not ship it.

##### 2.5 Tool: `sync_status`

**Description:**
```
Report how fresh quarry's data is: the snapshot the store was built from
and when it was taken, the dates its transactions cover, the checks sync
ran (balances reconciled to Quicken, splits, transfers), open findings,
and Bank of Canada rate coverage. quarry cannot refresh the data; if it
is old, ask the user to run quarry sync.
```

- Input: none. An empty object is accepted.
- Output: the `quarry status --json` document verbatim, including `store.path` (absolute) and `rates.fetch_error`.
- Config handling is identical to `status`. An unreadable config does not refuse: `findings.ignored` is null and `warnings` holds `cannot tell which findings you ignored: <problem, absolute>; findings you ignored are counted as open`.
- Do not add an age field. The model compares `snapshot.taken_at` with today, as the instructions say.

##### 2.6 Tool: `describe_schema`

**Description:**
```
Describe quarry's store: every table and view with its columns and types,
the conventions for amounts, signs, transfers and currencies, the
accounts, the category tree, and the first and last transaction dates.
Call this before writing SQL for query.
```

- Input: none.
- Output (new document, built by core, e.g. `report.Server.DescribeSchema`, so a later CLI or skill `schema.md` generator reuses it):
```json
{
  "conventions": "<shared text>",
  "relations": [{"name":"accounts","kind":"table","columns":[{"name":"id","type":"VARCHAR"}]}],
  "accounts": [{"id":"…","name":"…","type":"…","currency":"CAD","closed":false}],
  "categories": [{"id":"…","full_path":"Auto:Fuel","kind":"expense","hidden":false}],
  "dates": {"first":"2003-01-02","last":"2026-09-30"},
  "warnings": []
}
```
- **`relations`:**
  - Every table and view in the store's main schema, read by introspection at call time. Do not keep a hand-written list.
  - Sorted with tables before views, then by name.
  - `kind` is `table` or `view`. Column order is DDL order.
- **Field names are the store's own column names** (`full_path`, `closed`), so what the model sees is what it queries.
- Payees, tags and amounts are left out.
- **Bound:** `accounts` and `categories` each hold at most 500 entries.
  - Sorted: accounts by name then id; categories by `full_path`. (Orchestrator ruling, SCENARIO-07: accounts sort as `quarry accounts` does — `lower(name)`, then `name`, then `id` — so CLI and MCP never disagree. `<n>` formatted with `humanize.Thousands` (`1,001`); when both lists overflow, the accounts warning precedes the categories warning.)
  - Over the cap, add the warning `describe_schema lists the first 500 categories of <n>; query the categories table for the rest` (the same with `accounts`).
  - `relations` is not capped; its size is set by the schema, not by history.
- **`dates`** has the same null rules as status `dates`: both null when there are no transactions.
- **Conventions, one source.** The `quarry sql` Long paragraph from "Amounts are DECIMAL(18,2)…" through "…transfers.to_split_id." moves to one exported constant in core. It is rendered **byte-identical** into `sql --help` (CLI copy unchanged, and an existing help test pins it) and into `conventions`.
  - The `findings`/`finding_items` paragraph and the "List the tables…" line stay CLI-only.
  - Neither the conventions nor the instructions may mention masking.

##### 2.7 Tool: `data_quality`

**Description:**
```
List the data-quality findings quarry's last sync found: problems to fix
in Quicken (duplicates, one-sided or unlinked transfers, uncategorized
splits, payees in mixed categories, payee name variants, similar or
unused categories). Each finding has an id, the suggested fix, and the
transactions, payees or categories it is about. quarry never fixes them:
the user fixes them in Quicken and runs quarry sync, and fixed findings
drop off. To ignore a finding the user adds its id to findings.ignore in
quarry's config file.
```

**Input:**

| param | type | default | values |
|---|---|---|---|
| `status` | string enum | `open` | `open`, `ignored`, `fixed`, `all` |
| `type` | string enum | none | the 8 `finding.Types()` names |
| `limit` | integer | 50 | 1..500 (findings, not items) |

**Output:** `findingsListDocument` verbatim (`status, type, counts, findings[], warnings`). `counts` is never trimmed: it is the full tally for the `type` filter, across every status.

**Bounds**, with the cap reported only through `warnings` (no new fields):
- **More findings than `limit`:** the first `limit` findings, in CLI order, plus the warning
  `listed the first 50 of 1,234 open findings; pass type to narrow the list, or a larger limit (at most 500)`
  The total is the count for `status`, or the sum of the counts for `all`.
- **Items per finding: at most 25.** Over that, the warning
  `finding uncategorized:payee-88 lists the first 25 of 412 items; query finding_items WHERE finding_id = 'uncategorized:payee-88' for the rest`
  Uncategorized items are one per split, so they are unbounded. That is why this cap is needed.
  - Use `humanize.Count` grouping, the same as the CLI.

**Mid-feature copy ruling (SCENARIO-11).**
- Warning order: (1) config unknown-key lines `cfg.WarningsAbsolute` in loader order; (2) unmatched-ignore lines `document.UnmatchedIgnoreWarnings(cfg.Path, listing.Unmatched)`; (3) the findings-cap line (at most one); (4) items-cap lines, one per over-cap finding that was kept, in listing order. A finding dropped by the findings cap never gets an items-cap line. No warnings → `[]`.
- Findings-cap format: `listed the first <limit> of <total> <statusword>findings<tail>` — status word `open `, `ignored `, `fixed `; dropped for `all`. `type` never appears. Numbers grouped as CLI (`1,234`).
- Advice tail (example status=open, total 1,234):

| limit | type | exact line |
|---|---|---|
| < 500 | not given | `listed the first 50 of 1,234 open findings; pass type to narrow the list, or a larger limit (at most 500)` |
| < 500 | given | `listed the first 50 of 1,234 open findings; pass a larger limit (at most 500)` |
| = 500 | not given | `listed the first 500 of 1,234 open findings; pass type to narrow the list` |
| = 500 | given | `listed the first 500 of 1,234 open findings` |

**Config:**
- Unparseable config: `isError`, with the same refusal line `findings` prints, in `~` form.
- Unknown keys: `cfg.WarningsAbsolute` in `warnings`.
- Unmatched ignore ids: `unmatchedIgnoreWarnings` with the absolute config path, the same as `findings --json`.

**Currency:** none of the four tools takes a `currency` parameter, and `reporting.currency` doesn't affect them. Finding items carry native `currency`; `query` gets `_cad`/`_usd` columns from the views.

#### 3. Input-refusal ownership

| Input | Owner | Copy |
|---|---|---|
| Missing `sql`, wrong JSON type, `limit` out of range, enum mismatch, unknown param (if the SDK sets additionalProperties false) | SDK schema validation | **SDK text, not ruled.** The requirement is only that it refuses and never silently ignores. The architect verifies the unknown-param behaviour. If the SDK ignores unknown params, that is acceptable for these four tools (no param is safety-relevant). |
| `sql` blank, whitespace, or no statement (`;`, comments only) | handler (blank) / shared classifier `QueryFailureEmpty` (no statement; reaches the store, so with no store the no-store line wins) — mid-feature ruling, SCENARIO-03 | `query needs SQL in the sql parameter` |
| Unknown tool name | SDK | SDK text |

#### 4. Edge-case table

✗ is an `isError` line; R is a result.

| Input class | query | describe_schema | sync_status | data_quality |
|---|---|---|---|---|
| No store yet | ✗ `no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it` | same ✗ | same ✗ | same ✗ |
| Store from another quarry version | ✗ existing `OtherFormat` line verbatim | same | same | same |
| Store unreadable (not DuckDB, permission) | ✗ existing `cannot read the store at …: <reason>; run quarry sync to rebuild it` | same | same | same |
| Another program holds the store file (`OpenFaultLocked`) | ✗ existing `…; close that program and run the command again` verbatim, not forked | same | same | same |
| Sync running concurrently | Gets the old or new store. Sync renames a new file into place, so readers are not locked. No row. | same | same | same |
| Store has no transactions | R, rows [] | R, `dates` both null | R, `dates` null | R, findings [] |
| Store is old | R, no hint; freshness lives in `sync_status` and the instructions | R | R, `snapshot.taken_at` | R |
| Quicken closed / reconciliation failed / fingerprint changed | No row: MCP never sees Quicken, and a failed sync leaves the previous store, which is what gets reported | — | — | — |
| Config unparseable | n/a (no config read) | n/a | R + `cannot tell which findings you ignored…` warning | ✗ findings' config refusal line |
| Config unknown keys | n/a | n/a | n/a (status ignores them today; keep it that way) | R + absolute warning lines |
| Empty result | R, `rows: [], row_count: 0, truncated: false`, no warning | — | — | R, `findings: []` |
| Write attempt | ✗ `query only reads quarry's store; it cannot change data. Fixes are made in Quicken, then the user runs quarry sync` | — | — | — |
| External file, database or extension | ✗ `query reads only quarry's store; other files, databases and extensions are turned off` | — | — | — |
| Unprintable value | ✗ `cannot print column "x" of type JSON; cast it in the query, e.g. CAST(x AS VARCHAR)` (shared with CLI) | — | — | — |
| SQL error | ✗ `query failed: <DuckDB first line>` (shared) | — | — | — |
| Cap hit | R + truncation warning (§2.4) | R + per-list warning | — | R + findings or items warning |
| Timeout (30 s) | ✗ `query stopped after 30 seconds; aggregate or filter it in SQL, then try again` | ✗ `describe_schema stopped after 30 seconds; try again` | ✗ `sync_status stopped after 30 seconds; try again` | ✗ `data_quality stopped after 30 seconds; try again` |
| Client cancels (notifications/cancelled) or disconnects | Silence: no result, no stderr line; the server keeps serving (or exits 0 on EOF) | same | same | same |

Data text from the user's file (payees, memos, account and category names) is carried as JSON strings with standard escaping. Control characters are `\u00XX` escapes, and empty string stays separate from null, exactly as in `--json`. There is no table or CSV form in MCP, so no other channel needs a ruling.

#### 5. Redaction debt

- Put one honest line in `quarry mcp` Long help (§2.1, paragraph 3). Nowhere else in user-facing copy.
- Existing copy this makes false, to record as **doc debt** in STATE.md `## Open debts` (do not edit the PRD silently):
  - PRD §Security "Redaction on import" bullet;
  - PRD Risks table "masking on import" mitigation;
  - PRD §Decisions has no entry for the deferral. Add one when the user confirms the wording.
- Also update the `newRootCommand` doc comment (`internal/cli/root.go:6-10`) to name `mcp`.

#### Costs and what dies

- **New dependency:** official MCP Go SDK. Go-version and cgo compatibility are unverified offline; the architect runs `go get` first.
- **New on-disk or config surface:** none.
- **New contract:** tool names, param names, the instructions text, and the 500/30 s/50/25 bounds.
- **Nothing dies.** Every moved symbol keeps its CLI caller. CLI stdout and stderr copy is byte-unchanged, and the existing `--json` and help tests are the proof.

#### Verdict: SHIP WITH CHANGES

Ranked:

1. **Fold the shared-document extraction into Scenario 1**, limited to `sql`, `status` and `findings` plus their warning builders, the store-error classification and the conventions constant. MCP structured output is those documents verbatim. Reason: PRD:108/129; a second shape is the drift PRD:108 forbids.
2. **The query timeout is unproven, and proving it blocks this part of the slice.** Keep `DeadlineExceeded` separate from cancel, and add an acceptance test with a slow query. If the driver can't interrupt, come back to product. Reason: otherwise the timeout copy is a false claim and a runaway query hangs the client.
3. **State the never-hold-the-store-open and per-call-config invariant**, with the sync-while-serving scenario. Reason: without it, the server either blocks `sync` or serves stale data until restart, with no visible sign.
4. **Bound `data_quality`:** at most 50 findings by default (500 max), at most 25 items per finding, reported through `warnings` with the copy above. Reason: Q7; uncategorized items are one per split and unbounded.
5. **stdout carries only protocol traffic**, with a test. stderr holds only the TTY hint and the per-error log line, never SQL or row data. Reason: one stray byte on stdout breaks the client session.
6. **The `query` cap is a hard 500 the model can't raise.** Name `mcp.max_rows` and `mcp.query_timeout` as reserved keys, but don't build them. Reason: Q7, and operator policy gets a name before the design hardens.
7. **Record the exit-0-on-signal deviation and the redaction doc debts in the spec.** Reason: so the final gate doesn't reopen them.

Key files: `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/sql.go` (conventions paragraph, `queryFailure`), `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/json_status.go`, `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/json_findings.go`, `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/cli/json_sql.go`, `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/report/refusal.go`, `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/internal/store/duckstore/query.go:36-56` (`queryRefusal` folds deadline into interrupt), `/Users/koblas/repos/github.com/koblas/quarry/.claude/worktrees/synced-data-cleanup-b34b42/cmd/quarry/run.go:80-200`.

### Changes to existing surfaces

- `quarry sql --help` Long: the conventions paragraph ("Amounts are DECIMAL(18,2)…" through "…transfers.to_split_id.") moves to one exported constant in `internal/report`; rendered byte-identically — no copy change.
- `internal/cli/root.go:6-10` `newRootCommand` doc comment: name `mcp` among the commands.
- Root `quarry --help` command list gains `mcp` with the ruled Short.
- PRD `docs/initial-prd.md` §Security "Redaction on import" bullet, Risks table "masking on import" mitigation: now false until redaction ships. **Doc debt** → STATE.md `## Open debts` (do not edit the PRD silently; PRD §Decisions entry for the deferral added when the user confirms wording).
- No existing CLI stdout/stderr line changes.

---

## Scenarios (Gherkin)

Approved by the user 2026-10-02. SCENARIO-17 reworded with user approval after the sizing pass found the SDK always answers a cancelled request id. Listed in build order.

```gherkin
Scenario Outline: SCENARIO-01 — CLI output is unchanged after the shared documents move out of cli
  Given a store built from the v9 fixture
  When the user runs quarry <cmd>
  Then stdout, stderr and exit code are byte-identical to before the move
  And quarry sql with two statements returns only the last statement's rows

  Examples:
    | cmd             |
    | sql --json ...  |
    | status --json   |
    | findings --json |
    | sql --help      |

Scenario: SCENARIO-02 — An MCP client connects and sees quarry's four tools
  Given an MCP client has started quarry mcp
  When the client initializes and lists tools
  Then the server identifies as "quarry" with its build version and the ruled instructions
  And it lists describe_schema, query, sync_status and data_quality with the ruled descriptions and input schemas
  And every line quarry writes to stdout is a JSON-RPC message

Scenario Outline: SCENARIO-15 — quarry mcp ends with the ruled exit code
  Given quarry mcp is invoked <how>
  When the server stops
  Then it exits <code> with stderr <stderr>

  Examples:
    | how                              | code | stderr                         |
    | and the client closes stdin      | 0    | empty                          |
    | and receives SIGTERM             | 0    | empty                          |
    | and the client stops reading     | 0    | empty                          |
    | with a positional argument       | 2    | mcp takes no arguments         |
    | with --json                      | 2    | the ruled --json usage line    |
    | with $HOME unset                 | 1    | the ruled $HOME line           |

Scenario: SCENARIO-16 — quarry mcp run at a terminal says what it is
  Given stdin is a terminal
  When the user runs quarry mcp
  Then stderr gets the one ruled hint line and the server keeps serving

Scenario: SCENARIO-03 — query returns rows as the sql --json document
  Given a built store
  When the client calls query with a SELECT
  Then the result's structured content is the quarry sql --json document for that query, compact, keys in CLI order

Scenario: SCENARIO-04 — query over its limit returns the first rows and says so
  Given a built store and a query yielding more rows than limit
  When the client calls query
  Then the result has limit rows, truncated true, and the ruled cap warning naming the effective limit

Scenario Outline: SCENARIO-05 — query refuses what it cannot run
  Given a built store
  When the client calls query with <sql>
  Then the result is isError with <line>
  And stderr gets one "quarry: mcp: query: <line>" log line

  Examples:
    | sql                     | line                                  |
    | a write statement       | ruled write refusal                   |
    | read_csv of a file      | ruled external-access refusal         |
    | whitespace only         | query needs SQL in the sql parameter  |
    | ;                       | query needs SQL in the sql parameter  |
    | -- note                 | query needs SQL in the sql parameter  |
    | invalid SQL             | query failed: <DuckDB first line>     |
    | a JSON-typed column     | ruled unprintable-value line          |

Scenario: SCENARIO-07 — describe_schema describes the store
  Given a built store
  When the client calls describe_schema
  Then the result lists every table and view (introspected, tables before views, then by name) with columns in DDL order, the conventions text byte-identical to the quarry sql --help paragraph, the accounts, the category tree and the first and last transaction dates

Scenario: SCENARIO-08 — describe_schema bounds long account and category lists
  Given a store with more than 500 categories
  When the client calls describe_schema
  Then categories holds the first 500 by full_path and warnings carries the ruled overflow line

Scenario: SCENARIO-09 — sync_status returns the status document
  Given a built store
  When the client calls sync_status
  Then the structured content is the quarry status --json document

Scenario: SCENARIO-10 — sync_status still answers when the config is unreadable
  Given a built store and an unparseable config file
  When the client calls sync_status
  Then the result is not isError, findings.ignored is null and warnings carries the ruled "cannot tell which findings you ignored" line

Scenario: SCENARIO-14 — A sync while the server runs is seen without restart
  Given quarry mcp is serving and the client has called sync_status once
  When the user runs quarry sync --from another snapshot and the client calls sync_status again
  Then the second result shows the new store.built_at
  And an edit to findings.ignore between calls changes the ignored count

Scenario: SCENARIO-11 — data_quality lists open findings within its bounds
  Given a store with more open findings than limit and a finding with more than 25 items
  When the client calls data_quality with defaults
  Then it returns the first 50 open findings in CLI order with full counts, each with at most 25 items, and the ruled findings-cap and items-cap warnings

Scenario: SCENARIO-12 — data_quality refuses an unreadable config
  Given a built store and an unparseable config file
  When the client calls data_quality
  Then the result is isError with the findings command's config refusal line

Scenario: SCENARIO-06 — A slow call stops at its deadline
  Given a built store and the server's per-call timeout
  When the client calls query with SQL that runs past the timeout
  Then the call returns within the timeout plus 1 s as isError with the ruled timeout line
  And the error chain carries context.DeadlineExceeded, not context.Canceled

Scenario Outline: SCENARIO-13 — Every tool refuses before the first sync
  Given no store exists yet
  When the client calls <tool>
  Then the result is isError with "no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it"

  Examples:
    | tool            |
    | query           |
    | describe_schema |
    | sync_status     |
    | data_quality    |

Scenario: SCENARIO-17 — A cancelled call is interrupted quietly
  Given a query is running in the server
  When the client sends notifications/cancelled for it
  Then the store query is interrupted, quarry writes no stderr line, and the next call is served
  # The wire response for the cancelled id is written by the SDK and is not asserted.
```

---

## Sizing

Sizing pass (architect, opus) 2026-10-02 — 17 IDs → 8 runs. Full report and SDK facts in `## Sizing notes` below.

| Scenario | Verdict — numbers |
| --- | --- |
| SCENARIO-01 | OWNS A RUN — 4 batches (sql doc; status doc + ignore policy; findings doc + warnings; query-error classifier + conventions const); report, new report/document, cli. Green on arrival (behaviour-neutral); adds multi-statement pin |
| SCENARIO-02 | OWNS A RUN — 3 batches (SDK + internal/mcp skeleton with 4 stub tools and explicit schemas; cli `mcp` command + Env hook; cmd/quarry wiring + JSON-RPC-only stdout test) |
| SCENARIO-15 | OWNS A RUN — 3 batches (cli usage refusals; $HOME check + exit mapping EOF/SIGTERM/EPIPE → 0; TTY seam + hint). Absorbs SCENARIO-16 |
| SCENARIO-16 | FOLD into SCENARIO-15 — same start path |
| SCENARIO-03 | OWNS A RUN — 4 batches, internal/mcp (result envelope; query handler + blank refusal + MCP multi-statement pin; limit + truncation warning; refusal arms via shared classifier). Absorbs 04, 05 |
| SCENARIO-04 | FOLD into SCENARIO-03 — one warning line + schema bound |
| SCENARIO-05 | FOLD into SCENARIO-03 — refusal mapping over 03's handler |
| SCENARIO-07 | OWNS A RUN (opus) — 4 batches (store schema types + report.Store port + duckstore introspection in one open; report.Server.DescribeSchema; document + conventions + mcp handler; 500 caps). Port method lands in 4 implementers. Absorbs 08 |
| SCENARIO-08 | FOLD into SCENARIO-07 |
| SCENARIO-09 | OWNS A RUN (sonnet) — 3 batches (handler + per-call config; unparseable-config warning; cmd/quarry two-call re-sync test + per-call config pin). Absorbs 10, 14 |
| SCENARIO-10 | FOLD into SCENARIO-09 |
| SCENARIO-14 | FOLD into SCENARIO-09 — test-only cover of per-call open |
| SCENARIO-11 | OWNS A RUN — 3 batches (handler + per-call config warnings / refusal; findings limit; items cap). Absorbs 12 |
| SCENARIO-12 | FOLD into SCENARIO-11 |
| SCENARIO-06 | OWNS A RUN (opus) — 4 batches (DeadlineExceeded kept in chain through store/duckstore/report; mcp per-call timeout seam + outcome classifier; SCENARIO-13 table; SCENARIO-17). Ordered last so all 4 tools exist. Absorbs 13, 17 |
| SCENARIO-13 | FOLD into SCENARIO-06 — test-only, green on arrival |
| SCENARIO-17 | FOLD into SCENARIO-06 — third arm of the same outcome classifier |

## BDD Acceptance Progress
- [x] SCENARIO-01: CLI output is unchanged after the shared documents move out of cli — `cmd/quarry/run_shared_documents_test.go` `Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte`
- [x] SCENARIO-02: An MCP client connects and sees quarry's four tools — `cmd/quarry/run_mcp_test.go` `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc`
- [x] SCENARIO-15: quarry mcp ends with the ruled exit code — `cmd/quarry/run_mcp_exit_test.go` `Test_run_mcp_ends_with_the_ruled_exit_code`
- [x] SCENARIO-16: quarry mcp run at a terminal says what it is — delivered by SCENARIO-15 — `cmd/quarry/run_mcp_terminal_test.go` `Test_run_mcp_at_a_terminal_prints_the_hint_and_keeps_serving`
- [x] SCENARIO-03: query returns rows as the sql --json document — `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_returns_the_sql_json_document`
- [x] SCENARIO-04: query over its limit returns the first rows and says so — delivered by SCENARIO-03 — `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_over_its_limit_returns_the_first_rows_and_says_so`
- [x] SCENARIO-05: query refuses what it cannot run — delivered by SCENARIO-03 — `cmd/quarry/run_mcp_query_test.go` `Test_run_mcp_query_refuses_what_it_cannot_run`
- [x] SCENARIO-07: describe_schema describes the store — `cmd/quarry/run_mcp_describe_test.go` `Test_run_mcp_describe_schema_describes_the_store`
- [x] SCENARIO-08: describe_schema bounds long account and category lists — delivered by SCENARIO-07 — `cmd/quarry/run_mcp_describe_test.go` `Test_run_mcp_describe_schema_lists_the_first_500_categories_and_says_so`
- [x] SCENARIO-09: sync_status returns the status document — `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_returns_the_status_json_document`
- [x] SCENARIO-10: sync_status still answers when the config is unreadable — delivered by SCENARIO-09 — `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_answers_when_the_config_is_unreadable`
- [x] SCENARIO-14: A sync while the server runs is seen without restart — delivered by SCENARIO-09 — `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_sees_a_sync_between_calls`
- [ ] SCENARIO-11: data_quality lists open findings within its bounds
- [ ] SCENARIO-12: data_quality refuses an unreadable config
- [ ] SCENARIO-06: A slow call stops at its deadline
- [ ] SCENARIO-13: Every tool refuses before the first sync
- [ ] SCENARIO-17: A cancelled call is interrupted quietly

---

## Sizing notes

#### Sizing table (in build order)

All verdicts are code-first unless an architect finds otherwise. None of these scenarios touches the mandatory test-first set: there is no write guard, and the read lockdown itself is unchanged.

| # | Scenario | Verdict | Size |
|---|---|---|---|
| 1 | S01 extraction | OWNS A RUN | 4 batches: sql doc; status doc + ignore policy + cannotTell warning; findings doc + unmatched warnings; shared query-error classifier + conventions const. Packages: report (+ new report/document), cli. Also adds the multi-statement pin for `quarry sql` (see SDK facts). The acceptance test is green on arrival because the move is behaviour-neutral. |
| 2 | S02 SDK + skeleton + `quarry mcp` + tools/list | OWNS A RUN | 3 batches. (1) go.mod SDK, `internal/mcp` Server/options, the 4 tools with explicit input schemas, descriptions, instructions, stub handlers. (2) cli `mcp` command (Use/Short/Long) + `Env` hook + root-help/registration pins. (3) cmd/quarry wiring + version + "every stdout line is JSON-RPC" test, which is also the wiring pin. |
| 3 | S15 lifecycle exits **+ S16 FOLD** (TTY hint, a handful of lines in the same start path) | OWNS A RUN | 3 batches: cli usage refusals (positional, --json); cmd/quarry `$HOME` check + exit mapping (EOF/SIGTERM/EPIPE → 0); TTY seam + hint. Also carries the ruled "stdout write fails → 0" row. Not LIGHT, because it has 4 steps after the fold. |
| 4 | S03 query **+ S04 FOLD** (cap = a warning line + schema bound) **+ S05 FOLD** (refusal mapping over S03's handler) | OWNS A RUN | 4 batches, all in mcp: result envelope (raw doc / isError line / stderr log); query handler + blank-sql refusal + MCP multi-statement pin; limit bound + truncation warning; 4 refusal arms through the shared classifier |
| 5 | S07 describe_schema **+ S08 FOLD** (caps belong in the owning batch anyway) | OWNS A RUN, opus | 4 batches: `store` schema types + `report.Store` port method + duckstore introspection in one open; `report.Server.DescribeSchema`; document + conventions + mcp handler; 500 caps + warnings. Only one feature package (report). The new port method lands in 4 implementers: `duckstore.Store`, `internal/report/fakes_test.go:75 fakeStore`, `internal/cli/fakes_test.go:64 fakeReportStore`, `internal/cli/status_test.go:28 statusStore`. |
| 6 | S09 sync_status **+ S10 FOLD** (exercises S01's shared warning) **+ S14 FOLD** (test-only cover of per-call open) | OWNS A RUN, sonnet | 3 batches: handler + config per call; unparseable-config warning; cmd/quarry two-call test (re-sync between calls → new `built_at`). This also carries the §2.2 per-call **config** pin (edit `findings.ignore` between calls → ignored count changes), which no approved scenario owned. Store-building helpers exist (`cmd/quarry/run_findings_carry_test.go:37 syncNewBundle`, `run_accounts_test.go:109 syncAccountsFixture`). I rated it OWNS A RUN rather than LIGHT, because it carries binding invariant pins and an L run would sit right at 3 steps. |
| 7 | S11 data_quality **+ S12 FOLD** (config refusal is a single branch in S11's handler) | OWNS A RUN | 3 batches: handler + per-call config (unknown-key and unmatched warnings, unparseable → isError); findings limit 50/500 + warning; items cap 25 + warning |
| 8 | S06 timeout **+ S13 FOLD** (no-store × 4 tools, test-only, green on arrival) **+ S17 FOLD** (cancel is the third arm of the same outcome classifier) | OWNS A RUN, opus | 4 batches. (1) store/duckstore/report keep `context.DeadlineExceeded` in the chain for every read. (2) mcp per-call timeout seam + classifier: deadline → per-tool copy ×4; cancel → silent; else → refusal + stderr line. (3) S13 table. (4) S17. The acceptance test is a real-duckdb slow query with an injected short timeout. S06 is ordered last so that all 4 tools exist. |

Result: **8 architect runs** for 17 approved IDs.

**SPLIT check**
- Every unit is ≤5 batches, has one When, and has at most one feature package.
- Past units: 48 (2a, 2d, 2e, 2f). The p90 unit cost is about 2,810k IE.
- Closest twins:
  - S07 ↔ 2a SCENARIO-04 (view + read + command, 1,289k) and 2a SCENARIO-02 (port + duckstore read side, 1,798k).
  - S06 ↔ 2a SCENARIO-12 (classifier, 1,637k).
- All twins are under p90, so no mandatory split.

**Fold ticks**
- S04 and S05 are delivered by S03.
- S08 by S07.
- S10 and S14 by S09.
- S12 by S11.
- S13 and S17 by S06.
- S16 by S15.

Each folded scenario keeps its own acceptance test, recorded on an `Acceptance test (SCENARIO-NN, folded):` line.

#### Seams

**`internal/mcp` is a delivery peer of `internal/cli`, not a feature package.**
- cli registers `quarry mcp` (help, usage refusals) and calls a func field on `cli.Env` (e.g. `ServeMCP(ctx, stdin, stdout, stderr) error`). `cmd/quarry` wires that field to `mcp.NewServer(...)`, using the existing `newReportFactory` (`cmd/quarry/run.go:91`) and `newConfigLoader` (`:123`), both called per tool call.
- cli never imports mcp; mcp never imports cli.
- mcp imports report, report/document, config and store, the same way cli does.
- This is what keeps the "one feature package" counts above true. If cli imported mcp, then mcp → report would be a feature-imports-feature violation.

**Shared documents go in a new `internal/report/document`.**
- It holds the sql, status and findings documents, their builders and their warning builders. cli and mcp both import it; report never does.
- It takes plain values (ignore list, config problem string, absolute config path), **not** `config.Config`, so it never imports config.
- Builders take the warnings slice as a parameter. Today's `renderSQLJSON(result, limit, warnings)` already does this, which lets "pass --limit 0" and the MCP cap line differ.
- CLI renders the document indented and MCP renders it compact, from the same value.
- 3b's spend, cashflow, recurring and anomalies documents land here later.

**DescribeSchema follows the ADR-003 pattern.**
- `report.Store` gets one new method that returns a driver-free `store` value (relations, accounts, categories, dates) from **one** open.
- `duckstore` introspects `duckdb_tables`/`duckdb_views`/`duckdb_columns` at call time.
- `report.Server.DescribeSchema` wraps it with `readRefusal`; the document builder lives in report/document.
- The conventions text becomes one exported constant in report (e.g. `report.SQLConventions`). Both `sql --help` (`internal/cli/sql.go:30-60`) and the document render it byte-identically.
- The query-error classifier (today's `internal/cli/sql.go:190 queryFailure`) moves to report as a reason classification. Each surface owns its own copy.

**Deadline vs cancel is split across two layers.**
- *Chain preservation* is in `internal/store/query.go:65 Interrupted`, `internal/store/duckstore/query.go:17-38` (the `openRead` path and `queryRefusal`) and `internal/report/refusal.go:29 readRefusal`. Its cause must keep the ctx error.
- *The decision* is in internal/mcp's outcome classifier: `errors.Is(err, context.DeadlineExceeded)` → per-tool timeout line; `context.Canceled` → silent; anything else → refusal + stderr line.
- The timeout is a named constant (30 s) injected through an mcp option, so the acceptance test can use milliseconds. It needs a wiring pin.

#### SDK facts (verified offline in a throwaway module at `/tmp/claude-501/mcpprobe`; repo untouched)

**Resolution and dependencies**
- `github.com/modelcontextprotocol/go-sdk` **v1.8.0** (latest) resolves and builds under go 1.27.1 alongside duckdb-go v2.10505.0.
- The SDK needs go ≥1.25 and is pure Go (no cgo).
- It pulls in jsonschema-go v0.4.3, segmentio/encoding, golang-jwt, oauth2, uritemplate and x/time.
- It bumps indirect x/sys 0.40→0.41, x/tools 0.41→0.42 and x/sync 0.19→0.20.

**Output validation**
- `mcp.AddTool` with a typed `Out` validates against an inferred output schema. `*string` becomes `["null","string"]`, nil slices become `["null","array"]`, and `[][]any` passes.
- **Trap:** it then re-marshals through `any`. Keys come out **alphabetical**, so CLI field order is lost, and int64 values above 2^53 lose precision. Seen: 9007199254740993 → …992.
- Verified fix: `Out = any`, explicit permissive `OutputSchema: {"type":"object"}`, and the handler sets `StructuredContent: json.RawMessage(compactDoc)` plus one `TextContent` with the same bytes. Order and precision are then preserved on the wire.

**Input validation**
- Inferred input schemas set `additionalProperties:false`, so **unknown params are refused** (`isError`, SDK text: `unexpected additional properties ["bogus"]`).
- Inference produces **no** bounds, enum, minLength or default, so every tool needs an explicit `InputSchema`. With one, all of these were verified:

| Input | Result |
|---|---|
| `limit` 501 / 0 | refused |
| missing `limit` | reaches the handler as 500 (schema default applied) |
| bad `status` enum | refused |
| `"5"` for an integer | refused |
| `sql:""` | refused (minLength) |
| `sql:" "` | reaches the handler, so the ruled "query needs SQL…" line fires |

**Errors, transports and logging**
- A handler returning a Go error gives `isError: true` with the error text as the only content and no `structuredContent`, which matches the ruling.
- An unknown tool gets a JSON-RPC protocol error (SDK text).
- In-process transports are available: `mcp.NewInMemoryTransports()` (a net.Pipe pair), and `mcp.IOTransport{Reader, Writer}` for driving `Env` streams.
- **Trap:** never use `mcp.StdioTransport`. It reads `os.Stdin` and `os.Stdout` directly and bypasses `Env`.
- `Server.Run`:

| Event | `Run` returns |
|---|---|
| stdin EOF | `nil` |
| parent ctx cancel, even with a read blocked | `context.Canceled` within ~100 ms |
| stdout write fails | error matching `syscall.EPIPE` (map that arm only to exit 0) |

- The default `ServerOptions.Logger` is `slog.DiscardHandler`, so the SDK writes nothing to stderr. Never pass `slog.Default()`.
- `Instructions` goes in `ServerOptions`. The initialize response advertises the `logging` and `tools.listChanged` capabilities.

**Multi-statement**
- duckdb-go `QueryContext("SELECT 1 AS a; SELECT 2 AS b")` returns the **last** statement's rows (`columns=[b]`). quarry's `QueryTable` uses `QueryContext` (`internal/platform/duckdb/table.go:49`).
- That makes the ruled description sentence ("Send one statement; if you send several, only the last one's rows come back.") applicable.
- Put the `quarry sql` pin in S01 so S02 can ship the description with that sentence. S03 pins the MCP side.

#### Feasibility flags

**S06 (deadline interrupt): feasible.**
- duckdb-go `context.go:55` calls `duckdb_interrupt` every 500 ms after `ctx.Done()`, and `Done()` closes on a deadline as well as on cancel.
- Probe: a 1 s deadline on a 10^13-row cross join returned at **1.50 s** with `context deadline exceeded\nINTERRUPT Error: Interrupted!`. `errors.Is(DeadlineExceeded)` was true and `Canceled` was false. Same result with `QueryContext` + `rows.Next`.
- Expect up to deadline + 0.5 s. The real 30 s returns in at most about 30.5 s.
- The work needed is chain preservation through `store.Interrupted`, the `openRead` path and `readRefusal`, plus an injectable timeout.

**S15 (signal exits): feasible.**
- `cmd/quarry/run.go:38 signalContext` already turns SIGINT/SIGTERM into ctx cancel, and `Run` returns `context.Canceled` promptly.
- The mcp `Serve` must return nil when the parent ctx is done. The read commands' "<cmd> interrupted" → exit 1 path must not catch it.
- The SIGTERM test runs in-process with a cancelled ctx. Signal → ctx is already pinned by signalContext's own tests (`main.go:10` comment).

**S17: not buildable as worded. Needs a product-vision ruling before S06 is planned.**
- SDK v1.8.0 does propagate `notifications/cancelled` to the handler ctx (verified).
- But it **still writes a response for the cancelled id** whatever the handler returns. Observed: `{"id":3,"result":{"content":[{"type":"text","text":"context canceled"}],"isError":true}}`.
- I have not reworded it. Proposed assertable observable:
  - the SDK client's `CallTool` returns its own ctx error;
  - the handler's store call is interrupted;
  - **no stderr line** is written;
  - the next call is served.
- The wire response for the cancelled id is SDK-owned.

**S16 trap:** a `ModeCharDevice` check reports `/dev/null` as a TTY. Use a termios ioctl instead (`golang.org/x/sys/unix`, already an indirect dep, or `x/term`), injected as a seam with a wiring pin.

#### Risks / notes for architects

- **Stub handlers.** S02 registers all 4 tools so `tools/list` is complete, with stub handlers that later scenarios replace. Name them in S02's Left unbuilt.
- **Hand-written input schemas are now contract.** Do not rely on inference; pin `tools/list` input schemas in S02.
- **Reserved config keys.** `mcp.max_rows` and `mcp.query_timeout` are reserved by name only. Do not build them.
- **Doc-debt items from verdict §5** go to STATE.md `## Open debts` in S02: the PRD redaction lines, plus the `internal/cli/root.go:6-10` doc comment naming `mcp`.
