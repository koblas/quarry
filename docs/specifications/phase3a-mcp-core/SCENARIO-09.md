---
id: SCENARIO-09
status: done
---

# SCENARIO-09: sync_status returns the status document (folds SCENARIO-10, SCENARIO-14)

Cadence: code-first (no mandatory test-first item: no write guard, no atomic adapter, no bug fix)
Acceptance test: `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_returns_the_status_json_document`
Acceptance test (SCENARIO-10, folded): `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_answers_when_the_config_is_unreadable`
Acceptance test (SCENARIO-14, folded): `cmd/quarry/run_mcp_status_test.go` `Test_run_mcp_sync_status_sees_a_sync_between_calls`
Narrow loop: `go test ./internal/mcp/ -run 'sync_status|unbuilt|null_or_empty' && go test ./cmd/quarry/ -run 'run_mcp|MCPServe'` (filters checked with `go test -list`)
Mutation checks: config loaded once instead of per call (in `NewServer`/`Serve`) -> `Test_run_mcp_sync_status_sees_a_sync_between_calls/the_config_is_read_on_every_call`; `srv.Status` result memoised across calls -> `Test_run_mcp_sync_status_sees_a_sync_between_calls/a_re-sync_shows_the_new_store`; `IgnoreKnown` forced true -> `Test_run_mcp_sync_status_answers_when_the_config_is_unreadable`; `config.Problem` in place of `config.ProblemAbsolute` -> same test (`~` vs absolute path)
Runs: A (1-3) | B1 (4-6) | V (7-8)
Size: OWNS A RUN — 3 batches, 1 feature package (`internal/mcp`) plus `cmd/quarry` wiring; folds S10 and S14

Orientation: `go doc ./internal/mcp`, `./internal/report/document`, `./internal/config`, plus LSP-free reads of the ranges below; STATE.md read, no SCENARIO-XX.md opened.

## Surface inventory (what sync_status reuses; nothing new is invented)
- `quarry status` today (`internal/cli/status.go:35-50, 66-76`): `openReport` -> `srv.Status` -> config loaded AFTER the store read -> `document.StatusIgnore(cfg.Ignore, "")`, or on loader error `StatusIgnore(nil, config.ProblemAbsolute(err))` -> `document.FindingsTally{Counts: report.CountFindings(st, ignore), IgnoreKnown: len(warnings)==0}` -> `document.NewStatus`. sync_status makes the same four calls and keeps that order. `cfg.Warnings`/`WarningsAbsolute` (unknown keys) are never read: status ignores them and so does the tool.
- Store refusal: `report.Server.Status` (`internal/report/report.go:53-60`) maps open faults, including `the store has no import history` (`duckstore/status.go:74`), to a `report.RefusalError`; `handler` already sends that text verbatim. So the empty-`import_runs` case matches the CLI by construction; the plan pins it against the CLI line (Step 6) instead of adding mapping.
- Only `internal/config` is new to `internal/mcp`: it imports platform packages only, and `cli` already imports it, so it is a settings leaf, not a feature package.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_status_test.go` (new) `Test_run_mcp_sync_status_returns_the_status_json_document` — `syncStatusFindingsFixture` (`run_status_findings_test.go:23`) + a `newDescribePeer`-style peer (`run_mcp_describe_test.go:157-165`); result text equals `compactJSON` of `run(["status","--json"])`, `structuredFrame(peer.stdout)` equal (key order), `IsError` false, mcp stderr empty. Red: the stub answers `this tool is not available yet`
- [x] Step 2: same file `Test_run_mcp_sync_status_answers_when_the_config_is_unreadable` — `writeConfig` with `[snapshots]\nkeep = 0\n[findings]\nignore = [<dup id>]` (as `run_status_findings_test.go:58`); not `isError`, `findings.ignored` null, `findings.open` counts the ignored one, `warnings` == the one ruled line naming `configPath(home)` ABSOLUTE; whole document equals `status --json` of the same HOME. Table-less: one cause is enough, `statusConfigRefusals` already owns the matrix for the shared policy
- [x] Step 3: same file `Test_run_mcp_sync_status_sees_a_sync_between_calls`, one top-level test, two `t.Run` subtests each with its own HOME and peer: `a re-sync shows the new store` — `syncNewBundle` (`run_findings_carry_test.go:37`) twice with different builders (second has an extra account), serve, call, `runSyncFrom(t, firstSnapshotID)` (`run_sync_prune_test.go:131`), call again, no restart; second call's `snapshot.id` and `store.rows.accounts` changed and the document (including `store.built_at`) equals `status --json` taken after the re-sync; `the config is read on every call` — `writeConfig` ignore list between two calls on one peer; `findings.ignored` 0 -> 1, `open` falls by 1. Red at their assertion (stub); no production stubs needed, file compiles

### Build
- [x] Step 4: `internal/mcp/server.go:20-56` `ConfigLoader` + `WithConfig` + `Server.loadConfig`; `internal/mcp/tools.go:96` swap `notBuilt[noInput]` for `handler(s.syncStatus)` and drop the `sync_status` row of `Test_an_unbuilt_tool_answers_isError` (`server_test.go:153-172`; `data_quality` stays) in the same edit, since a bare `NewServer()` has no factory or loader; new `internal/mcp/sync_status.go` `(*Server).syncStatus` (factory -> `Status` -> config policy -> `NewStatus`) — `ConfigLoader` is `func(command string) (config.Config, error)` declared in mcp like `ReportFactory`, called per call with `commandName`; a Server with no `WithConfig` panics on this tool like a nil factory (precedent). Tests in new `internal/mcp/sync_status_test.go`; extend `fakeStore` (`query_helpers_test.go:33-48`) with `Status` + read counter and `newHarness` (`:70`) with a trailing `opts ...mcp.Option` (variadic keeps callers). Rows: document returned and equals `document.NewStatus` of the fake's status, compact; store with no transactions -> `dates` null; `Status` fault returned as `isError` text verbatim, one stderr line; factory failure (`errFactoryBroke`) as in describe_schema
- [x] Step 5: `internal/mcp/sync_status.go` ignore policy — `document.StatusIgnore(cfg.Ignore, "")`; loader error -> `StatusIgnore(nil, config.ProblemAbsolute(err))`; `IgnoreKnown` only when no warning. Tests in `sync_status_test.go`: ignore list moves a finding from open to ignored (fixed stays fixed); config refusal -> `findings.ignored` null + ruled warning with problem verbatim; non-config loader error (home-unset text) -> same warning, still not `isError`; config with unknown keys (`Config.Warnings`, `WarningsAbsolute` set) -> `warnings` stays `[]`; loader called once per call (two calls -> two loads, two `Status` reads, both with `"mcp"`)
- [x] Step 6: `cmd/quarry/run.go:135-150` `newMCPServe` add `mcp.WithConfig(mcp.ConfigLoader(newConfigLoader()))` after the `resolveHome` check, before `signal.Ignore` (keep that order); `Test_run_mcp_sync_status_refuses_a_store_without_an_import_run` in `run_mcp_status_test.go` (`syncAccountsFixture` + `editStore(...,"DELETE FROM import_runs")` as `run_read_refusals_test.go:267`) — `isError` text equals the CLI `status` stderr line minus `quarry: ` and newline, and mcp stderr is `quarry: mcp: sync_status: <that text>\n`. Debt closes: drop the first two package-clause comment lines of `run_mcp_query_test.go:1-2` and `run_mcp_describe_test.go:1-2` and add none to the new file; recheck `internal/report/document/status.go:12` (its MCP consumer now exists; edit only if it is still untrue)

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on `WithConfig`, `ConfigLoader`, `syncStatus` within budget (`go doc ./internal/mcp`)

### Verify
- [x] Step 8: full verification per `.claude/rules/agent-briefs.md`, `spec-check.py phase3a-mcp-core`; tick SCENARIO-09 with its acceptance test, and SCENARIO-10 and SCENARIO-14 with "delivered by SCENARIO-09" before each test reference; rewrite STATE.md (S09/S11 owed items: `WithConfig`, `notBuilt` shrinks to data_quality, close the S01 `status.go:12` and the duplicate-comment debts)

## Handoff

**Binding decisions:**
- `mcp.ConfigLoader`/`WithConfig` are declared in mcp and fed `newConfigLoader()` by cmd; the loader is called per tool call with `commandName`, never cached — spec rule 3 and §2.2 (config edits take effect with no restart). S11 reuses this seam unchanged
- sync_status config policy is status's: `StatusIgnore(cfg.Ignore, "")`, or `StatusIgnore(nil, config.ProblemAbsolute(err))` on any loader error; `Config.Warnings` are not surfaced here — S11's data_quality is where absolute unknown-key lines appear (spec §4 edge table)
- `internal/mcp` imports `internal/config` (leaf: platform-only imports); it still must not import `cli` or another feature package

**Left unbuilt:**
- `data_quality` handler, `notBuilt`, `errNotBuilt` and `Test_an_unbuilt_tool_answers_isError` (its last row) — S11
- sync_status `30 seconds` timeout line — S06; sync_status row of the cross-tool no-store table — S13 (S09 pins only the no-import-history refusal through the handler)
- Any `age` field: ruled out in §2.5

**Traps:**
- `store.built_at` is RFC 3339 to the second: two syncs in one second can share it. Tests prove a new store by `snapshot.id`, `store.rows` and equality with `status --json` after the sync; strict `built_at` inequality is not constructible without a >= 1 s wait
- CLI loads config AFTER `srv.Status`; keep the order, since a refusing store must not depend on config
- `fakeStore` embeds `report.Store`: any method not overridden panics; the new `Status` override is needed or the unit tests panic
- `status --json` refuses a store with no import run, so S11/S13 fixtures cannot use it as an oracle there

## Phase report

Run V done: all phases ticked.

Run B1 (steps 4-6) done. Narrow loop green; `golangci-lint run ./...` 0 issues; all four plan mutations run, each red (config cached -> `.../the_config_is_read_on_every_call`; Status memoised -> `.../a_re-sync_shows_the_new_store`; `IgnoreKnown: true` -> `..._answers_when_the_config_is_unreadable` plus two unit tests; `Problem` for `ProblemAbsolute` -> same acceptance test and `Test_sync_status_says_it_cannot_tell_what_is_ignored_when_the_config_is_refused`). Files restored, diffed identical.

Changed: `internal/mcp/server.go` (`ConfigLoader`, `WithConfig`, `newConfig`), `internal/mcp/tools.go` (sync_status -> `handler(s.syncStatus)`), new `internal/mcp/sync_status.go` (`syncStatus`, `statusIgnore`), new `internal/mcp/sync_status_test.go`, `query_helpers_test.go` (`fakeStore.Status`/`statusReads`, `newHarness(..., opts ...mcp.Option)`, `harness.syncStatus`), `server_test.go` (sync_status row dropped), `cmd/quarry/run.go` (`mcp.WithConfig` after the `resolveHome` check), `cmd/quarry/run_mcp_status_test.go` (+ `Test_run_mcp_sync_status_refuses_a_store_without_an_import_run`), package comments dropped from `run_mcp_query_test.go` and `run_mcp_describe_test.go`.

Notes for V: acceptance assertions in the A-run all held unchanged (snapshot.id differs after `sync --from`; no assertion was wrong). Plan's `Server.loadConfig` was not added: it would be a pass-through, `statusIgnore` calls `s.newConfig` directly (nil loader panics like a nil factory). `internal/report/document/status.go:12` comment is true now (no edit). `notBuilt`/`errNotBuilt` remain for data_quality (S11). V still owes: full verify block, `spec-check.py`, ticking S09 + folded S10/S14, STATE.md rewrite, `status: done`.
