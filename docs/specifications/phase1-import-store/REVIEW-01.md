# REVIEW-01 — phase1-import-store final gate, round 1

Range: `0ecb753..f823100`. Coverage gate: 0 uncovered added lines, 13 declared unreachable (one rejected below). `spec-check.py phase1-import-store`: OK.

## Triggered reviewers
- arch-reviewer, correctness-reviewer, refactor-advisor: `cmd/**/*.go`, `internal/**/*.go`
- test-reviewer: `**/*_test.go`

## Skipped reviewers
- api-reviewer: no HTTP surface. pipeline-reviewer: no `.claude/**` changes.

## BLOCKER
- test-reviewer — `internal/snapshot/import_from_test.go:171-177` `Test_import_from_returns_the_manifest_a_plain_sync_returned` cannot catch `--from` recomputing `TakenAt` (mutation `TakenAt: time.Now()…` stays green: both land in the same second). Fix: seed a far-past `taken_at` via editManifest (or a controllable clock) and assert the returned TakenAt equals the recorded value.

## MAJOR
- correctness-reviewer — `internal/importer/statements.go:27,46`: `rawReconcile.account` is plain int64; a non-deleted `ZRECONCILERECORD` with NULL `ZACCOUNT` aborts the import with a Scan error (S3, retry fails identically). Spec: skip it. Fix: `sql.NullInt64`, skip when invalid; test. Same shape `internal/importer/splits.go:107-108` (`Z_15USERTAGS` NULL end → Scan error; spec: link missing an end is not stored).
- correctness-reviewer — `internal/store/duckstore/duckstore.go:82-91`: `Replace` never creates its directory; `--from` on a machine without `~/Library/Application Support/quarry` fails S3, retry fails identically. Fix: `os.MkdirAll(s.dir, 0o700)` at top of Replace through `buildError` (EACCES → S1); test.
- correctness-reviewer — `internal/store/duckstore/duckstore.go:88-91`: on `create` failure with `duckdb.ErrExists`, Replace removes a partial (and `.wal`) owned by another live run started the same second, destroying its build. Fix: on `ErrExists` return `buildError(err)` without `removePartial`; test. (Unique partial names would change the ruled Build-file surface — not in this pass.)
- correctness-reviewer — `internal/snapshot/from.go:204-210`: F4b default declared unreachable is reachable (held `BEGIN EXCLUSIVE` on a rollback-journal snapshot → `database is locked`, correct F4b copy). Fix: test that reaches it (lock fixture); delete the `// unreachable:` marker.
- arch-reviewer — `internal/snapshot/import.go:102-117` `previousStoreExists` stats `s.storePath` directly and `result.Path = s.storePath` — snapshot inspects the store outside the wired Importer, contrary to ADR-002; drift between `WithStorePath` and the duckstore dir makes NOT REBUILT/NOT BUILT and `store.path` silently wrong. Fix: consumer-declared port in snapshot (e.g. `StoreProbe` with `Exists` and `Path`), implemented by `*duckstore.Store`, wired in cmd/quarry; derive both from it; update `WithStorePath` doc (or remove the option if the probe supplies the path) and ADR-002's trade-off paragraph.
- test-reviewer — `internal/importer/money_internal_test.go:13-38` / `internal/importer/money.go:74`: `parseMoney("real", "1.-5")` returns 95 cents silently; `"1.x"` reaches the "unreachable" branch. Fix: validate integer and fraction parts are digits-only in parseRealMoney; malformed REAL text refuses (never a silent value); add table cases; drop or correct the `// unreachable:` marker.

## MINOR
- correctness-reviewer — `internal/importer/money.go:22-23,72` REAL bound premise "15 significant digits" is false for SQLite 3.53 (prints up to 17); legit REALs ≥ $1e11 refused with misleading amounts. Spec ruling pending (see Rulings).
- correctness-reviewer — `internal/store/duckstore/duckstore.go:112-120` no dir fsync after rename (repo idiom `atomicfile.syncDir`). Fix: best-effort fsync of `s.dir` after rename.
- correctness-reviewer — `internal/snapshot/from.go:239-246` → `internal/importer/importer.go:51`: SHA verified once, file opened twice more (TOCTOU on a quarry-owned file). Record as debt.
- correctness-reviewer — `internal/importer/transactions.go:20` Core Data seconds → Duration overflow for |s| > ~9.2e9; `internal/importer/validate.go:110` reconciledSums int64 wrap on absurd values. Record as debt (needs corrupt input).
- arch-reviewer — `internal/snapshot/from.go` / `snapshot.go:245-283`: no read port for a committed snapshot's own files (root cause of the unreachable ctx-race wraps). Record as debt.
- arch-reviewer — `cmd/quarry/run.go:21`: add `var _ importer.Store = (*duckstore.Store)(nil)` beside the existing guard.
- refactor-advisor — `internal/snapshot/import.go:46-51,163-183` and `internal/cli/render.go:35-40`: four hand-rolled singular/plural helpers; snapshot stderr counts are not thousands-grouped while cli's are. Grouping ruling pending (see Rulings); share a leaf helper (e.g. `internal/platform/text`).
- refactor-advisor — `internal/importer/transactions.go:96-198`, `splits.go:34-92`, `accounts.go:59-106`: long scan closures — Compose method (follow `statements.go`'s `parseStatement`). Fix-if-cheap.
- refactor-advisor — `internal/cli/sync.go:77-130` RunE ~53 lines — extract classify/render. Fix-if-cheap.
- refactor-advisor — S4 class is a bare `int` across importer (`offender.class`, `s4ClassOrder`) — `type s4Class int` with named constants. Fix-if-cheap.
- test-reviewer — `cmd/quarry/run_from_refusals_test.go:85-100` case "no manifest / bad JSON / not SQLite" only exercises "no manifest" — rename or split.
- test-reviewer — spec/finding-id shorthand (V1, O1b, S4, I2) in test comments/names/filename: `cmd/quarry/run_json_test.go:103`; `cmd/quarry/run_validation_test.go:112,119,184,211,301`; `internal/importer/dangling_references_test.go:33,65`; `internal/importer/not_imported_test.go:84`; `internal/importer/transfers_test.go:219`; `internal/snapshot/import_from_test.go:472`; `internal/snapshot/sync_and_import_test.go:300`; names `v1MismatchRow`, `Test_run_reports_the_o1b_refusal_when_stdout_fails_during_a_v1_render/_v1_json_write`; file `internal/importer/s4_selection_test.go`. Replace with the behaviour.
- test-reviewer — `cmd/quarry/run_transfers_test.go:211-227` exit-code assertion last — move first.
- test-reviewer — `cmd/quarry/run_transfers_test.go:239-260` HasPrefix/HasSuffix — use exact syncBlock comparison like siblings.

## NIT
- arch-reviewer — `docs/adr/001-shared-store-package.md` Decision says `internal/store` holds "only" row types; it also carries Validation, SnapshotRef, ImportRun, four sentinels. Update text.
- correctness-reviewer — `internal/platform/duckdb/duckdb.go:166-169` unreachable rationale cites second-call idempotency but this is the first Close; fix wording.
- refactor-advisor — `internal/store/duckstore/duckstore.go:88-120` repeated fail paths (local `fail` helper); `internal/cli/render.go:258-340` three row builders share a shape (leave).
- test-reviewer — duplicated `writeManifestFor`/`writeManifestForTest` across packages (inherent); `internal/platform/duckdb/exec_query_test.go:124-131` `if calls == 1` in test body.

## Declared-unreachable verdicts
Accepted: cli/json.go:132; importer/entities.go:62; duckdb.go:60-61 (after the ErrExists fix), :106, :168 (wording NIT), :173; snapshot/from.go:28, :83 (darwin; no Linux CI in repo — Linux test has never run); snapshot/snapshot.go:227, :259, :268.
Rejected: snapshot/from.go:207-210 (MAJOR above); importer/money.go:74 (MAJOR above).

## Rulings (product-vision, 2026-09-29)
1. `realIntBound` = 1e9 (REAL |v| ≥ 1,000,000,000.00 → reason 6); pin with an in-repo n/100.0 scan test near the bound; correct the P1-7 rationale. Float noise below the bound stays reason 3/4, never rounded.
2. Comma-group every stderr count (V1 clauses, W2, same text in warnings[]); JSON numbers stay plain; test each stderr line with a count ≥ 1,000.

## Strengths
- DuckDB confinement and the dependency rule verified with `go list -deps` and LSP implementer checks.
- Mutation discipline: every STATE.md mutation claim spot-checked by three reviewers reproduced.
- Write path order and partial cleanup on every exit are correct; money never passes through float64.

## Verdict: FAIL
