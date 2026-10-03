---
id: SCENARIO-02
status: open
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
- [ ] lint, full suite, spec tick, `spec-check.py`, STATE.md

## Handoff
`generateSchemaReference(t, home)` is the one generator; S03 reads `schema.md` as a plain file and must not re-derive it.

## Phase report
Run L done; run V next (Sweep, Verify, spec tick, spec-check, STATE.md, status: done).
- Files: `cmd/quarry/run_skill_schema_reference_test.go` (generator `generateSchemaReference`, package-level `-update` flag, acceptance + 7 pins), `plugin/skills/quarry/references/schema.md` (generated, committed).
- Red (run A shape): acceptance failed at `require.NoError` on reading `schema.md`: `open ../../plugin/skills/quarry/references/schema.md: no such file or directory` (stub generator returned "").
- Green now: all 8 `Test_skill_schema_reference*` on `go test ./cmd/quarry -run 'Test_skill_schema_reference'`; `golangci-lint run ./cmd/quarry/...` 0 issues.
- `-update` check: no other `flag.` use in cmd/quarry (cobra owns `run`'s flags; subprocess tests pass only `-test.run`), so no conflict.
- Mutations run: (1) one `COMMENT ON VIEW` text in schema.go edited -> golden and view-comment pin red, restored; (2) generator leaking account names -> golden, names pin and empty-store-equality red, restored.
- Header is derived from the acceptance test's real function name (reflect), so a rename regenerates it; the header pin holds the literal.
- Do not redo: no production code touched; `populatedAnalysisStore` untouched.
- V: full suite + `uncovered-diff.py` (test-only files); tick SCENARIO-02 in spec as `cmd/quarry/run_skill_schema_reference_test.go` `Test_skill_schema_reference_matches_the_committed_file`.
