---
id: SCENARIO-20
status: open
---

# SCENARIO-20: leftovers from earlier builds are cleaned up

Cadence: test-first — write-safety guard, deletes files under `~/Library/Application Support/quarry` (`.claude/briefs/build.md` → *Build cadence*, "write-safety guards").
Acceptance test: `cmd/quarry/run_store_leftovers_test.go` `Test_run_removes_stale_store_leftovers_before_syncing`
Narrow loop: `go test ./internal/store/duckstore/... ./cmd/quarry/... -run 'Leftover|Replace|leftover'`
Mutation checks:
- age gate (`leftoverMaxAge`) → `Test_replace_removes_a_leftover_partial_just_past_the_age_gate` / `Test_replace_leaves_a_leftover_partial_just_inside_the_age_gate_alone`
- `leftoverPartialPattern` anchors → `Test_replace_sweep_leaves_near_miss_and_unrelated_files_alone`
- pre-swap WAL removal happens after the ctx gate, before rename → `Test_replace_does_not_remove_the_stale_wal_before_the_context_gate` (sibling of `Test_replace_does_not_swap_when_the_context_ends_after_the_checkpoint`)
- stale-WAL removal failure refuses the swap (not silent) → `Test_replace_refuses_the_swap_when_the_stale_wal_cannot_be_removed`
- sweep failure stays silent and does not block a build that can otherwise succeed → `Test_replace_leaves_a_leftover_alone_when_the_sweep_cannot_remove_it`

## Implementation Plan

### Acceptance (red)
- [ ] Step 1: `cmd/quarry/run_store_leftovers_test.go` `Test_run_removes_stale_store_leftovers_before_syncing` — new test beside the pattern at `run_success_test.go:70-96`/`run_success_test.go:177-187` (`writeAged`-style helper, backdate 2h); real `--quicken` bundle via `run(...)`. Plant `.quarry-<old-UTC>.duckdb.partial` + its `.partial.wal` and a `quarry.duckdb.wal`, all backdated. Assert exit 0, empty stderr (proves "silently"), both leftovers gone, no `quarry.duckdb.wal`, `quarry.duckdb` present. No stub needed — compiles today, fails at assertion because nothing removes any of the three files yet.

### Build
- [ ] Step 2: `internal/store/duckstore/duckstore.go:1-13` (imports), `:17-18` (beside `FileName`), `:74-76` (top of `Replace`, before `s.create`) — port `internal/snapshot/destination.go:17-21,47-66`'s sweep shape: `leftoverMaxAge = time.Hour` (own constant — duckstore cannot import `snapshot`; note the duplication in Handoff), `leftoverPartialPattern = regexp.MustCompile(`^\.quarry-\d{8}T\d{6}Z\.duckdb\.partial(\.wal)?$`)`, unexported `(*Store) sweepLeftovers()` (ReadDir + Remove failures swallowed, called first in `Replace`). New file `internal/store/duckstore/leftovers_test.go`: `Test_replace_removes_a_leftover_partial_just_past_the_age_gate` (61min) / `..._just_inside_the_age_gate_alone` (59min, control), `Test_replace_sweep_leaves_near_miss_and_unrelated_files_alone` (kept list: `quarry.duckdb`, an old `quarry.duckdb.wal`, `.quarry-…duckdb` with no `.partial`, `quarry-…duckdb.partial` with no leading dot, `.quarry-…duckdb.partial.bak`, `x.quarry-…duckdb.partial`, `.quarry-…duckdb.partial-wal` (SQLite suffix, not DuckDB's `.wal`), a snapshot-shaped `.20260927T143005Z.sqlite.partial`, an empty `snapshots/` dir), `Test_replace_leaves_a_fresh_partial_alone` (concurrent-run case: unaged, matches pattern, survives), `Test_replace_leaves_a_leftover_alone_when_the_sweep_cannot_remove_it` (wire `WithCreate` to return `errCreateBoom` per `duckstore_test.go:379-389`; assert `ErrorIs(err, errCreateBoom)` and the leftover still exists — proves the sweep's own fault never surfaces through `Replace`'s return).
- [ ] Step 3: `internal/store/duckstore/duckstore.go:97-107` (ctx gate through `os.Rename`) — after the existing `ctx.Err()` check, before `os.Rename`, remove `finalPath + ".wal"`; ignore `errors.Is(err, fs.ErrNotExist)`; any other failure goes through `removePartial(partialPath)` + `return "", buildError(err)`, same as every other pre-swap exit (no new ctx check — the single pre-swap ctx gate stays `Replace`'s only one, per STATE.md). Add "errors", "io/fs" imports. Tests in `duckstore_test.go` beside `Test_replace_does_not_swap_when_the_context_ends_after_the_checkpoint` (`:346-367`): `Test_replace_removes_a_stale_wal_before_swapping_in_the_new_store` (plant `quarry.duckdb.wal`, assert gone + swap succeeds), `Test_replace_leaves_a_missing_wal_alone` (control: no wal present, swap still succeeds), `Test_replace_does_not_remove_the_stale_wal_before_the_context_gate` (cancel ctx after checkpoint via `afterCheckpoint` like `:346-367`; planted wal must survive alongside the untouched old store), `Test_replace_refuses_the_swap_when_the_stale_wal_cannot_be_removed` (make `quarry.duckdb.wal` a non-empty directory so `os.Remove` fails ENOTEMPTY; assert error, old store byte-identical, no swap — reuses the S3 default arm in `internal/snapshot/import.go:133-134`, confirmed against `specification.md:274`'s S3 row, so no new copy).

### Sweep
- [ ] Step 4: fix what `go build ./... && golangci-lint run ./...` reports. Doc comments: `sweepLeftovers` and the two new consts get their own 1-2 line docs; `Replace`'s doc comment (`:69-73`, already an open MINOR at 5 lines) states the sweep and stale-WAL facts in ≤4 lines total, not as added sentences on top of today's 5.

### Verify
- [ ] Step 5: full verification (`go build ./...`, coverage-gated `go test ./...`, `go test -race ./internal/store/duckstore/... ./cmd/quarry/...`, `golangci-lint run ./...`, `test-stats.py`) + `.claude/scripts/spec-check.py phase1-import-store` → tick SCENARIO-20 with its acceptance test.

## Handoff

**Binding decisions**:
- P1-13's sweep runs inside `duckstore.Store.Replace`, not a separate `Prepare` step — `importer.Store` is one method (`Replace`), so a schema-mismatch or V1 sync (which never calls `Replace`) does not sweep. Matches "start of a later sync" loosely (start of the next *build*), not literally sync's start.
- Stale `quarry.duckdb.wal` removal sits after the existing ctx gate and before `os.Rename` — removing it earlier would delete the *old* store's WAL on a refused/interrupted build; removing it after the rename leaves no way to refuse if it fails.
- Stale-WAL removal failure is a build failure (`removePartial` + `buildError`), not swallowed — the sweep's own failures are silent (P1-13 says so), the pre-swap WAL removal is not.
- `leftoverMaxAge = time.Hour` is duplicated in `duckstore` (snapshot's copy stays in `internal/snapshot`) — a feature package may not import another feature package.

**Left unbuilt**: nothing — both P1-13 clauses are delivered here.

**Traps**:
- The two `leftoverMaxAge` constants (`snapshot`, `duckstore`) must change together if the age gate is ever retuned — no shared symbol links them.
- DuckDB's own WAL suffix is `.wal`, not SQLite's `-wal` (see `destination.go`'s snapshot pattern, which uses `-wal`) — do not copy that suffix into `leftoverPartialPattern`.
