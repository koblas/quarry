# phase3a-mcp-core — current state

Scenarios complete: SCENARIO-01. Last updated by SCENARIO-01.

## Binding decisions
- `internal/report/document` owns the sql, status and findings `--json` documents plus shared primitives (`DateLayout`, `Money`, `NullString`, `Rows`, `FindingCounts`, `NotImported`); MCP tools render from these same builders (`NewSQL`, `NewStatus`, `NewFindingsList`) — a second copy is the drift PRD:108 forbids (SCENARIO-01)
- document imports report, store, finding, platform (`tomlstr`); never cli, mcp or config. report never imports document (cycle) (SCENARIO-01)
- Builders return values; encoding is the caller's: cli indents via `marshalDocument`, mcp encodes compact (Rule 10). Warnings are always passed in, `[]` never null (SCENARIO-01)
- `document.StatusIgnore(ignore, problem)` is the status ignore policy: non-empty problem -> nil ignore, ignored unknown, one `CannotTellIgnored(problem)` warning, never refuses. S09/S10 call it with `config.ProblemAbsolute` (SCENARIO-01)
- `report.ClassifyQueryFailure` is reason-only (`QueryFailureKind` + `Detail` + `Err`); each surface owns its copy. cli `queryFailure` returns the same error types/unwrap chain as before; S05's MCP refusal lines map from it (SCENARIO-01)
- `report.SQLConventions` is the single conventions text; `sql` Long concatenates it, S07 `describe_schema` embeds it, neither may mention masking (SCENARIO-01)
- `BasicString` lives in `internal/platform/tomlstr`; config (`keyPartText`) and document (`UnmatchedIgnoreWarnings`) both call it (SCENARIO-01)

## Left unbuilt
- spend/cashflow/recurring/anomalies/accounts/snapshots/sync documents stay in `internal/cli/json_*.go` — phase 3b moves the first four (SCENARIO-01)
- `marshalDocument` stays in cli; no compact encoder in document — mcp owns it (SCENARIO-02+)
- Multi-statement `sql` pin is CLI-only so far; MCP side is SCENARIO-03 (SCENARIO-01)
- `quarry mcp` command and `internal/mcp` package — SCENARIO-02 (SCENARIO-01)

## Traps
- `queryFailure` empty-query arm must return `errSQLNeedsQuery` (a `cli.UsageError`): `cmd/quarry/run.go` `exitCode` maps it to exit 2; another type changes the exit code (SCENARIO-01)
- findings `--json` pins in `run_findings_json_test.go` use `JSONEq` — green after a field reorder; only `cmd/quarry/run_shared_documents_test.go` sees key order (SCENARIO-01)
- `document.DateLayout` is used by text renderers (`render.go`, `render_status.go`) as well as JSON (SCENARIO-01)
- `marshalDocument`'s `// unreachable:` encode-failure claim depends on `document.NewSQL` being the only producer of sql cell values; a new cell kind extends it there (SCENARIO-01)

## Open debts
- PRD `docs/initial-prd.md` §Security "Redaction on import" bullet and Risks table "masking on import" mitigation are now false: redaction deferred by user 2026-10-02. Do not edit silently; PRD §Decisions entry added when user confirms wording — unowned until then
- `internal/cli/root.go:6-10` `newRootCommand` doc comment lists subcommands and must name `mcp` — SCENARIO-02
- No direct unit test on `report.SQLConventions`; pinned only through the `sql --help` byte pin — unowned, MINOR
- Checkpoint S01 comment MINORs (fold into SCENARIO-02's run): `internal/report/sql_conventions.go:3-4` and `internal/report/document/status.go:12`, `document/doc.go:1-2` name MCP consumers not built yet — state only what code does now, or leave until the consumer lands in S02/S07/S09 and re-check then
- Checkpoint S01 MINOR: `internal/report/query_failure_test.go:12` pins precedence only QueryError over Interrupted; unprintable vs QueryError and read-only vs external-access order unpinned (add rows if constructible) — unowned
- Checkpoint S01 MINOR: `internal/report/document/findings_test.go:42` two behaviours under one "and" name; `assert.Empty(fixedEntry.Items)` on a finding given no items proves nothing — split, give items — unowned
- Checkpoint S01 MINOR: `internal/report/document/status_test.go:41` derefs `*read.Findings.Ignored` without `require.NotNil` — unowned
