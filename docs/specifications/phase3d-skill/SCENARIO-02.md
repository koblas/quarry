---
id: SCENARIO-02
status: done
---

# SCENARIO-02: references/schema.md is generated from the store and carries no user data

Size: LIGHT — 2 steps, cmd/quarry
Cadence: code-first (test code only; no mandatory test-first item touched)
Runs: L | V
Acceptance test: `cmd/quarry/run_skill_schema_reference_test.go` `Test_skill_schema_reference_matches_the_committed_file`
Narrow loop: `go test ./cmd/quarry -run 'Test_skill_schema_reference'`
Mutation checks: change one `COMMENT ON VIEW` text in `internal/store/duckstore/schema.go` -> the golden goes red; restore with `git checkout`.
Surface & Copy delivered: the §S.6 header line only, pinned by `Test_skill_schema_reference_opens_with_its_regeneration_header`.

## Implementation Plan

### Acceptance (red)
- [x] `run_skill_schema_reference_test.go`: golden test over `populatedAnalysisStore` (read-only) vs `plugin/skills/quarry/references/schema.md`, plus signature-only stub `generateSchemaReference` (returns "")

### Build
- [x] Step 1: generator (`duckstore.Schema` relations, `duckdb_views()` comments via `duckdb.OpenReadOnly`, `report.SQLConventions`, `findings holds` paragraph from `run sql --help`), package-level `-update` flag (no other `flag.` use in `cmd/quarry`; cobra owns `run`'s flags), commit `schema.md`
- [x] Step 2: boundary pins: no account/category/payee name; empty store equals populated store; conventions verbatim; no `v_balances_daily`/`v_net_worth`/`v_holdings`; view comments and findings paragraph present; header per §S.6

### Sweep / Verify (run V)
- [x] lint, full suite, spec tick, `spec-check.py`, STATE.md

## Handoff
`generateSchemaReference(t, home)` is the one generator; S03 reads `schema.md` as a plain file and must not re-derive it.

## Phase report
Run V done; scenario complete, `status: done`.
- Verify: `go build ./...` rc=0; `golangci-lint run ./...` 0 issues rc=0; covered full suite rc=0; `uncovered-diff.py` rc=0 (no production Go lines added); `go test -race ./cmd/quarry/...` rc=0; `spec-check.py --run phase3d-skill` OK.
- Ticked SCENARIO-02 in specification.md; STATE.md rewritten.
- Do not redo: no production code in this scenario.
