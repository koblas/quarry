---
id: SCENARIO-01
status: done
---

# SCENARIO-01: CLI output is unchanged after the shared documents move out of cli

Cadence: code-first — behaviour-neutral move; no write guard, atomic adapter or bug fix touched
Acceptance test: `cmd/quarry/run_shared_documents_test.go` `Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte`
Narrow loop: `go test ./internal/report/... ./internal/cli/ ./internal/config/ ./internal/platform/... ./cmd/quarry/ -run 'json|JSON|sql|SQL|status|Status|findings|Findings|Money|BasicString|Basic|Query|help|byte_for_byte'`
Mutation checks: none (code-first). Optional, non-blocking: swap two fields of the moved findings entry struct → acceptance `findings` row red (proves the byte pin sees key order that `JSONEq` pins cannot)
Runs: A (1) | B1 (2-4) | B2 (5-6) | V (7-8)
Size: OWNS A RUN — 5 batches, 1 feature package (report + new report/document; cli is delivery). `internal/config` and new `internal/platform/tomlstr` are touched only to relocate one pure func (`BasicString`), no behaviour change

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_shared_documents_test.go` (new) `Test_run_prints_the_sql_status_and_findings_documents_byte_for_byte` — table through `run()`, each row asserts exact stdout (`assert.Equal`, never `JSONEq`), exact stderr, exit code. Rows: `sql --json "SELECT 1 AS a; SELECT 2 AS b"` (only `b`'s column and row — the multi-statement pin); `status --json` with an unreadable config (`"ignored": null`, absolute-path warning; reuse `run_status_findings_test.go:23 syncStatusFindingsFixture`, `run_status_json_test.go:21-115` substitution for `built_at`/paths); `findings --json` with an unmatched ignore id needing TOML escaping (quote/backslash); `sql --help` full stdout. **Green on arrival**: write and commit it on the unmodified tree so the literals are pre-move output; no stubs exist to write. A pin written after the move proves nothing. Report it green, with this reason

### Build
- [x] Step 2: `internal/report/document/doc.go` (new) + shared primitives moved from `internal/cli/json.go:14-15` `jsonDateLayout`, `:56-64` `findingsDocument`, `:66-76` `rowsDocument`, `:144-147` `notImportedDocument`, `:222-225` `newFindingCountsDocument`, `:235-241` `newRowsDocument`, `:315-321` `jsonNullString`, `:323-335` `jsonMoney` — exported in document, cli copies deleted, every cli caller retargeted (`json.go`, `json_accounts.go`, `json_anomalies.go`, `json_cashflow.go`, `json_spend.go`, `json_recurring.go`, `json_snapshots.go`, `render.go`, `render_status.go`); move `json_internal_test.go:19-37 Test_jsonMoney` to document. Narrow loop must stay green on every `cmd/quarry/run_*_json_test.go` and `run_json_test.go` byte pin (sync/spend/cashflow/accounts/anomalies/recurring/snapshots)
- [x] Step 3: `internal/cli/json_sql.go:11-84` `sqlDocument`, `sqlColumnDocument`, `jsonSQLCell`, `jsonFloat` → document as exported SQL document + builder taking `(report.QueryResult, limit, warnings)`; cli `renderSQLJSON` stays as `marshalDocument(document.<builder>(…))`. Move `json_sql_internal_test.go:35-91` (per-kind cell/float tests) to document, driven through the exported builder; `:92-156` stay on the wrapper
- [x] Step 4: `internal/cli/json_status.go:10-166` `statusDocument` (+ sub-documents, `newStatusDocument`, `newStatusFindingsDocument`, `jsonTimestamp`/`jsonNullTimestamp`/`jsonNullDate`), `render_status.go:20-25` `statusFindings`, `status.go:65-68` `cannotTellIgnored` → document (exported builder, exported tally type, exported `CannotTellIgnored`). Status ignore policy moves too as one plain-value func: takes the ignore list and a config-problem string ("" = config read); non-empty problem → nil ignore, ignored count unknown, one `CannotTellIgnored(problem)` warning; never refuses. `status.go:54-63 statusIgnore` shrinks to the loader call + policy invoked with `config.Problem` / `config.ProblemAbsolute`. Retarget fixtures `json_status_internal_test.go:76` and `render_status_internal_test.go:83`; document test: one row per policy arm (config read / problem given)
- [x] Step 5: `internal/cli/json_findings.go:9-106` `findingsListDocument`, `findingEntryDocument`, `findingItemDocument`, `newFindingEntryDocument`, `newFindingItemDocument` and `findings.go:121-130 unmatchedIgnoreWarnings` → document (exported; list builder takes `report.FindingsListing`, `finding.Status`, `finding.Type`, warnings — not cli's `findingsView`). Retarget `csv_findings.go:32,41,65`. `internal/config/parse.go:369-391 BasicString` → new `internal/platform/tomlstr` (`doc.go` + func), `parse.go:360-367 keyPartText` retargeted, `internal/config/basic_string_test.go` moved; document imports tomlstr, never config. Document test for the warning builder: plain id, id needing escaping, empty list → `[]`
- [x] Step 6: `internal/report/query_failure.go` (new) — query-error classification (reason + detail: unprintable column, `QueryError.Reason`, empty, read-only, external access, interrupted, other) out of `internal/cli/sql.go:188-208 queryFailure`; cli `queryFailure` keeps its copy and returns the **same error types and unwrap chain** (`errSQLNeedsQuery` UsageError, `*refusalError`, err unchanged for other). `report` test: one row per reason built from the error shape duckstore returns (wrapped, not bare sentinel), plus a `RefusalError`/unknown error → other. `internal/report/sql_conventions.go` (new) `SQLConventions` = `sql.go:39-51` paragraph verbatim; `sql.go:28-64` Long concatenates it. CLI pins: `sql_test.go:191` help, `:250` each refusal, `:292` unprintable, `run_sql_test.go:83-173`

### Sweep
- [x] Step 7: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; doc comments on every exported document/report/tomlstr symbol; `json.go:171` unreachable comment names `jsonSQLCell` — rename to the moved symbol; `go doc ./internal/report/document` reads as a contract

### Verify
- [x] Step 8: full verification + `spec-check.py phase3a-mcp-core` → tick SCENARIO-01 with its acceptance test; STATE.md written (first one for this feature)

## Handoff

**Binding decisions** — a later scenario must not contradict these without saying so:
- `internal/report/document` owns the sql, status and findings `--json` documents plus the shared primitives (date layout, money, null string, rows, finding counts, not-imported) — S03/S09/S11 render MCP results from these same builders; a second copy is the drift PRD:108 forbids.
- document imports report, store, finding, platform; never cli, mcp or config. report never imports document — else report→document→report cycle.
- Builders return values; encoding is the caller's: cli indents via `marshalDocument`, mcp encodes compact (Rule 10). Warnings always passed in, `[]` never null.
- Status ignore policy takes `(ignore []string, problem string)` — S09/S10 call it with `config.ProblemAbsolute`; it never refuses over config.
- `report` query-error classifier is reason-only; each surface owns its copy (S05's MCP refusal lines map from it). cli copy and error types byte-unchanged.
- `report.SQLConventions` is the single conventions text — S07 `describe_schema` embeds it.
- `BasicString` lives in `internal/platform/tomlstr`; config and document both call it.

**Left unbuilt** — named so nobody assumes it exists:
- spend/cashflow/recurring/anomalies/accounts/snapshots/sync documents stay in `internal/cli/json_*.go` (3b moves the first four).
- `marshalDocument` stays in cli (CLI encoding only); no compact encoder in document — mcp owns that.
- Multi-statement pin is CLI-only here; the MCP side is S03's.

**Traps** — things that look right and are not:
- `queryFailure` empty-query arm returns `errSQLNeedsQuery` (a `cli.UsageError`), which `cmd/quarry/run.go:186-195 exitCode` maps to exit 2 — matching the copy but returning a different type changes the exit code.
- Existing findings `--json` pins (`run_findings_json_test.go:26,129`) use `JSONEq`: green after a field reorder. Only the acceptance test sees key order.
- `jsonDateLayout` is used by text renderers (`render.go`, `render_status.go`) too — deleting the cli copy breaks text, not JSON.

## Phase report

Run V (done): steps 7-8 green; scenario complete, status: done, spec ticked, STATE.md written.

- Lint: 4 issues fixed (`internal/report/query_failure_test.go` err113 -> reuses `errDiskRead`, gofumpt table layout; `internal/report/document/findings_test.go` two modernize `embedlit` literals flattened). `internal/cli/json_accounts.go:56` and `internal/cli/json.go:140` comments retargeted to `document.Money` / `document.NewSQL`. `golangci-lint run ./...` -> `0 issues`.
- Verify: `go test -count=1 -coverpkg=./... ./...` rc=0; `uncovered-diff.py` -> 0 uncovered added lines; `-race` green on report, report/document, cli, tomlstr, config, cmd/quarry.
- test-stats vs base: cmd/quarry 503 (+1); internal/cli 415 (-2); internal/config 57 (-1); internal/platform/tomlstr 1 (+1); internal/report 278 (+2); internal/report/document 20 (+20); TOTAL 1274 (+21).
- Acceptance test was green on arrival by design (pins pre-move output); never red.
- Judgement: no direct test on `report.SQLConventions` (pinned only through the `sql --help` byte pin).
