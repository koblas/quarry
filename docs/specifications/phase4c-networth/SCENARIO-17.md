---
id: SCENARIO-17
status: open
---

# SCENARIO-17: MCP net_worth

Cadence: code-first (no write-safety guard, atomic adapter or bug fix touched)
Acceptance test: `cmd/quarry/run_mcp_net_worth_test.go` `Test_run_mcp_net_worth_returns_the_networth_json_document`
Narrow loop: `go test ./internal/mcp/ -run 'NetWorth|net_worth|Each_tool|AsOf|Window'` then `go test ./cmd/quarry/ -run 'run_mcp|Test_run_mcp|mcp_describes|Test_skill|Test_every_quarry'`
Mutation checks: as_of-vs-since/until guard in `(*Server).netWorth` (tested on non-nil pointers, so `since: ""` still conflicts) → `Test_net_worth_refuses_as_of_with_since_or_until`; `capList` call → `Test_net_worth_lists_the_first_500_month_ends_and_warns_on_501` (500 control, 501 cut); `refusal.Noun` in `asOfWording` → holdings and net_worth after-today rows, one each
Runs: A (1-2) | B1 (3-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 2 batches, 1 feature package (`internal/mcp`; `cmd/quarry`, `internal/cli`, SKILL.md are copy and table pins). 6 ruled lines, over the light lane.

Surveyed first: `go doc` of `report.NetWorthRequest`, `ParseMonthEndWindow`, `ResolveAsOf`, `document.NewNetWorth`/`NetWorthWarnings`. Triage/STATE inherit: Totals, FirstRate, FirstBalance, empty-result line all arrive through `report.NetWorth` and `NetWorthWarnings`; MCP recomputes none of them.

Rules this plan fixes (4b twin: `holdings.go:15-40`, `tools.go:293-297`):
- Order: one `now := s.now()`; conflict, `ResolveAsOf(report.NetWorthNoun)`, `ParseMonthEndWindow` (never `ParseWindow`, never a hand-built window: the history renderer indexes `Dates[0]`), then `resolveCurrency(in.Currency, "networth")`, then `newReport`. Every refusal precedes the config read and the store (as holdings; the CLI's currency-first order exists only for its flag errors, which the schema enum covers).
- History iff `in.Since != nil || in.Until != nil`; `AsOf` always set from `ResolveAsOf`, `Window` nil for a snapshot. One `srv.NetWorth` call.
- Cap: `capList(doc.Dates, doc.Warnings, toolNetWorth, "month ends", "pass a later since, or query v_net_worth for the rest")` on the finished document. Totals are per-date (`dates[i].totals`), there is no document-level total, so the cut drops whole dates; warnings are composed from the full `report.NetWorth` before the cut, so they count every month end, including cut ones. Cut note goes last, after config and composer warnings.
- Warnings are `document.NetWorthWarnings` unchanged: rate lines still say `pass --currency native` (CLI wording) in MCP; spec says same document, so deliberately not reworded.
- Conflict refusal reuses `asOfRefusedError` and `asOfRefusedLog` (names the as_of param); no new log constant.
- `windowWording`'s `WindowNetWorthSinceAfterToday` case (`window.go:30-31`) already reads `since <d> is after today; net worth is valued up to today only, so pass an earlier since`, pinned at `window_internal_test.go:50-53`: confirmed, no edit.

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_mcp_net_worth_test.go` `Test_run_mcp_net_worth_returns_the_networth_json_document` — `runBothSurfaces` (`run_mcp_documents_helpers_test.go:40`), body and warnings equal to `networth --json`. Rows (each its own case): default snapshot; `as_of` day; history `since`+`until`; `since` alone; `currency native` snapshot and history; history window entirely before the data and snapshot before the first balance (the `--json` arm checkpoint 16 left unpinned; seed with `seedUncountedOnlyStore` or a store whose first balance is later); a USD-reporting call with CAD rows and no rate (totals and rate warning unchanged). Stores need a past clock fixture under `toolClock` (2026-09-29); `v_balances_daily` runs on DuckDB's real today
- [ ] Step 2: `internal/mcp/tools.go` `toolNetWorth` (:27), `netWorthDescription` and param descriptions (:130 area, description verbatim from Surface & Copy), `netWorthInput{AsOf, Since, Until *string; Currency string}` (:234), registration after holdings (:297), `net_worth.go` signature-only `(*Server).netWorth` returning `document.NetWorth{}` — so the test fails at the body assertion, not at an unknown tool

### Build
- [ ] Step 3 (B1): `internal/mcp/net_worth.go` `(*Server).netWorth` per the rules above; `netWorthTwin = "networth"`; `holdings.go:60-63` `asOfWording` uses `refusal.Noun` (bytes for holdings unchanged; update comments at :56-58, :64-67). Tests in `net_worth_test.go` against `fakeStore` (add `netWorth store.NetWorth`, `netWorthAsked []store.NetWorthParams`, `NetWorth` method at `query_helpers_test.go` beside `Holdings` ~:58; `h.netWorth` helper beside :227): `Test_net_worth_refuses_as_of_with_since_or_until` (rows: as_of+since, as_of+until, as_of+`since: ""`; with a bad as_of too, conflict wins; line `as_of cannot be combined with since or until; pass as_of for one day, or since and until for month ends`; as_of class log, config stub untouched, `h.built` zero); as_of rows (not a date, empty `""` alone, after today: `as_of 2099 is after today; net worth is valued up to today only, so pass an earlier as_of`); window rows via the real parser (since after today ruled line, since after until, until before default since, not a date; `until` far future alone = clamped, no refusal, last listed date is today) with `windowRefusedLog`; absent / null / `{}` arguments = snapshot today; currency given skips config, absent reads config as `mcp` (config warnings first in the document); asked params: snapshot asks one date, history asks every month end plus mid-month today, `len(netWorthAsked) == 1`; clock read once per call (copy `Test_holdings_reads_today_once…`); fake rows pass through with Totals and rate warning unchanged and equal to `document.NewNetWorth` of the same report; faults, one per call: config unreadable (class line `quarry networth`), report factory failure, store refusal verbatim, plain store fault (generic log line), schema-rejected arguments (unknown key, `as_of` number, bad `currency`) with `argsRefusedLog`, before config and store
- [ ] Step 4 (B1): cap batch in the same file: `Test_net_worth_lists_the_first_500_month_ends_and_warns_on_501` pair (since chosen so the window lists exactly 500, then 501; fake returns no rows so each date is empty), 501 case asserts 500 dates, note `net_worth lists the first 500 month ends of 501; pass a later since, or query v_net_worth for the rest` last, after config and the empty-result line; 500 case asserts no note
- [ ] Step 5 (B2): `internal/mcp/timeout_test.go` `stallingStore.NetWorth` (beside `Holdings` :75-78) + table row `{"net_worth", map[string]any{}, time.Second, "net_worth stopped after 1 second; try again"}` (:126)
- [ ] Step 6 (B2): cmd and cli tables, one edit each: `run_mcp_test.go:43-46` tool list; `run_mcp_no_store_test.go:33` row; `run_mcp_store_faults_test.go:80-86` rows `net_worth, store cannot be opened` and `net_worth, its view dropped` (`DROP VIEW v_net_worth`); `run_mcp_descriptions_test.go:98-110` `mcpNetWorthDescription`/`mcpNetWorthInputSchema` literals (verbatim, hand-written, not read from production) + map row :249 + help line :268; `internal/cli/mcp.go:38-39` Long Tools line and `internal/cli/mcp_test.go:43-44` (`... search_transactions, holdings,` then `net_worth.` on its own wrapped line; width of the line 2 is 71, adding the name overflows); `plugin/skills/quarry/SKILL.md:95` and `run_skill_text_test.go:237` §9 (`holdings`, `net_worth`, `data_quality`; the drift test `mcpToolNames` reads the live tool list so it needs no edit). Also `cmd/quarry/run_mcp_net_worth_test.go` refusal test through the real stack: after-today as_of, after-today since, conflict, each with its stderr line `quarry: mcp: net_worth: refused the call's as_of; details went to the client only` (conflict) / `... since or until ...` (windows)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `netWorth`, `netWorthInput`, `toolNetWorth`

### Verify
- [ ] Step 8: full verification block per `.claude/rules/agent-briefs.md`, `spec-check.py phase4c-networth`, tick SCENARIO-17 with its acceptance test, rewrite STATE.md (drop the "Left unbuilt" MCP line and the checkpoint-16 JSON-arm debt, update the `windowWording` entry to "confirmed")

## Handoff

**Binding decisions:**
- MCP `net_worth` refusals run before the config read and the store, in the order conflict, as_of, window; `since`/`until` presence is a non-nil pointer, as the CLI tests `Changed` — `since: ""` beside `as_of` is a conflict, alone is a not-a-date refusal.
- Cap cuts `dates` only; warnings and per-date totals are computed from the full report first, so cut dates still count in "on N month ends" — S19 timing and any future paging read this.
- `asOfWording` takes its noun from `AsOfError.Noun`; holdings bytes stay pinned.

**Left unbuilt:**
- Reworded MCP rate warning (`pass --currency native` is CLI wording in a tool result) — a product-vision call at the final pass, not built here.
- MCP-specific stderr class line for the conflict refusal — it reuses the as_of line; rule otherwise at the final pass.

**Traps:**
- Tool registration without a handler stub makes the acceptance test fail at "unknown tool" inside the helper, not at an assertion; hence step 2 stubs a handler.
- `fakeStore` embeds a nil `report.Store`: a `NetWorth` call without the new method panics instead of failing; `stallingStore` likewise (step 5).
- `Server.NetWorth` drops store rows whose `Date` was not requested; fake rows must carry `Date` (STATE trap).
- A window built by hand into `Server.NetWorth` can be empty and panic the history renderer's `Dates[0]`; only `ParseMonthEndWindow` is allowed here.
