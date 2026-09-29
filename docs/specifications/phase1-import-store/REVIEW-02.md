# REVIEW-02 — phase1-import-store final gate, round 2 (re-gate of fix pass 1)

Range: `e0bffd3..efc8a75`. Coverage vs `0ecb753`: 0 uncovered, 11 declared unreachable (all accepted). `spec-check`: OK.

## Reviewers
- arch-reviewer: PASS — round-1 MAJOR (StoreProbe), compile guards, ADR NITs closed; DuckDB confinement re-verified module-wide.
- correctness-reviewer: BLOCKED — round-1 MAJORs 1–4 closed (mutation-verified); one new MAJOR.
- test-reviewer: PASS WITH FOLLOW-UPS — round-1 BLOCKER and MAJOR closed (mutation-verified).
- refactor-advisor: PASS WITH FOLLOW-UPS — grouping helper and named class type closed.

## MAJOR
- correctness-reviewer — `internal/store/duckstore/duckstore.go:103-109` (+ `internal/platform/duckdb/duckdb.go:35-47`): stat-then-open window. Two runs in the same UTC second share the partial name; both pass `duckdb.Create`'s stat; the loser's open fails without `ErrExists`, so its cleanup unlinks the winner's live partial → winner fails S3 (store survives). Second fix pass on this surface → ruled as one decision point (see Rulings 1).

## MINOR
- correctness-reviewer — `internal/importer/money.go:58`: near-zero REALs render in exponent form (`5.5511151231257827e-17`, `1.0e-05`) and refuse as reason 6 "too large" (false). Ruled (see Rulings 2).
- correctness-reviewer + refactor-advisor — `internal/importer/statements.go:46-53`, `internal/importer/splits.go:106-111`: NULL handled via "reads as 0, no Z_PK is 0"; sibling code checks `.Valid`. Fix: explicit `.Valid` checks; drop the 0-sentinel comments.
- refactor-advisor — `internal/importer/offenders.go:19-38`: `unmappableClassOrder` map + numeric doc; declare constants in report order with `iota` and compare directly.
- refactor-advisor — `internal/snapshot/ports.go:48-50` vs `internal/store/duckstore/duckstore.go:74-76`: `Exists` contract duplicated; keep it on the implementation, one line on the interface.
- refactor-advisor — `internal/store/duckstore/duckstore.go:121-124`: `Replace` doc grew (`(0700)`); drop the mode.
- test-reviewer — `internal/snapshot/discover_test.go:222-224`: `R3` in test name/comment; describe the behaviour.
- test-reviewer — `internal/snapshot/sync_and_import_test.go:67-69`: `newImportServer` doc grew to 3 lines; one sentence.
- test-reviewer — `internal/platform/duckdb/exec_query_test.go:108-112`: 5-line test comment touched this pass; ≤2 lines.
- test-reviewer — `internal/snapshot/import_from_test.go` locked-snapshot test costs ~5.4s (driver busy_timeout default); `t.Parallel()` if safe, else leave.

## NIT
- refactor-advisor — `errNoImporter` message widened to "no importer or store probe configured"; rename identifier.
- refactor-advisor — `internal/snapshot/import.go:100-101` `storeProbe.Path()` called twice; optional.
- test-reviewer — `parseRealMoney("1.")` returns 100 cents while the doc grammar says a dot needs digits; reject empty fraction after a dot (SQLite never renders it) and add a table case.

## Rulings (product-vision, 2026-09-29)
1. Build file `.quarry-<UTC>-<pid>.duckdb.partial`, unique per run; one function owns the name and the sweep pattern `.quarry-*.duckdb.partial`; failure cleanup unlinks only the run's own path; a create collision falls into S3. No run lock, no new copy. (DuckDB refuses a pre-created empty file, so uniqueness comes from the name, not O_EXCL.)
2. REAL exponent form split by sign: negative exponent → reason 3/4/5 (value quoted as rendered); positive exponent → reason 6. Tests for `e-17`, `e-05` (reason 3) and one positive exponent (reason 6). Superseded by P1-7 (2026-09-29).

## Verdict: FAIL (1 MAJOR)
