---
id: SCENARIO-06
status: open
---

# SCENARIO-06: A slow call stops at its deadline

Cadence: code-first (no bug fix, write-safety guard or atomic adapter touched)
Acceptance test: `cmd/quarry/run_mcp_timeout_test.go` `Test_run_mcp_query_stops_at_its_deadline`
Acceptance test (SCENARIO-13, folded): `cmd/quarry/run_mcp_no_store_test.go` `Test_run_mcp_every_tool_refuses_before_the_first_sync`
Acceptance test (SCENARIO-17, folded): `cmd/quarry/run_mcp_cancel_test.go` `Test_run_mcp_cancelled_query_is_interrupted_quietly`
Narrow loop: `go test ./internal/store/duckstore/ ./internal/report/ ./internal/mcp/ -run 'Interrupt|Deadline|Timeout|Refusal|errorLog' && go test ./cmd/quarry/ -run 'Test_run_mcp_(query_stops|every_tool|cancelled)'`
Mutation checks: timeout wrap in `handler` (`result.go`) → `Test_run_mcp_query_stops_at_its_deadline`; `opts...` pass-through in `newMCPServe` → `Test_run_mcp_query_stops_at_its_deadline`; `NewServer`'s `callTimeout` default → `Test_a_call_without_WithTimeout_gets_a_30_second_deadline`; ctx error attached in `readRefusal` → `Test_reads_keep_the_deadline_apart_from_a_cancel`; ctx error attached on the open path `duckstore/query.go:21-22` → `Test_query_interrupted_by_its_deadline_is_not_a_cancel` (open row)
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (report; store/duckstore chain sites; mcp delivery peer + cmd wiring)

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_timeout_test.go` `Test_run_mcp_query_stops_at_its_deadline` — `syncAccountsFixture` home; `startMCP` tweak sets `ServeMCP` to `newMCPServe(nil, mcp.WithTimeout(1s), mcp.WithReport(<recording factory>))`, the factory wrapping `duckstore.New(storeDirUnder(home))` in a `report.Store` that records `Query`'s error; slow SQL = `range()` cross join; `CallTool` ctx = timeout + 1 s so red fails fast. Asserts: returned ≤ timeout + 1 s; `isError`, one text line = `query stopped after 1 second; aggregate or filter it in SQL, then try again` (the configured timeout, ruled); stderr exactly `quarry: mcp: query: <line>\n`; recorded error `errors.Is` `context.DeadlineExceeded` true, `context.Canceled` false
- [x] Step 2: `internal/mcp/server.go:31-35,55-68` `timeout` field + `WithTimeout(time.Duration) Option` (signature only, no effect yet); `cmd/quarry/run.go:137-146` `newMCPServe(info, opts ...mcp.Option)` appending `opts` after the shipped options (callers `run.go:190`, `run_mcp_version_test.go:31`, `run_mcp_wiring_test.go:17,28` compile unchanged). Red = Step 1's elapsed/`CallTool` assertion
- [x] Step 3: folded acceptance tests, both expected **green on arrival** (say so, don't manufacture red):
  - `cmd/quarry/run_mcp_no_store_test.go` `Test_run_mcp_every_tool_refuses_before_the_first_sync` — fresh HOME, no store, no config; table over the 4 tools: `isError` with `no store at ~/Library/Application Support/quarry/quarry.duckdb yet; run quarry sync to build it` + its one `quarry: mcp: <tool>: …` stderr line
  - `cmd/quarry/run_mcp_cancel_test.go` `Test_run_mcp_cancelled_query_is_interrupted_quietly` — no `WithTimeout` (shipped default); Step 1's recording factory, extended to signal on `Query` entry; slow query under a cancellable `CallTool` ctx, cancel after that signal (never a sleep). Asserts: recorded error arrives within a 3 s bound and `errors.Is` `Canceled` true, `DeadlineExceeded` false (the "store query interrupted" observable); then wait for the SDK's response frame for that JSON-RPC id on `peer.stdout` (sync point only, matched by id, content not asserted); next call = a refusal (e.g. `sql: "INSERT …"`) is served and stderr holds exactly that call's one line (control arm for the silence). Chain half goes green only after Step 4 if the open path is hit — report which

### Build
- [x] Step 4: chain preservation — `internal/store/duckstore/query.go:19-25,36-40` and `internal/report/refusal.go:28-33` `readRefusal`: every "interrupted" decision carries the ctx's own error, so `errors.Is` matches exactly one of `DeadlineExceeded`/`Canceled`; `store.Interrupted` signature and every `Error()` text unchanged (`query interrupted: context canceled`, `<cmd> interrupted`). A site whose real error already carries the ctx error (the driver's query error does: `interruptFault()` at `query_test.go:183` is `errors.Join(context.Canceled, *duckdb.Error)`) gets its test and no code change; say which each site was. Tests: `internal/store/duckstore/query_test.go:135-170` add `Test_query_interrupted_by_its_deadline_is_not_a_cancel` — query path {deadline ctx + a deadline-shaped fault `errors.Join(context.DeadlineExceeded, *duckdb.Error)`, cancel ctx + `interruptFault()`}, open path (`ioFault`, as `:159`) under {expired, cancelled} ctx, + one real-duckdb row (slow cross join, 1 s deadline); `internal/report/refusal_test.go:136-180` add `Test_reads_keep_the_deadline_apart_from_a_cancel` over Status, Findings, DescribeSchema × {deadline, cancel}, fake store error a `*store.OpenError` (as the table at `:170`). CLI copy guarded by existing `EqualError "<cmd> interrupted"` tests (`refusal_test.go:136-180`, `describe_schema_test.go:193`, `findings_test.go:269`, `cli/sql_test.go:148,273`) — no new CLI tests
- [x] Step 5: mcp per-call timeout + outcome classifier — `internal/mcp/tools.go:19-24` `callTimeout = 30 * time.Second` beside `maxRows`; `server.go:61-68` default it, `WithTimeout` overrides; `result.go:16-33` `handler` takes the tool's timeout-line builder (configured whole seconds via `humanize.Count(n, "second", "seconds")`), wraps the call ctx in `context.WithTimeout`, and maps `errors.Is(err, context.DeadlineExceeded)` → that line; every other error (Canceled included) returned unchanged; `tools.go:81-92` passes each tool's line; `query_refusal.go:31` splits `QueryFailureInterrupted` into its own arm returning `err` unchanged. Tests (new `internal/mcp/timeout_test.go`, fake store whose read blocks on `ctx.Done()` and returns the real adapter's shape): `Test_each_tool_answers_its_deadline_with_its_ruled_line` (4 rows, §4 lines rendered for the injected seconds + stderr line each; one row with a 30 s default or 2 s timeout pins the plural); `Test_a_call_finishing_inside_its_deadline_is_answered` (in-bound control); `Test_a_call_without_WithTimeout_gets_a_30_second_deadline` (fake records `ctx.Deadline()`, ≈ now + 30 s); `query_refusal_test.go:36` row: interrupted-by-deadline keeps `DeadlineExceeded` in the chain
- [x] Step 6: `internal/mcp/log_internal_test.go` `Test_errorLog_keeps_concurrent_refusals_lines_whole` — white-box, N goroutines through `errorLog` into one writer, run with `-race` (pins the S03 mutex MINOR)

### Sweep
- [ ] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `WithTimeout`, `callTimeout`, `handler`; trim `internal/mcp/data_quality.go:13-15` (`dataQuality` doc → 1-2 lines, drop load order) and `:73-75` (`findingsCapWarning` doc → 1-2 lines, drop tail logic) (S11 debt; STATE's `:14-17`/`:89-91` anchors have drifted); update `Serve`/`WithReport` docs only if now false

### Verify
- [ ] Step 8: full verification + `spec-check.py phase3a-mcp-core` → tick SCENARIO-06, SCENARIO-13 and SCENARIO-17 (13, 17 "delivered by SCENARIO-06") with their acceptance tests; rewrite STATE.md (drop the S06/S13 owed items from Left unbuilt, close S03 mutex and S11 doc debts)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- The deadline is applied **inside `handler`** (`result.go`), never as a receiving middleware — `errorLog` is silent when its ctx is done, so an outer deadline would silence the timeout's stderr line (Rule 4); inside, the middleware ctx is done only on client cancel, which is exactly S17's silence
- Classification is by `errors.Is` on the chain: `DeadlineExceeded` → per-tool line; everything else unchanged. Canceled needs no arm: `errorLog`'s done-ctx check is the silence and the SDK owns the wire response
- Interrupted errors carry the ctx's own error at the point that decides "interrupted" (duckstore query sites, `readRefusal`), never by trusting the driver's chain; texts and `store.Interrupted`'s signature unchanged, so CLI copy and exit 1 stay byte-identical
- Timeout lines render the configured timeout as `humanize.Count(whole seconds, "second", "seconds")` (orchestrator ruling, spec §2.4): production "30 seconds", test with `WithTimeout(1s)` "1 second". `WithTimeout` takes whole seconds only
- `newMCPServe` takes trailing `...mcp.Option` applied after the shipped ones (precedent `newReportFactory(storeOpts...)`)

**Left unbuilt** — named so nobody assumes it exists:
- config keys `mcp.max_rows`, `mcp.query_timeout` — reserved by name only (spec §2.4)
- process-level SIGTERM and positive pty tests — unchanged from STATE (unowned MINORs)

**Traps** — things that look right and are not:
- `report.ClassifyQueryFailure`'s Interrupted arm returns `Err: store.ErrQueryInterrupted` and drops the chain; mcp `queryRefusal` must return the original `err`, or query never gets its timeout line
- A test asserting "no stderr line" before the cancelled call's middleware has run is vacuous: sync on that id's response frame first. `peer.stdout` is written by the server goroutine; read it race-free
- The SDK may serve calls concurrently — "next call answered" does not prove the cancelled one finished
- `mcpTestDeadline` (30 s) equals `callTimeout`: an S06 test that relies on the default deadline never sees it fire

## Phase report

Run B1 (steps 4-6) done. Commit: see `git log` (feat(mcp): per-call deadline ...).

Files: `internal/store/query.go` (`InterruptedBy(ctx, err)`: `Interrupted(err)` text, also unwraps to `ctx.Err()`; `Interrupted` untouched), `internal/store/duckstore/query.go:22,40` (open path AND `queryRefusal` use it), `internal/report/refusal.go` `readRefusal` (cause = `errors.Join(err, ctx.Err())`), `internal/mcp/{tools,server,result,query_refusal}.go` (`callTimeout`, `NewServer` default, `handler(timeout, stopped, run)` + `stoppedError`, `stoppedLine`/`queryStoppedLine`, split Interrupted arm), tests `internal/mcp/timeout_test.go`, `log_internal_test.go` (errorLog -race pin), `internal/report/refusal_test.go`, `internal/store/duckstore/query_test.go`, `cmd/quarry/run_mcp_timeout_test.go` (recorder now opens via `duckstore.WithOpenReadOnly` and signals `running` when the statement starts), `cmd/quarry/run_mcp_cancel_test.go`.

Green: all three acceptance tests (`-count=10 -race`), narrow loop, CLI "<cmd> interrupted" tests, full-package runs of cmd/quarry, cli, mcp, report, store, duckstore; lint `0 issues`.
Finding: the real driver does NOT put the ctx error in its chain when it interrupts a running query ("query interrupted: INTERRUPT Error: Interrupted!" with no Canceled; the 1 s real-deadline row passes even without the fix, the cancel does not). So the query path needed `InterruptedBy` too, not only the open path; `interruptFault()` (joins Canceled) is a shape the driver does not reliably return, hence `driverInterrupt()` in the new rows.
Cancel acceptance now lands the cancel after the statement started (QueryTable entry) via the exported opener seam; no production seam added.
Mutations (all restored, `git diff` clean): handler WithTimeout removed -> acceptance red + 30_second red; `opts` dropped in newMCPServe (`nil...`) -> acceptance red; NewServer default `0` -> 30_second red; readRefusal join removed -> 6 `deadline_apart` rows red; duckstore open path `InterruptedBy`->`Interrupted` -> open rows red; query path same -> 2 query rows red (real-duckdb deadline row stays green); errorLog mutex removed -> `-race` DATA RACE; queryRefusal Interrupted arm returning `failure.Err` -> query row of `ruled_line` red. The plan's `query_refusal_test.go:36` chain row was not added: that table asserts text only; the chain is pinned by the query row of `Test_each_tool_answers_its_deadline_with_its_ruled_line`.
Already done for V: Step 7 doc trims of `dataQuality`/`findingsCapWarning` (data_quality.go) and lint to 0. Left for V: coverage gate, full verify, spec tick, STATE.md (add: real driver chain lacks ctx error; `InterruptedBy` is the attach point; close S03 mutex and S11 doc debts).
Do not redo: mutations, stubs, tests above.
