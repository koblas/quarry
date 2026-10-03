---
id: SCENARIO-13
status: open
---

# SCENARIO-13: the server describes all eight tools

Cadence: code-first (copy and schema descriptions only; no mandatory test-first item)
Acceptance test: `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_all_eight_tools` (two subtests: `tools/list and instructions`, `mcp --help`)
Narrow loop: `go test ./cmd/quarry/ -run 'Test_run_mcp_(describes_all_eight|lists_quarrys)' && go test ./internal/cli/ -run '(?i)mcp'`
Mutation checks: delete `described(...)` on data_quality `limit` (`tools.go:~187`) -> acceptance `tools/list and instructions`; delete `Instructions: instructions` (`server.go:95`) -> same subtest
Runs: A (1-2) | B1 (3-4) | V (5-6)
Size: OWNS A RUN — 2 batches, 1 feature package (`internal/mcp`; `internal/cli` and `cmd/quarry` are copy and tests only)

## What already exists (do not re-plan)

S02/S09/S10 already shipped the four new tools' descriptions, their per-param descriptions (`tools.go:99-122` consts, `cmd/quarry/run_mcp_test.go:62-182` pins) and the 8-tool `ListTools` loop. Left: `query` (`sql`, `limit`) and `data_quality` (`status`, `type`, `limit`) per-param descriptions (§3.4); `instructions` (§3.7); `queryDescription` (§3.8); `quarry mcp` Long (§3.6). `report.SQLConventions` (`internal/report/sql_conventions.go`) and its pins (`internal/cli/sql_test.go:216`, `cmd/quarry/run_shared_documents_test.go:354`) are NOT touched; the §3.8 edit is the `queryDescription` const only (grep confirmed the old "they already leave out" sentence lives in `tools.go:50` and the cmd pin `run_mcp_test.go:36` besides SQLConventions).

## Rename decision

Orchestrator ruling: `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc` is RENAMED to `Test_run_mcp_lists_quarrys_tools_over_json_rpc` and trimmed to the handshake (server info, tool names, JSON-RPC frames, empty stderr); the description/schema/instructions pin moves to the new S13 acceptance test. The ticked line in `phase3a-mcp-core/specification.md:681` is repointed to the new name and `.claude/scripts/spec-check.py --run phase3a-mcp-core` must pass. No STATE debt for the old name.

## Implementation Plan

### Acceptance (red)
- [x] Step 1: `cmd/quarry/run_mcp_descriptions_test.go` (new) `Test_run_mcp_describes_all_eight_tools` — move `mcpInstructions`, the eight `mcp*Description` consts, the `mcp*InputSchema` consts, `mcpObjectOutputSchema` and `assertJSONEqualAny` here from `run_mcp_test.go:25-182,229-235`, updating to ruled copy: `mcpInstructions` = §3.7, `mcpQueryDescription` = §3.8 (keep the `"`limit`"` concat), `mcpQueryInputSchema` `sql`/`limit` and `mcpDataQualityInputSchema` `status`/`type`/`limit` gain the exact §3.4 `description` strings. Subtest `tools/list and instructions`: `startMCP`, `ListTools`, assert instructions, per-tool description, input and output schema (the loop at `run_mcp_test.go:196-216`). Subtest `mcp --help`: `runWith(ctx, ["mcp","--help"], testEnv(...))` exit 0, stdout contains the changed sentence `SQL runs read-only, and every list a tool returns\nstops at 500 entries.` and the closing `Tools: ... recurring_charges, anomalies.` paragraph (full Long is pinned in step 4)
- [x] Step 2: `cmd/quarry/run_mcp_test.go:184-227` rename `Test_run_mcp_lists_quarrys_four_tools_over_json_rpc` to `Test_run_mcp_lists_quarrys_tools_over_json_rpc`, repoint `phase3a-mcp-core/specification.md:681` to it — drop the description/schema/instructions asserts and the consts it no longer uses; keep server info, the eight tool names, frame check, empty stderr. Run: both new subtests fail at their assertions (old instructions; old Long)

### Build
- [x] Step 3: `internal/mcp/tools.go:38-45,47-54,180-189` `instructions`, `queryDescription`, `addTools` — replace the two consts verbatim from §3.7 and §3.8 (queryDescription stays a backtick-concat around "`limit`"; no trailing newline); add consts `querySQLDescription`, `queryLimitDescription`, `findingsStatusDescription`, `findingsTypeDescription`, `findingsLimitDescription` (§3.4 strings verbatim) and wrap the five schemas with `described(...)` (`limitSchema` stays shared; wrap its result per call). No handler or enum change. Gate: `Test_run_mcp_describes_all_eight_tools/tools/list_and_instructions` green; also `go test ./internal/mcp/` (the `absentNullArguments` test must stay green — defaults untouched)
- [x] Step 4: `internal/cli/mcp.go:24-37` Long and `internal/cli/mcp_test.go:28-50` `Test_mcp_help_prints_the_ruled_long_text` — replace Long with §3.6 verbatim (hard line breaks as ruled, ends `anomalies.`), update the test's `long` const to the same bytes (full-Long prefix assert stays). Gate: `mcp --help` subtest green. Fault/bound/validation matrix: n/a (no new input, branch or fallible call; `--json` refusal and arg refusal rows unchanged)

### Sweep
- [ ] Step 5: fix what `go build ./... && golangci-lint run ./...` reports, down to `0 issues`; `go doc ./internal/mcp` unchanged surface; no history in comments

### Verify
- [ ] Step 6: full verification block + `.claude/scripts/spec-check.py phase3b-analysis-tools`; confirm `git diff --stat` shows `internal/report/sql_conventions.go`, `internal/cli/sql_test.go` and `run_shared_documents_test.go` untouched; also `.claude/scripts/spec-check.py --run phase3a-mcp-core`; tick SCENARIO-13 with `cmd/quarry/run_mcp_descriptions_test.go` `Test_run_mcp_describes_all_eight_tools`; rewrite STATE.md (drop the S13 Left-unbuilt and the "OWNED BY S13" debt; mark all scenarios complete)

## Handoff

**Binding decisions:**
- The old phase3a acceptance test is renamed `Test_run_mcp_lists_quarrys_tools_over_json_rpc` (phase3a spec tick repointed); the new test is the only pin of descriptions, schemas and instructions — a later change to any ruled string edits `run_mcp_descriptions_test.go` only.
- Per-param `description` text lives in `tools.go` consts and is never typed twice in production; the cmd test holds its own byte copy by design (pin must not import the const).
- `report.SQLConventions` is untouched: it renders byte-identical into `sql --help` and `describe_schema`, and `query`'s description carries the "call spending or cash_flow instead" redirect on its own.

**Left unbuilt:**
- `search_transactions` and its CLI twin — phase 3c.
- Replacing "David's" in `instructions` — deferred NIT (spec §3.7).
- Phase3a gate R1/R2/R4 MINOR/NIT debts (unowned, phase3a STATE).

**Traps:**
- Renaming the handshake test without repointing `phase3a-mcp-core/specification.md:681` passes build and lint but turns `spec-check.py phase3a-mcp-core` red.
- `mcpQueryDescription` contains a literal backtick pair around `limit`: raw-string consts need the `+ "`limit`" +` concat in both production and test.
- JSON-schema `description` is compared through `assertJSONEqualAny` (key order blind), so the pin catches a missing or altered string but not property order; instructions and descriptions are byte-compared with `assert.Equal`.
- `limitSchema` is shared by `query` and `data_quality` with different descriptions: set the description on the returned schema per call, never inside `limitSchema` (a shared pointer would not be an issue, a shared string would).
- Do not "fix" `sql --help` / `SQLConventions` wording to match §3.8: the "they already leave out" sentence there is correct for the CLI.

## Phase report

Run B1 (steps 3-4) done. Acceptance and narrow loop green; `go build ./...` and `golangci-lint run ./...` already 0 issues.

Files:
- `internal/mcp/tools.go`: `instructions` and `queryDescription` replaced verbatim (§3.7, §3.8); five new param-description consts (`querySQLDescription`, `queryLimitDescription`, `findingsStatusDescription`, `findingsTypeDescription`, `findingsLimitDescription`) wrapped with `described(...)` in `addTools`; new `findingStatuses()` helper (extracted to keep the status line under the 200-col lint limit). Defaults and enums untouched.
- `internal/cli/mcp.go` Long and `internal/cli/mcp_test.go` `long` const: §3.6 verbatim.
- `cmd/quarry/run_mcp_descriptions_test.go`: only the two over-long JSON lines (data_quality `status`, `limit`) broken across lines for `lll`; bytes of copy unchanged.

Mutations (both reddened `Test_run_mcp_describes_all_eight_tools/tools/list_and_instructions`, restored): unwrapped `described(...)` on data_quality `limit`; `Instructions: instructions` -> `ServerOptions{}`.

Green: `cmd/quarry`, `internal/cli`, `internal/mcp` packages. `internal/report`, `sql_test.go`, `run_shared_documents_test.go` byte-unchanged (git diff empty).

Next (V, steps 5-6): full verification block, spec-check (phase3b and `--run phase3a-mcp-core`), tick SCENARIO-13, rewrite STATE.md, `status: done`. Lint already clean; do not redo the wrap fixes.
